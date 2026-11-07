import { workspaceKey, type Project } from "./workspace";
import type { Issue, Meta, Network, NetworkSpec, Plan } from "./types";

let tokenProvider: (() => string | undefined) | null = null;
export function setTokenProvider(provider: (() => string | undefined) | null) {
  tokenProvider = provider;
}

export class APIError extends Error {
  constructor(
    message: string,
    public code: string,
    public issues: Issue[] = [],
    public status: number = 0,
  ) {
    super(message);
  }
}

export async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`/api/v1${path}`, {
      ...options,
      headers: {
        "Content-Type": "application/json",
        ...(tokenProvider?.()
          ? { Authorization: `Bearer ${tokenProvider()}` }
          : {}),
        ...options.headers,
      },
    });
  } catch {
    throw new APIError(
      "Cannot reach the control plane. Check that the Go API is running.",
      "connection",
    );
  }
  if (response.status === 204) return undefined as T;
  const body = await response.json().catch(() => null);
  if (response.status === 401)
    window.dispatchEvent(new Event("local-auth-expired"));
  if (!response.ok)
    throw new APIError(
      body?.error?.message ??
        (response.status === 401
          ? "Invalid or expired token."
          : response.status === 403
            ? "Your grant does not permit this operation."
            : "The request could not be completed."),
      body?.error?.code ?? "request",
      body?.error?.issues ?? [],
      response.status,
    );
  return body as T;
}

function intentKey(action: string) {
  const key = workspaceKey() + ":intent:" + action;
  let value = sessionStorage.getItem(key);
  if (!value) {
    value = crypto.randomUUID();
    sessionStorage.setItem(key, value);
  }
  return value;
}

