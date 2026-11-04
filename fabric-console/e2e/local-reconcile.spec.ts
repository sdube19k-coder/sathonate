import { test, expect } from "@playwright/test";

test("saved local deployment resumes, verifies another transaction, reads logs and cleans up", async ({
  page,
  request,
}) => {
  const original = process.env.SMP_EXISTING_OPERATION_ID;
  test.skip(
    process.env.SMP_LOCAL_E2E !== "1" || !original,
    "Explicit existing test operation required; this test deletes its own selected network",
  );
  test.setTimeout(600000);
  page.setDefaultTimeout(30000);
  const op = await (await request.get(`/api/v1/operations/${original}`)).json();
  expect(op.kind).toBe("local-kubernetes-deploy");
  const before = await (
    await request.get(`/api/v1/deployments/${original}`)
  ).json();
  expect(before.verified).toBe(true);
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("button", { name: "Networks", exact: true })
    .click();
  await page.getByLabel("Search networks").fill(before.network.metadata.name);
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", {
        name: before.network.metadata.name,
        exact: true,
      }),
    })
    .click();
  await expect(
    page.getByRole("button", { name: "View Network Dashboard", exact: true }),
  ).toBeEnabled({ timeout: 15000 });
  await page
    .getByRole("button", { name: "View Network Dashboard", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Run Transaction Check", exact: true })
    .click();
  await expect
    .poll(
      () =>
        page.evaluate(
          () =>
            JSON.parse(sessionStorage.getItem("fabric-smp-flow-v2")!)
              .deploymentOperationId,
        ),
      { timeout: 15000 },
    )
    .not.toBe(original);
  const id = await page.evaluate(
    () =>
      JSON.parse(sessionStorage.getItem("fabric-smp-flow-v2")!)
        .deploymentOperationId,
  );
  await expect
    .poll(
      async () => {
        const result = await (
          await request.get(`/api/v1/operations/${id}`)
        ).json();
        if (result.status === "failed") throw new Error(result.error);
        return result.status;
      },
      { timeout: 300000, intervals: [2000] },
    )
    .toBe("succeeded");
  const result = await (await request.get(`/api/v1/deployments/${id}`)).json();
  expect(result.transactionId).not.toBe(before.transactionId);
  expect(result.verified).toBe(true);
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
  await expect(page.locator(".smp-node-logs")).not.toContainText("PRIVATE KEY");
  await page
    .getByRole("button", { name: "Delete Local Network", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(
    (await (await request.get(`/api/v1/deployments/${id}`)).json()).phase,
  ).toBe("running");
  if (process.env.SMP_KEEP_ACCEPTANCE_NETWORK === "1") {
    await page.screenshot({
      path: "test-results/local-kubernetes-dashboard.png",
      fullPage: true,
    });
    console.log(
      JSON.stringify({
        operationId: id,
        transactionId: result.transactionId,
        retained: true,
      }),
    );
    return;
  }
  await page
    .getByRole("button", { name: "Delete Local Network", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Delete Network and Volumes", exact: true })
    .click();
  await expect
    .poll(
      async () =>
        (await (await request.get(`/api/v1/deployments/${id}`)).json()).phase,
      { timeout: 120000, intervals: [2000] },
    )
    .toBe("removed");
});
