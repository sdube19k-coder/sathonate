import { randomBytes } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";

async function fixture(page: Page, owner: boolean, enabled = true) {
  const token = randomBytes(32).toString("hex");
  const requests: string[] = [];
  let trial = false;
  let accepted = false;
  let project = "";
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace("/api/v1", "");
    if (path === "/auth/config")
      return route.fulfill({ json: { mode: "local-token" } });
    if (request.headers().authorization !== "Bearer " + token)
      return route.fulfill({ status: 401 });
    if (path === "/projects")
      return route.fulfill({
        json: { items: [{ id: "existing", tenantId: "fixture-tenant" }] },
      });
    if (path === "/meta")
      return route.fulfill({
        json: {
          customerPilot: enabled,
          deploymentMode: "disabled",
          executionEnabled: false,
        },
      });
    if (path === "/customer")
      return route.fulfill({
        json: {
          account: {
            name: owner || accepted ? "Pilot organization" : "",
            isOwner: owner,
            projects: project
              ? [
                  {
                    id: "project-fixture",
                    name: project,
                    state: "awaiting-identity-approval",
                  },
                ]
              : [],
            subscription: trial ? { plan: "pilot", status: "trialing" } : {},
            activeEntitlement: trial,
            invoices: owner
              ? [
                  {
                    id: "invoice-fixture",
                    currency: "USD",
                    amountMinor: 4900,
                    status: "paid",
                  },
                ]
              : [],
            invitations: [],
            pendingInvitations:
              owner || accepted
                ? []
                : [{ id: "invite-fixture", expiresAt: "2027-01-15T00:00:00Z" }],
          },
          plans: [
            {
              id: "pilot",
              monthlyMinor: 0,
              currency: "USD",
              activeProjects: 3,
            },
          ],
          canManage: owner,
          paymentsEnabled: false,
          infrastructureEnabled: false,
        },
      });
    if (
      path === "/customer/actions" ||
      path === "/customer/invitations/accept"
    ) {
      const key = request.headers()["idempotency-key"];
      expect(key).toMatch(/^[a-f0-9-]{36}$/);
      const data = request.postDataJSON();
      requests.push(key);
      if (owner && data.action === "start-trial") {
        trial = true;
        if (requests.length === 1)
          return route.fulfill({
            status: 503,
            json: { error: { message: "Retry after temporary failure." } },
          });
      } else if (owner && data.action === "request-project") {
        project = data.name;
      } else if (!owner && data.action === "accept-invitation") {
        expect(path).toBe("/customer/invitations/accept");
        expect(data.reference).toBe("invite-fixture");
        accepted = true;
      } else {
        throw new Error("Unexpected customer mutation");
      }
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/");
  await page.getByLabel("Development token").fill(token);
  await page.getByRole("button", { name: "Authenticate" }).click();
  await page.getByLabel("Authorized project").selectOption("existing");
  await page.getByRole("button", { name: /open project/i }).click();
  return requests;
}

test("owner can retry a trial and request a project", async ({ page }) => {
  const requests = await fixture(page, true);
  await page.getByRole("button", { name: "Customer account" }).click();
  await expect(
    page.getByRole("heading", { name: "Pilot organization" }),
  ).toBeVisible();
  await expect(
    page.getByText(/payments and infrastructure provisioning are disabled/i),
  ).toBeVisible();
  await page.getByRole("button", { name: "Start 14-day trial" }).click();
  await expect(page.getByRole("alert")).toHaveText(
    "Retry after temporary failure.",
  );
  await page.getByRole("button", { name: "Start 14-day trial" }).click();
  await expect(page.getByText("Entitlement active")).toBeVisible();
  expect(requests[0]).toBe(requests[1]);
  await page.getByLabel("New project name").fill("Customer-owned Kubernetes");
  await page.getByRole("button", { name: "Request project" }).click();
  await expect(
    page.getByText(/Customer-owned Kubernetes · awaiting identity approval/),
  ).toBeVisible();
  await expect(page.getByText(/invoice-fixture.*paid/)).toBeVisible();
});

test("invitee can accept without owner controls", async ({ page }) => {
  await fixture(page, false);
  await page.getByRole("button", { name: "Customer account" }).click();
  await page.getByRole("button", { name: /accept invitation/i }).click();
  await expect(
    page.getByRole("heading", { name: "Pilot organization" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Invoices and usage" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Create invitation" }),
  ).toHaveCount(0);
});

test("customer navigation is absent when the pilot is disabled", async ({
  page,
}) => {
  await fixture(page, true, false);
  await expect(
    page.getByRole("button", { name: "Customer account" }),
  ).toHaveCount(0);
});
