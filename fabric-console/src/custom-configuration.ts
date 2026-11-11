import type { NetworkSpec } from "./types";

// This is an input usability check. The API remains authoritative for schema,
// inventory, authentication, tenant/project grants and transition approval.
export function parseCustomerConfiguration(text: string): NetworkSpec {
  if (text.length > 1024 * 1024)
    throw new Error("Configuration exceeds 1 MiB.");
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    throw new Error("Enter one valid JSON network configuration.");
  }
  function inspect(v: unknown): void {
    if (typeof v === "string" && /-----BEGIN .*PRIVATE KEY-----/.test(v))
      throw new Error("Private keys do not belong in network configuration.");
    if (Array.isArray(v)) {
      v.forEach(inspect);
      return;
    }
    if (v && typeof v === "object")
      for (const [key, entry] of Object.entries(v)) {
        if (
          /^(password|secret|token|accesstoken|privatekey|authorization|kubeconfig|credentials|command|shell|template|manifest)$/i.test(
            key.replaceAll("_", ""),
          )
        )
          throw new Error(
            "Credentials and executable templates are not accepted.",
          );
        inspect(entry);
      }
  }
  inspect(value);
  const s = value as NetworkSpec;
  const object = (v: unknown): v is Record<string, unknown> =>
    v !== null && typeof v === "object" && !Array.isArray(v);
  const objects = (v: unknown): v is Record<string, unknown>[] =>
    Array.isArray(v) && v.every(object);
  if (
    s?.apiVersion !== "fabric.sathonate.io/v1alpha1" ||
    s.kind !== "FabricNetwork" ||
    !s.metadata?.id ||
    !s.metadata?.name ||
    s.spec?.environment !== "development" ||
    s.spec.deployment?.targetRef !== "smp-dynamic-local-v1" ||
    s.spec.deployment.provider !== "local" ||
    s.spec.deployment.engine !== "bevel" ||
    !object(s.spec.configuration) ||
    !object(s.spec.configuration.placements) ||
    !object(s.spec.configuration.nodes) ||
    !object(s.spec.configuration.anchors) ||
    !objects(s.spec.configuration.authorities) ||
    !objects(s.spec.configuration.chaincodes) ||
    !objects(s.spec.nodes) ||
    !objects(s.spec.channels) ||
    !objects(s.spec.organizations) ||
    s.spec.configuration.authorities.some(
      (a) => !Array.isArray(a.roles) || !object(a.resources),
    ) ||
    s.spec.configuration.chaincodes.some(
      (c) => !object(c.endorsement) || !Array.isArray(c.endorsement.mspIds),
    ) ||
    s.spec.channels.some(
      (c) =>
        !Array.isArray(c.organizationRefs) ||
        !Array.isArray(c.peerRefs) ||
        !Array.isArray(c.ordererRefs),
    ) ||
    Object.values(s.spec.configuration.anchors).some((v) => !Array.isArray(v))
  )
    throw new Error(
      "Use a customer-defined development network for the explicit dynamic local target.",
    );
  return s;
}

export function customerSummary(s: NetworkSpec) {
  return `${s.spec.organizations.length} organizations · ${s.spec.nodes.filter((n) => n.role === "peer").length} peers · ${s.spec.nodes.filter((n) => n.role === "orderer").length} orderers · ${s.spec.configuration?.authorities.length ?? 0} CAs · ${s.spec.channels.length} channels · ${s.spec.configuration?.chaincodes.length ?? 0} contracts`;
}
