import { useEffect, useRef, useState } from "react";
import { api, type WorkloadSnapshot } from "./api";
import type { Operation } from "./api";
import NodeRecovery from "./NodeRecovery";

export default function WorkloadStatus({
  operationId,
  onStarted,
}: {
  operationId: string;
  onStarted?: (op: Operation) => void;
}) {
  const [snapshot, setSnapshot] = useState<WorkloadSnapshot | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    setSnapshot(null);
    setError("");
    setBusy(false);
    return () => {
      controller.current?.abort();
      controller.current = null;
    };
  }, [operationId]);
  async function refresh() {
    controller.current?.abort();
    const pending = new AbortController();
    controller.current = pending;
    setBusy(true);
    setError("");
    setSnapshot(null);
    const timeout = setTimeout(() => pending.abort(), 30000);
    try {
      const data = await api.workloads(operationId, pending.signal);
      if (
        data.operationId !== operationId ||
        !data.readOnly ||
        data.ledgerChecked
      )
        throw new Error("The workload response could not be verified.");
      if (controller.current === pending) setSnapshot(data);
    } catch (e) {
      if (controller.current === pending)
        setError(
          pending.signal.aborted
            ? "Workload refresh timed out. Reconnect before trying again."
            : e instanceof Error
              ? e.message
              : "Current workload status is unavailable.",
        );
    } finally {
      clearTimeout(timeout);
      if (controller.current === pending) setBusy(false);
    }
  }
  return (
    <section aria-label="Current workload troubleshooting">
      <h4>Current workload status</h4>
      <p>
        Refresh Kubernetes workload and volume observations. This does not retry
        deployment, restart components or verify the ledger.
      </p>
      <button
        className="smp-btn secondary"
        disabled={busy}
        onClick={() => void refresh()}
      >
        {busy ? "Reading workloads…" : "Refresh current workload status"}
      </button>
      {error && <p role="alert">{error}</p>}
      {snapshot && (
        <div aria-live="polite">
          <p>
            {snapshot.workloadsReady
              ? "Required workloads ready"
              : "Workloads need attention"}{" "}
            · observed {snapshot.checkedAt} · saved revision {snapshot.revision}
          </p>
          <p>Ledger consistency was not checked by this refresh.</p>
          {snapshot.alerts?.map((a) => (
            <p role="status" key={a.component + a.code}>
              {a.component}: {a.code}. {a.action}
            </p>
          ))}
          <div className="smp-workload-scroll">
            <table>
              <thead>
                <tr>
                  <th>Component</th>
                  <th>Pod</th>
                  <th>Phase</th>
                  <th>Ready</th>
                  <th>Restarts</th>
                  <th>Reason</th>
                </tr>
              </thead>
              <tbody>
                {snapshot.pods.map((p) => (
                  <tr key={p.name}>
                    <td>
                      {p.component} ({p.kind})
                    </td>
                    <td>{p.name}</td>
                    <td>{p.phase}</td>
                    <td>{p.ready ? "Yes" : "No"}</td>
                    <td>{p.restarts}</td>
                    <td>{p.waitingReason || p.lastExitReason || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {snapshot.pods.length === 0 && (
            <p>No managed pod observations available.</p>
          )}
          {snapshot.missing.length > 0 && (
            <p>Missing or unready components: {snapshot.missing.join(", ")}</p>
          )}
          <p>Volumes referenced by observed managed pods:</p>
          <ul>
            {snapshot.volumes.map((v) => (
              <li key={v.name}>
                {v.name}: {v.phase}
              </li>
            ))}
          </ul>
          <ul>
            {snapshot.guidance.map((hint) => (
              <li key={hint}>{hint}</li>
            ))}
          </ul>
          {onStarted && (
            <NodeRecovery
              key={operationId}
              operationId={operationId}
              snapshot={snapshot}
              onStarted={onStarted}
            />
          )}
        </div>
      )}
    </section>
  );
}
