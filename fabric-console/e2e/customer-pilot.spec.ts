import { randomBytes } from "node:crypto";
import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";

async function fixture(
  page: Page,
  owner: boolean,
  enabled = true,
  deploymentMode = "disabled",
  networks: unknown[] = [],
) {
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
          deploymentMode,
          executionEnabled: false,
        },
      });
    if (path === "/networks")
      return route.fulfill({ json: { items: networks } });
    if (path === "/readiness")
      return route.fulfill({
        json: {
          checkedAt: "2026-01-01T00:00:00Z",
          ready: false,
          executionEnabled: false,
          probed: false,
          checks: [],
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
  const signOut = page.locator("header").getByRole("button", {
    name: "Sign out of local workspace",
  });
  await expect(signOut).toBeVisible();
  await expect(signOut).toHaveCSS("position", "static");
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

for (const mode of ["disabled", "secure-eks"]) {
  test(`saved EKS design respects the ${mode} runner`, async ({ page }) => {
    const desired = JSON.parse(
      readFileSync(
        new URL(
          "../../fabric-control-plane/examples/eks-qualification-network.json",
          import.meta.url,
        ),
        "utf8",
      ),
    );
    desired.spec.deployment.targetRef = "smp-secure-eks-v1";
    await fixture(page, true, true, mode, [
      { id: "eks-fixture", revision: 3, desired, issues: [], archived: false },
    ]);
    await page.route("**/api/v1/networks/eks-fixture/deploy", async (route) => {
      expect(mode).toBe("secure-eks");
      expect(route.request().method()).toBe("POST");
      expect(route.request().headers()["if-match"]).toBe('"3"');
      expect(route.request().postDataJSON()).toEqual({});
      await route.fulfill({
        status: 403,
        json: { error: { message: "Test membership is read-only." } },
      });
    });
    await page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("button", { name: "Networks", exact: true })
      .click();
    await page.getByRole("button", { name: /EKS qualification/ }).click();
    const deploy = page.getByRole("button", {
      name: "Deploy approved EKS network",
    });
    if (mode === "disabled") {
      await expect(deploy).toBeDisabled();
    } else {
      await deploy.click();
      await expect(
        page.getByText("Test membership is read-only."),
      ).toBeVisible();
    }
  });
}

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

test("workspace navigation and network builder open with deployment disabled", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await fixture(page, true);
  const navigation = page.getByRole("navigation", { name: "Main navigation" });
  for (const name of [
    "Networks",
    "Customer configurations",
    "Nodes",
    "Chaincode",
    "Operations",
    "Cloud readiness",
  ]) {
    const item = navigation.getByRole("button", { name, exact: true });
    await item.click();
    await expect(item).toHaveAttribute("aria-current", "page");
    await expect(
      page.getByRole("heading", { name, exact: true }),
    ).toBeVisible();
  }
  await page
    .getByRole("button", { name: "Create Network", exact: true })
    .click();
  await expect(
    page
      .getByRole("navigation", { name: "Network creation steps" })
      .getByRole("button"),
  ).toHaveCount(6);
  expect(errors).toEqual([]);
});
