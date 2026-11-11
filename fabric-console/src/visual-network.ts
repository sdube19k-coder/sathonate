import type {
  DynamicCapabilities,
  DynamicResources,
  NetworkSpec,
} from "./types";

export const defaultResources = (): DynamicResources => ({
  cpuRequestMilli: 100,
  cpuLimitMilli: 1000,
  memoryRequestMiB: 128,
  memoryLimitMiB: 1024,
});
export function emptyVisualNetwork(
  capabilities?: DynamicCapabilities,
): NetworkSpec {
  return {
    apiVersion: "fabric.sathonate.io/v1alpha1",
    kind: "FabricNetwork",
    metadata: {
      id: `network-${crypto.randomUUID().slice(0, 12)}`,
      name: "Untitled network",
    },
    spec: {
      environment: "development",
      fabricVersion: "2.5.15",
      deployment: {
        engine: "bevel",
        runtime: "kubernetes",
        provider: "local",
        targetRef: "smp-dynamic-local-v1",
      },
      ordering: { consensus: "raft", channelCapability: "V2_0" },
      organizations: [],
      nodes: [],
      channels: [],
      configuration: {
        placements: Object.fromEntries(
          Object.entries(capabilities?.placements ?? {}).map(
            ([ref, nodeSelector]) => [
              ref,
              { nodeSelector: structuredClone(nodeSelector) },
            ],
          ),
        ),
        nodes: {},
        authorities: [],
        anchors: {},
        chaincodes: [],
      },
    },
  };
}
export function validateVisualID(id: string) {
  if (!/^[a-z][a-z0-9-]{0,62}$/.test(id))
    throw new Error(
      "Use a lowercase identifier beginning with a letter, up to 63 characters.",
    );
}
export function addVisualEntity(
  network: NetworkSpec,
  kind: "organization" | "peer" | "orderer" | "authority" | "channel",
  id: string,
  organization: string,
  placement: string,
  storageClass: string,
): NetworkSpec {
  validateVisualID(id);
  const next = structuredClone(network),
    s = next.spec,
    cfg = s.configuration!;
  if (
    [...s.organizations, ...s.nodes, ...cfg.authorities, ...s.channels].some(
      (x) => x.id === id,
    )
  )
    throw new Error(
      "Choose an identifier that is not already used in this design.",
    );
  if (kind === "organization")
    s.organizations.push({
      id,
      mspId: `${id.replaceAll("-", "")}MSP`,
      identityProviderRef: "",
    });
  else if (kind === "channel") {
    s.channels.push({
      id,
      organizationRefs: s.organizations.map((o) => o.id),
      peerRefs: s.nodes.filter((n) => n.role === "peer").map((n) => n.id),
      ordererRefs: s.nodes.filter((n) => n.role === "orderer").map((n) => n.id),
    });
    cfg.anchors[id] = s.organizations.flatMap(
      (o) =>
        s.nodes.find((n) => n.role === "peer" && n.organizationRef === o.id)
          ?.id ?? [],
    );
  } else {
    if (!s.organizations.some((o) => o.id === organization))
      throw new Error("Select a configured organization first.");
    if (!cfg.placements[placement])
      throw new Error("Select a configured approved placement first.");
    if (!storageClass)
      throw new Error("The server must provide an approved storage class.");
    const settings = {
      resources: defaultResources(),
      storage: { class: storageClass, capacityGiB: 1 },
    };
    if (kind === "authority") {
      cfg.authorities.push({
        id,
        organizationRef: organization,
        placementRef: placement,
        roles: ["signing", "tls"],
        ...settings,
      });
      const org = s.organizations.find((o) => o.id === organization)!;
      if (!org.identityProviderRef) org.identityProviderRef = id;
    } else {
      s.nodes.push({
        id,
        role: kind,
        organizationRef: organization,
        placementRef: placement,
        failureDomain: "local-host",
      });
      cfg.nodes[id] = settings;
    }
  }
  return next;
}

export function visualWarnings(n: NetworkSpec): string[] {
  const s = n.spec,
    c = s.configuration!,
    issues: string[] = [];
  if (!s.organizations.length) issues.push("Add at least one organization.");
  if (!s.nodes.some((n) => n.role === "peer"))
    issues.push("Add peers for transaction endorsement and ledger storage.");
  if (!s.nodes.some((n) => n.role === "orderer"))
    issues.push("Add the required Raft orderers.");
  if (!s.channels.length) issues.push("Add a channel and select its members.");
  for (const o of s.organizations)
    if (
      !c.authorities.some(
        (a) =>
          a.id === o.identityProviderRef &&
          a.organizationRef === o.id &&
          a.roles.includes("signing"),
      )
    )
      issues.push(`${o.id}: select its signing authority.`);
  for (const channel of s.channels) {
    if (!channel.peerRefs.length || !channel.ordererRefs.length)
      issues.push(`${channel.id}: select peers and orderers.`);
    for (const id of channel.peerRefs)
      if (
        !s.nodes.some(
          (n) =>
            n.id === id &&
            n.role === "peer" &&
            channel.organizationRefs.includes(n.organizationRef),
        )
      )
        issues.push(
          `${channel.id}: peer ${id} must belong to a member organization.`,
        );
  }
  return issues;
}
