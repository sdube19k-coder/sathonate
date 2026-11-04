import {
  CheckCircle2,
  Clock3,
  Database,
  LoaderCircle,
  RefreshCw,
  Server,
  ShieldCheck,
  TriangleAlert,
  Code2,
  GitBranch,
  Building2,
} from "lucide-react";
import { formatDate } from "./components";
import LifecycleManagement from "./LifecycleManagement";
import { Card, Note, Status, TopologyDiagram } from "./SmpScreens";
import type { FlowDraft } from "./smp-model";
import type {
  LocalState,
  Operation,
  RuntimeHealth,
  DeploymentState,
} from "./api";

export function ProgressScreen({
  operation,
  retrying = false,
  onRetry,
  onDashboard,
  error,
}: {
  operation: Operation | null;
  retrying?: boolean;
  onRetry: () => void;
  onDashboard: () => void;
  error: string;
}) {
  const completed =
      operation?.stages.filter((s) => s.status === "succeeded").length ?? 0,
    total = operation?.stages.length ?? 0,
    percent = total ? Math.round((completed / total) * 100) : 0;
  const current = operation?.stages.find(
    (s) => s.key === operation.currentStage,
  );
  return (
    <>
      <Card
        title="Overall Progress"
        action={<strong className="smp-progress-percent">{percent}%</strong>}
      >
        <div
          className="smp-progress-track"
          role="progressbar"
          aria-label="Deployment stage completion"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={percent}
        >
          <span style={{ width: `${percent}%` }} />
        </div>
        <div className="smp-progress-caption">
          <span>
            Current step:{" "}
            <strong>
              {current?.label ??
                (operation
                  ? "Queued for execution"
                  : "Waiting for an operation")}
            </strong>
          </span>
          <span>
            {completed} of {total} stages completed
          </span>
        </div>
        {operation && (
          <code className="smp-operation-id">{operation.operationId}</code>
        )}
      </Card>
      <div className="smp-progress-grid">
        <Card title="Deployment Stages">
          <ol className="smp-stage-list">
            {operation?.stages.map((stage, i) => (
              <li key={stage.key} className={stage.status}>
                {stage.status === "succeeded" ? (
                  <CheckCircle2 size={21} />
                ) : stage.status === "running" ? (
                  <LoaderCircle className="spin" size={21} />
                ) : stage.status === "failed" ||
                  stage.status === "interrupted" ? (
                  <TriangleAlert size={21} />
                ) : (
                  <Clock3 size={21} />
                )}
                <div>
                  <strong>
                    {i + 1}. {stage.label}
                  </strong>
                  <p>
                    {stage.startedAt ? formatDate(stage.startedAt) : "Pending"}
                  </p>
                </div>
                <Status
                  tone={
                    stage.status === "succeeded"
                      ? "green"
                      : stage.status === "running"
                        ? "blue"
                        : stage.status === "pending"
                          ? "neutral"
                          : "amber"
                  }
                >
                  {stage.status}
                </Status>
              </li>
            ))}
          </ol>
        </Card>
        <div className="smp-stack">
          <Card title="Step Details" icon={ShieldCheck}>
            {current ? (
              <>
                <h3 className="smp-current-stage">{current.label}</h3>
                <p className="smp-subcopy">
                  {operation?.status === "succeeded"
                    ? "Execution completed. The real Fabric transaction and peer queries passed."
                    : "Stage status comes from the backend worker, with a persisted operation record."}
                </p>
                <ul className="smp-checklist">
                  {operation?.events
                    .filter((e) => e.stage === current.key)
                    .slice(-5)
                    .map((e) => (
                      <li key={e.sequence}>
                        <CheckCircle2 size={15} />
                        <span>{e.message}</span>
                      </li>
                    ))}
                </ul>
              </>
            ) : (
              <Note>
                {operation
                  ? "Operation recorded. Waiting for the worker to begin its first stage."
                  : "No operation selected. Create the supported local network from the review screen."}
              </Note>
            )}
          </Card>
          <Card
            title="Live Logs"
            action={
              <Status
                tone={operation?.status === "running" ? "green" : "neutral"}
              >
                {operation?.status === "running" ? "Live" : "Recorded"}
              </Status>
            }
          >
            <div
              className="smp-terminal"
              role="log"
              aria-label="Deployment event log"
            >
              {operation?.events.slice(-30).map((e) => (
                <div key={e.sequence}>
                  <time>
                    [
                    {new Date(e.time).toLocaleTimeString([], { hour12: false })}
                    ]
                  </time>{" "}
                  <span>INFO</span> {e.message}
                </div>
              ))}
            </div>
          </Card>
        </div>
      </div>
      {(operation?.error || error) && (
        <Note tone="amber">{operation?.error || error}</Note>
      )}
      {(operation?.status === "failed" ||
        operation?.status === "interrupted") && (
        <div className="smp-inline-actions">
          <button
            className="smp-btn secondary"
            disabled={retrying}
            onClick={onRetry}
          >
            <RefreshCw size={16} />
            Retry operation
          </button>
          <span className="smp-subcopy">
            A linked retry reconciles resources. Identities and ledgers are
            retained.
          </span>
        </div>
      )}
      {operation?.status === "succeeded" && (
        <Note tone="green">
          <strong>Deployment verification passed.</strong> Open the dashboard
          for live channel, orderer, CA and contract health.{" "}
          <button className="smp-text-btn" onClick={onDashboard}>
            View Network Dashboard →
          </button>
        </Note>
      )}
    </>
  );
}

