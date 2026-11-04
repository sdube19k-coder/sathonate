import { useEffect, useState } from "react";
import {
  CheckCircle2,
  Clock3,
  LoaderCircle,
  RotateCcw,
  ShieldAlert,
} from "lucide-react";
import { api, type Operation } from "./api";
import { Badge, formatDate } from "./components";

export default function OperationProgress({
  operationId,
  onRetry,
}: {
  operationId?: string;
  onRetry: () => void;
}) {
  const [items, setItems] = useState<Operation[]>([]);
  const [selected, setSelected] = useState(operationId ?? "");
  const [operation, setOperation] = useState<Operation | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (operationId) setSelected(operationId);
  }, [operationId]);
  useEffect(() => {
    let disposed = false;
    const load = async () => {
      try {
        const list = await api.operations();
        const id = selected || list.items[0]?.operationId;
        const op = id ? await api.operation(id) : null;
        if (!disposed) {
          setItems(list.items);
          setOperation(op);
          setError("");
        }
      } catch (e) {
        if (!disposed)
          setError(e instanceof Error ? e.message : "Cannot read operations");
      }
    };
    void load();
    const interval = setInterval(() => void load(), 2500);
    return () => {
      disposed = true;
      clearInterval(interval);
    };
  }, [selected]);
  const retry = async () => {
    if (!operation) return;
    setBusy(true);
    try {
      const op = await api.retryOperation(operation.operationId);
      setSelected(op.operationId);
      setOperation(op);
      onRetry();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Retry failed");
    } finally {
      setBusy(false);
    }
  };
  return (
    <section
      className="panel local-runtime-panel operation-panel"
      aria-label="Deployment progress"
    >
      <div className="panel-heading">
        <div>
          <h2>Deployment progress</h2>
          <p>Durable operation records and actual stage events.</p>
        </div>
        {operation && (
          <Badge
            tone={
              operation.status === "succeeded"
                ? "green"
                : operation.status === "running"
                  ? "blue"
                  : "amber"
            }
          >
            {operation.status}
          </Badge>
        )}
      </div>
      <div className="operation-body">
        {error && (
          <div role="alert" className="error-box">
            {error}
          </div>
        )}
        {items.length > 0 && (
          <label className="operation-select">
            Operation history
            <select
              aria-label="Operation history"
              value={selected || items[0]?.operationId}
              onChange={(e) => setSelected(e.target.value)}
            >
              {items.slice(0, 30).map((item) => (
                <option key={item.operationId} value={item.operationId}>
                  {item.kind} · {formatDate(item.createdAt)} · {item.status}
                </option>
              ))}
            </select>
          </label>
        )}
        {operation ? (
          <>
            <div className="operation-identity">
              <code>{operation.operationId}</code>
              <span>{operation.runtimeVersion}</span>
            </div>
            {operation.retryOf && (
              <p className="muted">
                Retry of <code>{operation.retryOf}</code>. Completed resources
                are reconciled and verified again.
              </p>
            )}
            <ol className="operation-stages">
              {operation.stages.map((stage) => (
                <li
                  key={stage.key}
                  className={`operation-stage ${stage.status}`}
                >
                  {stage.status === "succeeded" ? (
                    <CheckCircle2 size={20} />
                  ) : stage.status === "running" ? (
                    <LoaderCircle className="spin" size={20} />
                  ) : stage.status === "failed" ||
                    stage.status === "interrupted" ? (
                    <ShieldAlert size={20} />
                  ) : (
                    <Clock3 size={20} />
                  )}
                  <div>
                    <strong>{stage.label}</strong>
                    <span>
                      {stage.startedAt
                        ? formatDate(stage.startedAt)
                        : "Waiting for the previous stage"}
                      {stage.finishedAt
                        ? ` → ${formatDate(stage.finishedAt)}`
                        : ""}
                    </span>
                  </div>
                  <Badge
                    tone={
                      stage.status === "succeeded"
                        ? "green"
                        : stage.status === "running"
                          ? "blue"
                          : "neutral"
                    }
                  >
                    {stage.status}
                  </Badge>
                </li>
              ))}
            </ol>
            {operation.error && (
              <div role="alert" className="error-box">
                {operation.error}
              </div>
            )}
            {(operation.status === "failed" ||
              operation.status === "interrupted") && (
              <div className="operation-retry">
                <p>
                  Retry creates a new linked operation and reconciles existing
                  resources. No automatic rollback or ledger deletion is
                  performed.
                </p>
                <button
                  className="button secondary"
                  disabled={busy}
                  onClick={() => void retry()}
                >
                  <RotateCcw size={16} />
                  Retry operation
                </button>
              </div>
            )}
            <details className="operation-events">
              <summary>Stage event log ({operation.events.length})</summary>
              <div>
                {operation.events
                  .slice(-60)
                  .reverse()
                  .map((e) => (
                    <p key={e.sequence}>
                      <time>{formatDate(e.time)}</time>
                      <span>{e.message}</span>
                    </p>
                  ))}
              </div>
            </details>
          </>
        ) : (
          <p className="muted">
            Start the network or run a transaction check to create its first
            operation record.
          </p>
        )}
      </div>
    </section>
  );
}
