import { test, expect } from "@playwright/test";

test("step-one reference shell remains consistent through design and cloud readiness", async ({
  page,
  request,
}) => {
  await page.setViewportSize({ width: 1536, height: 1024 });
  await page.goto("/");
  await expect(
    page.getByLabel("Control plane connected", { exact: true }),
  ).toBeVisible();
  const shell = async (step: number) => {
    await expect(page.locator(".smp-reference")).toHaveAttribute(
      "data-step",
      String(step),
    );
    const header = await page.locator(".smp-header").boundingBox();
    const steps = await page
      .getByRole("navigation", { name: "Network creation steps" })
      .boundingBox();
    const panel = await page.locator(".smp-content").boundingBox();
    expect(header?.height).toBe(60);
    expect(header?.x).toBe(0);
    expect(steps?.width).toBe(318);
    expect(steps?.x).toBe(0);
    expect(steps?.y).toBe(60);
    expect(panel?.x).toBe(329);
    expect(panel?.y).toBe(79);
    expect(await page.locator(".smp-sidebar").count()).toBe(0);
    await expect(
      page
        .getByRole("navigation", { name: "Network creation steps" })
        .getByRole("button"),
    ).toHaveCount(6);
    await page.screenshot({
      path: `test-results/reference-step-${step}.png`,
      fullPage: true,
    });
  };
  await expect(page.locator(".smp-providers .smp-provider")).toHaveCount(4);
  await shell(1);
  await page
    .getByRole("button", {
      name: "Next: Kubernetes Configuration",
      exact: true,
    })
    .click();
  await shell(2);
  await page
    .getByRole("button", { name: "Next: Network Design", exact: true })
    .click();
  await shell(3);
  await page
    .getByLabel("Network Name *", { exact: true })
    .fill("Visual reference review");
  await page
    .getByRole("button", { name: "Next: Review & Cost Estimate", exact: true })
    .click();
  await shell(4);
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("button", { name: "Cloud readiness", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Cloud Launch Readiness", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Run Read-only Cloud Checks", exact: true })
    .click();
  await expect(
    page.getByText("No cloud command was run.", { exact: false }),
  ).toBeVisible();
  const report = await (await request.get("/api/v1/readiness")).json();
  expect(report.ready).toBe(false);
  expect(report.executionEnabled).toBe(false);
  await page.screenshot({
    path: "test-results/cloud-launch-readiness.png",
    fullPage: true,
  });
  const downloadReady = page.waitForEvent("download");
  await page
    .getByRole("button", {
      name: "Download Configuration Template",
      exact: true,
    })
    .click();
  expect((await downloadReady).suggestedFilename()).toBe(
    "fabric-smp-cloud.env.template",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
});
