import { useState } from "react";
import { api, type OperationDiagnostics } from "./api";

export function downloadDiagnostics(report: OperationDiagnostics) {
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(report, null, 2) + "\n"], {
      type: "application/json",
    }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = "sathonate-operation-diagnostics.json";
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

export default function OperationDiagnosticsButton({
  operationId,
}: {
  operationId: string;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <>
      <button
        className="smp-text-btn"
        disabled={busy}
        aria-label={`Download diagnostics for ${operationId}`}
        onClick={async () => {
          setBusy(true);
          setError("");
          try {
            downloadDiagnostics(await api.operationDiagnostics(operationId));
          } catch {
            setError(
              "Could not download diagnostics. Check your session and retry.",
            );
          } finally {
            setBusy(false);
          }
        }}
      >
        {busy ? "Preparing report…" : "Download diagnostics"}
      </button>
      {error && <span role="alert">{error}</span>}
    </>
  );
}