export function DashboardScreen({
  onLifecycle,
  verification,
  draft,
  health,
  runtime,
  loading,
  onRefresh,
  onTest,
  onStop,
  onLogs,
  logs,
  node,
  setNode,
  busy,
}: {
  onLifecycle: (operation: Operation) => void;
  verification?: DeploymentState | null;
  draft: FlowDraft;
  health: RuntimeHealth | null;
  runtime: LocalState | null;
  loading: boolean;
  onRefresh: () => void;
  onTest: () => void;
  onStop: () => void;
  onLogs: () => void;
  logs: string;
  node: string;
  setNode: (node: string) => void;
  busy: boolean;
}) {
  return (
    <>
      {draft.secureProfile && (
        <Card title="Committed transaction readback">
          <p>
            This is the server-recorded deployment verification, not a new
            transaction.
          </p>
          {verification?.verified &&
          !verification.simulated &&
          verification.transactionId ? (
            <>
              <p>
                Transaction ID:{" "}
                <code className="smp-transaction-id">
                  {verification.transactionId}
                </code>
              </p>
              <p>Verified at: {formatDate(verification.checkedAt)}</p>
              <ul>
                {verification.health?.peers.map((peer) => (
                  <li key={peer.peer}>
                    {peer.peer}:{" "}
                    {peer.available
                      ? "Committed readback verified"
                      : "Not verified"}
                    ; ledger height {peer.height}
                  </li>
                ))}
              </ul>
            </>
          ) : (
            <Note>No committed readback evidence is available.</Note>
          )}
        </Card>
      )}
      <div className="smp-dashboard-top">
        <Card
          title={
            draft.provider === "kubernetes"
              ? draft.form.name
              : "Sathonate Development Network"
          }
          action={
            <Status tone={health?.ready ? "green" : "amber"}>
              {health?.ready ? "Healthy" : "Checking / degraded"}
            </Status>
          }
        >
          <p className="smp-subcopy">
            {draft.provider === "kubernetes"
              ? "Real local Kubernetes · Bevel operator · CA-enrolled identities"
              : "CA-enrolled local Hyperledger Fabric network"}
          </p>
          <dl className="smp-facts">
            <div>
              <dt>
                {draft.provider === "kubernetes"
                  ? "Deployment ID"
                  : "Network ID"}
              </dt>
              <dd>
                {draft.provider === "kubernetes"
                  ? runtime?.operationId
                  : "sathonate-local"}
              </dd>
            </div>
            <div>
              <dt>Fabric Version</dt>
              <dd>{runtime?.version ?? "2.5.15"}</dd>
            </div>
            <div>
              <dt>Channel</dt>
              <dd>{health?.channel ?? "sathonate"}</dd>
            </div>
            <div>
              <dt>
                {draft.provider === "kubernetes"
                  ? "Last Verification"
                  : "Last Health Check"}
              </dt>
              <dd>{health ? formatDate(health.checkedAt) : "Not checked"}</dd>
            </div>
          </dl>
        </Card>
        <Card title="Saved Design Topology">
          <p>
            Observed members are listed in the peer and orderer tables below.
            Lifecycle changes do not overwrite the saved design.
          </p>
          <TopologyDiagram draft={draft} />
        </Card>
      </div>
      <div className="smp-dashboard-counts">
        {[
          {
            icon: Building2,
            n: health
              ? new Set(health.peers.map((peer) => peer.mspId)).size
              : "—",
            label: "Organizations",
          },
          { icon: Server, n: health?.peers.length ?? "—", label: "Peers" },
          {
            icon: Database,
            n: health?.orderers.length ?? "—",
            label: "Orderer Nodes",
          },
          {
            icon: ShieldCheck,
            n: health?.certificateAuthorities.length ?? "—",
            label: "Certificate Authorities",
          },
          { icon: GitBranch, n: health ? 1 : "—", label: "Channel" },
          {
            icon: CheckCircle2,
            n: health?.ready ? "Healthy" : "Unconfirmed",
            label: "Network Status",
          },
        ].map((s, i) => (
          <div className={`smp-card c${i % 3}`} key={s.label}>
            <s.icon size={25} />
            <div>
              <strong>{s.n}</strong>
              <span>{s.label}</span>
            </div>
          </div>
        ))}
      </div>
      {health?.alerts.map((a) => (
        <Note tone="amber" key={a}>
          {a}
        </Note>
      ))}
      {draft.secureProfile &&
        verification?.mode === "secure-local" &&
        verification.verified && (
          <LifecycleManagement
            deployment={verification}
            onAccepted={onLifecycle}
          />
        )}
      {draft.provider === "kubernetes" && (
        <Note>
          Ledger and transaction evidence comes from the last completed Fabric
          verification. Refresh Health checks current Kubernetes workloads. The
          secure profile verification control reloads its recorded committed
          transaction; it does not redeploy the network.
        </Note>
      )}
      <div className="smp-grid two">
        <Card
          title="Peer Ledger Health"
          action={
            <button
              className="smp-text-btn"
              disabled={loading}
              onClick={onRefresh}
            >
              <RefreshCw size={14} />
              Refresh
            </button>
          }
        >
          <div className="smp-table-wrap">
            <table className="smp-table">
              <thead>
                <tr>
                  <th>Peer / Organization</th>
                  <th>
                    {draft.provider === "kubernetes"
                      ? "Last verified height"
                      : "Block Height"}
                  </th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {health?.peers.map((p) => (
                  <tr key={p.peer}>
                    <td>
                      <strong>{p.peer}</strong>
                      <small>{p.mspId}</small>
                    </td>
                    <td>{p.available ? p.height : "—"}</td>
                    <td>
                      <Status tone={p.available ? "green" : "amber"}>
                        {p.available ? "Responding" : "Unavailable"}
                      </Status>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="smp-subcopy">
            Block hashes{" "}
            {health?.ready
              ? "match across both peers."
              : "are checked directly from Fabric; refresh after pending transactions settle."}
          </p>
        </Card>
        <Card title="Network Resources">
          <div className="smp-live-resource">
            <BoxesMark />
            <span>
              {draft.provider === "kubernetes"
                ? "Fabric Workloads"
                : "Docker Containers"}
            </span>
            <Status tone={health?.ready ? "green" : "amber"}>
              {draft.provider === "kubernetes"
                ? health?.ready
                  ? "Verified"
                  : "Check required"
                : `${runtime?.containers.filter((c) => c.state === "running").length ?? 0} running`}
            </Status>
          </div>
          {health?.certificateAuthorities.map((ca) => (
            <div className="smp-live-resource" key={ca.name}>
              <ShieldCheck size={19} />
              <span>{ca.name}</span>
              <Status tone={ca.available ? "green" : "amber"}>
                {ca.available ? "TLS verified" : "Unavailable"}
              </Status>
            </div>
          ))}
          <div className="smp-live-resource">
            <Code2 size={19} />
            <span>{health?.chaincode.name ?? "Chaincode"}</span>
            <Status tone={health?.chaincode.available ? "green" : "amber"}>
              {health?.chaincode.available ? "Committed" : "Unconfirmed"}
            </Status>
          </div>
        </Card>
      </div>
      <Card title="Ordering Service" icon={Database}>
        <div className="smp-orderer-health">
          {health?.orderers.map((o) => (
            <div key={o.name}>
              <Database size={23} />
              <div>
                <strong>{o.name}</strong>
                <p>
                  {o.consensusRelation || "Unconfirmed"} · Block {o.height}
                </p>
              </div>
              <Status tone={o.status === "active" ? "green" : "amber"}>
                {o.status}
              </Status>
            </div>
          ))}
        </div>
      </Card>
      <Card title="Last Verified Transaction" icon={Code2}>
        {runtime?.transactionId ? (
          <>
            <code className="smp-transaction-id">{runtime.transactionId}</code>
            <p className="smp-subcopy">
              {health?.chaincode.name ?? "sathonate_smoke"} · Put / Get ·
              committed write verified on both peers ·{" "}
              {runtime.verifiedAt ? formatDate(runtime.verifiedAt) : ""}
            </p>
          </>
        ) : (
          <p className="smp-subcopy">
            No verified transaction has been recorded.
          </p>
        )}
        <div className="smp-inline-actions">
          <button
            className="smp-btn secondary"
            disabled={busy || !runtime?.ready}
            onClick={onTest}
          >
            <CheckCircle2 size={16} />
            {draft.secureProfile
              ? "Verify recorded transaction"
              : "Run Transaction Check"}
          </button>
          <button
            className="smp-btn secondary danger"
            disabled={busy || draft.secureProfile}
            title={
              draft.secureProfile
                ? "Trusted disposable-cluster teardown is required for this profile."
                : undefined
            }
            onClick={onStop}
          >
            {draft.provider === "kubernetes"
              ? "Delete Local Network"
              : "Stop Network"}
          </button>
        </div>
      </Card>
      <Card title="Fabric Node Logs">
        <div className="smp-inline-actions">
          <select
            aria-label="Fabric log node"
            value={node}
            onChange={(e) => setNode(e.target.value)}
          >
            {(health
              ? [
                  ...health.peers.map((p) => p.peer),
                  ...health.orderers.map((o) => o.name),
                ]
              : ["peer1", "peer2", "orderer1", "orderer2", "orderer3"]
            ).map((n) => (
              <option key={n}>{n}</option>
            ))}
          </select>
          <button className="smp-btn secondary" onClick={onLogs}>
            Read Node Logs
          </button>
        </div>
        {logs && <pre className="smp-terminal smp-node-logs">{logs}</pre>}
      </Card>
    </>
  );
}
function BoxesMark() {
  return <Server size={19} />;
}
