import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

test("PDF cloud screens share one shell and save/resume a validated design", async ({
  page,
  request,
}) => {
  const name = `PDF design ${Date.now()}`;
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await expect(
    page.getByRole("heading", {
      name: "Connect Your Cloud Account",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByLabel("Control plane connected", { exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("navigation", { name: "Network creation steps" })
      .getByRole("button"),
  ).toHaveCount(6);
  await expect(
    page.getByRole("button", { name: /Google Cloud Platform/ }),
  ).toBeDisabled();
  await page
    .getByLabel("IAM Role ARN", { exact: true })
    .fill("arn:aws:iam::123456789012:role/FabricDeploymentRole");
  await page.getByLabel("AWS Account ID", { exact: true }).fill("123456789012");
  await page.getByLabel("AWS Region", { exact: true }).fill("ap-south-1");
  await page
    .getByRole("button", { name: "Test Connection", exact: true })
    .click();
  await expect(
    page.getByText(/Not connected\. Cloud role onboarding/),
  ).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-1-cloud.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-1-cloud-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page
    .getByRole("button", {
      name: "Next: Kubernetes Configuration",
      exact: true,
    })
    .click();
  await page.getByRole("button", { name: /Create New Cluster/ }).click();
  await page.getByLabel("Cluster Name", { exact: true }).fill("fabric-prod");
  await page.getByLabel("VPC CIDR Block", { exact: true }).fill("10.0.0.0/16");
  await page.getByLabel("Service CIDR", { exact: true }).fill("10.0.0.0/24");
  await page
    .getByRole("button", { name: "Next: Network Design", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("CIDRs overlap");
  await page.getByLabel("Service CIDR", { exact: true }).fill("10.1.0.0/16");
  await page.getByLabel("Pod CIDR", { exact: true }).fill("10.2.0.0/16");
  await page
    .getByRole("checkbox", { name: "Persistent Volume CSI Driver" })
    .check();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-2-kubernetes.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-2-kubernetes-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page
    .getByRole("button", { name: "Next: Network Design", exact: true })
    .click();
  await page.getByLabel("Network Name", { exact: false }).fill(name);
  await page.getByLabel("Domain", { exact: true }).fill("fabric.example.com");
  await page
    .getByRole("button", { name: "Add Organization", exact: true })
    .click();
  await page
    .getByLabel("Organization 3 name", { exact: true })
    .fill("Distributor");
  await page
    .getByLabel("Peers per Organization", { exact: false })
    .selectOption("2");
  await page.getByLabel("Orderer Nodes", { exact: false }).selectOption("5");
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-3-design.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-3-design-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page
    .getByRole("button", { name: "Next: Review & Cost Estimate", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Review & Cost Estimate", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Create Network", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByText("Estimate unavailable", { exact: true }),
  ).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-4-review.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-4-review-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1100 });
  const downloadReady = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Prepare Deployment JSON", exact: true })
    .click();
  const download = await downloadReady;
  const downloadPath = await download.path();
  expect(downloadPath).toBeTruthy();
  const deployment = JSON.parse(await readFile(downloadPath!, "utf8"));
  expect(deployment.network.spec.deployment.cloud.accountId).toBe(
    "123456789012",
  );
  expect(deployment.network.spec.deployment.cloud.clusterName).toBe(
    "fabric-prod",
  );
  expect(deployment.network.spec.deployment.cloud.addons).toContain(
    "Persistent Volume CSI Driver",
  );
  expect(deployment.networkSHA256).toMatch(/^[a-f0-9]{64}$/);
  expect(deployment.executable).toBe(false);
  expect(deployment.checks[0].id).toBe("registered-target");
  await expect(page.getByRole("status")).toContainText(
    "Deployment JSON prepared",
  );
  const all = await (await request.get("/api/v1/networks")).json();
  const draft = all.items.find(
    (n: { desired: { metadata: { name: string } } }) =>
      n.desired.metadata.name === name,
  );
  expect(draft).toBeTruthy();
  expect(draft.desired.spec.organizations).toHaveLength(3);
  expect(draft.desired.spec.deployment.cloud.region).toBe("ap-south-1");
  expect(
    draft.desired.spec.nodes.filter((n: { role: string }) => n.role === "peer"),
  ).toHaveLength(6);
  await page.reload();
  await page
    .getByRole("navigation", { name: "Network creation steps" })
    .getByRole("button", { name: /Network Design/ })
    .click();
  await expect(page.getByLabel("Network Name", { exact: false })).toHaveValue(
    name,
  );
  await page
    .getByLabel("Network Name", { exact: false })
    .fill(`${name} revised`);
  await page.getByRole("button", { name: "Save Changes", exact: true }).click();
  await expect
    .poll(
      async () =>
        (await (await request.get(`/api/v1/networks/${draft.id}`)).json())
          .revision,
    )
    .toBe(2);
  const archived = await request.delete(`/api/v1/networks/${draft.id}`, {
    headers: { "If-Match": '"2"' },
  });
  expect(archived.status()).toBe(204);
  expect(errors).toEqual([]);
});

test("all six consistent screens drive real deployment and live dashboard", async ({
  page,
  request,
}) => {
  test.skip(
    process.env.SMP_DOCKER_E2E !== "1",
    "Opt in to the separate Docker-only profile",
  );
  test.setTimeout(150000);
  const state = await (await request.get("/api/v1/local-runtime")).json();
  test.skip(!state.dockerAvailable, "Requires Linux Docker runtime");
  await page.goto("/");
  await page
    .getByRole("button", { name: "Other / Existing", exact: false })
    .click();
  await page
    .getByRole("button", { name: "Local / Existing Docker", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Test Connection", exact: true })
    .click();
  await expect(
    page.getByText(/Connected to the Linux Docker engine/),
  ).toBeVisible();
  await page
    .getByRole("button", {
      name: "Next: Kubernetes Configuration",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "Kubernetes Configuration",
      exact: true,
    }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Next: Network Design", exact: true })
    .click();
  await expect(
    page.getByLabel("Fabric Version", { exact: false }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: "Next: Review & Cost Estimate", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Create Network", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Deployment Progress", exact: true }),
  ).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-5-progress.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-5-progress-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1100 });
  await expect(
    page.getByRole("button", { name: "View Network Dashboard", exact: true }),
  ).toBeEnabled({ timeout: 100000 });
  await page
    .getByRole("button", { name: "View Network Dashboard", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Network Dashboard", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("TLS verified", { exact: true })).toHaveCount(3, {
    timeout: 25000,
  });
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Network Dashboard", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("TLS verified", { exact: true })).toHaveCount(3, {
    timeout: 25000,
  });
  await expect(
    page
      .getByRole("navigation", { name: "Network creation steps" })
      .getByRole("button"),
  ).toHaveCount(6);
  await expect(
    page.getByRole("navigation", { name: "Network creation steps" }),
  ).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-6-dashboard.png",
    fullPage: true,
  });
  const before = (await (await request.get("/api/v1/local-runtime")).json())
    .transactionId;
  await page
    .getByRole("button", { name: "Run Transaction Check", exact: true })
    .click();
  await expect
    .poll(
      async () => {
        const s = await (await request.get("/api/v1/local-runtime")).json();
        return s.ready && s.transactionId !== before;
      },
      { timeout: 30000 },
    )
    .toBe(true);
  await expect(
    page.getByRole("button", { name: "View Network Dashboard", exact: true }),
  ).toBeEnabled({ timeout: 15000 });
  await page
    .getByRole("button", { name: "View Network Dashboard", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Read Node Logs", exact: true })
    .click();
  await expect(page.locator(".smp-node-logs")).not.toBeEmpty();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-6-mobile.png",
    fullPage: true,
  });
});

test("mobile layout, provider-specific fields and auth failure recovery", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await page.getByRole("button", { name: /Microsoft Azure/ }).click();
  await expect(
    page.getByLabel("Subscription ID", { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("IAM Role ARN", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("button", { name: "Cloud readiness", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Workspace Configuration", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Create Network", exact: true })
    .click();
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-1-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/pdf-step-1-mobile-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.route("**/api/v1/auth/config", (r) =>
    r.fulfill({ status: 503, body: "unavailable" }),
  );
  await page.reload();
  await expect(page.getByRole("alert")).toContainText(
    "Cannot load authentication settings",
  );
  await expect(page.locator(".smp")).toHaveCount(0);
  await page.unroute("**/api/v1/auth/config");
  await page.getByRole("button", { name: "Retry connection" }).click();
  await expect(page.locator(".smp")).toBeVisible();
});
