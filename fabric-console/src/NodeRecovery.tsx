import { useState } from "react";
import {
  api,
  type Operation,
  type RecoveryPreview,
  type WorkloadSnapshot,
} from "./api";
export default function NodeRecovery({
  operationId,
  snapshot,
  onStarted,
}: {
  operationId: string;
  snapshot: WorkloadSnapshot;
  onStarted: (op: Operation) => void;
}) {
  const [node, setNode] = useState("");
  const [preview, setPreview] = useState<RecoveryPreview | null>(null);
  const [approved, setApproved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function act(apply: boolean) {
    setBusy(true);
    setError("");
    try {
      if (apply && preview) {
        onStarted(
          await api.applyNodeRecovery(operationId, node, preview.plan.digest),
        );
        setPreview(null);
        setApproved(false);
      } else {
        setPreview(null);
        setApproved(false);
        const reviewed = await api.nodeRecoveryPreview(operationId, node);
        if (
          reviewed.parentOperationId !== operationId ||
          reviewed.plan.node !== node ||
          !reviewed.plan.digest
        )
          throw new Error(
            "The recovery preview does not match the selected operation and node.",
          );
        setPreview(reviewed);
        setApproved(false);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Recovery unavailable");
      setApproved(false);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-label="Reviewed node recovery">
      <h4>Recovery and transaction verification</h4>
      <p>
        Recover the recorded configuration. Identities and persistent data are
        retained. A new transaction verifies recovery on every channel.
      </p>
      <label>
        Recovery target
        <select
          aria-label="Recovery target"
          value={node}
          disabled={busy}
          onChange={(e) => {
            setNode(e.target.value);
            setPreview(null);
            setApproved(false);
          }}
        >
          <option value="">
            Reconcile and verify without restarting a node
          </option>
          {snapshot.components
            ?.filter((c) => ["peer", "orderer", "authority"].includes(c.kind))
            .map((c) => (
              <option key={c.name} value={c.name}>
                {c.name} ({c.kind})
              </option>
            ))}
        </select>
      </label>
      <button
        className="smp-btn secondary"
        disabled={busy}
        onClick={() => void act(false)}
      >
        Review recovery
      </button>
      {error && <p role="alert">{error}</p>}
      {preview && (
        <section aria-label="Recovery approval">
          <p>{preview.impact}</p>
          <p>{preview.recovery}</p>
          <label>
            <input
              type="checkbox"
              checked={approved}
              disabled={busy}
              onChange={(e) => setApproved(e.target.checked)}
            />
            I approve this exact node recovery and fresh transaction
            verification.
          </label>
          <button
            className="smp-btn primary"
            disabled={busy || !approved}
            onClick={() => void act(true)}
          >
            Apply approved recovery
          </button>
        </section>
      )}
    </section>
  );
}
