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
  let saved;
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/api/v1/networks" && request.method() === "POST") {
      saved = request.postDataJSON();
      return route.fulfill({
        status: 201,
        json: {
          id: "fixture-network",
          desired: saved,
          revision: 1,
          issues: [],
        },
      });
    }
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
  expect(errors).toEqual([]);
  console.log(
    "PASS: mobile visual builder, six steps, server inventory and save payload (controlled API fixture)",
  );
} finally {
  await browser?.close();
  server.kill();
}
