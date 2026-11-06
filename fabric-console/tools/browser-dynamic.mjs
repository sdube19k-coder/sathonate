import { chromium, expect } from "@playwright/test";
import { createHash } from "node:crypto";
import { writeFileSync } from "node:fs";
let input = "";
for await (const chunk of process.stdin) input += chunk;
const config = JSON.parse(input);
input = "";
const tokens = config.tokens,
  checks = {},
  records = [];
const safe = (value) =>
  Object.values(tokens).reduce(
    (s, t) => s.split(t).join("[REDACTED]"),
    String(value),
  );
const assert = (ok, message) => {
  if (!ok) throw new Error(message);
};
let browser, page;
try {
  browser = await chromium.launch({ channel: "msedge", headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1050 },
  });
  page = await context.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(safe(e.message)));
  page.on("request", (r) => {
    for (const token of Object.values(tokens))
      assert(!r.url().includes(token), "Token in URL");
  });
  page.on("response", (r) => {
    const u = new URL(r.url());
    if (u.pathname.startsWith("/api/"))
      records.push({
        path: u.pathname,
        status: r.status(),
        method: r.request().method(),
      });
  });
  async function signIn(token) {
    await expect(
      page.getByLabel("Development token", { exact: true }),
    ).toBeVisible();
    await page.getByLabel("Development token", { exact: true }).fill(token);
    await page
      .getByRole("button", { name: "Authenticate", exact: true })
      .click();
    await expect(
      page.getByLabel("Authorized project", { exact: true }),
    ).toBeVisible();
    await page
      .getByLabel("Authorized project", { exact: true })
      .selectOption(config.project);
    await page
      .getByRole("button", { name: "Open project", exact: true })
      .click();
    await expect(
      page.getByLabel("Control plane connected", { exact: true }),
    ).toBeVisible();
  }
  async function customView() {
    await page
      .getByRole("button", { name: "Networks", exact: true })
      .first()
      .click();
    await page
      .getByRole("button", { name: "Customer configurations", exact: true })
      .click();
    await expect(
      page.getByRole("heading", {
        name: "Customer-defined Fabric networks",
        exact: true,
      }),
    ).toBeVisible();
  }
  async function call(
    path,
    token = tokens.primary,
    method = "GET",
    body,
    headers = {},
  ) {
    return page.evaluate(
      async ({ path, token, method, body, headers }) => {
        const res = await fetch("/api/v1" + path, {
          method,
          headers: {
            "Content-Type": "application/json",
            Authorization: "Bearer " + token,
            ...headers,
          },
          ...(body ? { body: JSON.stringify(body) } : {}),
        });
        return { status: res.status, body: await res.json().catch(() => null) };
      },
      { path, token, method, body, headers },
    );
  }
  async function waitDeployment(id) {
    const deadline = Date.now() + 720000;
    while (Date.now() < deadline) {
      const op = await call("/operations/" + id);
      assert(op.status === 200, "Operation read failed");
      if (["failed", "cancelled", "interrupted"].includes(op.body.status))
        throw new Error("Recorded operation failed: " + op.body.error);
      if (op.body.status === "succeeded") {
        const result = await call("/deployments/" + id);
        assert(
          result.status === 200 && result.body.verified,
          "Verified deployment result missing",
        );
        return result.body;
      }
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
    throw new Error("Recorded deployment did not finish before the deadline");
  }
  async function saveSpec(spec, expectedStatus) {
    const response = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname.startsWith("/api/v1/networks") &&
        ["POST", "PUT"].includes(r.request().method()) &&
        r.status() === expectedStatus,
    );
    const editor = page.getByLabel("Customer network JSON");
    if (!(await editor.isVisible()))
      await page
        .getByText("Edit desired configuration", { exact: true })
        .click();
    await editor.fill(JSON.stringify(spec, null, 2));
    await page
      .getByRole("button", { name: "Save desired configuration", exact: true })
      .click();
    return (await response).json();
  }
  await page.goto("http://127.0.0.1:5173/");
  await page
    .getByLabel("Development token", { exact: true })
    .fill(tokens.invalid);
  await page.getByRole("button", { name: "Authenticate", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(/Invalid or expired/);
  checks.invalidToken = "PASS";
  await signIn(tokens.primary);
  await customView();
  const design = structuredClone(config.design);
  design.metadata.id = "browser-customer-" + config.cluster.slice(-8);
  design.metadata.name = "Browser customer-defined network";
  for (const ref of Object.keys(design.spec.configuration.placements))
    design.spec.configuration.placements[ref].nodeSelector = {
      "kubernetes.io/hostname": config.cluster + "-control-plane",
    };
  const artifacts = Object.entries(config.runtime.codeArtifacts),
    first = artifacts.find(([, a]) => a.codeVersion === "1.0"),
    second = artifacts.find(([, a]) => a.codeVersion === "2.0");
  assert(first && second, "Two approved executable versions required");
  for (const cc of design.spec.configuration.chaincodes) {
    cc.runtimeImage = first[0];
    cc.packageSHA256 = first[1].sha256;
  }
  const draft = await saveSpec(design, 201);
  checks.customerConfigurationSaved = "PASS";
  const deployPath = "/networks/" + draft.id + "/deploy",
    headers = { "If-Match": '"1"', "Idempotency-Key": "browser-denied-deploy" };
  assert(
    (await call(deployPath, tokens.noDeploy, "POST", {}, headers)).status ===
      403,
    "Read/write grant escalated to deployment",
  );
  checks.missingDeploymentGrantDenied = "PASS";
  const started = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === "/api/v1" + deployPath &&
      r.status() === 202,
  );
  await page
    .getByRole("button", { name: "Deploy saved network", exact: true })
    .click();
  const initial = await waitDeployment(
    (await (await started).json()).operationId,
  );
  assert(
    initial.verifications.length === 1 &&
      initial.verifications[0].health.peers.length === 2,
    "Initial customer topology evidence differs",
  );
  checks.initialBrowserProvisioningAndTransaction = "PASS";
  await page.reload();
  await signIn(tokens.primary);
  await customView();
  await page.getByLabel("Saved customer network").selectOption(draft.id);
  await expect(page.getByLabel("Recorded Fabric verification")).toContainText(
    initial.transactionId,
  );
  checks.refreshRestoresServerOwnedOperation = "PASS";
  const expanded = structuredClone(design),
    s = expanded.spec,
    cfg = s.configuration,
    owner = s.organizations[0],
    placement = s.nodes[0].placementRef,
    settings = structuredClone(cfg.nodes[s.nodes[0].id]);
  s.organizations.push({
    id: "inspection",
    mspId: "InspectionMSP",
    identityProviderRef: "inspection-root",
  });
  cfg.authorities.push({
    ...structuredClone(cfg.authorities[0]),
    id: "inspection-root",
    organizationRef: "inspection",
  });
  const peers = [
      {
        id: owner.id + "-expanded",
        organizationRef: owner.id,
        role: "peer",
        placementRef: placement,
        failureDomain: "local-host",
      },
      {
        id: "inspection-east",
        organizationRef: "inspection",
        role: "peer",
        placementRef: placement,
        failureDomain: "local-host",
      },
    ],
    orders = [
      {
        id: "raft-expanded-east",
        organizationRef: owner.id,
        role: "orderer",
        placementRef: placement,
        failureDomain: "local-host",
      },
      {
        id: "raft-expanded-west",
        organizationRef: owner.id,
        role: "orderer",
        placementRef: placement,
        failureDomain: "local-host",
      },
    ];
  for (const n of [...peers, ...orders]) {
    s.nodes.push(n);
    cfg.nodes[n.id] = structuredClone(settings);
  }
  for (const c of s.channels) {
    c.organizationRefs.push("inspection");
    c.peerRefs.push(...peers.map((n) => n.id));
    c.ordererRefs.push(...orders.map((n) => n.id));
    cfg.anchors[c.id] = cfg.anchors[c.id]
      .filter((id) => id !== design.spec.nodes[0].id)
      .concat(peers.map((n) => n.id));
  }
  for (const cc of cfg.chaincodes) {
    cc.sequence++;
    cc.version = "2.0";
    cc.runtimeImage = second[0];
    cc.packageSHA256 = second[1].sha256;
    cc.endorsement.mspIds.push("InspectionMSP");
    cc.endorsement.minimum = cc.endorsement.mspIds.length;
  }
  const extra = structuredClone(s.channels[0]);
  extra.id = "customer-added-ledger";
  s.channels.push(extra);
  cfg.anchors[extra.id] = [...cfg.anchors[s.channels[0].id]];
  cfg.chaincodes.push({
    ...structuredClone(cfg.chaincodes[0]),
    channelRef: extra.id,
    sequence: 1,
  });
  const updated = await saveSpec(expanded, 200);
  assert(updated.revision === 2, "Revision guard lost");
  await page
    .getByRole("button", { name: "Review running-network change", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "Review configuration change",
      exact: true,
    }),
  ).toBeVisible();
  const apply = page.getByRole("button", {
    name: "Apply approved change",
    exact: true,
  });
  await expect(apply).toBeDisabled();
  await page.getByRole("checkbox", { name: /I approve this exact/ }).check();
  await expect(apply).toBeEnabled();
  const applied = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith("/dynamic/deploy") &&
      r.status() === 202,
  );
  await apply.click();
  const final = await waitDeployment(
    (await (await applied).json()).operationId,
  );
  assert(final.verifications.length === 2, "Added channel not verified");
  for (const v of final.verifications) {
    assert(
      v.health.peers.length === 4 &&
        v.health.orderers.length === 5 &&
        v.codeVersion === "2.0",
      "Expanded members/code not verified",
    );
    assert(
      v.health.peers.every(
        (p) =>
          p.height === v.health.peers[0].height &&
          p.currentBlockHash === v.health.peers[0].currentBlockHash,
      ),
      "Expanded ledger inconsistency",
    );
  }
  assert(
    final.verifications.find((v) => v.health.channel === initial.health.channel)
      .packageId !== initial.verifications[0].packageId,
    "Upgrade retained previous package",
  );
  checks.reviewedExpansionMembershipChannelAnchorAndCodeUpgrade = "PASS";
  for (const t of [tokens.foreignTenant, tokens.foreignProject])
    assert(
      (await call("/deployments/" + final.operationId, t)).status === 404,
      "Foreign project/tenant saw deployment",
    );
  checks.crossTenantProjectDenied = "PASS";
  await expect(page.getByLabel("Recorded Fabric verification")).toContainText(
    final.transactionId,
  );
  assert(errors.length === 0, "Browser runtime error");
  const storage = await page.evaluate(() => [
    ...Object.values(localStorage),
    ...Object.values(sessionStorage),
  ]);
  assert(
    !storage.some((v) => Object.values(tokens).some((t) => v.includes(t))),
    "Token persisted in browser storage",
  );
  checks.credentialsMemoryOnly = "PASS";
  await page.setViewportSize({ width: 390, height: 844 });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
    "Customer dashboard overflows mobile viewport",
  );
  checks.responsiveDashboard = "PASS";
  await page.screenshot({
    path: config.logDirectory + "/dynamic-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1050 });
  await page.screenshot({
    path: config.logDirectory + "/dynamic-desktop.png",
    fullPage: true,
  });
  const expectedReleases = [
    ...cfg.authorities.map((a) => a.id),
    ...s.nodes.map((n) => n.id),
  ];
  const expectedWorkloads = [
    ...expectedReleases,
    ...cfg.chaincodes.map(
      (cc) =>
        "cc-" +
        createHash("sha256")
          .update(cc.channelRef + "/" + cc.name)
          .digest("hex")
          .slice(0, 16) +
        "-s" +
        cc.sequence,
    ),
  ];
  const tenantKey =
    "lp-" +
    createHash("sha256")
      .update(JSON.stringify([config.tenant, config.project]))
      .digest("hex")
      .slice(0, 48);
  const namespace =
    "smp-s-" +
    createHash("sha256")
      .update(JSON.stringify([tenantKey, "token-bound-project", draft.id]))
      .digest("hex")
      .slice(0, 32);
  writeFileSync(
    config.result,
    JSON.stringify(
      {
        schema: "sathonate.sprint4-dynamic-browser/v1",
        status: "PASS",
        checks,
        namespace,
        initial,
        final,
        expectedReleases,
        expectedWorkloads,
        inspectionPeers: s.nodes
          .filter((n) => n.role === "peer")
          .map((n) => n.id),
        networkId: draft.id,
        requests: records,
      },
      null,
      2,
    ) + "\n",
  );
  console.log("Dynamic browser acceptance PASS");
} catch (error) {
  writeFileSync(
    config.result,
    JSON.stringify(
      {
        schema: "sathonate.sprint4-dynamic-browser/v1",
        status: "FAIL",
        checks,
        requests: records,
        error: safe(error.message),
      },
      null,
      2,
    ) + "\n",
  );
  console.error("Dynamic browser acceptance failed: " + safe(error.message));
  process.exitCode = 1;
} finally {
  await browser?.close();
}