export const api = {
  deletionPreview: (id: string) =>
    request<DeletionPreview>(
      `/deployments/${encodeURIComponent(id)}/cleanup-preview`,
    ),
  deleteRetainingData: (id: string, digest: string) =>
    request<{ removed: boolean }>(`/deployments/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers: { "X-Sathonate-Cleanup-Approval": digest },
    }),
  nodeRecoveryPreview: (id: string, node: string) =>
    request<RecoveryPreview>(
      `/deployments/${encodeURIComponent(id)}/recovery/preview?node=${encodeURIComponent(node)}`,
    ),
  applyNodeRecovery: (id: string, node: string, approval: string) =>
    request<Operation>(
      `/deployments/${encodeURIComponent(id)}/recovery/deploy`,
      {
        method: "POST",
        headers: {
          "Idempotency-Key": intentKey(`recovery:${id}:${node}:${approval}`),
        },
        body: JSON.stringify({ node, approval }),
      },
    ),
  previewDynamic: (network: Network, parent: string) =>
    request<DynamicPreview>(
      `/networks/${encodeURIComponent(network.id)}/dynamic/preview?parent=${encodeURIComponent(parent)}`,
      { headers: { "If-Match": `"${network.revision}"` } },
    ),
  applyDynamic: (network: Network, parent: string, approval: string) =>
    request<Operation>(
      `/networks/${encodeURIComponent(network.id)}/dynamic/deploy`,
      {
        method: "POST",
        headers: {
          "If-Match": `"${network.revision}"`,
          "Idempotency-Key": intentKey(
            `dynamic:${network.id}:${parent}:${approval}`,
          ),
        },
        body: JSON.stringify({ parentOperationId: parent, approval }),
      },
    ),
  projects: () => request<{ items: Project[] }>("/projects"),
  verifiedIdentity: () =>
    request<{ issuer: string; subject: string }>("/auth/identity"),
  readiness: () => request<LaunchReadiness>("/readiness"),
  probeReadiness: () =>
    request<LaunchReadiness>("/readiness/probe", {
      method: "POST",
      body: "{}",
    }),
  deploy: (network: Network) =>
    request<Operation>(`/networks/${network.id}/deploy`, {
      method: "POST",
      headers: {
        "If-Match": `"${network.revision}"`,
        "Idempotency-Key": intentKey(
          `deploy:${network.id}:${network.revision}`,
        ),
      },
      body: "{}",
    }),
  deployment: (id: string) =>
    request<DeploymentState>(`/deployments/${encodeURIComponent(id)}`),
  workloads: (id: string, signal?: AbortSignal) =>
    request<WorkloadSnapshot>(
      `/deployments/${encodeURIComponent(id)}/workloads`,
      {
        signal,
      },
    ),
  lifecyclePreview: (id: string, action: LifecycleAction) =>
    request<ChangePreview>(
      `/deployments/${encodeURIComponent(id)}/lifecycle/${action}/preview`,
    ),
  lifecycleApply: (id: string, action: LifecycleAction, approval: string) =>
    request<Operation>(
      `/deployments/${encodeURIComponent(id)}/lifecycle/${action}/deploy`,
      {
        method: "POST",
        headers: {
          "Idempotency-Key": intentKey(`change:${id}:${action}:${approval}`),
        },
        body: JSON.stringify({ approval }),
      },
    ),
  deploymentHealth: (id: string) =>
    request<RuntimeHealth>(`/deployments/${encodeURIComponent(id)}/health`),
  deploymentLogs: (id: string, node: string) =>
    request<{ logs: string }>(
      `/deployments/${encodeURIComponent(id)}/logs/${encodeURIComponent(node)}`,
    ),
  deploymentCleanup: (id: string) =>
    request<{ removed: boolean }>(`/deployments/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  deploymentPackage: (network: Network) =>
    request<DeploymentPackage>(`/networks/${network.id}/deployment-package`, {
      method: "POST",
      headers: { "If-Match": `"${network.revision}"` },
      body: "{}",
    }),
  operations: () => request<{ items: Operation[] }>("/operations"),
  operation: (id: string) =>
    request<Operation>(`/operations/${encodeURIComponent(id)}`),
  operationDiagnostics: (id: string) =>
    request<OperationDiagnostics>(
      `/operations/${encodeURIComponent(id)}/diagnostics`,
    ),
  retryOperation: (id: string) =>
    request<Operation>(`/operations/${encodeURIComponent(id)}/retry`, {
      method: "POST",
      headers: { "Idempotency-Key": intentKey(`retry:${id}`) },
      body: "{}",
    }),
  localHealth: () => request<RuntimeHealth>("/local-runtime/health"),
  localTest: () =>
    request<Operation>("/local-runtime/test", {
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
      body: "{}",
    }),
  localStatus: () => request<LocalState>("/local-runtime"),
  localStart: () =>
    request<Operation>("/local-runtime/start", {
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
      body: "{}",
    }),
  localStop: () =>
    request<Operation>("/local-runtime/stop", {
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
      body: "{}",
    }),
  localLogs: (node: string) =>
    request<{ logs: string }>(
      `/local-runtime/logs/${encodeURIComponent(node)}`,
    ),
  cloudTargets: () =>
    request<{ items: CloudTarget[]; executionEnabled: boolean }>(
      "/cloud/targets",
    ),
  cloudPreflight: (network: Network) =>
    request<CloudReport>(`/networks/${network.id}/cloud-preflight`, {
      method: "POST",
      headers: { "If-Match": `"${network.revision}"` },
      body: "{}",
    }),
  cloudBundle: (network: Network) =>
    request<{
      executable: false;
      bundle: unknown;
      kubernetesManifests: unknown;
      notice: string;
    }>(`/networks/${network.id}/cloud-bundle`, {
      method: "POST",
      headers: { "If-Match": `"${network.revision}"` },
      body: "{}",
    }),
  meta: () => request<Meta>("/meta"),
  networks: () => request<{ items: Network[] }>("/networks"),
  create: (spec: NetworkSpec) =>
    request<Network>("/networks", {
      method: "POST",
      body: JSON.stringify(spec),
    }),
  update: (network: Network, spec: NetworkSpec) =>
    request<Network>(`/networks/${network.id}`, {
      method: "PUT",
      headers: { "If-Match": `"${network.revision}"` },
      body: JSON.stringify(spec),
    }),
  review: (network: Network) =>
    request<Network>(`/networks/${network.id}/review`, {
      method: "POST",
      headers: { "If-Match": `"${network.revision}"` },
      body: "{}",
    }),
  archive: (network: Network) =>
    request<void>(`/networks/${network.id}`, {
      method: "DELETE",
      headers: { "If-Match": `"${network.revision}"` },
    }),
  validate: (spec: NetworkSpec) =>
    request<{
      valid: boolean;
      issues: Issue[];
      deploymentProfile?: { id: string; executionEnabled: boolean };
    }>("/validate", {
      method: "POST",
      body: JSON.stringify(spec),
    }),
  plan: (spec: NetworkSpec) =>
    request<Plan>("/plans", { method: "POST", body: JSON.stringify(spec) }),
};

export type OperationDiagnostics = {
  schema: "sathonate.operation-diagnostics/v1";
  generatedAt: string;
  evidenceType: "recorded-operation";
  liveHealthChecked: false;
  operationId: string;
  networkId: string;
  status: string;
  createdAt: string;
  updatedAt: string;
  nextStep: string;
  eventCount: number;
  stages: { status: string; startedAt?: string; finishedAt?: string }[];
};

export type DeploymentPackage = {
  apiVersion: "deployment.sathonate.io/v1alpha1";
  revision: number;
  network: NetworkSpec;
  networkSHA256: string;
  executable: boolean;
  checks: { id: string; status: string; message: string }[];
  bevelBundle?: unknown;
  kubernetesManifests?: unknown;
};
export type DeploymentState = {
  verifications?: DynamicVerification[];
  lifecycle?: { peerAdded: boolean; sequence: number };
  lifecycleAction?: string;
  mode: string;
  simulated: boolean;
  network: NetworkSpec;
  revision: number;
  operationId: string;
  phase: string;
  ready: boolean;
  verified: boolean;
  checkedAt: string;
  transactionId?: string;
  notice: string;
  health?: RuntimeHealth;
  resources: {
    name: string;
    kind: string;
    status: string;
    organization?: string;
  }[];
};

export type DynamicPreview = {
  parentOperationId: string;
  digest: string;
  revision: number;
  before: NetworkSpec;
  after: NetworkSpec;
  recovery: string;
};
export type DynamicVerification = {
  health: RuntimeHealth;
  transactionId: string;
  packageId: string;
  artifactSHA256: string;
  codeVersion: string;
};

export type LifecycleAction =
  "add-peer" | "upgrade-chaincode" | "verify-network" | "restart-peer";
export type ChangePreview = {
  parentOperationId: string;
  action: LifecycleAction;
  digest: string;
  before: { peerAdded: boolean; sequence: number };
  after: { peerAdded: boolean; sequence: number };
  impact: string[];
  recovery: string;
  liveHealthChecked: boolean;
};

export type LocalState = {
  operationId?: string;
  transactionId?: string;
  verifiedAt?: string;
  phase: string;
  error?: string;
  ready: boolean;
  verified: boolean;
  channel: string;
  version: string;
  dockerAvailable: boolean;
  events: { time: string; message: string }[];
  containers: { name: string; state: string; status: string; ports: string }[];
};

export type Operation = {
  operationId: string;
  networkId: string;
  kind: string;
  status: string;
  currentStage?: string;
  profileDigest: string;
  runtimeVersion: string;
  retryOf?: string;
  createdAt: string;
  updatedAt: string;
  finishedAt?: string;
  error?: string;
  stages: {
    key: string;
    label: string;
    status: string;
    startedAt?: string;
    finishedAt?: string;
  }[];
  events: { sequence: number; time: string; stage?: string; message: string }[];
};
export type RuntimeHealth = {
  checkedAt: string;
  ready: boolean;
  channel: string;
  alerts: string[];
  peers: {
    peer: string;
    mspId: string;
    available: boolean;
    height: number;
    currentBlockHash: string;
    error?: string;
  }[];
  orderers: {
    name: string;
    status: string;
    consensusRelation: string;
    height: number;
  }[];
  certificateAuthorities: { name: string; available: boolean; tls: boolean }[];
  chaincode: {
    name: string;
    available: boolean;
    version?: string;
    sequence?: number;
  };
};

export type CloudTarget = {
  id: string;
  name: string;
  region: string;
  clusterName: string;
  provider: string;
  executionEnabled: boolean;
};
export type CloudReport = {
  targetId: string;
  checkedAt: string;
  ready: boolean;
  checks: { id: string; status: "passed" | "blocked"; message: string }[];
};

export type LaunchReadiness = {
  checkedAt: string;
  ready: boolean;
  executionEnabled: boolean;
  probed: boolean;
  checks: {
    id: string;
    group: string;
    title: string;
    status: "missing" | "configured" | "passed" | "blocked";
    detail: string;
    required?: string;
  }[];
};

export interface WorkloadSnapshot {
  components: { name: string; kind: string; ready: boolean }[];
  alerts: { code: string; component: string; action: string }[];
  operationId: string;
  revision: number;
  checkedAt: string;
  readOnly: boolean;
  ledgerChecked: boolean;
  workloadsReady: boolean;
  pods: {
    name: string;
    component: string;
    kind: string;
    phase: string;
    ready: boolean;
    restarts: number;
    waitingReason?: string;
    lastExitReason?: string;
  }[];
  volumes: { name: string; phase: string }[];
  missing: string[];
  guidance: string[];
}
export interface RecoveryPreview {
  parentOperationId: string;
  plan: { node: string; kind: string; digest: string };
  impact: string;
  recovery: string;
}

export interface DeletionPreview {
  schema: string;
  digest: string;
  notice: string;
  delete: { resource: string; name: string; uid: string }[];
  retainedObjects: { resource: string; name: string; uid: string }[];
}
