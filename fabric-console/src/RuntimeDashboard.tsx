import { useEffect, useState } from "react";
import { RefreshCw, ShieldCheck, LoaderCircle } from "lucide-react";
import { api, type RuntimeHealth } from "./api";
import { Badge, formatDate } from "./components";

export default function RuntimeDashboard() {
  const [report, setReport] = useState<RuntimeHealth | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    let disposed = false;
    let pending = false;
    const load = async () => {
      if (pending) return;
      pending = true;
      setBusy(true);
      try {
        const h = await api.localHealth();
        if (!disposed) {
          setReport(h);
          setError("");
        }
      } catch (e) {
        if (!disposed)
          setError(e instanceof Error ? e.message : "Health checks failed");
      } finally {
        pending = false;
        if (!disposed) setBusy(false);
      }
    };
    void load();
    const interval = setInterval(() => void load(), 20000);
    return () => {
      disposed = true;
      clearInterval(interval);
    };
  }, [refresh]);
  return (
    <section
      className="panel local-runtime-panel runtime-dashboard"
      aria-label="Verified network inventory"
    >
      <div className="panel-heading">
        <div>
          <h2>Verified network inventory</h2>
          <p>
            {report
              ? `Last checked ${formatDate(report.checkedAt)} · TLS Fabric queries`
              : "Checking channel ledgers, consenters, CAs and contract definition…"}
          </p>
        </div>
        <button
          className="button secondary"
          disabled={busy}
          onClick={() => setRefresh((v) => v + 1)}
        >
          {busy ? (
            <LoaderCircle size={15} className="spin" />
          ) : (
            <RefreshCw size={15} />
          )}
          Check health
        </button>
      </div>
      <div className="operation-body">
        {error && (
          <div role="alert" className="error-box">
            {error} Previous observations may be stale.
          </div>
        )}
        {report && (
          <>
            <div className="dashboard-health-line">
              <ShieldCheck size={20} />
              <strong>Channel {report.channel}</strong>
              <Badge tone={report.ready && !error ? "green" : "amber"}>
                {report.ready && !error ? "Health verified" : "Needs attention"}
              </Badge>
            </div>
            {report.alerts.map((alert) => (
              <div className="error-box" role="alert" key={alert}>
                {alert}
              </div>
            ))}
            <div className="dashboard-inventory-grid">
              <div>
                <h3>Peer organizations & ledgers</h3>
                {report.peers.map((p) => (
                  <div className="inventory-item" key={p.peer}>
                    <div>
                      <strong>
                        {p.peer} · {p.mspId}
                      </strong>
                      <p>
                        Block height: {p.available ? p.height : "Unavailable"}
                      </p>
                      {p.currentBlockHash && (
                        <code title={p.currentBlockHash}>
                          {p.currentBlockHash.slice(0, 24)}…
                        </code>
                      )}
                    </div>
                    <Badge tone={p.available ? "green" : "amber"}>
                      {p.available ? "Responding" : "Unreachable"}
                    </Badge>
                  </div>
                ))}
              </div>
              <div>
                <h3>Raft orderers</h3>
                {report.orderers.map((o) => (
                  <div className="inventory-item" key={o.name}>
                    <div>
                      <strong>{o.name}</strong>
                      <p>
                        {o.consensusRelation || "Unconfirmed membership"} ·
                        height {o.height}
                      </p>
                    </div>
                    <Badge tone={o.status === "active" ? "green" : "amber"}>
                      {o.status}
                    </Badge>
                  </div>
                ))}
              </div>
              <div>
                <h3>Certificate authorities</h3>
                {report.certificateAuthorities.map((ca) => (
                  <div className="inventory-item" key={ca.name}>
                    <strong>{ca.name}</strong>
                    <Badge tone={ca.available ? "green" : "amber"}>
                      {ca.available ? "TLS verified" : "Unavailable"}
                    </Badge>
                  </div>
                ))}
              </div>
              <div>
                <h3>Committed chaincode</h3>
                <div className="inventory-item">
                  <div>
                    <strong>{report.chaincode.name}</strong>
                    <p>
                      Version {report.chaincode.version ?? "—"} · sequence{" "}
                      {report.chaincode.sequence ?? "—"}
                    </p>
                  </div>
                  <Badge tone={report.chaincode.available ? "green" : "amber"}>
                    {report.chaincode.available ? "Committed" : "Unconfirmed"}
                  </Badge>
                </div>
                <p className="inventory-disclosure">
                  Health comes from live Fabric and CA responses. No estimated
                  TPS, CPU usage or transaction rows are generated.
                </p>
              </div>
            </div>
          </>
        )}
      </div>
    </section>
  );
}
