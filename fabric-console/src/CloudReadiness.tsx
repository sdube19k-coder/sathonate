import { useState } from "react";
import { api, type CloudReport } from "./api";
import type { Network } from "./types";

// Revision-bound checks for the original saved-design detail view.
export default function CloudReadiness({ network }: { network: Network }) {
  const [report, setReport] = useState<CloudReport | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const check = async () => {
    setBusy(true);
    setError("");
    try {
      setReport(await api.cloudPreflight(network));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Cloud preflight failed");
    } finally {
      setBusy(false);
    }
  };
  const bundle = async () => {
    setBusy(true);
    setError("");
    try {
      const result = await api.cloudBundle(network);
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(result, null, 2)], {
          type: "application/json",
        }),
      );
      const a = document.createElement("a");
      a.href = url;
      a.download = `${network.desired.metadata.id}-cloud-candidate.json`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Candidate export failed");
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="panel">
      <h2>Registered cloud target readiness</h2>
      <p>
        Read-only checks and non-executable candidate export for saved revision{" "}
        {network.revision}. Cloud execution is disabled.
      </p>
      <div className="actions">
        <button disabled={busy} onClick={() => void check()}>
          Run cloud preflight
        </button>
        <button disabled={busy || !report?.ready} onClick={() => void bundle()}>
          Export cloud candidate
        </button>
      </div>
      {error && <p role="alert">{error}</p>}
      {report?.checks.map((c) => (
        <div key={c.id} className="summary-row">
          <strong>{c.status}</strong>
          <span>{c.message}</span>
        </div>
      ))}
    </section>
  );
}
