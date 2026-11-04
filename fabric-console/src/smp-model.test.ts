import { describe, it, expect } from "vitest";
import {
  initialDraft,
  selectProvider,
  cidrError,
  deploymentSpec,
} from "./smp-model";
import { stepError } from "./network-form";
describe("PDF workflow configuration", () => {
  it("keeps local Kubernetes execution separate from cloud credentials and Docker execution", () => {
    const draft = selectProvider(
      {
        ...initialDraft(),
        accountId: "123456789012",
        targetRef: "cloud-target",
      },
      "kubernetes",
    );
    const spec = deploymentSpec(draft);
    expect(spec.spec.deployment).toEqual({
      engine: "bevel",
      runtime: "kubernetes",
      provider: "local",
      targetRef: "smp-qualification",
    });
    expect(spec.spec.environment).toBe("development");
    expect(spec.spec.nodes.filter((node) => node.role === "peer")).toHaveLength(
      2,
    );
    const orderers = spec.spec.nodes.filter((node) => node.role === "orderer");
    expect(orderers).toHaveLength(3);
    expect(new Set(orderers.map((node) => node.organizationRef)).size).toBe(1);
    expect(spec.spec.deployment.cloud).toBeUndefined();
    expect(spec.spec.channels).toHaveLength(1);
  });
  it("serializes cloud configuration into the saved network without browser state or credentials", () => {
    const d = {
      ...initialDraft(),
      accountId: "123456789012",
      region: "ap-south-1",
      clusterName: "fabric-cluster",
      targetRef: "registered-eks",
      addons: ["Metrics Server"],
    };
    const spec = deploymentSpec(d);
    expect(spec.spec.deployment.targetRef).toBe("registered-eks");
    expect(spec.spec.deployment.cloud?.accountId).toBe(d.accountId);
    expect(spec.spec.deployment.cloud?.clusterName).toBe(d.clusterName);
    expect(JSON.stringify(spec)).not.toContain("recordId");
    expect(
      deploymentSpec(selectProvider(d, "local")).spec.deployment.cloud,
    ).toBeUndefined();
    d.addons.push("External DNS");
    expect(spec.spec.deployment.cloud?.addons).toEqual(["Metrics Server"]);
  });
  it("locks the local runtime to the actual supported topology", () => {
    const d = selectProvider(initialDraft(), "local");
    expect(d.form.version).toBe("2.5.15");
    expect(d.form.orderers).toBe(3);
    expect(d.form.organizations).toHaveLength(2);
    expect(d.form.channel).toBe("sathonate");
    expect(d.form.consensus).toBe("raft");
    expect(stepError(d.form, 3)).toBeNull();
  });
  it("rejects overlapping ranges and invalid node scaling", () => {
    const d = {
      ...initialDraft(),
      clusterMode: "new" as const,
      vpcCidr: "10.0.0.0/16",
      serviceCidr: "10.0.1.0/24",
      podCidr: "10.2.0.0/16",
    };
    expect(cidrError(d)).toContain("overlap");
    expect(cidrError({ ...d, serviceCidr: "10.1.0.0/16" })).toBeNull();
    expect(cidrError({ ...d, minNodes: 5, desiredNodes: 3 })).toContain(
      "Node counts",
    );
    expect(cidrError({ ...d, vpcCidr: "300.1.0.0/16" })).toContain(
      "valid IPv4",
    );
  });
});
