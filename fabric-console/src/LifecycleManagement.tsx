import { useState } from "react";
import {
  api,
  type ChangePreview,
  type DeploymentState,
  type LifecycleAction,
  type Operation,
} from "./api";
import { Card, Note } from "./SmpScreens";

export default function LifecycleManagement({
  deployment,
  onAccepted,
}: {
  deployment: DeploymentState;
  onAccepted: (operation: Operation) => void;
}) {
  const [preview, setPreview] = useState<ChangePreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const inspect = async (action: LifecycleAction) => {
    setBusy(true);
    setError("");
    setPreview(null);
    try {
      setPreview(await api.lifecyclePreview(deployment.operationId, action));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Preflight failed");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Card title="Manage Fabric lifecycle">
      <p>
        Changes use the protected local profile and require a current health
        preflight and explicit preview approval. Organization and Raft
        membership changes are not enabled.
      </p>
      <div className="smp-inline-actions">
        <button
          className="smp-btn secondary"
          disabled={busy}
          onClick={() => void inspect("verify-network")}
        >
          Preview new transaction
        </button>
        <button
          className="smp-btn secondary"
          disabled={busy}
          onClick={() => void inspect("restart-peer")}
        >
          Preview peer restart
        </button>
        <button
          className="smp-btn secondary"
          disabled={busy || !!deployment.lifecycle?.peerAdded}
          onClick={() => void inspect("add-peer")}
        >
          Preview adding Org1 peer
        </button>
        <button
          className="smp-btn secondary"
          disabled={busy || deployment.lifecycle?.sequence === 2}
          onClick={() => void inspect("upgrade-chaincode")}
        >
          Preview definition upgrade
        </button>
      </div>
      {busy && <p role="status">Checking the approved change…</p>}
      {error && <div role="alert">{error}</div>}
      {preview && (
        <section aria-label="Lifecycle change preview">
          <h3>
            {
              {
                "add-peer": "Add org1-peer-2",
                "upgrade-chaincode": "Upgrade smp_smoke to sequence 2",
                "verify-network": "Submit a new verification transaction",
                "restart-peer": "Restart org1-peer-1 and verify persistence",
              }[preview.action]
            }
          </h3>
          <ul>
            {preview.impact.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
          <Note>{preview.recovery}</Note>
          <div className="smp-inline-actions">
            <button
              className="smp-btn primary"
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  onAccepted(
                    await api.lifecycleApply(
                      deployment.operationId,
                      preview.action,
                      preview.digest,
                    ),
                  );
                } catch (reason) {
                  setError(
                    reason instanceof Error
                      ? reason.message
                      : "Change failed; refresh the preview before retrying",
                  );
                } finally {
                  setBusy(false);
                }
              }}
            >
              Approve and execute change
            </button>
            <button
              className="smp-btn secondary"
              disabled={busy}
              onClick={() => setPreview(null)}
            >
              Cancel preview
            </button>
          </div>
        </section>
      )}
    </Card>
  );
}
