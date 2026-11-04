import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";

test("real kind wizard, interrupted job recovery, Fabric transaction and dashboard", async ({
  page,
  request,
}) => {
  test.skip(
    process.env.SMP_LOCAL_E2E !== "1",
    "Requires the documented real local environment",
  );
  test.setTimeout(1800000);
  page.setDefaultTimeout(30000);
  await page.setViewportSize({ width: 1536, height: 1024 });
  const recordStep = async (step: number) => {
    await expect(page.locator(".smp-reference")).toHaveAttribute(
      "data-step",
      String(step),
    );
    await page.evaluate(() => window.scrollTo(0, 0));
    const panel = await page.locator(".smp-content").boundingBox();
    const sidebar = await page.locator(".smp-stepper").boundingBox();
    expect(panel?.x).toBe(329);
    expect(panel?.y).toBe(79);
    expect(sidebar?.width).toBe(318);
    await page.screenshot({
      path: `test-results/local-step-${step}.png`,
      fullPage: true,
    });
  };
  const kubeconfig =
    process.env.SMP_KUBECONFIG ??
    "E:\\Sathonate\\.cache\\smp-qualification.kubeconfig";
  const kub = (...args: string[]) =>
    execFileSync(
      "kubectl",
      [
        "--kubeconfig",
        kubeconfig,
        "--context",
        "kind-smp-qualification",
        ...args,
      ],
      { encoding: "utf8", timeout: 120000 },
    );
  await page.goto("/");
  await page.getByRole("button", { name: /Other \/ Existing/ }).click();
  await page
    .getByRole("button", { name: "Local Kubernetes", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Test Connection", exact: true })
    .click();
  await expect(
    page.getByText(/Local Kubernetes runner configured/),
  ).toBeVisible();
  await recordStep(1);
  await page
    .getByRole("button", {
      name: "Next: Kubernetes Configuration",
      exact: true,
    })
    .click();
  await recordStep(2);
  await page
    .getByRole("button", { name: "Next: Network Design", exact: true })
    .click();
  await page
    .getByLabel("Network Name *", { exact: true })
    .fill(`Local acceptance ${Date.now()}`);
  await recordStep(3);
  await page
    .getByRole("button", { name: "Next: Review & Cost Estimate", exact: true })
    .click();
  await recordStep(4);
  await page
    .getByRole("button", { name: "Create Network", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Deployment Progress", exact: true }),
  ).toBeVisible();
  await recordStep(5);
  const draft = await page.evaluate(() =>
    JSON.parse(sessionStorage.getItem("fabric-smp-flow-v2")!),
  );
  let id = draft.deploymentOperationId as string;
  expect(id).toMatch(/^op_[a-f0-9]{32}$/);
  const state = async () =>
    (await request.get(`/api/v1/deployments/${id}`)).json();
  const op = async () => (await request.get(`/api/v1/operations/${id}`)).json();
  // Terminate only this test's worker after CA creation; retry must adopt its identities.
  await expect
    .poll(async () => (await op()).currentStage, {
      timeout: 300000,
      intervals: [2000],
    })
    .toMatch(/ca|orderers|peers|channels|verify/);
  const job = `smp-${id.slice(3)}`;
  kub(
    "delete",
    "jobs",
    "-A",
    "-l",
    `fabric.sathonate.io/operation=${job}`,
    "--wait=true",
  );
  await expect
    .poll(async () => (await op()).status, {
      timeout: 60000,
      intervals: [2000],
    })
    .toBe("failed");
  await expect(page.getByRole("button", { name: /Retry/ })).toBeEnabled({
    timeout: 15000,
  });
  await page.getByRole("button", { name: /Retry/ }).click();
  await expect
    .poll(
      async () =>
        page.evaluate(
          () =>
            JSON.parse(sessionStorage.getItem("fabric-smp-flow-v2")!)
              .deploymentOperationId,
        ),
      { timeout: 15000 },
    )
    .not.toBe(id);
  id = await page.evaluate(
    () =>
      JSON.parse(sessionStorage.getItem("fabric-smp-flow-v2")!)
        .deploymentOperationId,
  );
  await expect
    .poll(
      async () => {
        const v = await op();
        if (v.status === "failed") throw new Error(v.error);
        return v.status;
      },
      { timeout: 1200000, intervals: [3000] },
    )
    .toBe("succeeded");
  const result = await state();
  expect(result.mode).toBe("local-kubernetes");
  expect(result.simulated).toBe(false);
  expect(result.verified).toBe(true);
  expect(result.ready).toBe(true);
  expect(result.transactionId).toMatch(/^[a-f0-9]{64}$/);
  const health = result.health;
  expect(health.ready).toBe(true);
  expect(health.peers).toHaveLength(2);
  expect(health.peers[0].height).toBeGreaterThan(1);
  expect(health.peers[0].height).toBe(health.peers[1].height);
  expect(health.peers[0].currentBlockHash).toBe(
    health.peers[1].currentBlockHash,
  );
  expect(health.orderers).toHaveLength(3);
  for (const o of health.orderers) {
    expect(o.status).toBe("active");
    expect(o.consensusRelation).toBe("consenter");
  }
  expect(health.certificateAuthorities).toHaveLength(2);
  for (const ca of health.certificateAuthorities) {
    expect(ca.available).toBe(true);
    expect(ca.tls).toBe(true);
  }
  // Discover the namespace from this operation's unique job label (tenant is server owned).
  const jobs = JSON.parse(
    kub(
      "get",
      "jobs",
      "-A",
      "-l",
      `fabric.sathonate.io/operation=smp-${id.slice(3)}`,
      "-o",
      "json",
    ),
  );
  const ns = jobs.items[0].metadata.namespace as string;
  expect(ns).toMatch(/^smp-n-[a-f0-9]{16}$/);
  const secret = JSON.parse(
    kub("-n", ns, "get", "secret", "smp-enrollment-org1", "-o", "json"),
  );
  const password = Buffer.from(secret.data.password, "base64").toString();
  const journal = JSON.stringify(await op());
  expect(journal.includes(password)).toBe(false);
  expect(journal).not.toContain("PRIVATE KEY");
  await expect(
    page.getByRole("button", { name: "View Network Dashboard", exact: true }),
  ).toBeEnabled({ timeout: 15000 });
  await page
    .getByRole("button", { name: "View Network Dashboard", exact: true })
    .click();
  await expect(page.getByText("TLS verified", { exact: true })).toHaveCount(2);
  await recordStep(6);
  await page.reload();
  await expect(page.getByText("TLS verified", { exact: true })).toHaveCount(2, {
    timeout: 15000,
  });
  await page.screenshot({
    path: "test-results/local-kubernetes-dashboard.png",
    fullPage: true,
  });
  // Controller status alone must not mask a stopped peer.
  const peer = health.peers[0].peer;
  try {
    kub("-n", ns, "scale", "deployment", peer, "--replicas=0");
    kub(
      "-n",
      ns,
      "wait",
      "--for=delete",
      "pod",
      "-l",
      `release=${peer}`,
      "--timeout=60s",
    );
    await expect
      .poll(
        async () =>
          (await (await request.get(`/api/v1/deployments/${id}/health`)).json())
            .ready,
        { timeout: 30000 },
      )
      .toBe(false);
  } finally {
    kub("-n", ns, "scale", "deployment", peer, "--replicas=1");
  }
  kub("-n", ns, "rollout", "status", `deployment/${peer}`, "--timeout=120s");
  if (process.env.SMP_KEEP_ACCEPTANCE_NETWORK !== "1") {
    const response = await request.delete(`/api/v1/deployments/${id}`);
    expect(response.ok()).toBe(true);
    expect((await state()).phase).toBe("removed");
  }
  console.log(
    JSON.stringify({
      operationId: id,
      namespace: ns,
      transactionId: result.transactionId,
      height: health.peers[0].height,
    }),
  );
});
