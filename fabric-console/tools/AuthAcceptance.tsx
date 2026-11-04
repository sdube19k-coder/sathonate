// Explicit development-only browser acceptance; never imported in production.
import { useState } from "react";
import { request, APIError } from "../src/api";
import { useWorkspace } from "../src/workspace";
import { SignOut } from "../src/AuthGate";
export default function AuthAcceptance() {
  const workspace = useWorkspace();
  const [results, setResults] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const params = new URLSearchParams(window.location.search);
  const network = params.get("network") || "missing";
  const operation = params.get("operation") || "missing";
  const foreign = params.get("foreign") || "missing";
  const run = async () => {
    setBusy(true);
    const outcomes: Record<string, string> = {};
    const cases: [string, string, string, number][] = [
      ["projects", "GET", "/projects", 200],
      ["network", "GET", "/networks/" + network, 200],
      ["history", "GET", "/operations", 200],
      ["operation", "GET", "/operations/" + operation, 200],
      ["deployment", "GET", "/deployments/" + operation, 200],
      ["recorded health", "GET", "/deployments/" + operation + "/health", 200],
      ["foreign network", "GET", "/networks/" + foreign, 404],
      ["create network denied", "POST", "/networks", 403],
      ["deploy denied", "POST", "/networks/" + network + "/deploy", 403],
      ["delete denied", "DELETE", "/networks/" + network, 403],
      ["retry denied", "POST", "/operations/" + operation + "/retry", 403],
      ["cancel denied", "POST", "/deployments/" + operation + "/cancel", 403],
      [
        "rotation denied",
        "POST",
        "/deployments/" + operation + "/rotation-plans",
        403,
      ],
      ["membership change denied", "POST", "/memberships", 403],
    ];
    for (const [name, method, path, expected] of cases) {
      let status = 200;
      try {
        await request(path, {
          method,
          headers: {
            "If-Match": '"0"',
            "Idempotency-Key": "readonly-denial-probe",
          },
          ...(method === "POST" ? { body: "{}" } : {}),
        });
      } catch (error) {
        status = error instanceof APIError ? error.status : 0;
      }
      outcomes[name] =
        status === expected
          ? "PASS (" + status + ")"
          : "FAIL (" + status + ", expected " + expected + ")";
      setResults({ ...outcomes });
    }
    setBusy(false);
  };
  return (
    <main className="auth-screen">
      <section className="auth-card">
        <h1>Development OIDC acceptance</h1>
        <p>
          {workspace?.tenantId} / {workspace?.id}
        </p>
        <p>
          Live Fabric checks are separate. This page tests recorded reads and
          authorization denials; execution is disabled.
        </p>
        <button
          className="button primary"
          disabled={busy}
          onClick={() => void run()}
        >
          Run read-only OIDC acceptance
        </button>
        <ul>
          {Object.entries(results).map(([name, result]) => (
            <li key={name}>
              {name}: {result}
            </li>
          ))}
        </ul>
        <SignOut />
      </section>
    </main>
  );
}
