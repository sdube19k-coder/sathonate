import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { chromium, expect } from "@playwright/test";

// Browser interaction and serialization test. The API is an in-browser fixture.
const origin = "http://127.0.0.1:5197";
const server = spawn(
  process.execPath,
  [
    "node_modules/vite/bin/vite.js",
    "--host",
    "127.0.0.1",
    "--port",
    "5197",
    "--strictPort",
  ],
  {
    env: {
      ...process.env,
      VITE_LOCAL_AUTH_ENABLED: "true",
      VITE_APP_ENV: "local-development",
    },
    stdio: "ignore",
    windowsHide: true,
  },
);
let browser;
try {
  for (let attempt = 0; ; attempt++) {
    if (server.exitCode !== null || attempt === 40)
      throw new Error("Vite fixture unavailable");
    if (
      await fetch(origin)
        .then((r) => r.ok)
        .catch(() => false)
    )
      break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  browser = await chromium.launch({ channel: "msedge", headless: true });
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.name));
  const artifact = `sathonate/example@sha256:${"a".repeat(64)}`;
  const placements = {
    "fixture-worker": { "kubernetes.io/hostname": "fixture-worker" },
  };
  let saved,
    deployed,
    revision = 0,
    latest = "";
  let staleExpansion = true,
    staleRecovery = true;
  const operations = new Map();
  const now = new Date().toISOString();
  const start = (id, kind) => {
    latest = id;
    const op = {
      operationId: id,
      networkId: "fixture-network",
      kind,
      status: "succeeded",
      stages: [],
      events: [],
      createdAt: now,
      updatedAt: now,
    };
    operations.set(id, op);
    return op;
  };
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    const headers = request.headers();
    const parent = new URL(request.url()).searchParams.get("parent");
    const reply = (json, status = 200) => route.fulfill({ json, status });
    const record = () => ({
      id: "fixture-network",
      desired: saved,
      revision,
      issues: [],
    });
    if (path === "/api/v1/networks" && method === "POST") {
      saved = request.postDataJSON();
      revision = 1;
      return reply(record(), 201);
    }
    if (path === "/api/v1/networks/fixture-network" && method === "PUT") {
      expect(headers["if-match"]).toBe('"1"');
      saved = request.postDataJSON();
      revision = 2;
      return reply(record());
    }
    if (
      path === "/api/v1/networks/fixture-network/deploy" &&
      method === "POST"
    ) {
      expect(headers["if-match"]).toBe('"1"');
      expect(headers["idempotency-key"]).toBeTruthy();
      deployed = structuredClone(saved);
      return reply(start("initial-op", "secure-local-deploy"), 202);
    }
    if (path.endsWith("/dynamic/preview")) {
      expect(parent).toBe("initial-op");
      expect(headers["if-match"]).toBe('"2"');
      const reviewedRevision = staleExpansion ? revision - 1 : revision;
      staleExpansion = false;
      return reply({
        parentOperationId: parent,
        digest: "expansion-digest",
        revision: reviewedRevision,
        before: deployed,
        after: saved,
        recovery: "Fixture reviewed expansion",
      });
    }
    if (path.endsWith("/dynamic/deploy")) {
      expect(headers["if-match"]).toBe('"2"');
      expect(headers["idempotency-key"]).toBeTruthy();
      expect(request.postDataJSON()).toEqual({
        parentOperationId: "initial-op",
        approval: "expansion-digest",
      });
      deployed = structuredClone(saved);
      return reply(start("expanded-op", "secure-local-dynamic"), 202);
    }
    if (path === "/api/v1/operations")
      return reply({ items: [...operations.values()].reverse() });
    if (path.startsWith("/api/v1/operations/"))
      return reply(
        operations.get(path.split("/").at(-1)) ?? {},
        operations.has(path.split("/").at(-1)) ? 200 : 404,
      );
    if (path === "/api/v1/deployments/expanded-op/workloads")
      return reply({
        operationId: latest,
        revision,
        readOnly: true,
        ledgerChecked: false,
        checkedAt: now,
        workloadsReady: false,
        pods: [],
        volumes: [],
        guidance: [],
        missing: ["retailer-peer"],
        alerts: [
          {
            code: "Unavailable",
            component: "retailer-peer",
            action: "Review recovery",
          },
        ],
        components: [{ name: "retailer-peer", kind: "peer", ready: false }],
      });
    if (path === "/api/v1/deployments/expanded-op/recovery/preview") {
      expect(new URL(request.url()).searchParams.get("node")).toBe(
        "retailer-peer",
      );
      const reviewedNode = staleRecovery ? "foreign-peer" : "retailer-peer";
      staleRecovery = false;
      return reply({
        parentOperationId: "expanded-op",
        plan: {
          node: reviewedNode,
          kind: "peer",
          digest: "recovery-digest",
        },
        impact: "Fixture peer recovery",
        recovery: "Fixture policy",
      });
    }
    if (path === "/api/v1/deployments/expanded-op/recovery/deploy") {
      expect(headers["idempotency-key"]).toBeTruthy();
      expect(request.postDataJSON()).toEqual({
        node: "retailer-peer",
        approval: "recovery-digest",
      });
      return reply(start("recovered-op", "secure-local-recovery"), 202);
    }
    if (path === "/api/v1/deployments/" + latest)
      return reply({
        operationId: latest,
        phase: latest === "recovered-op" ? "removed" : "running",
        verified: true,
        ready: true,
        checkedAt: now,
        resources: [],
        network: deployed,
      });
    if (path === "/api/v1/networks")
      return reply({ items: saved ? [record()] : [] });
    const fixtures = {
      "/api/v1/auth/config": { mode: "local-token" },
      "/api/v1/projects": {
        items: [
          {
            id: "project-a",
            tenantId: "fixture-tenant",
            developer: "fixture-user",
          },
        ],
      },
      "/api/v1/meta": {
        name: "Fixture",
        mode: "local-token",
        deploymentMode: "secure-local",
        dynamicLocal: {
          enabled: true,
          storageClass: "standard",
          placements,
          artifacts: {
            [artifact]: { sha256: "b".repeat(64), codeVersion: "1.0" },
          },
        },
      },
    };
    return route.fulfill({ json: fixtures[path] ?? { items: [] } });
  });
  await page.goto(origin);
  await page
    .getByLabel("Development token", { exact: true })
    .fill(randomBytes(32).toString("hex"));
  await page.getByRole("button", { name: "Authenticate", exact: true }).click();
  await page
    .getByLabel("Authorized project", { exact: true })
    .selectOption("project-a");
  await page.getByRole("button", { name: "Open project", exact: true }).click();
  await page.getByRole("button", { name: "Create visual design" }).click();
  await page
    .getByLabel("Network name", { exact: true })
    .fill("Local visual fixture");
  const add = async (type, id) => {
    if (type === "authority")
      await page
        .getByLabel("authority organization", { exact: true })
        .selectOption("retailer");
    await page.getByLabel(`New ${type} identifier`).fill(id);
    await page
      .getByRole("button", { name: `Add ${type}`, exact: true })
      .click();
  };
  await page.getByRole("tab", { name: /Organizations & CAs/ }).click();
  await add("organization", "retailer");
  await add("authority", "retailer-ca");
  await page.getByRole("tab", { name: /Peers & orderers/ }).click();
  await add("peer", "retailer-peer");
  await add("orderer", "raft-one");
  await page.getByRole("tab", { name: /Channels/ }).click();
  await add("channel", "shipments");
  await page.getByRole("tab", { name: /Chaincode/ }).click();
  await page.getByRole("button", { name: "Add chaincode" }).click();
  await page.getByLabel("Approved executable").selectOption(artifact);
  await page.getByRole("tab", { name: /Review/ }).click();
  await expect(
    page.getByText("Local completeness checks passed.", { exact: false }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  const save = page.waitForResponse(
    (r) => r.url().endsWith("/networks") && r.status() === 201,
  );
  await page
    .getByRole("button", { name: "Save desired configuration" })
    .click();
  await save;
  expect(saved.metadata.name).toBe("Local visual fixture");
  expect(
    saved.spec.configuration.placements["fixture-worker"].nodeSelector,
  ).toEqual(placements["fixture-worker"]);
  expect(saved.spec.configuration.authorities[0].id).toBe("retailer-ca");
  expect(saved.spec.configuration.chaincodes[0].runtimeImage).toBe(artifact);
  expect(saved.spec.channels[0]).toMatchObject({
    peerRefs: ["retailer-peer"],
    ordererRefs: ["raft-one"],
  });
  await page.getByRole("button", { name: "Deploy saved network" }).click();
  await expect(
    page.getByRole("heading", { name: "Operation initial-op" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Review running-network change" }),
  ).toBeEnabled();
  await page.getByRole("tab", { name: /Peers & orderers/ }).click();
  await add("peer", "retailer-peer-two");
  await page
    .getByRole("button", { name: "Save desired configuration" })
    .click();
  await expect.poll(() => revision).toBe(2);
  await page
    .getByRole("button", { name: "Review running-network change" })
    .click();
  await expect(
    page.getByText(/expansion preview does not match/),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Apply approved change" }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Review running-network change" })
    .click();
  const apply = page.getByRole("button", { name: "Apply approved change" });
  await expect(apply).toBeDisabled();
  await page
    .getByRole("checkbox", { name: /I approve this exact saved revision/ })
    .check();
  await apply.click();
  await expect(
    page.getByRole("heading", { name: "Operation expanded-op" }),
  ).toBeVisible();
  expect(deployed.spec.nodes.map((n) => n.id)).toContain("retailer-peer-two");
  await page
    .getByRole("button", { name: "Refresh current workload status" })
    .click();
  await expect(
    page.getByText("Workloads need attention", { exact: true }),
  ).toBeVisible();
  await page.getByLabel("Recovery target").selectOption("retailer-peer");
  await page
    .getByRole("button", { name: "Review recovery", exact: true })
    .click();
  await expect(page.getByText(/recovery preview does not match/)).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Apply approved recovery" }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Review recovery", exact: true })
    .click();
  const recover = page.getByRole("button", { name: "Apply approved recovery" });
  await expect(recover).toBeDisabled();
  await page
    .getByRole("checkbox", { name: /I approve this exact node recovery/ })
    .check();
  await recover.click();
  await expect(
    page.getByRole("heading", { name: "Operation recovered-op" }),
  ).toBeVisible();
  await expect(page.getByText(/This network is retired/)).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Save desired configuration" }),
  ).toBeDisabled();
  expect(errors).toEqual([]);
  console.log(
    "PASS: mobile builder, create, reviewed expansion, monitoring, approved recovery and retired fence (controlled API fixture)",
  );
} finally {
  await browser?.close();
  server.kill();
}
