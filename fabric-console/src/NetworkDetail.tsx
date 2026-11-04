import { useState } from "react";
import {
  ArrowDownToLine,
  ArrowLeft,
  Archive,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Code2,
  FileCheck2,
  GitBranch,
  Hexagon,
  Info,
  Layers3,
  LoaderCircle,
  Pencil,
  Play,
  ShieldCheck,
} from "lucide-react";
import { api } from "./api";
import {
  Badge,
  DestinationIcon,
  Empty,
  formatDate,
  Topology,
} from "./components";
import type { Network } from "./types";
import { useDialog } from "./use-dialog";
import CloudReadiness from "./CloudReadiness";

export default function NetworkDetail({
  network,
  onBack,
  onEdit,
  onChange,
  onArchived,
}: {
  network: Network;
  onBack: () => void;
  onEdit: () => void;
  onChange: (network: Network) => void;
  onArchived: () => void;
}) {
  const [tab, setTab] = useState<
    "overview" | "orderers" | "cloud" | "plan" | "activity" | "configuration"
  >("overview");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [archiveConfirm, setArchiveConfirm] = useState(false);
  const dialogRef = useDialog(archiveConfirm, () => setArchiveConfirm(false));
  const spec = network.desired.spec;
  const review = async () => {
    setBusy(true);
    setError("");
    try {
      onChange(await api.review(network));
      setTab("plan");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not generate plan.");
    } finally {
      setBusy(false);
    }
  };
  const archive = async () => {
    setBusy(true);
    setError("");
    try {
      await api.archive(network);
      onArchived();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not archive.");
      setArchiveConfirm(false);
    } finally {
      setBusy(false);
    }
  };
  const download = () => {
    const blob = new Blob([JSON.stringify(network.desired, null, 2)], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${network.desired.metadata.id}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <>
      <button className="text-button back" onClick={onBack}>
        <ArrowLeft size={15} />
        All networks
      </button>
      <div className="page-heading detail-heading">
        <div>
          <div className="title-line">
            <h1>{network.desired.metadata.name}</h1>
            <Badge tone={network.plan ? "blue" : "neutral"}>
              {network.plan ? "Plan reviewed" : "Draft"}
            </Badge>
          </div>
          <p>
            <DestinationIcon
              cloud={spec.deployment.engine === "bevel"}
              size={15}
            />
            {spec.deployment.engine === "native"
              ? "Private servers"
              : spec.deployment.provider === "aws"
                ? "Amazon Web Services"
                : "Microsoft Azure"}
            <span className="dot-separator">·</span>
            {spec.environment}
            <span className="dot-separator">·</span>Fabric {spec.fabricVersion}
          </p>
        </div>
        <div className="heading-actions">
          <button className="button secondary" onClick={onEdit}>
            <Pencil size={15} />
            Edit design
          </button>
          <button
            className="button primary"
            disabled={busy}
            onClick={() => void review()}
          >
            {busy ? (
              <LoaderCircle className="spin" size={16} />
            ) : (
              <FileCheck2 size={17} />
            )}
            Review plan
          </button>
        </div>
      </div>
      <div className="notice-bar">
        <Info size={17} />
        <span>
          This is a network design. No infrastructure has been deployed.
        </span>
        <span className="tiny">Revision {network.revision}</span>
      </div>
      {error && (
        <div className="error-box" role="alert">
          {error}
        </div>
      )}
      <div className="tabs" role="tablist" aria-label="Network details">
        {(
          [
            { id: "overview", label: "Overview", icon: Layers3 },
            { id: "orderers", label: "Orderers", icon: ShieldCheck },
            ...(spec.deployment.engine === "bevel"
              ? [
                  {
                    id: "cloud",
                    label: "Cloud readiness",
                    icon: ShieldCheck,
                  } as const,
                ]
              : []),
            { id: "plan", label: "Deployment plan", icon: FileCheck2 },
            { id: "activity", label: "Activity", icon: Clock3 },
            { id: "configuration", label: "Configuration", icon: Code2 },
          ] as const
        ).map((item) => (
          <button
            key={item.id}
            role="tab"
            aria-selected={tab === item.id}
            onClick={() => setTab(item.id)}
            className={tab === item.id ? "active" : ""}
          >
            <item.icon size={16} />
            {item.label}
            {item.id === "activity" && <span>{network.events.length}</span>}
          </button>
        ))}
      </div>
      {tab === "orderers" && (
        <div className="panel">
          <div className="panel-heading">
            <div>
              <h2>Ordering service</h2>
              <p>
                {spec.ordering?.consensus === "bft"
                  ? "SmartBFT · Byzantine fault tolerance"
                  : "Raft · Crash fault tolerance"}{" "}
                · {spec.ordering?.channelCapability ?? "V2_0"} channel
                capability
              </p>
            </div>
            <Badge>Design only</Badge>
          </div>
          <div className="orderer-inventory">
            {spec.nodes
              .filter((node) => node.role === "orderer")
              .map((node) => (
                <article key={node.id} className="orderer-inventory-row">
                  <span className="org-icon">
                    <ShieldCheck size={18} />
                  </span>
                  <div>
                    <strong>{node.id}</strong>
                    <span>Organization: {node.organizationRef}</span>
                  </div>
                  <div>
                    <small>Placement reference</small>
                    <code>{node.placementRef}</code>
                  </div>
                  <div>
                    <small>Failure domain</small>
                    <code>{node.failureDomain}</code>
                  </div>
                </article>
              ))}
          </div>
          <div className="info-note orderer-inventory-note">
            <Info size={17} />
            <p>
              These are configured orderers, not running nodes. Placement, fault
              tolerance and trust assumptions must be verified against actual
              infrastructure. Use Edit design to change ownership or consensus.
            </p>
          </div>
        </div>
      )}
      {tab === "cloud" && (
        <CloudReadiness
          key={`${network.id}-${network.revision}`}
          network={network}
        />
      )}
      {tab === "overview" && (
        <div className="detail-layout">
          <div>
            <div className="panel topology-panel">
              <div className="panel-heading">
                <div>
                  <h2>Network topology</h2>
                  <p>A clear view of how your participants connect.</p>
                </div>
                <Badge>Planned</Badge>
              </div>
              <Topology spec={network.desired} />
            </div>
            <div className="panel organizations-panel">
              <div className="panel-heading">
                <h2>Organizations</h2>
                <span className="count-tag">
                  {spec.organizations.length} participants
                </span>
              </div>
              {spec.organizations.map((org) => (
                <div className="organization-detail" key={org.id}>
                  <span className="org-icon">
                    <Hexagon size={19} />
                  </span>
                  <div>
                    <strong>{org.id}</strong>
                    <span>{org.mspId}</span>
                  </div>
                  <span>
                    {
                      spec.nodes.filter(
                        (n) =>
                          n.role === "peer" && n.organizationRef === org.id,
                      ).length
                    }{" "}
                    peer(s)
                  </span>
                  <Badge>Configured</Badge>
                </div>
              ))}
            </div>
          </div>
          <aside>
            <div className="panel summary-panel">
              <h3>Design summary</h3>
              {[
                ["Organizations", spec.organizations.length],
                ["Peers", spec.nodes.filter((n) => n.role === "peer").length],
                [
                  "Orderers",
                  spec.nodes.filter((n) => n.role === "orderer").length,
                ],
                ["Channels", spec.channels.length],
              ].map(([key, value]) => (
                <div className="summary-row" key={key}>
                  <span>{key}</span>
                  <strong>{value}</strong>
                </div>
              ))}
              <div className="form-divider" />
              <div className="summary-row">
                <span>Target reference</span>
                <code>{spec.deployment.targetRef}</code>
              </div>
              <p className="tiny muted">
                Target references and placements are design placeholders. They
                are not connected infrastructure.
              </p>
            </div>
            <div className="panel next-step">
              <span className="soft-icon">
                <ShieldCheck size={21} />
              </span>
              <h3>Review before you deploy.</h3>
              <p>
                See the steps, dependencies, and checks your network will need.
              </p>
              <button
                className="text-button teal"
                disabled={busy}
                onClick={() => void review()}
              >
                Review deployment plan
                <ChevronRight size={15} />
              </button>
            </div>
            <div className="small-note">
              <Clock3 size={14} />
              Updated {formatDate(network.updatedAt)}
            </div>
          </aside>
        </div>
      )}
      {tab === "plan" && (
        <div className="panel plan-panel">
          <div className="panel-heading">
            <div>
              <h2>Deployment plan</h2>
              <p>
                A preview of the work required to bring this design to life.
              </p>
            </div>
            <Badge tone="amber">Execution unavailable</Badge>
          </div>
          {!network.plan ? (
            <Empty icon={FileCheck2} title="Know what happens next.">
              <p>
                Generate a plan to review the steps and checks for this network.
              </p>
              <button
                className="button primary"
                disabled={busy}
                onClick={() => void review()}
              >
                Generate review plan
                <ChevronRight size={16} />
              </button>
            </Empty>
          ) : (
            <>
              <div className="plan-intro">
                <CheckCircle2 size={20} />
                <div>
                  <strong>
                    Plan generated from revision {network.revision}
                  </strong>
                  <p>
                    These are planned steps, not completed actions.
                    Infrastructure execution will be added later.
                  </p>
                </div>
              </div>
              <ol className="plan-timeline">
                {network.plan.steps.map((step, i) => (
                  <li key={step.id}>
                    <span className="timeline-number">{i + 1}</span>
                    <div>
                      <strong>{step.action}</strong>
                      <p>{step.verification}</p>
                      <span className="step-state">Planned</span>
                    </div>
                  </li>
                ))}
              </ol>
              <div className="plan-hash">
                <span>Plan fingerprint</span>
                <code>{network.plan.id}</code>
              </div>
              <div className="deploy-footer">
                <p>
                  <Info size={16} />
                  Deployment engines are not connected in this release.
                </p>
                <button className="button primary" disabled>
                  <Play size={15} />
                  Deploy network
                </button>
              </div>
            </>
          )}
        </div>
      )}
      {tab === "activity" && (
        <div className="panel">
          <div className="panel-heading">
            <div>
              <h2>Network activity</h2>
              <p>Saved changes and reviews, recorded by the control plane.</p>
            </div>
          </div>
          <div className="activity-list">
            {[...network.events].reverse().map((event) => (
              <div className="activity-item" key={event.id}>
                <span className="activity-icon">
                  <Clock3 size={17} />
                </span>
                <div>
                  <strong>
                    {event.kind === "draft_created"
                      ? "Network draft created"
                      : event.kind === "draft_updated"
                        ? "Design updated"
                        : "Deployment plan reviewed"}
                  </strong>
                  <p>{event.message}</p>
                  <span className="tiny muted">
                    {formatDate(event.at)} · Revision {event.revision}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
      {tab === "configuration" && (
        <div className="panel">
          <div className="panel-heading">
            <div>
              <h2>Network configuration</h2>
              <p>
                The portable desired specification. No credentials are included.
              </p>
            </div>
            <button className="button secondary" onClick={download}>
              <ArrowDownToLine size={15} />
              Export JSON
            </button>
          </div>
          <pre className="configuration-code">
            {JSON.stringify(network.desired, null, 2)}
          </pre>
        </div>
      )}
      <div className="detail-bottom">
        <span>
          <GitBranch size={14} />
          {network.desired.metadata.id}
        </span>
        <button
          className="text-button muted"
          onClick={() => setArchiveConfirm(true)}
        >
          <Archive size={14} />
          Archive draft
        </button>
      </div>
      {archiveConfirm && (
        <div className="modal-backdrop">
          <div
            ref={dialogRef}
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="archive-title"
          >
            <h2 id="archive-title">Archive this draft?</h2>
            <p>
              “{network.desired.metadata.name}” will leave your active
              workspace. This does not delete or change any infrastructure.
            </p>
            <div className="modal-actions">
              <button
                className="button secondary"
                autoFocus
                onClick={() => setArchiveConfirm(false)}
              >
                Keep draft
              </button>
              <button
                className="button danger"
                disabled={busy}
                onClick={() => void archive()}
              >
                {busy && <LoaderCircle className="spin" size={15} />}Archive
                draft
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
