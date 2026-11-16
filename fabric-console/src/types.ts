export type Engine = "bevel" | "native";
export type Consensus = "raft" | "bft";
export type Issue = {
  severity: "error" | "warning";
  path: string;
  message: string;
};
export type Organization = {
  id: string;
  mspId: string;
  identityProviderRef: string;
};
export type FabricNode = {
  id: string;
  organizationRef: string;
  role: "peer" | "orderer";
  placementRef: string;
  failureDomain: string;
};
export type Channel = {
  id: string;
  organizationRefs: string[];
  ordererRefs: string[];
  peerRefs: string[];
};
export type NetworkSpec = {
  apiVersion: "fabric.sathonate.io/v1alpha1";
  kind: "FabricNetwork";
  metadata: { id: string; name: string };
  spec: {
    configuration?: DynamicConfiguration;
    environment: "development" | "production";
    fabricVersion: string;
    ordering?: { consensus: Consensus; channelCapability: "V2_0" | "V3_0" };
    deployment: {
      engine: Engine;
      targetRef: string;
      runtime: string;
      provider?: "aws" | "azure" | "local";
      cloud?: CloudIntent;
    };
    organizations: Organization[];
    nodes: FabricNode[];
    channels: Channel[];
  };
};
export type CloudIntent = {
  accountId: string;
  roleArn: string;
  region: string;
  tenantId: string;
  subscriptionId: string;
  resourceGroup: string;
  domain: string;
  clusterMode: "new" | "existing";
  clusterName: string;
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
};
export type Plan = {
  id: string;
  executable: false;
  operation: string;
  steps: {
    id: string;
    action: string;
    verification: string;
    dependsOn: string[];
  }[];
  warnings: Issue[];
};
export type Activity = {
  id: string;
  kind: string;
  message: string;
  at: string;
  revision: number;
};
export type Network = {
  id: string;
  revision: number;
  status: "draft" | "reviewed";
  desired: NetworkSpec;
  issues: Issue[];
  plan?: Plan;
  events: Activity[];
  createdAt: string;
  updatedAt: string;
  archived: boolean;
};
export type Meta = {
  customerPilot?: boolean;
  dynamicLocal?: DynamicCapabilities;
  deploymentMode?:
    | "disabled"
    | "mock"
    | "local-kubernetes"
    | "secure-local"
    | "secure-eks"
    | "history-only";
  name: string;
  mode: string;
  executionEnabled: boolean;
  schemaVersion: string;
  storage: string;
};

export type DynamicResources = {
  cpuRequestMilli: number;
  cpuLimitMilli: number;
  memoryRequestMiB: number;
  memoryLimitMiB: number;
};
export type DynamicStorage = { class: string; capacityGiB: number };
export type DynamicConfiguration = {
  placements: Record<string, { nodeSelector: Record<string, string> }>;
  nodes: Record<
    string,
    { resources: DynamicResources; storage: DynamicStorage }
  >;
  authorities: {
    id: string;
    organizationRef: string;
    roles: ("signing" | "tls")[];
    placementRef: string;
    resources: DynamicResources;
    storage: DynamicStorage;
  }[];
  anchors: Record<string, string[]>;
  chaincodes: {
    name: string;
    channelRef: string;
    version: string;
    sequence: number;
    packageSHA256: string;
    runtimeImage: string;
    endorsement: { mspIds: string[]; minimum: number };
  }[];
};
export type DynamicCapabilities = {
  enabled: boolean;
  developmentOnly?: boolean;
  productionEnabled?: boolean;
  artifacts?: Record<string, { sha256: string; codeVersion: string }>;
  placements?: Record<string, Record<string, string>>;
  storageClass?: string;
  maximumVolumeSize?: string;
};
