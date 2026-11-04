import { useState, type ReactNode } from "react";
import {
  Boxes,
  Building2,
  CheckCircle2,
  Cloud,
  CloudUpload,
  Code2,
  Cpu,
  Database,
  GitBranch,
  Globe,
  Info,
  KeyRound,
  Layers3,
  Link2,
  Network,
  Plus,
  Server,
  Settings2,
  ShieldCheck,
  Trash2,
  Pencil,
  TriangleAlert,
} from "lucide-react";
import { Field } from "./components";
import { addons, type FlowDraft, type Provider } from "./smp-model";
import { slug, withOrdererCount } from "./network-form";
import type { CloudTarget } from "./api";
import type { Issue } from "./types";

export function Card({
  title,
  icon: Icon,
  action,
  children,
  className = "",
}: {
  title: string;
  icon?: typeof Boxes;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`smp-card ${className}`}>
      <div className="smp-card-heading">
        <h2>
          {Icon && <Icon size={19} />} {title}
        </h2>
        {action}
      </div>
      {children}
    </section>
  );
}
export function Note({
  children,
  tone = "blue",
}: {
  children: ReactNode;
  tone?: "blue" | "amber" | "green";
}) {
  return (
    <div className={`smp-note ${tone}`}>
      <Info size={18} />
      <div>{children}</div>
    </div>
  );
}
export function Status({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "green" | "blue" | "amber" | "neutral";
}) {
  return (
    <span className={`smp-status ${tone}`}>
      <i />
      {children}
    </span>
  );
}
export function ProviderMark({ provider }: { provider: Provider | "gcp" }) {
  return provider === "aws" ? (
    <span className="provider-aws">
      aws<span>⌣</span>
    </span>
  ) : provider === "azure" ? (
    <svg
      className="provider-azure-mark"
      width="52"
      height="52"
      viewBox="0 0 52 52"
      aria-hidden="true"
    >
      <defs>
        <linearGradient id="azure-fold" x1="0" y1="0" x2="1" y2="1">
          <stop stopColor="#0078d4" />
          <stop offset="1" stopColor="#59c9ed" />
        </linearGradient>
      </defs>
      <path d="M19 3h13L14 48H2Z" fill="#0078d4" />
      <path d="M32 3 49 48H31L19 35h17L25 9Z" fill="url(#azure-fold)" />
      <path d="m19 35 12 13H14Z" fill="#005ba1" />
    </svg>
  ) : provider === "gcp" ? (
    <svg
      className="provider-gcp-mark"
      width="58"
      height="52"
      viewBox="0 0 64 52"
      aria-hidden="true"
    >
      <path d="M17 22C17 5 39 0 49 14l-8 8C35 12 24 15 24 24Z" fill="#ea4335" />
      <path d="M44 13c21 2 26 30 6 35H33V36h16c9-2 7-13-1-14Z" fill="#4285f4" />
      <path
        d="M17 20C-1 22-1 46 17 48h17V36H18c-7-1-7-10 0-12Z"
        fill="#fbbc05"
      />
      <path
        d="M17 36h21v12H17C9 48 5 44 3 39l9-8c0 3 2 5 5 5Z"
        fill="#34a853"
      />
    </svg>
  ) : (
    <Server className="provider-local" size={34} />
  );
}
type Props = { draft: FlowDraft; setDraft: (d: FlowDraft) => void };
export function CloudAccount({
  draft: d,
  setDraft,
  targets,
  onProvider,
  testResult,
}: {
  targets: CloudTarget[];
  onProvider: (p: Provider) => void;
  onTest: () => void;
  testResult: string;
} & Props) {
  const [policy, setPolicy] = useState(false);
  const field = (k: keyof FlowDraft, v: string) => setDraft({ ...d, [k]: v });
  return (
    <>
      <Card title="1. Select Cloud Provider">
        <div className="smp-providers">
          {(
            [
              {
                id: "aws",
                title: "Amazon Web Services",
                detail: "EKS, VPC, IAM",
              },
              {
                id: "azure",
                title: "Microsoft Azure",
                detail: "AKS, VNet, federated identity",
              },
              {
                id: "gcp",
                title: "Google Cloud Platform",
                detail: "Provider integration planned",
              },
              {
                id: "kubernetes",
                title: "Other / Existing",
                detail:
                  "Use an existing Kubernetes cluster or local environment",
              },
            ] as const
          ).map((p) => (
            <button
              className={`smp-provider ${d.provider === p.id || (p.id === "kubernetes" && d.provider === "local") ? "selected" : ""}`}
              disabled={p.id === "gcp"}
              key={p.id}
              onClick={() => p.id !== "gcp" && onProvider(p.id)}
              aria-pressed={
                d.provider === p.id ||
                (p.id === "kubernetes" && d.provider === "local")
              }
            >
              <span className="smp-radio" />
              <ProviderMark provider={p.id} />
              <strong>{p.title}</strong>
              <small>{p.detail}</small>
              {p.id === "gcp" && (
                <span className="provider-planned">Planned</span>
              )}
            </button>
          ))}
        </div>
      </Card>
      {["local", "kubernetes"].includes(d.provider) ? (
        <>
          <div
            className="smp-local-options"
            aria-label="Local runtime selection"
          >
            <button
              className={`smp-btn ${d.provider === "kubernetes" ? "primary" : "secondary"}`}
              onClick={() => onProvider("kubernetes")}
            >
              Local Kubernetes
            </button>
            <button
              className={`smp-btn ${d.provider === "local" ? "primary" : "secondary"}`}
              onClick={() => onProvider("local")}
            >
              Local / Existing Docker
            </button>
          </div>
          <Card title="2. Connect Your Local Runtime" icon={Server}>
            <Note>
              The selected local profile runs real Fabric CA, peer, Raft orderer
              and chaincode workloads. No cloud credentials are needed.
              Kubernetes mode uses the dedicated kind cluster and Bevel
              operator.
            </Note>
            <div className="smp-method selected">
              <ShieldCheck size={24} />
              <div>
                <strong>
                  {d.provider === "kubernetes"
                    ? "Local kind Kubernetes / Bevel"
                    : "Local Docker Engine"}
                </strong>
                <p>
                  Official Fabric binaries · CA-enrolled identities ·
                  Loopback-only endpoints
                </p>
              </div>
              <CheckCircle2 size={19} />
            </div>
          </Card>
        </>
      ) : (
        <>
          <Card
            title={`2. Select Authentication Method (${d.provider === "aws" ? "AWS" : "Azure"})`}
          >
            <Note>
              Use a scoped, short-lived identity. Permanent access keys and
              private credentials are never collected in this form.
            </Note>
            <div className="smp-method-grid">
              <div className="smp-method selected">
                <ShieldCheck size={24} />
                <div>
                  <strong>
                    {d.provider === "aws" ? "IAM Role" : "Workload Identity"}{" "}
                    <span className="smp-recommended">Recommended</span>
                  </strong>
                  <p>
                    {d.provider === "aws"
                      ? "Cross-account role assumption with short-lived sessions."
                      : "Federated identity or managed identity in your environment."}
                  </p>
                </div>
                <span className="smp-radio" />
              </div>
              <div className="smp-method unavailable">
                <KeyRound size={23} />
                <div>
                  <strong>Access Keys</strong>
                  <p>
                    Not collected. Use a locally configured short-lived AWS
                    profile.
                  </p>
                </div>
                <span className="smp-radio" />
              </div>
              <div className="smp-method unavailable">
                <Info size={23} />
                <div>
                  <strong>Temporary Credentials</strong>
                  <p>
                    Managed through AWS SSO or workload identity; never pasted
                    here.
                  </p>
                </div>
                <span className="smp-radio" />
              </div>
            </div>
          </Card>
          <Card
            title={`3. Enter ${d.provider === "aws" ? "AWS" : "Azure"} Details`}
          >
            <div className="smp-fields two">
              {d.provider === "aws" ? (
                <>
                  <Field
                    label="IAM Role ARN"
                    hint="The deployment role is registered and validated on the server before access is granted."
                  >
                    <input
                      placeholder="arn:aws:iam::123456789012:role/FabricDeploymentRole"
                      value={d.roleArn}
                      maxLength={2048}
                      onChange={(e) => field("roleArn", e.target.value)}
                    />
                  </Field>
                </>
              ) : (
                <>
                  <Field label="Tenant ID">
                    <input
                      value={d.tenantId}
                      onChange={(e) => field("tenantId", e.target.value)}
                      placeholder="Microsoft Entra tenant ID"
                    />
                  </Field>
                  <Field label="Subscription ID">
                    <input
                      value={d.subscriptionId}
                      onChange={(e) => field("subscriptionId", e.target.value)}
                      placeholder="Azure subscription ID"
                    />
                  </Field>
                </>
              )}
              <Field
                label={d.provider === "aws" ? "AWS Region" : "Azure Region"}
                hint="Use the region of your intended cluster."
              >
                <input
                  value={d.region}
                  onChange={(e) => field("region", e.target.value)}
                  placeholder={
                    d.provider === "aws"
                      ? "Select a region (e.g. ap-south-1)"
                      : "e.g. centralindia"
                  }
                />
              </Field>
              {d.provider === "aws" ? (
                <Field
                  label="External ID"
                  hint="Generated and verified by the future authorization service."
                >
                  <input
                    disabled
                    value="Available when role onboarding is enabled"
                  />
                </Field>
              ) : (
                <Field label="Resource Group">
                  <input
                    value={d.resourceGroup}
                    onChange={(e) => field("resourceGroup", e.target.value)}
                    placeholder="Your cluster resource group"
                  />
                </Field>
              )}
              {d.provider === "aws" && (
                <Field
                  label="AWS Account ID"
                  hint="12-digit account identifier, not an access key."
                >
                  <input
                    placeholder="123456789012"
                    value={d.accountId}
                    maxLength={12}
                    onChange={(e) => field("accountId", e.target.value)}
                  />
                </Field>
              )}
            </div>
            <p className="smp-cloud-availability">
              {targets.filter((target) => target.provider === d.provider).length
                ? `${targets.filter((target) => target.provider === d.provider).length} existing server-registered target(s) available. Select one on the next screen.`
                : "No cloud account is connected. You can prepare a design; cloud authorization and provisioning remain unavailable."}
            </p>
            <div className="smp-inline-actions smp-permissions">
              <div className="smp-permissions-copy">
                <Info size={22} />
                <div>
                  <strong>Required Permissions</strong>
                  <p>
                    Cloud IAM and Kubernetes permissions are configured
                    separately. Access is verified before deployment.
                  </p>
                </div>
              </div>
              <button
                className="smp-btn secondary"
                onClick={() => setPolicy(!policy)}
              >
                <ShieldCheck size={16} />
                View Recommended Policy
              </button>
            </div>
            {policy && (
              <div className="smp-policy">
                <strong>Keep access boundaries separate</strong>
                <p>
                  Cloud IAM authorizes provider API calls. Kubernetes access is
                  separately scoped to the registered cluster. GitOps repository
                  access and Fabric CA identities use their own credentials. An
                  unregistered role ARN does not authorize Fabric SMP to access
                  an account.
                </p>
              </div>
            )}
          </Card>
        </>
      )}
      {testResult && (
        <Note tone={testResult.startsWith("Connected") ? "green" : "amber"}>
          {testResult}
        </Note>
      )}
    </>
  );
}
export function Kubernetes({
  draft: d,
  setDraft,
  targets,
  onTest,
  testResult,
}: Props & { targets: CloudTarget[]; onTest: () => void; testResult: string }) {
  const field = (k: keyof FlowDraft, v: string | number) =>
    setDraft({ ...d, [k]: v });
  if (["local", "kubernetes"].includes(d.provider))
    return (
      <>
        <Card title="Deployment Runtime" icon={Server}>
          <div className="smp-mode-grid">
            <div className="smp-option selected">
              <Server size={28} />
              <div>
                <strong>
                  {d.provider === "kubernetes"
                    ? "Use existing kind-smp-qualification"
                    : "Use Local Docker Engine"}
                </strong>
                <p>Existing Linux container runtime on this computer</p>
              </div>
              <CheckCircle2 size={20} />
            </div>
            <div className="smp-option unavailable">
              <CloudUpload size={28} />
              <div>
                <strong>Remote Kubernetes</strong>
                <p>Choose a cloud provider for cluster planning</p>
              </div>
            </div>
          </div>
          <Note>
            {d.provider === "kubernetes"
              ? "The worker uses the explicit kind-smp-qualification context, standard persistent storage and the pinned Bevel Fabric operator. All resources run on this computer."
              : "The Docker profile does not require Kubernetes. Continue with topology, review and a real deployment operation."}
          </Note>
        </Card>
        <Card title="Runtime Configuration" icon={Settings2}>
          <div className="smp-fields two">
            <Field label="Runtime name">
              <input disabled value="Sathonate local development" />
            </Field>
            <Field label="Container platform">
              <input
                disabled
                value={
                  d.provider === "kubernetes"
                    ? "Kubernetes (kind) · Linux / amd64"
                    : "Linux / amd64 · Docker Compose"
                }
              />
            </Field>
            <Field label="Storage">
              <input
                disabled
                value={
                  d.provider === "kubernetes"
                    ? "PersistentVolumeClaims · standard storage"
                    : "Persistent named ledger volumes"
                }
              />
            </Field>
            <Field label="Network exposure">
              <input
                disabled
                value={
                  d.provider === "kubernetes"
                    ? "Internal cluster DNS · ClusterIP services"
                    : "127.0.0.1 published endpoints"
                }
              />
            </Field>
          </div>
          <Note tone="amber">
            All containers run on one computer. Three Raft orderers do not
            provide machine-level fault isolation here.
          </Note>
          <button className="smp-btn secondary" onClick={onTest}>
            <Settings2 size={16} />
            Test Configuration
          </button>
          {testResult && <p className="smp-test-result">{testResult}</p>}
        </Card>
      </>
    );
  return (
    <>
      <Card title="Choose Your Kubernetes Cluster" icon={Layers3}>
        <div className="smp-mode-grid">
          {(
            [
              {
                id: "new",
                title: "Create New Cluster",
                detail: "Plan a new managed Kubernetes cluster",
                icon: CloudUpload,
              },
              {
                id: "existing",
                title: "Use Existing Cluster",
                detail: "Select a registered cluster or plan a connection",
                icon: Link2,
              },
            ] as const
          ).map((m) => (
            <button
              key={m.id}
              className={`smp-option ${d.clusterMode === m.id ? "selected" : ""}`}
              onClick={() => setDraft({ ...d, clusterMode: m.id })}
              aria-pressed={d.clusterMode === m.id}
            >
              <m.icon size={29} />
              <div>
                <strong>{m.title}</strong>
                <p>{m.detail}</p>
              </div>
              <span className="smp-radio" />
            </button>
          ))}
        </div>
      </Card>
      <Card title="Cluster Configuration" icon={Layers3}>
        {d.clusterMode === "existing" && targets.length > 0 && (
          <Field label="Registered cluster">
            <select
              value={d.targetRef}
              onChange={(e) => {
                const t = targets.find((t) => t.id === e.target.value);
                setDraft({
                  ...d,
                  targetRef: e.target.value,
                  clusterName: t?.clusterName ?? "",
                  region: t?.region ?? d.region,
                  form: {
                    ...d.form,
                    target: e.target.value || `${d.provider}-workspace`,
                  },
                });
              }}
            >
              <option value="">Select a registered target</option>
              {targets
                .filter((t) => t.provider === d.provider)
                .map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name || t.clusterName} · {t.region}
                  </option>
                ))}
            </select>
          </Field>
        )}
        <div className="smp-fields three">
          <Field label="Cluster Name" hint="A name for your intended cluster.">
            <input
              value={d.clusterName}
              placeholder="fabric-cluster"
              onChange={(e) => field("clusterName", e.target.value)}
            />
          </Field>
          <Field label="Region">
            <input
              value={d.region}
              placeholder="Provider region"
              onChange={(e) => field("region", e.target.value)}
            />
          </Field>
          <Field
            label="Kubernetes Version"
            hint="Must be qualified against the provider and Bevel revision."
          >
            <input
              value={d.kubeVersion}
              placeholder="Compatibility check required"
              onChange={(e) => field("kubeVersion", e.target.value)}
            />
          </Field>
        </div>
        <Note tone="amber">
          {d.clusterMode === "new"
            ? "Infrastructure provisioning is not enabled. These fields describe your intended cluster; they do not create it."
            : "Cluster access is not inferred from its name. Only registered targets can be checked by the backend."}
        </Note>
      </Card>
      {d.clusterMode === "new" && (
        <>
          <Card title="Node Configuration" icon={Cpu}>
            <div className="smp-fields two">
              <Field
                label="Instance Type"
                hint="Desired instance identifier; availability is not yet verified."
              >
                <input
                  value={d.instanceType}
                  placeholder="Select after provider qualification"
                  onChange={(e) => field("instanceType", e.target.value)}
                />
              </Field>
              <div>
                <span className="field-label">Node Count</span>
                <div className="smp-fields three">
                  {(["minNodes", "desiredNodes", "maxNodes"] as const).map(
                    (k, i) => (
                      <Field
                        key={k}
                        label={["Minimum", "Desired", "Maximum"][i]}
                      >
                        <input
                          type="number"
                          min={1}
                          max={100}
                          value={d[k]}
                          onChange={(e) => field(k, Number(e.target.value))}
                        />
                      </Field>
                    ),
                  )}
                </div>
              </div>
            </div>
          </Card>
          <Card title="Networking" icon={Network}>
            <div className="smp-mode-grid">
              {(["new", "existing"] as const).map((mode) => (
                <button
                  className={`smp-option compact ${d.vpcMode === mode ? "selected" : ""}`}
                  key={mode}
                  onClick={() => setDraft({ ...d, vpcMode: mode })}
                >
                  <Globe size={22} />
                  <strong>
                    {mode === "new" ? "Create New VPC" : "Use Existing VPC"}
                  </strong>
                  <span className="smp-radio" />
                </button>
              ))}
            </div>
            <div className="smp-fields three">
              {(["vpcCidr", "serviceCidr", "podCidr"] as const).map((k, i) => (
                <Field
                  key={k}
                  label={["VPC CIDR Block", "Service CIDR", "Pod CIDR"][i]}
                >
                  <input
                    value={d[k]}
                    placeholder="e.g. 10.0.0.0/16"
                    onChange={(e) => field(k, e.target.value)}
                  />
                </Field>
              ))}
            </div>
          </Card>
          <Card title="Add-ons and Components" icon={Settings2}>
            <p className="smp-subcopy">
              Planned components. Selection does not install or certify an
              add-on.
            </p>
            <div className="smp-addon-grid">
              {addons.map((addon, i) => (
                <label key={addon}>
                  <input
                    type="checkbox"
                    checked={d.addons.includes(addon)}
                    onChange={() =>
                      setDraft({
                        ...d,
                        addons: d.addons.includes(addon)
                          ? d.addons.filter((a) => a !== addon)
                          : [...d.addons, addon],
                      })
                    }
                  />
                  <span className={`smp-addon-icon c${i % 3}`}>
                    <Boxes size={19} />
                  </span>
                  <span>{addon}</span>
                </label>
              ))}
            </div>
          </Card>
        </>
      )}
      <div className="smp-config-test">
        <button className="smp-btn secondary" onClick={onTest}>
          <Settings2 size={16} />
          Test Configuration
        </button>
        <span>
          {testResult ||
            "Checks validate input only until a real cloud target is registered."}
        </span>
      </div>
    </>
  );
}

