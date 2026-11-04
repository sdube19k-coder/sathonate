import { useEffect, useState } from "react";
import {
  Play,
  Square,
  RefreshCw,
  Server,
  CheckCircle2,
  LoaderCircle,
} from "lucide-react";
import { api, type LocalState } from "./api";
import { Badge, formatDate } from "./components";
import OperationProgress from "./OperationProgress";
import RuntimeDashboard from "./RuntimeDashboard";
import { useDialog } from "./use-dialog";

export default function LocalRuntime() {
  const [state, setState] = useState<LocalState | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [logs, setLogs] = useState("");
  const [node, setNode] = useState("peer1");
  const [reviewOpen, setReviewOpen] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const dialogRef = useDialog(reviewOpen, () => setReviewOpen(false));
  const load = async () => {
    try {
      setState(await api.localStatus());
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Cannot reach local runtime");
    }
  };
  useEffect(() => {
    void load();
    const interval = setInterval(() => void load(), 4000);
    return () => clearInterval(interval);
  }, []);
  const action = async (start: boolean) => {
    setBusy(true);
    setError("");
    try {
      if (start) await api.localStart();
      else await api.localStop();
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Operation failed");
    } finally {
      setBusy(false);
    }
  };
  const operation =
    state?.phase === "starting" ||
    state?.phase === "stopping" ||
    state?.phase === "verifying";
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow page-eyebrow">LOCAL DOCKER NETWORK</div>
          <h1>Run Fabric.</h1>
          <p>
            A real CA-enrolled network on this computer. No cloud account
            needed.
          </p>
        </div>
        <button className="button secondary" onClick={() => void load()}>
          <RefreshCw size={16} />
          Refresh
        </button>
      </div>
      {error && (
        <div role="alert" className="error-box">
          {error}
        </div>
      )}
      <div className="panel local-runtime-summary">
        <div className="panel-heading">
          <div>
            <h2>Sathonate development network</h2>
            <p>Fabric 2.5.15 · Raft · Channel: sathonate</p>
          </div>
          <Badge tone={state?.ready ? "green" : "amber"}>
            {state?.ready ? "Network ready" : (state?.phase ?? "Connecting")}
          </Badge>
        </div>
        <div className="local-runtime-stats">
          {[
            ["3", "Raft orderers"],
            ["2", "Organizations / peers"],
            ["3", "Fabric CAs"],
            ["TLS", "Node connections"],
          ].map(([n, label]) => (
            <div key={label}>
              <strong>{n}</strong>
              <span>{label}</span>
            </div>
          ))}
        </div>
        <div className="info-note">
          <Server size={19} />
          <p>
            This local development profile runs independently of saved cloud
            designs. Identities are registered and enrolled through Fabric CA.
            Start preserves existing identities and ledgers; stop retains all
            data. It provides no multi-host fault isolation.
          </p>
        </div>
        {state?.error && (
          <div role="alert" className="error-box">
            {state.error}
          </div>
        )}
        {state && !state.dockerAvailable && (
          <div className="error-box">
            Start Docker Desktop with the Linux engine, then refresh.
          </div>
        )}
        <div className="cloud-readiness-actions">
          <button
            className="button primary"
            disabled={
              busy || operation || !state?.dockerAvailable || state?.ready
            }
            onClick={() => {
              setConfirmed(false);
              setReviewOpen(true);
            }}
          >
            {operation ? (
              <LoaderCircle className="spin" size={16} />
            ) : (
              <Play size={16} />
            )}{" "}
            {state?.phase === "starting"
              ? "Creating network…"
              : "Start local network"}
          </button>
          <button
            className="button secondary"
            disabled={busy || operation || !state?.containers.length}
            onClick={() => void action(false)}
          >
            <Square size={15} />
            Stop network
          </button>
          <button
            className="button secondary"
            disabled={busy || operation || !state?.ready}
            onClick={() => {
              setBusy(true);
              void api
                .localTest()
                .then(() => load())
                .catch((e) => setError(e.message))
                .finally(() => setBusy(false));
            }}
          >
            <CheckCircle2 size={16} />
            Run transaction check
          </button>
        </div>
        {state?.ready && (
          <div className="notice-bar">
            <CheckCircle2 size={18} />
            <span>
              A Go contract write was committed and queried on both peers. All
              runtime containers are running.
            </span>
          </div>
        )}
        {state?.transactionId && (
          <div className="local-transaction-proof">
            <strong>Last verified transaction</strong>
            <code>{state.transactionId}</code>
            <p>Channel: sathonate · Contract: sathonate_smoke · Value: ready</p>
          </div>
        )}
      </div>
      <OperationProgress
        operationId={state?.operationId}
        onRetry={() => void load()}
      />
      {state?.phase === "running" && state.verified && <RuntimeDashboard />}
      <div className="panel local-runtime-panel">
        <div className="panel-heading">
          <div>
            <h2>Live containers</h2>
            <p>Observed from Docker, refreshed every four seconds.</p>
          </div>
          <Badge>{state?.containers.length ?? 0} containers</Badge>
        </div>
        <div className="local-container-list">
          {state?.containers.map((c) => (
            <div className="local-container" key={c.name}>
              <Server size={18} />
              <div>
                <strong>{c.name}</strong>
                <p>{c.status}</p>
                <code>{c.ports || "Internal Docker network"}</code>
              </div>
              <Badge tone={c.state === "running" ? "green" : "amber"}>
                {c.state}
              </Badge>
            </div>
          ))}
          {!state?.containers.length && (
            <p className="muted">Start the network to create its containers.</p>
          )}
        </div>
      </div>
      <div className="panel local-runtime-panel">
        <div className="panel-heading">
          <div>
            <h2>Creation activity</h2>
            <p>Actual enrollment and channel setup stages.</p>
          </div>
        </div>
        <div className="local-events">
          {state?.events
            .slice(-30)
            .reverse()
            .map((e, i) => (
              <div key={i}>
                <span>{formatDate(e.time)}</span>
                <p>{e.message}</p>
              </div>
            ))}
        </div>
      </div>
      <div className="panel local-runtime-panel">
        <div className="panel-heading">
          <div>
            <h2>Fabric node logs</h2>
            <p>Last 80 lines from the selected container.</p>
          </div>
        </div>
        <div className="cloud-readiness-actions">
          <label>
            Node{" "}
            <select
              aria-label="Fabric log node"
              value={node}
              onChange={(e) => setNode(e.target.value)}
            >
              {["peer1", "peer2", "orderer1", "orderer2", "orderer3"].map(
                (n) => (
                  <option key={n}>{n}</option>
                ),
              )}
            </select>
          </label>
          <button
            className="button secondary"
            onClick={() =>
              void api
                .localLogs(node)
                .then((r) => setLogs(r.logs))
                .catch((e) => setError(e.message))
            }
          >
            Read node logs
          </button>
        </div>
        {logs && <pre className="local-log-output">{logs}</pre>}
      </div>
      {reviewOpen && (
        <div className="modal-backdrop">
          <div
            ref={dialogRef}
            className="modal local-launch-review"
            role="dialog"
            aria-modal="true"
            aria-labelledby="local-launch-title"
          >
            <div className="eyebrow">FABRIC SMP · REVIEW & CREATE</div>
            <h2 id="local-launch-title">Create your local Fabric network.</h2>
            <p>
              Review the supported development profile before starting real
              containers.
            </p>
            <dl className="launch-review-grid">
              <div>
                <dt>Deployment target</dt>
                <dd>This computer · Linux Docker</dd>
              </div>
              <div>
                <dt>Fabric & consensus</dt>
                <dd>2.5.15 · 3 Raft orderers</dd>
              </div>
              <div>
                <dt>Organizations</dt>
                <dd>Org1MSP and Org2MSP · 1 peer each</dd>
              </div>
              <div>
                <dt>Identities</dt>
                <dd>3 Fabric CAs · TLS enrollment · No cryptogen</dd>
              </div>
              <div>
                <dt>Channel & contract</dt>
                <dd>sathonate · Go shim smoke contract</dd>
              </div>
              <div>
                <dt>Resources</dt>
                <dd>10 containers · Persistent ledger volumes</dd>
              </div>
            </dl>
            <div className="info-note">
              <Server size={18} />
              <p>
                No cloud resources are provisioned. Cloud pricing is not
                applicable to this profile; local hardware, electricity and
                Docker licensing costs are excluded. Existing local identities
                and ledgers are retained.
              </p>
            </div>
            <label className="launch-confirm">
              <input
                type="checkbox"
                checked={confirmed}
                onChange={(e) => setConfirmed(e.target.checked)}
              />
              I understand this starts a development network on this computer.
            </label>
            <div className="modal-actions">
              <button
                className="button secondary"
                onClick={() => setReviewOpen(false)}
              >
                Back to workspace
              </button>
              <button
                className="button primary"
                disabled={!confirmed || busy}
                onClick={() => {
                  setReviewOpen(false);
                  void action(true);
                }}
              >
                <Play size={16} />
                Create network
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
