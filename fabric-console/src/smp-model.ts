import { workspaceKey, secureWorkspace } from "./workspace";
import {
  newForm,
  withOrdererCount,
  toSpec,
  type NetworkForm,
} from "./network-form";

export function deploymentSpec(d: FlowDraft) {
  const spec = toSpec(d.form);
  if (d.provider === "local") return spec;
  if (d.provider === "kubernetes") {
    spec.spec.deployment = {
      engine: "bevel",
      runtime: "kubernetes",
      provider: "local",
      targetRef: d.secureProfile ? "smp-secure-local-v1" : "smp-qualification",
    };
    if (d.secureProfile)
      spec.spec.nodes = spec.spec.nodes.map((node) => ({
        ...node,
        placementRef: "kind-node",
        failureDomain: "local-host",
      }));
    return spec;
  }
  spec.spec.deployment.targetRef = d.targetRef || d.form.target;
  spec.spec.deployment.cloud = {
    accountId: d.accountId,
    roleArn: d.roleArn,
    region: d.region,
    tenantId: d.tenantId,
    subscriptionId: d.subscriptionId,
    resourceGroup: d.resourceGroup,
    domain: d.domain,
    clusterMode: d.clusterMode,
    clusterName: d.clusterName,
    instanceType: d.instanceType,
    kubeVersion: d.kubeVersion,
    minNodes: d.minNodes,
    desiredNodes: d.desiredNodes,
    maxNodes: d.maxNodes,
    vpcMode: d.vpcMode,
    vpcCidr: d.vpcCidr,
    serviceCidr: d.serviceCidr,
    podCidr: d.podCidr,
    addons: [...d.addons],
  };
  return spec;
}

