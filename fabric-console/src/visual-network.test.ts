import { describe, expect, it } from "vitest";
import {
  addVisualEntity,
  emptyVisualNetwork,
  visualWarnings,
} from "./visual-network";
const capabilities = {
  enabled: true,
  storageClass: "standard",
  placements: { floor: { "kubernetes.io/hostname": "approved-worker" } },
};
const add = (
  n: ReturnType<typeof emptyVisualNetwork>,
  kind: Parameters<typeof addVisualEntity>[1],
  id: string,
  org = "supplier",
) => addVisualEntity(n, kind, id, org, "floor", "standard");
describe("customer-defined visual topology", () => {
  it("starts empty and clones only server-owned inventory", () => {
    const n = emptyVisualNetwork(capabilities);
    expect(n.spec.organizations).toEqual([]);
    expect(n.spec.nodes).toEqual([]);
    expect(n.spec.configuration?.placements.floor.nodeSelector).toEqual(
      capabilities.placements.floor,
    );
    n.spec.configuration!.placements.floor.nodeSelector.changed = "value";
    expect(capabilities.placements.floor).not.toHaveProperty("changed");
  });
  it("generates independent entities and channel references without mutating the input", () => {
    const original = emptyVisualNetwork(capabilities);
    let n = add(original, "organization", "supplier");
    n = add(n, "authority", "supply-ca");
    n = add(n, "peer", "warehouse-east");
    n = add(n, "orderer", "raft-east");
    n = add(n, "channel", "orders");
    expect(original.spec.organizations).toEqual([]);
    expect(n.spec.organizations[0].identityProviderRef).toBe("supply-ca");
    expect(n.spec.channels[0].peerRefs).toEqual(["warehouse-east"]);
    expect(n.spec.configuration!.anchors.orders).toEqual(["warehouse-east"]);
    expect(visualWarnings(n)).toEqual([]);
  });
  it("refuses duplicate identifiers, foreign organizations and invented placements", () => {
    let n = add(emptyVisualNetwork(capabilities), "organization", "supplier");
    expect(() => add(n, "channel", "supplier")).toThrow();
    expect(() => add(n, "peer", "warehouse", "foreign")).toThrow();
    expect(() =>
      addVisualEntity(
        n,
        "peer",
        "warehouse",
        "supplier",
        "unapproved",
        "standard",
      ),
    ).toThrow();
    expect(() => add(n, "peer", "__proto__")).toThrow();
  });
  it("preserves existing nodes and channels during additions", () => {
    let n = add(emptyVisualNetwork(capabilities), "organization", "supplier");
    n = add(n, "authority", "supplier-ca");
    n = add(n, "peer", "primary-peer");
    n = add(n, "orderer", "raft-one");
    n = add(n, "channel", "original-ledger");
    const before = structuredClone(n);
    n = add(n, "organization", "carrier");
    n = add(n, "authority", "carrier-ca", "carrier");
    n = add(n, "peer", "carrier-peer", "carrier");
    expect(n.spec.channels).toEqual(before.spec.channels);
    expect(n.spec.nodes[0]).toEqual(before.spec.nodes[0]);
    expect(n.spec.nodes).toHaveLength(3);
  });
  it("flags missing signing and member consistency without pretending server validation", () => {
    let n = add(emptyVisualNetwork(capabilities), "organization", "supplier");
    expect(visualWarnings(n)).toContain(
      "supplier: select its signing authority.",
    );
  });
});
