import { useState } from "react";
import { api, type DeletionPreview } from "./api";
export default function RetainedDeletion({
  operationId,
  onRemoved,
}: {
  operationId: string;
  onRemoved: () => void;
}) {
  const [preview, setPreview] = useState<DeletionPreview | null>(null),
    [approved, setApproved] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function perform(remove: boolean) {
    setBusy(true);
    setError("");
    try {
      if (remove && preview) {
        await api.deleteRetainingData(operationId, preview.digest);
        setPreview(null);
        setApproved(false);
        onRemoved();
      } else {
        setPreview(await api.deletionPreview(operationId));
        setApproved(false);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Deletion unavailable");
      setApproved(false);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-label="Retained-data network deletion">
      <h4>Remove network workloads and retain data</h4>
      <p>
        Administrator permission and an exact preview are required. This stops
        the network and blocks automatic redeployment. Identities, ledger
        storage and configuration remain; this does not erase data or create a
        backup.
      </p>
      <button
        className="smp-btn secondary"
        disabled={busy}
        onClick={() => void perform(false)}
      >
        Review retained-data deletion
      </button>
      {error && <p role="alert">{error}</p>}
      {preview && (
        <section aria-label="Deletion approval">
          <p>{preview.notice}</p>
          <p>
            Remove {preview.delete.length} exact resources; retain{" "}
            {preview.retainedObjects.length} recorded objects.
          </p>
          <details>
            <summary>Exact resources and retained storage</summary>
            <ul>
              {preview.delete.map((r) => (
                <li key={r.resource + r.name}>
                  Remove {r.resource}/{r.name} · UID {r.uid}
                </li>
              ))}
            </ul>
            <ul>
              {preview.retainedObjects.map((r) => (
                <li key={r.resource + r.name}>
                  Retain {r.resource}/{r.name} · UID {r.uid}
                </li>
              ))}
            </ul>
          </details>
          <label>
            <input
              type="checkbox"
              disabled={busy}
              checked={approved}
              onChange={(e) => setApproved(e.target.checked)}
            />
            I approve removal of these exact workloads while preserving data and
            blocking automatic redeployment.
          </label>
          <button
            className="smp-btn primary"
            disabled={busy || !approved}
            onClick={() => void perform(true)}
          >
            Remove approved workloads
          </button>
        </section>
      )}
    </section>
  );
}