export function NetworkDesign({
  draft: d,
  setDraft,
  onValidate,
  validation,
}: { onValidate: () => void; validation: string } & Props) {
  const f = d.form,
    local = ["local", "kubernetes"].includes(d.provider);
  const patch = (p: Partial<typeof f>) =>
    setDraft({ ...d, form: { ...f, ...p } });
  const editOrg = (key: string, name: string, peers?: number) =>
    patch({
      organizations: f.organizations.map((o) =>
        o.key === key ? { ...o, name, peers: peers ?? o.peers } : o,
      ),
    });
  return (
    <>
      <div className="smp-grid two">
        <Card title="Network Basics" icon={Layers3}>
          <Field label="Network Name *">
            <input
              value={f.name}
              disabled={d.provider === "local"}
              placeholder="supply-chain-network"
              onChange={(e) => patch({ name: e.target.value })}
            />
          </Field>
          <Field
            label="Domain"
            hint={
              local
                ? "Local Docker service discovery; no public domain required."
                : "Design metadata only; DNS provisioning is not enabled."
            }
          >
            <input
              value={local ? "Local container DNS" : d.domain}
              disabled={local}
              placeholder="e.g. fabric.company.com"
              onChange={(e) => setDraft({ ...d, domain: e.target.value })}
            />
          </Field>
          <div className="smp-fields two">
            <Field label="Fabric Version *">
              <select
                value={f.version}
                disabled={local}
                onChange={(e) => patch({ version: e.target.value })}
              >
                {["2.5.15", "2.5.16"].map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            </Field>
            <Field label="Environment">
              <select
                value={f.environment}
                disabled={local}
                onChange={(e) =>
                  patch({ environment: e.target.value as typeof f.environment })
                }
              >
                <option value="development">Development</option>
                <option value="production">Production design</option>
              </select>
            </Field>
          </div>
        </Card>
        <Card
          title="Organizations"
          icon={Building2}
          action={
            <button
              className="smp-btn small"
              disabled={local || f.organizations.length >= 8}
              onClick={() =>
                patch({
                  organizations: [
                    ...f.organizations,
                    {
                      key: crypto.randomUUID(),
                      name: `Org${f.organizations.length + 1}`,
                      peers: 1,
                    },
                  ],
                })
              }
            >
              <Plus size={14} />
              Add Organization
            </button>
          }
        >
          <div className="smp-org-list">
            {f.organizations.map((o, i) => (
              <div key={o.key} className={`smp-org c${i % 3}`}>
                <span className="smp-org-dot" />
                <div>
                  <input
                    aria-label={`Organization ${i + 1} name`}
                    value={o.name}
                    disabled={local}
                    onChange={(e) => editOrg(o.key, e.target.value)}
                  />
                  <small>
                    {o.peers} peer{o.peers === 1 ? "" : "s"} · 1 CA{" "}
                    {i === 0 ? "· Primary" : ""}
                  </small>
                </div>
                <button
                  className="smp-icon-btn"
                  disabled={local || f.organizations.length <= 2}
                  aria-label={`Remove organization ${i + 1}`}
                  onClick={() => {
                    const orgs = f.organizations.filter((x) => x.key !== o.key);
                    patch({
                      organizations: orgs,
                      ordererNodes: f.ordererNodes.map((n) =>
                        n.organizationKey === o.key
                          ? { ...n, organizationKey: orgs[0].key }
                          : n,
                      ),
                    });
                  }}
                >
                  <Trash2 size={15} />
                </button>
              </div>
            ))}
          </div>
          <p className="smp-subcopy">
            {local
              ? "The executable local profile has two fixed peer organizations and a separate ordering MSP."
              : "Organization names become stable MSP and identity-provider references."}
          </p>
        </Card>
      </div>
      <div className="smp-grid three">
        <Card title="Peers" icon={Server}>
          <Field label="Peers per Organization *">
            <select
              disabled={local}
              value={f.organizations[0]?.peers ?? 1}
              onChange={(e) =>
                patch({
                  organizations: f.organizations.map((o) => ({
                    ...o,
                    peers: Number(e.target.value),
                  })),
                })
              }
            >
              {[1, 2, 3, 4].map((n) => (
                <option key={n}>{n}</option>
              ))}
            </select>
          </Field>
          <Field label="State Database">
            <input disabled value="GoLevelDB" />
          </Field>
          <span className="smp-inline-check">
            <CheckCircle2 size={16} />
            Persistent peer ledgers
          </span>
        </Card>
        <Card title="Orderers" icon={Database}>
          <Field label="Orderer Nodes *">
            <select
              disabled={local}
              value={f.orderers}
              onChange={(e) =>
                setDraft({
                  ...d,
                  form: withOrdererCount(f, Number(e.target.value)),
                })
              }
            >
              {[3, 5, 7].map((n) => (
                <option key={n}>{n}</option>
              ))}
            </select>
          </Field>
          <Field label="Consensus Type *">
            <select value="raft" disabled>
              <option>Raft</option>
            </select>
          </Field>
          <span className="smp-subcopy">
            Solo is retired. BFT is not available in this supported execution
            flow.
          </span>
        </Card>
        <Card title="Certificate Authorities" icon={ShieldCheck}>
          <Field label="CA per Organization">
            <input disabled value="1 Fabric CA" />
          </Field>
          <Field label="Identity Enrollment">
            <input disabled value="Fabric CA registration & enrollment" />
          </Field>
          <span className="smp-inline-check">
            <CheckCircle2 size={16} />
            TLS enabled · No cryptogen
          </span>
        </Card>
      </div>
      <div className="smp-grid two">
        <Card title="Channels" icon={GitBranch}>
          <Field label="Channel Name *">
            <input
              value={f.channel}
              disabled={local}
              onChange={(e) => patch({ channel: slug(e.target.value) })}
            />
          </Field>
          <div className="smp-member-chips">
            {f.organizations.map((o) => (
              <span key={o.key}>{o.name}</span>
            ))}
          </div>
          <small className="smp-subcopy">
            All configured organizations and ordering nodes participate.
          </small>
        </Card>
        <Card title="Chaincode" icon={Code2}>
          <div className="smp-contract">
            <Code2 size={24} />
            <div>
              <strong>
                {local
                  ? d.provider === "kubernetes"
                    ? "smp_smoke"
                    : "sathonate_smoke"
                  : "Initial chaincode not configured"}
              </strong>
              <p>
                {local
                  ? "Go · v1.0 · Install, approve, commit & verify"
                  : "Custom chaincode lifecycle will be integrated separately."}
              </p>
            </div>
            <Status tone={local ? "blue" : "neutral"}>
              {local ? "Included" : "Optional"}
            </Status>
          </div>
        </Card>
      </div>
      {local && (
        <Note>
          The local execution profile is fixed to the topology shown. Choose a
          cloud target to prepare a custom design; custom topology deployment is
          not enabled yet.
        </Note>
      )}
      <div className="smp-config-test">
        <button className="smp-btn secondary" onClick={onValidate}>
          <Settings2 size={16} />
          Test Configuration
        </button>
        <span>
          {validation || "The backend validates the topology before review."}
        </span>
      </div>
    </>
  );
}
export function TopologyDiagram({ draft: d }: { draft: FlowDraft }) {
  return (
    <div className="smp-topology">
      <div className="smp-topology-orgs">
        {d.form.organizations.map((o, i) => (
          <div className={`smp-topology-node c${i % 3}`} key={o.key}>
            <Building2 size={24} />
            <div>
              <strong>{o.name}</strong>
              <span>
                {o.peers} peer{o.peers === 1 ? "" : "s"} | 1 CA
              </span>
            </div>
          </div>
        ))}
      </div>
      <div className="smp-topology-lines" />
      <div className="smp-topology-bottom">
        <div className="smp-topology-node ordering">
          <Database size={24} />
          <div>
            <strong>Ordering Service</strong>
            <span>{d.form.orderers} orderers (Raft)</span>
          </div>
        </div>
        <span className="smp-topology-dash" />
        <div className="smp-topology-node channel">
          <GitBranch size={20} />
          <div>
            <strong>{d.form.channel || "Channel"}</strong>
            <span>All organizations</span>
          </div>
        </div>
      </div>
    </div>
  );
}
export function Review({
  draft: d,
  onEdit,
  issues,
  valid,
}: {
  draft: FlowDraft;
  onEdit: (step: number) => void;
  issues: Issue[];
  valid: boolean;
}) {
  const local = ["local", "kubernetes"].includes(d.provider),
    f = d.form;
  return (
    <>
      <Card title="Configuration Summary">
        <div className="smp-review-sections">
          {[
            {
              title: "Network Details",
              icon: Building2,
              step: 2,
              items: [
                ["Network Name", f.name],
                [
                  "Domain",
                  local ? "Local container DNS" : d.domain || "Not specified",
                ],
                ["Environment", f.environment],
              ],
            },
            {
              title: "Infrastructure",
              icon: Cloud,
              step: 1,
              items: [
                [
                  "Cloud Provider",
                  local
                    ? d.provider === "kubernetes"
                      ? "Local Kubernetes"
                      : "Local Docker"
                    : d.provider.toUpperCase(),
                ],
                [
                  "Region",
                  local ? "This computer" : d.region || "Not selected",
                ],
                [
                  "Kubernetes Cluster",
                  local
                    ? "Not required"
                    : `${d.clusterName || "Not selected"} (${d.clusterMode})`,
                ],
              ],
            },
            {
              title: "Network Design",
              icon: Boxes,
              step: 2,
              items: [
                ["Organizations", String(f.organizations.length)],
                [
                  "Peers",
                  String(f.organizations.reduce((n, o) => n + o.peers, 0)),
                ],
                ["Orderers", `${f.orderers} · Raft`],
                [
                  "Certificate Authorities",
                  String(
                    f.organizations.length + (d.provider === "local" ? 1 : 0),
                  ),
                ],
                ["Channels", f.channel],
              ],
            },
            {
              title: "Chaincode",
              icon: Code2,
              step: 2,
              items: [
                [
                  "Initial Chaincode",
                  local
                    ? d.provider === "kubernetes"
                      ? "smp_smoke"
                      : "sathonate_smoke"
                    : "None",
                ],
                ["Language", local ? "Go" : "—"],
                ["Version", local ? "1.0" : "—"],
                ["Channel", f.channel],
              ],
            },
          ].map((section) => (
            <div className="smp-review-section" key={section.title}>
              <div className="smp-review-section-title">
                <span className="smp-tile">
                  <section.icon size={20} />
                </span>
                <strong>{section.title}</strong>
                <button
                  className="smp-text-btn"
                  onClick={() => onEdit(section.step)}
                >
                  <Pencil size={14} />
                  Edit
                </button>
              </div>
              <dl>
                {section.items.map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            </div>
          ))}
        </div>
      </Card>
      <Card title="Infrastructure Resources">
        <div className="smp-resource-counts">
          {[
            {
              icon: Layers3,
              n: local ? "—" : "1",
              label:
                d.provider === "kubernetes"
                  ? "Local kind cluster"
                  : local
                    ? "No Kubernetes"
                    : "Planned cluster",
            },
            {
              icon: Building2,
              n: String(f.organizations.length),
              label: "Organizations",
            },
            {
              icon: Server,
              n: String(f.organizations.reduce((n, o) => n + o.peers, 0)),
              label: "Peer nodes",
            },
            { icon: Database, n: String(f.orderers), label: "Orderers" },
            {
              icon: ShieldCheck,
              n: String(
                f.organizations.length + (d.provider === "local" ? 1 : 0),
              ),
              label: "Fabric CAs",
            },
            { icon: GitBranch, n: "1", label: "Channel" },
          ].map((r, i) => (
            <div key={r.label} className={`c${i % 3}`}>
              <r.icon size={25} />
              <strong>{r.n}</strong>
              <span>{r.label}</span>
            </div>
          ))}
        </div>
      </Card>
      <Card title="Network Topology Preview">
        <TopologyDiagram draft={d} />
      </Card>
      {issues.length > 0 && (
        <Card title="Validation Findings" icon={TriangleAlert}>
          {issues.map((issue, i) => (
            <div className="smp-finding" key={i}>
              <Status tone={issue.severity === "error" ? "amber" : "neutral"}>
                {issue.severity}
              </Status>
              <span>{issue.message}</span>
            </div>
          ))}
        </Card>
      )}
      {!valid && (
        <Note tone="amber">
          Validate the configuration before creating or saving the network.
        </Note>
      )}
    </>
  );
}
