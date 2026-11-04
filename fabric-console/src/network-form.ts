import type { Consensus, NetworkSpec } from "./types";

export type OrgForm = { key: string; name: string; peers: number };
export type OrdererForm = {
  id: string;
  organizationKey: string;
  placementRef: string;
  failureDomain: string;
};
export type NetworkForm = {
  name: string;
  networkId: string;
  environment: "development" | "production";
  destination: "aws" | "azure" | "private";
  target: string;
  version: string;
  organizations: OrgForm[];
  orderers: number;
  consensus: Consensus;
  ordererNodes: OrdererForm[];
  channel: string;
  original?: NetworkSpec;
};

export const slug = (text: string) =>
  text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 45);
export function newForm(template = false): NetworkForm {
  const form: NetworkForm = {
    name: template ? "Supply chain sandbox" : "",
    networkId: "",
    environment: "development",
    destination: "private",
    target: "private-lab",
    version: "2.5.16",
    organizations: [
      {
        key: crypto.randomUUID(),
        name: template ? "Manufacturer" : "Organization 1",
        peers: 1,
      },
      {
        key: crypto.randomUUID(),
        name: template ? "Retailer" : "Organization 2",
        peers: 1,
      },
    ],
    orderers: 3,
    consensus: "raft",
    ordererNodes: [],
    channel: "main-channel",
  };
  return withOrdererCount(form, 3);
}

export function withOrdererCount(
  form: NetworkForm,
  count: number,
): NetworkForm {
  const ordererNodes = form.ordererNodes.slice(0, count);
  while (ordererNodes.length < count) {
    let index = ordererNodes.length + 1;
    while (ordererNodes.some((node) => node.id === `orderer-${index}`)) index++;
    ordererNodes.push({
      id: `orderer-${index}`,
      organizationKey:
        form.organizations[ordererNodes.length % form.organizations.length]
          ?.key ?? "",
      placementRef: `orderer-placement-${index}`,
      failureDomain: `orderer-domain-${index}`,
    });
  }
  return { ...form, orderers: count, ordererNodes };
}

export function toSpec(form: NetworkForm): NetworkSpec {
  const organizations = form.organizations.map((org) => {
    const id = slug(org.name);
    return (
      form.original?.spec.organizations.find(
        (existing) => existing.id === id,
      ) ?? {
        id,
        mspId: `${id
          .split("-")
          .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
          .join("")}MSP`,
        identityProviderRef: `${id}-ca`,
      }
    );
  });
  const nodes: NetworkSpec["spec"]["nodes"] = [];
  organizations.forEach((org, i) => {
    const previous =
      form.original?.spec.nodes.filter(
        (node) => node.organizationRef === org.id && node.role === "peer",
      ) ?? [];
    for (let n = 0; n < form.organizations[i].peers; n++)
      nodes.push(
        previous[n] ?? {
          id: `${org.id}-peer-${n + 1}`,
          organizationRef: org.id,
          role: "peer",
          placementRef: `placement-${i + 1}`,
          failureDomain: `domain-${i + 1}`,
        },
      );
  });
  for (const orderer of form.ordererNodes)
    nodes.push({
      id: orderer.id,
      organizationRef: slug(
        form.organizations.find((org) => org.key === orderer.organizationKey)
          ?.name ?? "",
      ),
      role: "orderer",
      placementRef: orderer.placementRef,
      failureDomain: orderer.failureDomain,
    });
  return {
    apiVersion: "fabric.sathonate.io/v1alpha1",
    kind: "FabricNetwork",
    metadata: { id: form.networkId || slug(form.name), name: form.name.trim() },
    spec: {
      environment: form.environment,
      fabricVersion: form.version,
      ordering: {
        consensus: form.consensus,
        channelCapability: form.version.startsWith("3.") ? "V3_0" : "V2_0",
      },
      deployment: {
        engine: form.destination === "private" ? "native" : "bevel",
        targetRef: slug(form.target),
        runtime:
          form.destination === "private" ? "linux-containers" : "kubernetes",
        ...(form.destination !== "private"
          ? { provider: form.destination }
          : {}),
      },
      organizations,
      nodes,
      channels: [
        {
          id: slug(form.channel),
          organizationRefs: organizations.map((org) => org.id),
          ordererRefs: nodes
            .filter((node) => node.role === "orderer")
            .map((node) => node.id),
          peerRefs: nodes
            .filter((node) => node.role === "peer")
            .map((node) => node.id),
        },
        ...(form.original?.spec.channels.slice(1) ?? []),
      ],
    },
  };
}

