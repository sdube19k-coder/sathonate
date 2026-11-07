import { chromium, expect } from "@playwright/test";
import { writeFileSync, readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
let input = "";
for await (const chunk of process.stdin) input += chunk;
const c = JSON.parse(input);
input = "";
let browser;
const check = (ok, message) => {
  if (!ok) throw new Error(message);
};
try {
  browser = await chromium.launch({ channel: "msedge", headless: true });
  const page = await browser.newPage();
  const errors = [];
  page.on("pageerror", () => errors.push("browser-runtime-error"));
  await page.goto("http://127.0.0.1:5173");
  await page
    .getByLabel("Development token", { exact: true })
    .fill(c.tokens.administrator);
  await page.getByRole("button", { name: "Authenticate", exact: true }).click();
  await page
    .getByLabel("Authorized project", { exact: true })
    .selectOption(c.project);
  await page.getByRole("button", { name: "Open project", exact: true }).click();
  await expect(
    page.getByLabel("Control plane connected", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Networks", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: "Customer configurations", exact: true })
    .click();
  await page.getByLabel("Saved customer network").selectOption(c.networkId);
  const api = async (
    path,
    token = c.tokens.administrator,
    method = "GET",
    headers = {},
  ) =>
    page.evaluate(
      async ({ path, token, method, headers }) => {
        const r = await fetch("/api/v1" + path, {
          method,
          headers: { Authorization: "Bearer " + token, ...headers },
        });
        return { status: r.status, body: await r.json().catch(() => null) };
      },
      { path, token, method, headers },
    );
  const path = "/deployments/" + c.operationId;
  for (const token of [c.tokens.foreignTenant, c.tokens.foreignProject])
    check(
      (await api(path, token)).status === 404,
      "Restored control rows leaked across tenant/project scope",
    );
  check(
    (await api(path + "/cleanup-preview", c.tokens.primary)).status === 403,
    "Ordinary operator obtained administrator deletion preview",
  );
  check(
    (await api(path, c.tokens.noDeploy, "DELETE")).status === 403,
    "Viewer deleted workloads",
  );
  check(
    (
      await api(path, c.tokens.administrator, "DELETE", {
        "X-Sathonate-Cleanup-Approval": "f".repeat(64),
      })
    ).status === 409,
    "Stale deletion approval accepted",
  );
  check(
    /^smp-sprint1-[a-f0-9]{8}$/.test(c.tenant),
    "Disposable cluster required",
  );
  const runtime = JSON.parse(
    readFileSync("E:/Sathonate/.cache/" + c.tenant + "-runtime.json", "utf8"),
  );
  const scope =
    "lp-" +
    createHash("sha256")
      .update(JSON.stringify([c.tenant, c.project]))
      .digest("hex")
      .slice(0, 48);
  const ns =
    "smp-s-" +
    createHash("sha256")
      .update(JSON.stringify([scope, "token-bound-project", c.networkId]))
      .digest("hex")
      .slice(0, 32);
  const kub = (args, input) => {
    try {
      return execFileSync(
        "kubectl",
        [
          "--kubeconfig",
          runtime.kubeconfig,
          "--context",
          "kind-" + c.tenant,
          "--request-timeout=20s",
          "-n",
          ns,
          ...args,
        ],
        { input, encoding: "utf8", stdio: ["pipe", "pipe", "pipe"] },
      );
    } catch {
      throw new Error("Guarded disposable deletion UID fixture failed");
    }
  };
  check(
    kub([
      "get",
      "node",
      c.tenant + "-control-plane",
      "-o",
      "jsonpath={.spec.providerID}",
    ]) ===
      "kind://docker/" + c.tenant + "/" + c.tenant + "-control-plane",
    "Disposable provider mismatch",
  );
  const canary = {
    apiVersion: "batch/v1",
    kind: "Job",
    metadata: {
      name: "deletion-owned-canary",
      namespace: ns,
      labels: {
        "fabric.sathonate.io/tenant": scope,
        "fabric.sathonate.io/network": c.networkId,
      },
    },
    spec: {
      backoffLimit: 0,
      template: {
        spec: {
          automountServiceAccountToken: false,
          restartPolicy: "Never",
          containers: [
            {
              name: "canary",
              image: runtime.workerImage,
              imagePullPolicy: "Never",
              command: ["python", "-c", "pass"],
            },
          ],
        },
      },
    },
  };
  const first = JSON.parse(
    kub(["create", "-f", "-", "-o", "json"], JSON.stringify(canary)),
  );
  const oldPlan = await api(path + "/cleanup-preview");
  check(oldPlan.status === 200, "Owned deletion canary preview failed");
  const controllerUID = JSON.parse(
    kub(["get", "deployment", "secure-controller", "-o", "json"]),
  ).metadata.uid;
  kub(
    [
      "delete",
      "--raw",
      "/apis/batch/v1/namespaces/" + ns + "/jobs/deletion-owned-canary",
      "-f",
      "-",
    ],
    JSON.stringify({
      apiVersion: "v1",
      kind: "DeleteOptions",
      propagationPolicy: "Foreground",
      preconditions: { uid: first.metadata.uid },
    }),
  );
  kub(["wait", "--for=delete", "job/deletion-owned-canary", "--timeout=60s"]);
  const replacement = JSON.parse(
    kub(["create", "-f", "-", "-o", "json"], JSON.stringify(canary)),
  );
  check(
    replacement.metadata.uid !== first.metadata.uid,
    "Canary UID was not replaced",
  );
  const staleUID = await api(path, c.tokens.administrator, "DELETE", {
    "X-Sathonate-Cleanup-Approval": oldPlan.body.digest,
  });
  check(
    staleUID.status === 409,
    "Replaced-resource deletion approval accepted",
  );
  check(
    JSON.parse(kub(["get", "deployment", "secure-controller", "-o", "json"]))
      .metadata.uid === controllerUID &&
      !JSON.parse(kub(["get", "namespace", ns, "-o", "json"])).metadata.labels[
        "fabric.sathonate.io/retired"
      ],
    "Stale UID approval mutated the network",
  );
  const previewResponse = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith("/cleanup-preview") &&
      r.status() === 200,
  );
  await page
    .getByRole("button", { name: "Review retained-data deletion", exact: true })
    .click();
  const preview = await (await previewResponse).json();
  check(
    preview.schema === "sathonate.local-retained-deletion/v1" &&
      preview.delete.length > 0 &&
      preview.retainedObjects.some(
        (x) => x.resource === "persistentvolumeclaims",
      ),
    "Deletion retention preview incomplete",
  );
  const button = page.getByRole("button", {
    name: "Remove approved workloads",
    exact: true,
  });
  await expect(button).toBeDisabled();
  await page
    .getByRole("checkbox", {
      name: /I approve removal of these exact workloads/,
    })
    .check();
  const deleted = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === "/api/v1" + path &&
      r.request().method() === "DELETE",
    { timeout: 330000 },
  );
  await button.click();
  const response = await deleted;
  const acknowledgement = await response.json().catch(() => null);
  check(response.status() === 200, "Approved deletion failed: HTTP " + response.status() + " " + (acknowledgement?.error?.message || "no API acknowledgement"));
  await expect(
    page.getByText(
      "Network workloads removed. Persistent data retained; automatic redeployment is blocked.",
      { exact: true },
    ),
  ).toBeVisible();
  const result = await api(path);
  check(
    result.status === 200 &&
      result.body.phase === "removed" &&
      !result.body.verified &&
      result.body.localDeletion.completed &&
      result.body.localDeletion.retentionVerified,
    "Deletion completion not durable",
  );
  const ack = await api(path, c.tokens.administrator, "DELETE", {
    "X-Sathonate-Cleanup-Approval": preview.digest,
  });
  check(ack.status === 200, "Deletion acknowledgement not idempotent");
  check(
    (await api(path + "/recovery/preview?node=", c.tokens.primary)).status ===
      409,
    "Retired network could be reactivated",
  );
  check(errors.length === 0, "Browser runtime failed");
  writeFileSync(
    c.result,
    JSON.stringify(
      {
        status: "PASS",
        checks: {
          administratorRequired: "PASS",
          restoredTenantProjectIsolation: "PASS",
          staleApprovalDenied: "PASS",
          replacedResourceUIDDenied: "PASS",
          explicitBrowserApproval: "PASS",
          retainedObjectsVerified: "PASS",
          acknowledgementIdempotent: "PASS",
          automaticRecoveryBlocked: "PASS",
        },
        operationId: c.operationId,
        preview,
        completion: result.body.localDeletion,
      },
      null,
      2,
    ),
  );
  console.log("Retained-data deletion browser acceptance PASS");
} catch (error) {
  console.error(
    "Retained-data deletion failed: " +
      Object.values(c.tokens).reduce(
        (s, t) => s.split(t).join("[REDACTED]"),
        String(error.message),
      ),
  );
  process.exitCode = 1;
} finally {
  await browser?.close();
}
