import { afterEach, describe, expect, it } from "vitest";
import { localAuthAllowed, setWorkspace, workspaceKey } from "./workspace";
import { deploymentSpec, secureDraft, sameDesign } from "./smp-model";
afterEach(() => setWorkspace(null));
describe("secure local console boundary", () => {
  it("preserves the deployment revision when JSON property ordering changes", () => {
    expect(
      sameDesign(
        { id: "n", spec: { version: 1 } },
        { spec: { version: 1 }, id: "n" },
      ),
    ).toBe(true);
    expect(
      sameDesign(
        { id: "n", spec: { version: 1 } },
        { id: "n", spec: { version: 2 } },
      ),
    ).toBe(false);
  });
  it("requires every explicit development gate", () => {
    const enabled = {
      DEV: true,
      VITE_LOCAL_AUTH_ENABLED: "true",
      VITE_APP_ENV: "local-development",
    };
    expect(localAuthAllowed(enabled)).toBe(true);
    for (const env of [
      { ...enabled, DEV: false },
      { ...enabled, VITE_LOCAL_AUTH_ENABLED: undefined },
      { ...enabled, VITE_APP_ENV: "production" },
      { ...enabled, VITE_APP_ENV: "staging" },
      { ...enabled, VITE_APP_ENV: undefined },
    ])
      expect(localAuthAllowed(env)).toBe(false);
  });
  it("builds the exact supported topology without cloud or Kubernetes references", () => {
    const spec = deploymentSpec(secureDraft());
    expect(spec.spec.deployment).toEqual({
      engine: "bevel",
      runtime: "kubernetes",
      provider: "local",
      targetRef: "smp-secure-local-v1",
    });
    expect(spec.spec.fabricVersion).toBe("2.5.15");
    expect(spec.spec.channels[0].id).toBe("smpchannel");
    expect(spec.spec.organizations.map((o) => o.mspId)).toEqual([
      "Org1MSP",
      "Org2MSP",
    ]);
    expect(spec.spec.nodes.filter((n) => n.role === "peer")).toHaveLength(2);
    expect(spec.spec.nodes.filter((n) => n.role === "orderer")).toHaveLength(3);
    expect(
      spec.spec.nodes.every(
        (n) =>
          n.placementRef === "kind-node" && n.failureDomain === "local-host",
      ),
    ).toBe(true);
  });
  it("keeps browser draft state separate for every server-granted tenant/project", () => {
    setWorkspace({ id: "project", tenantId: "tenant-a", developer: "a" });
    const a = workspaceKey();
    setWorkspace({ id: "project", tenantId: "tenant-b", developer: "a" });
    expect(workspaceKey()).not.toBe(a);
    setWorkspace({ id: "other", tenantId: "tenant-a", developer: "a" });
    expect(workspaceKey()).not.toBe(a);
  });
});