export function fromNetwork(spec: NetworkSpec): NetworkForm {
  return {
    original: spec,
    name: spec.metadata.name,
    networkId: spec.metadata.id,
    environment: spec.spec.environment,
    destination:
      spec.spec.deployment.engine === "native"
        ? "private"
        : spec.spec.deployment.provider === "local"
          ? "private"
          : (spec.spec.deployment.provider ?? "aws"),
    target: spec.spec.deployment.targetRef,
    version: spec.spec.fabricVersion,
    organizations: spec.spec.organizations.map((org) => ({
      key: org.id,
      name: org.id,
      peers: spec.spec.nodes.filter(
        (node) => node.organizationRef === org.id && node.role === "peer",
      ).length,
    })),
    orderers: spec.spec.nodes.filter((node) => node.role === "orderer").length,
    consensus: spec.spec.ordering?.consensus ?? "raft",
    ordererNodes: spec.spec.nodes
      .filter((node) => node.role === "orderer")
      .map((node) => ({
        id: node.id,
        organizationKey: node.organizationRef,
        placementRef: node.placementRef,
        failureDomain: node.failureDomain,
      })),
    channel: spec.spec.channels[0]?.id ?? "main-channel",
  };
}

export function stepError(form: NetworkForm, step: number): string | null {
  if (step === 0 && !form.name.trim())
    return "Give your network a name to continue.";
  if (step === 0 && !/^[a-z]/.test(slug(form.name)))
    return "Start the network name with a letter.";
  if (step === 1 && !/^[a-z][a-z0-9-]{0,62}$/.test(slug(form.target)))
    return "Enter a target reference starting with a letter.";
  if (step === 2) {
    const ids = form.organizations.map((org) => slug(org.name));
    if (ids.some((id) => !/^[a-z][a-z0-9-]{0,44}$/.test(id)))
      return "Organization names must start with a letter and contain a name.";
    if (new Set(ids).size !== ids.length)
      return "Each organization needs a different name.";
    if (!slug(form.channel) || !/^[a-z]/.test(slug(form.channel)))
      return "Give the channel a name starting with a letter.";
  }
  if (step === 3) {
    if (
      form.consensus === "bft" &&
      (form.destination !== "private" || !form.version.startsWith("3."))
    )
      return "BFT requires native private-server planning with Fabric 3.x.";
    if (
      form.consensus === "bft" &&
      (form.orderers < 4 || (form.orderers - 1) % 3 !== 0)
    )
      return "BFT needs 3f+1 orderers: choose 4, 7 or 10.";
    if (
      form.consensus === "raft" &&
      form.environment === "production" &&
      (form.orderers < 3 || form.orderers % 2 === 0)
    )
      return "A production Raft design needs at least three orderers, with an odd total.";
    const ids = new Set<string>();
    for (const node of form.ordererNodes) {
      if (!/^[a-z][a-z0-9-]{0,62}$/.test(node.id))
        return "Orderer names must be lowercase identifiers starting with a letter.";
      if (ids.has(node.id)) return "Each orderer needs a unique name.";
      ids.add(node.id);
      if (!form.organizations.some((org) => org.key === node.organizationKey))
        return "Assign every orderer to a current organization.";
      if (
        !/^[a-z][a-z0-9-]{0,62}$/.test(node.placementRef) ||
        !/^[a-z][a-z0-9-]{0,62}$/.test(node.failureDomain)
      )
        return "Enter valid placement and failure-domain references for every orderer.";
    }
  }
  return null;
}
