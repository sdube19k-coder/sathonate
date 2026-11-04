import { chromium, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
let input = "";
for await (const chunk of process.stdin) input += chunk;
const config = JSON.parse(input),
  tokens = config.tokens;
input = "";
const checks = {},
  errors = [],
  records = [],
  progress = {};
let browser, page;
const safe = (value) => {
  let text = String(value);
  for (const token of Object.values(tokens))
    text = text.split(token).join("[REDACTED]");
  return text;
};
const assert = (value, message) => {
  if (!value) throw new Error(message);
};
const kub = (...args) =>
  execFileSync(
    "kubectl",
    [
      "--kubeconfig",
      config.kubeconfig,
      "--context",
      "kind-" + config.cluster,
      "--request-timeout=20s",
      ...args,
    ],
    { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] },
  );
try {
  browser = await chromium.launch({ channel: "msedge", headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1050 },
  });
  page = await context.newPage();
  let startupReadInterrupted = false;
  await page.route("**/api/v1/meta", async (route) => {
    if (!startupReadInterrupted) {
      startupReadInterrupted = true;
      await route.abort("connectionfailed");
    } else {
      await route.continue();
    }
  });
  page.on("pageerror", (error) => errors.push(safe(error.message)));
  page.on("request", (request) => {
    for (const token of Object.values(tokens))
      assert(!request.url().includes(token), "Token in URL");
  });
  page.on("response", (response) => {
    const url = new URL(response.url());
    if (url.pathname.startsWith("/api/"))
      records.push({
        path: url.pathname,
        method: response.request().method(),
        status: response.status(),
      });
  });
  page.on("requestfailed", (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith("/api/"))
      records.push({
        path: url.pathname,
        method: request.method(),
        failure: safe(request.failure()?.errorText ?? "request failed"),
      });
  });
  const enter = async (token) => {
    const field = page.getByLabel("Development token", { exact: true });
    await expect(field).toBeVisible();
    await field.evaluate((element, value) => {
      element.value = value;
    }, token);
    const responsePromise = page.waitForResponse(
      (response) => new URL(response.url()).pathname === "/api/v1/projects",
    );
    await page
      .getByRole("button", { name: "Authenticate", exact: true })
      .click();
    const response = await responsePromise;
    console.log("Authentication HTTP status: " + response.status());
  };
  const signIn = async (token) => {
    await enter(token);
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
    assert(
      startupReadInterrupted,
      "Startup read interruption was not exercised",
    );
    checks.workspaceReadConnectionRecovery = "PASS";
  };
  const fetchAs = async (token, path, method = "GET") =>
    page.evaluate(
      async ({ token, path, method }) => {
        const response = await fetch(path, {
          method,
          headers: {
            Authorization: "Bearer " + token,
            "Content-Type": "application/json",
            ...(method === "POST"
              ? { "Idempotency-Key": "negative-browser-probe" }
              : {}),
          },
          ...(method === "POST" ? { body: "{}" } : {}),
        });
        let body = null;
        try {
          body = await response.json();
        } catch {}
        return { status: response.status, body };
      },
      { token, path, method },
    );
  const configure = async (name) => {
    await page
      .getByRole("button", {
        name: "Next: Kubernetes Configuration",
        exact: true,
      })
      .click();
    await page
      .getByRole("button", { name: "Next: Network Design", exact: true })
      .click();
    await page.getByLabel("Secure network name", { exact: true }).fill(name);
    await page
      .getByRole("button", {
        name: "Next: Review & Cost Estimate",
        exact: true,
      })
      .click();
    await expect(
      page.getByRole("button", { name: "Create Network", exact: true }),
    ).toBeEnabled();
  };
  await page.goto("http://127.0.0.1:5173/");
  await enter(tokens.invalid);
  await expect(page.getByRole("alert")).toContainText(/Invalid or expired/);
  checks.invalidToken = "PASS";
  await page.waitForTimeout(13000);
  await enter(tokens.expired);
  await expect(page.getByRole("alert")).toContainText(/Invalid or expired/);
  checks.expiredToken = "PASS";
  await enter(tokens.primary);
  const projectSelect = page.getByLabel("Authorized project", { exact: true });
  await expect(projectSelect).toBeVisible();
  await projectSelect.evaluate((element) => {
    const option = document.createElement("option");
    option.value = "unauthorized-project";
    option.textContent = "Unauthorized";
    element.append(option);
    element.value = option.value;
    element.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await projectSelect.evaluate((element) =>
    element.form.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    ),
  );
  await expect(page.getByRole("alert")).toContainText(
    "Select an authorized project",
  );
  checks.unauthorizedProjectSelection = "PASS";
  if (config.authOnly) {
    console.log("Authentication browser checks: PASS (Fabric not exercised)");
    await browser.close();
    process.exit(0);
  }
  await projectSelect.selectOption(config.project);
  await page.getByRole("button", { name: "Open project", exact: true }).click();
  await expect(
    page.getByLabel("Control plane connected", { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Secure deployment profile")).toHaveValue(
    "smp-secure-local-v1",
  );
  await configure("Browser Fabric " + config.cluster.slice(-8));
  checks.profileAndServerValidation = "PASS";
  const started = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith("/deploy"),
  );
  await page
    .getByRole("button", { name: "Create Network", exact: true })
    .dblclick();
  const startResponse = await started;
  assert(startResponse.status() === 202, "UI deployment was not accepted");
  const initial = await startResponse.json();
  assert(initial.kind === "secure-local-deploy", "Wrong runner selected");
  await expect(page.locator(".smp-operation-id")).toHaveText(
    initial.operationId,
  );
  checks.uiSubmission = "PASS";
  await expect
    .poll(
      async () => {
        const value = await fetchAs(
          tokens.primary,
          "/api/v1/operations/" + initial.operationId,
        );
        return value.body?.currentStage;
      },
      { timeout: 180000 },
    )
    .toBe("ca");
  await page.reload();
  await expect(
    page.getByLabel("Development token", { exact: true }),
  ).toBeVisible();
  await signIn(tokens.primary);
  await expect(page.locator(".smp-operation-id")).toHaveText(
    initial.operationId,
  );
  checks.refreshWhileRunning = "PASS";
  await context.setOffline(true);
  await expect(page.getByRole("alert")).toContainText(/Cannot reach/, {
    timeout: 15000,
  });
  await context.setOffline(false);
  await expect(page.getByRole("alert")).toHaveCount(0, { timeout: 15000 });
  checks.networkInterruption = "PASS";
  assert(
    kub(
      "get",
      "node",
      config.cluster + "-control-plane",
      "-o",
      "jsonpath={.spec.providerID}",
    ) ===
      "kind://docker/" +
        config.cluster +
        "/" +
        config.cluster +
        "-control-plane",
    "Disposable identity mismatch",
  );
  const jobName = "smp-" + initial.operationId.replace(/^op_/, "");
  const jobs = JSON.parse(
    kub(
      "get",
      "jobs",
      "-A",
      "-l",
      "fabric.sathonate.io/operation=" + jobName,
      "-o",
      "json",
    ),
  ).items;
  assert(jobs.length === 1, "Exclusive disposable worker missing");
  const namespace = jobs[0].metadata.namespace;
  assert(
    namespace.startsWith("smp-s-") &&
      jobs[0].metadata.labels["fabric.sathonate.io/network"] ===
        initial.networkId,
    "Worker ownership mismatch",
  );
  kub(
    "-n",
    namespace,
    "delete",
    "pod",
    "-l",
    "job-name=" + jobName,
    "--wait=true",
    "--timeout=60s",
  );
  await expect(
    page.getByRole("button", { name: "Retry operation", exact: true }),
  ).toBeVisible({ timeout: 180000 });
  checks.realWorkerFailureVisible = "PASS";
  const retried = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith("/retry"),
  );
  await page
    .getByRole("button", { name: "Retry operation", exact: true })
    .click();
  const retryResponse = await retried;
  assert(retryResponse.status() === 202, "Authorized UI retry rejected");
  const retry = await retryResponse.json();
  assert(retry.retryOf === initial.operationId, "Retry linkage missing");
  checks.uiRetry = "PASS";
  await expect(
    page.getByText("Deployment verification passed.", { exact: true }),
  ).toBeVisible({ timeout: 1200000 });
  await page
    .getByRole("button", { name: "View Network Dashboard", exact: true })
    .click();
  const readback = page.locator(".smp-card").filter({
    has: page.getByRole("heading", {
      name: "Committed transaction readback",
      exact: true,
    }),
  });
  await expect(readback).toContainText(
    "org1-peer-1: Committed readback verified",
  );
  await expect(readback).toContainText(
    "org2-peer-1: Committed readback verified",
  );
  const result = (
    await fetchAs(tokens.primary, "/api/v1/deployments/" + retry.operationId)
  ).body;
  progress.baseline = result;
  assert(
    result.verified && !result.simulated && result.ready,
    "No real deployment verification",
  );
  assert(
    result.health.peers.length === 2 &&
      result.health.orderers.length === 3 &&
      result.health.certificateAuthorities.length === 2 &&
      result.health.chaincode.available,
    "Incomplete Fabric health",
  );
  assert(
    result.health.peers.every((peer) => peer.available) &&
      result.health.peers[0].currentBlockHash ===
        result.health.peers[1].currentBlockHash,
    "Peer ledger verification incomplete",
  );
  assert(/^[a-f0-9]{64}$/.test(result.transactionId), "Transaction ID missing");
  for (const name of [
    "org1-ca",
    "org2-ca",
    "orderer-1",
    "orderer-2",
    "orderer-3",
    "smp_smoke",
  ])
    await expect(page.getByText(name, { exact: true }).first()).toBeVisible();
  checks.caPeerOrdererChannelChaincodeAndBothReadbacks = "PASS";
  const before = records.filter(
    (row) =>
      row.method === "POST" &&
      row.path.endsWith("/deploy") &&
      row.status === 202,
  ).length;
  await page
    .getByRole("button", { name: "Verify recorded transaction", exact: true })
    .click();
  await expect(
    page.getByText(
      "Loaded server-recorded transaction and peer readback verification.",
      { exact: true },
    ),
  ).toBeVisible();
  assert(
    records.filter(
      (row) =>
        row.method === "POST" &&
        row.path.endsWith("/deploy") &&
        row.status === 202,
    ).length === before,
    "Verification redeployed the network",
  );
  assert(before === 1, "Repeated clicks created multiple deployment requests");
  checks.noDuplicateDeployment = "PASS";
  await page.screenshot({
    path: config.logDirectory + "/dashboard.png",
    fullPage: true,
    animations: "disabled",
  });
  await page.reload();
  await signIn(tokens.primary);
  await expect(readback).toContainText(result.transactionId);
  checks.finalRefreshRestoresStatus = "PASS";
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(readback).toBeVisible();
  await expect
    .poll(
      () =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 2,
        ),
      { timeout: 10000, message: "Mobile page overflows viewport" },
    )
    .toBe(true);
  await page.screenshot({
    path: config.logDirectory + "/mobile-dashboard.png",
    fullPage: true,
    animations: "disabled",
  });
  checks.responsiveDashboard = "PASS";
  await page.setViewportSize({ width: 1440, height: 1050 });
  let latestResult = result,
    latestOperationId = retry.operationId;
  const lifecycleChanges = [];
  progress.lifecycleChanges = lifecycleChanges;
  let canonicalGenesisSHA256;
  if (config.lifecycle) {
    const baseOperation = await fetchAs(
      tokens.primary,
      "/api/v1/operations/" + retry.operationId,
    );
    const canonicalEvent = baseOperation.body.events.find((e) =>
      e.message.startsWith("Verified canonical channel genesis SHA256 "),
    );
    canonicalGenesisSHA256 = canonicalEvent?.message.split(" ").at(-1);
    assert(
      /^[a-f0-9]{64}$/.test(canonicalGenesisSHA256 ?? ""),
      "Canonical genesis evidence missing",
    );
    for (const [action, label] of [
      ["add-peer", "Preview adding Org1 peer"],
      ["upgrade-chaincode", "Preview definition upgrade"],
      ["restart-peer", "Preview peer restart"],
      ["verify-network", "Preview new transaction"],
    ]) {
      let beforePod, beforePVC;
      if (action === "restart-peer") {
        beforePod = kub(
          "-n",
          namespace,
          "get",
          "pod",
          "-l",
          "release=org1-peer-1",
          "-o",
          "jsonpath={.items[0].metadata.uid}",
        );
        beforePVC = kub(
          "-n",
          namespace,
          "get",
          "pvc",
          "org1-peer-1",
          "-o",
          "jsonpath={.metadata.uid}",
        );
      }
      const parent = latestOperationId;
      const previewResponse = page.waitForResponse(
        (r) =>
          new URL(r.url()).pathname ===
          `/api/v1/deployments/${parent}/lifecycle/${action}/preview`,
      );
      await page.getByRole("button", { name: label, exact: true }).click();
      assert(
        (await previewResponse).status() === 200,
        "Lifecycle preview rejected",
      );
      await expect(
        page.getByRole("region", { name: "Lifecycle change preview" }),
      ).toBeVisible();
      const submitted = page.waitForResponse(
        (r) =>
          r.request().method() === "POST" &&
          new URL(r.url()).pathname ===
            `/api/v1/deployments/${parent}/lifecycle/${action}/deploy`,
      );
      await page
        .getByRole("button", {
          name: "Approve and execute change",
          exact: true,
        })
        .click();
      const response = await submitted;
      assert(response.status() === 202, "Approved lifecycle change rejected");
      const operation = await response.json();
      latestOperationId = operation.operationId;
      await expect(
        page.getByText("Deployment verification passed.", { exact: true }),
      ).toBeVisible({ timeout: 240000 });
      await page
        .getByRole("button", { name: "View Network Dashboard", exact: true })
        .click();
      const verified = await fetchAs(
        tokens.primary,
        "/api/v1/deployments/" + latestOperationId,
      );
      latestResult = verified.body;
      assert(
        verified.status === 200 &&
          latestResult.verified &&
          latestResult.health.peers.length === 3,
        "Lifecycle ledger evidence missing",
      );
      if (action !== "add-peer")
        assert(
          latestResult.health.chaincode.sequence === 2,
          "Upgrade sequence not live",
        );
      assert(
        new Set(latestResult.health.peers.map((p) => p.currentBlockHash))
          .size === 1,
        "Managed peer ledgers diverged",
      );
      await expect(readback).toContainText(
        "org1-peer-2: Committed readback verified",
      );
      if (action === "restart-peer") {
        assert(
          kub(
            "-n",
            namespace,
            "get",
            "pod",
            "-l",
            "release=org1-peer-1",
            "-o",
            "jsonpath={.items[0].metadata.uid}",
          ) !== beforePod,
          "Peer restart did not replace pod",
        );
        assert(
          kub(
            "-n",
            namespace,
            "get",
            "pvc",
            "org1-peer-1",
            "-o",
            "jsonpath={.metadata.uid}",
          ) === beforePVC,
          "Peer restart replaced storage",
        );
      }
      const completedOperation = await fetchAs(
        tokens.primary,
        "/api/v1/operations/" + latestOperationId,
      );
      const joinEvent = completedOperation.body.events.find((e) =>
        e.message.startsWith("Verified canonical channel genesis SHA256 "),
      );
      assert(
        joinEvent?.message.split(" ").at(-1) === canonicalGenesisSHA256,
        "Lifecycle change used a different channel genesis",
      );
      lifecycleChanges.push({
        action,
        operationId: latestOperationId,
        transactionId: latestResult.transactionId,
        health: latestResult.health,
      });
    }
    for (const action of ["add-orderer", "add-organization"])
      assert(
        (
          await fetchAs(
            tokens.primary,
            `/api/v1/deployments/${latestOperationId}/lifecycle/${action}/preview`,
          )
        ).status === 409,
        "Unsupported membership mutation enabled",
      );
    checks.guardedPeerAdditionAndSynchronization = "PASS";
    checks.chaincodeSequenceUpgrade = "PASS";
    checks.approvedPeerRestartAndPersistence = "PASS";
    checks.newTransactionOnExpandedNetwork = "PASS";
    checks.unsupportedMembershipChangesRejected = "PASS";
  }
  await page.getByRole("button", { name: "Operations", exact: true }).click();
  const downloadPromise = page.waitForEvent("download");
  await page
    .getByRole("button", {
      name: `Download diagnostics for ${retry.operationId}`,
      exact: true,
    })
    .click();
  const diagnosticDownload = await downloadPromise;
  assert(
    diagnosticDownload.suggestedFilename() ===
      "sathonate-operation-diagnostics.json",
    "Diagnostic download missing",
  );
  const diagnostics = await fetchAs(
    tokens.primary,
    "/api/v1/operations/" + retry.operationId + "/diagnostics",
  );
  assert(
    diagnostics.status === 200 && diagnostics.body.liveHealthChecked === false,
    "Diagnostic evidence mislabeled",
  );
  checks.diagnosticDownload = "PASS";
  for (const key of ["foreignTenant", "foreignProject"]) {
    for (const path of [
      "/api/v1/networks/" + initial.networkId,
      "/api/v1/operations/" + retry.operationId,
      "/api/v1/deployments/" + retry.operationId,
      "/api/v1/operations/" + retry.operationId + "/diagnostics",
    ])
      assert(
        (await fetchAs(tokens[key], path)).status === 404,
        "Foreign scope exposed an object",
      );
    assert(
      (
        await fetchAs(
          tokens[key],
          "/api/v1/operations/" + initial.operationId + "/retry",
          "POST",
        )
      ).status >= 400,
      "Foreign retry accepted",
    );
  }
  checks.crossTenantAndProjectDenials = "PASS";
  await page
    .getByRole("button", { name: "Sign out of local workspace", exact: true })
    .click();
  await signIn(tokens.noDeploy);
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("button", { name: "Networks", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Create Network", exact: true })
    .click();
  await configure("Denied browser deployment " + config.cluster.slice(-8));
  const denied = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith("/deploy"),
  );
  await page
    .getByRole("button", { name: "Create Network", exact: true })
    .click();
  assert((await denied).status() === 403, "Missing deploy permission accepted");
  await expect(page.getByRole("alert")).toContainText("does not permit");
  checks.missingPermissionVisible = "PASS";
  const storage = await page.evaluate(() =>
    JSON.stringify({
      local: { ...localStorage },
      session: { ...sessionStorage },
      url: location.href,
    }),
  );
  for (const token of Object.values(tokens))
    assert(!storage.includes(token), "Token persisted in browser storage");
  checks.memoryOnlyTokens = "PASS";
  const production = await context.newPage();
  await production.goto("http://127.0.0.1:5174/");
  await expect(production.getByRole("alert")).toContainText(
    "Local token authentication is disabled",
  );
  checks.productionBuildRejectsLocalTokens = "PASS";
  await production.close();
  const legacy = await context.newPage();
  await legacy.route("**/api/v1/auth/config", (route) =>
    route.fulfill({ json: { mode: "local-development" } }),
  );
  await legacy.goto("http://127.0.0.1:5173/");
  await expect(legacy.getByRole("alert")).toContainText(
    "Fixed developer authentication is disabled",
  );
  checks.fixedDeveloperBypassRejected = "PASS";
  await legacy.close();
  assert(errors.length === 0, "Browser runtime errors occurred");
  writeFileSync(
    config.result,
    JSON.stringify(
      {
        schema: "sathonate.ui-acceptance/v1",
        status: "PASS",
        checks,
        namespace,
        initialOperationId: initial.operationId,
        operationId: latestOperationId,
        baseOperationId: retry.operationId,
        lifecycleChanges,
        canonicalGenesisSHA256,
        transactionId: latestResult.transactionId,
        channel: latestResult.health.channel,
        chaincode: latestResult.health.chaincode,
        peers: latestResult.health.peers,
        orderers: latestResult.health.orderers,
        certificateAuthorities: latestResult.health.certificateAuthorities,
        browser: "Microsoft Edge via Playwright",
        browserRuntimeErrors: errors.length,
        apiResponses: records.length,
        checkedAt: new Date().toISOString(),
        productionEnabled: false,
        awsEnabled: false,
      },
      null,
      2,
    ) + "\n",
  );
  console.log("Real browser-to-Fabric acceptance: PASS");
} catch (error) {
  writeFileSync(
    config.result,
    JSON.stringify(
      {
        schema: "sathonate.ui-acceptance/v1",
        status: "FAIL",
        checks,
        progress,
        failure: safe(error.message),
        checkedAt: new Date().toISOString(),
      },
      null,
      2,
    ) + "\n",
  );
  if (page) {
    try {
      await page
        .locator("input[type=password]")
        .evaluateAll((inputs) =>
          inputs.forEach((element) => (element.value = "")),
        );
      await page.screenshot({
        path: config.logDirectory + "/failure.png",
        fullPage: true,
        animations: "disabled",
      });
    } catch {}
  }
  console.error("Browser acceptance failed: " + safe(error.message));
  console.error(
    safe(
      JSON.stringify({
        checks,
        requests: records,
        alerts: page ? await page.getByRole("alert").allTextContents() : [],
      }),
    ),
  );
  process.exitCode = 1;
} finally {
  await browser?.close();
}
