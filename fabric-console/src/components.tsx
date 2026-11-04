import {
  ArrowRight,
  Check,
  Cloud,
  GitBranch,
  Hexagon,
  Layers3,
  Server,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";
import type { NetworkSpec } from "./types";
import {
  cloneElement,
  isValidElement,
  useId,
  type ReactElement,
  type ReactNode,
} from "react";

export function Badge({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "green" | "amber" | "blue";
}) {
  return (
    <span className={`badge badge-${tone}`}>
      <span className="badge-dot" />
      {children}
    </span>
  );
}
export function Empty({
  icon: Icon = Layers3,
  title,
  children,
}: {
  icon?: LucideIcon;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <Icon size={26} />
      </div>
      <h3>{title}</h3>
      {children}
    </div>
  );
}
export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  const id = useId();
  const control = isValidElement(children)
    ? cloneElement(
        children as ReactElement<{ id: string; "aria-describedby"?: string }>,
        { id, "aria-describedby": hint ? `${id}-hint` : undefined },
      )
    : children;
  return (
    <div className="field">
      <label className="field-label" htmlFor={id}>
        {label}
      </label>
      {control}
      {hint && (
        <span className="field-hint" id={`${id}-hint`}>
          {hint}
        </span>
      )}
    </div>
  );
}
export function Topology({ spec }: { spec: NetworkSpec }) {
  return (
    <div className="topology" aria-label="Planned network topology">
      <div className="topology-label">
        <span className="eyebrow">NETWORK BLUEPRINT</span>
        <span className="tiny muted">Desired configuration</span>
      </div>
      <div className="orderer-group">
        <div className="topology-cloud">
          <ShieldCheck size={19} />
          <span>
            {spec.spec.ordering?.consensus === "bft" ? "SmartBFT" : "Raft"}{" "}
            ordering service
          </span>
          <span className="count-tag">
            {spec.spec.nodes.filter((n) => n.role === "orderer").length}{" "}
            orderers
          </span>
        </div>
      </div>
      <div className="connector-vertical" />
      <div className="channel-line">
        <GitBranch size={15} />
        <span>{spec.spec.channels[0]?.id ?? "Channel"}</span>
      </div>
      <div className="org-branches">
        {spec.spec.organizations.map((org) => (
          <div className="org-branch" key={org.id}>
            <div className="branch-line" />
            <div className="org-topology-card">
              <div className="org-icon">
                <Hexagon size={20} />
              </div>
              <strong>{org.id}</strong>
              <span>
                {
                  spec.spec.nodes.filter(
                    (n) => n.organizationRef === org.id && n.role === "peer",
                  ).length
                }{" "}
                peer(s) · Organization CA
              </span>
            </div>
          </div>
        ))}
      </div>
      <div className="topology-footer">
        <span>
          <span className="outline-dot" />
          Planned component
        </span>
        <span>No resources deployed</span>
      </div>
    </div>
  );
}
export function DestinationIcon({
  cloud,
  size = 20,
}: {
  cloud: boolean;
  size?: number;
}) {
  return cloud ? <Cloud size={size} /> : <Server size={size} />;
}
export function ProgressSteps({
  current,
  labels,
}: {
  current: number;
  labels: string[];
}) {
  return (
    <ol className="progress-steps">
      {labels.map((label, i) => (
        <li
          className={current === i ? "current" : current > i ? "complete" : ""}
          key={label}
        >
          <span>{current > i ? <Check size={14} /> : i + 1}</span>
          <b>{label}</b>
          {i < labels.length - 1 && (
            <ArrowRight size={14} className="step-arrow" />
          )}
        </li>
      ))}
    </ol>
  );
}
export const formatDate = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