export const flowSteps = [
  { title: "Cloud Account", detail: "Connect your deployment target" },
  {
    title: "Kubernetes Configuration",
    detail: "Create or select your cluster",
  },
  { title: "Network Design", detail: "Organizations, peers & orderers" },
  { title: "Review & Cost Estimate", detail: "Review and validate" },
  { title: "Deployment Progress", detail: "Live stages and logs" },
  { title: "Network Dashboard", detail: "Manage your network" },
];
export type Provider = "aws" | "azure" | "local" | "kubernetes";
export type FlowDraft = {
  secureProfile?: boolean;
  recordId?: string;
  deploymentOperationId?: string;
  provider: Provider;
  accountId: string;
  roleArn: string;
  region: string;
  tenantId: string;
  subscriptionId: string;
  resourceGroup: string;
  clusterMode: "new" | "existing";
  clusterName: string;
  targetRef: string;
  instanceType: string;
  kubeVersion: string;
  minNodes: number;
  desiredNodes: number;
  maxNodes: number;
  vpcMode: "new" | "existing";
  vpcCidr: string;
  serviceCidr: string;
  podCidr: string;
  addons: string[];
  domain: string;
  form: NetworkForm;
};
export const addons = [
  "Load Balancer Controller",
  "Persistent Volume CSI Driver",
  "Metrics Server",
  "Cluster Autoscaler",
  "Prometheus & Grafana",
  "External DNS",
];
export function initialDraft(): FlowDraft {
  return {
    provider: "aws",
    accountId: "",
    roleArn: "",
    region: "",
    tenantId: "",
    subscriptionId: "",
    resourceGroup: "",
    clusterMode: "existing",
    clusterName: "",
    targetRef: "",
    instanceType: "",
    kubeVersion: "",
    minNodes: 2,
    desiredNodes: 3,
    maxNodes: 5,
    vpcMode: "new",
    vpcCidr: "",
    serviceCidr: "",
    podCidr: "",
    addons: [],
    domain: "",
    form: withOrdererCount(
      {
        ...newForm(),
        ordererNodes: [],
        destination: "aws",
        target: "aws-workspace",
        organizations: [
          { key: "org1", name: "Org1", peers: 1 },
          { key: "org2", name: "Org2", peers: 1 },
        ],
      },
      3,
    ),
  };
}
export function selectProvider(d: FlowDraft, p: Provider): FlowDraft {
  if (p === "kubernetes") {
    const base = selectProvider(d, "local");
    return {
      ...base,
      provider: p,
      deploymentOperationId: undefined,
      targetRef: "smp-qualification",
      clusterName: "kind-smp-qualification",
      form: {
        ...base.form,
        name: "Sathonate Kubernetes network",
        ordererNodes: base.form.ordererNodes.map((n) => ({
          ...n,
          organizationKey: "org1",
          placementRef: "kind-node",
          failureDomain: "local-host",
        })),
      },
    };
  }
  if (p === "local") {
    const form = withOrdererCount(
      {
        ...newForm(),
        ordererNodes: [],
        name: "Sathonate development network",
        version: "2.5.15",
        destination: "private",
        target: "sathonate-local",
        channel: "sathonate",
        organizations: [
          { key: "org1", name: "Org1", peers: 1 },
          { key: "org2", name: "Org2", peers: 1 },
        ],
      },
      3,
    );
    return {
      ...d,
      recordId: undefined,
      deploymentOperationId: undefined,
      provider: p,
      form,
      targetRef: "sathonate-local",
      clusterMode: "existing",
      clusterName: "Local Docker Engine",
    };
  }
  return {
    ...d,
    recordId: undefined,
    deploymentOperationId: undefined,
    provider: p,
    targetRef: "",
    clusterName: "",
    form: { ...newForm(), destination: p, target: `${p}-workspace` },
  };
}
export function readDraft(): FlowDraft {
  try {
    const raw = sessionStorage.getItem(workspaceKey());
    if (raw) {
      const d = JSON.parse(raw) as FlowDraft;
      if (
        ["aws", "azure", "local", "kubernetes"].includes(d.provider) &&
        Array.isArray(d.form?.organizations) &&
        Array.isArray(d.addons) &&
        d.form.organizations.length > 0
      ) {
        d.form.ordererNodes = d.form.ordererNodes.map((node, i) =>
          d.form.organizations.some((o) => o.key === node.organizationKey)
            ? node
            : {
                ...node,
                organizationKey:
                  d.form.organizations[i % d.form.organizations.length].key,
              },
        );
        if (
          secureWorkspace() &&
          (!d.secureProfile || d.provider !== "kubernetes")
        )
          return secureDraft();
        return d;
      }
    }
  } catch {
    /* A malformed tab draft must not prevent opening the app. */
  }
  return secureWorkspace() ? secureDraft() : initialDraft();
}
export function secureDraft(): FlowDraft {
  const d = selectProvider(initialDraft(), "kubernetes");
  return {
    ...d,
    secureProfile: true,
    targetRef: "smp-secure-local-v1",
    clusterName: "Server-bound disposable kind",
    form: {
      ...d.form,
      name: "Secure Fabric network",
      channel: "smpchannel",
      version: "2.5.15",
      environment: "development",
      consensus: "raft",
    },
  };
}
export function cidrError(d: FlowDraft): string | null {
  if (d.provider === "local" || d.clusterMode === "existing") return null;
  if (!(
    d.minNodes > 0 &&
    d.minNodes <= d.desiredNodes &&
    d.desiredNodes <= d.maxNodes
  ))
    return "Node counts must satisfy minimum ≤ desired ≤ maximum.";
  const ranges: [string, number, number][] = [];
  for (const [label, value] of [
    ["VPC", d.vpcCidr],
    ["Service", d.serviceCidr],
    ["Pod", d.podCidr],
  ]) {
    if (!value) continue;
    const match =
      /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})$/.exec(value);
    if (!match) return `${label} CIDR must be a valid IPv4 CIDR.`;
    const parts = match.slice(1, 5).map(Number),
      prefix = Number(match[5]);
    if (parts.some((n) => n > 255) || prefix < 8 || prefix > 30)
      return `${label} CIDR must use valid IPv4 octets and a /8–/30 prefix.`;
    const ip = parts.reduce((n, x) => n * 256 + x, 0);
    const size = 2 ** (32 - prefix);
    const start = Math.floor(ip / size) * size;
    ranges.push([label, start, start + size - 1]);
  }
  for (let i = 0; i < ranges.length; i++)
    for (let j = i + 1; j < ranges.length; j++)
      if (ranges[i][1] <= ranges[j][2] && ranges[j][1] <= ranges[i][2])
        return `${ranges[i][0]} and ${ranges[j][0]} CIDRs overlap.`;
  return null;
}

export function sameDesign(a: unknown, b: unknown): boolean {
  const canonical = (value: unknown): unknown =>
    Array.isArray(value)
      ? value.map(canonical)
      : value && typeof value === "object"
        ? Object.fromEntries(
            Object.entries(value)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([key, item]) => [key, canonical(item)]),
          )
        : value;
  return JSON.stringify(canonical(a)) === JSON.stringify(canonical(b));
}
