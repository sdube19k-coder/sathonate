import { describe, expect, it } from "vitest";
import {
  fromNetwork,
  newForm,
  slug,
  stepError,
  toSpec,
  withOrdererCount,
} from "./network-form";

describe("guided network design", () => {
  it("creates independent cloud and native contracts without credentials", () => {
    const form = newForm(true);
    const native = toSpec(form);
    expect(native.apiVersion).toBe("fabric.sathonate.io/v1alpha1");
    expect(native.spec.deployment.engine).toBe("native");
    expect(
      native.spec.nodes.filter((node) => node.role === "orderer"),
    ).toHaveLength(3);
    expect(JSON.stringify(native)).not.toMatch(
      /password|private_key|root_token/,
    );
    form.destination = "azure";
    const cloud = toSpec(form);
    expect(cloud.spec.deployment).toMatchObject({
      engine: "bevel",
      provider: "azure",
      runtime: "kubernetes",
    });
  });
  it("catches empty names, duplicate organizations, and unsafe production ordering", () => {
    const form = newForm();
    expect(stepError(form, 0)).toBeTruthy();
    form.name = "Our network";
    expect(stepError(form, 0)).toBeNull();
    form.organizations[1].name = form.organizations[0].name;
    expect(stepError(form, 2)).toMatch(/different name/);
    form.organizations[1].name = "Retailer";
    form.environment = "production";
    form.orderers = 1;
    expect(stepError(form, 3)).toMatch(/three orderers/);
  });
  it("preserves stable identifiers, existing MSPs and node placements during edits", () => {
    const original = toSpec(newForm(true));
    original.spec.organizations[0].mspId = "CustomMSP";
    original.spec.organizations[0].identityProviderRef = "external-ca";
    original.spec.nodes[0].placementRef = "verified-host";
    const form = fromNetwork(original);
    form.name = "Renamed display label";
    const updated = toSpec(form);
    expect(updated.metadata.id).toBe(original.metadata.id);
    expect(updated.spec.organizations[0]).toEqual(
      original.spec.organizations[0],
    );
    expect(updated.spec.nodes[0]).toEqual(original.spec.nodes[0]);
  });
  it("retains additional channels instead of silently dropping them", () => {
    const original = toSpec(newForm(true));
    original.spec.channels.push({
      ...original.spec.channels[0],
      id: "private-channel",
    });
    expect(toSpec(fromNetwork(original)).spec.channels).toHaveLength(2);
    expect(slug(" Our  First Network! ")).toBe("our-first-network");
  });
  it("exports BFT profiles and preserves explicit orderer ownership and placement", () => {
    let form = newForm(true);
    form = withOrdererCount({ ...form, consensus: "bft", version: "3.1.5" }, 4);
    form.ordererNodes[0].organizationKey = form.organizations[1].key;
    form.ordererNodes[0].placementRef = "private-host";
    expect(stepError(form, 3)).toBeNull();
    const spec = toSpec(form);
    expect(spec.spec.ordering).toEqual({
      consensus: "bft",
      channelCapability: "V3_0",
    });
    expect(
      spec.spec.nodes.find((node) => node.id === "orderer-1"),
    ).toMatchObject({
      organizationRef: "retailer",
      placementRef: "private-host",
    });
    expect(
      toSpec(fromNetwork(spec)).spec.nodes.filter(
        (node) => node.role === "orderer",
      ),
    ).toEqual(spec.spec.nodes.filter((node) => node.role === "orderer"));
    form.destination = "aws";
    expect(stepError(form, 3)).toMatch(/native/);
    form.destination = "private";
    form.ordererNodes[0].organizationKey = "missing";
    expect(stepError(form, 3)).toMatch(/current organization/);
  });
});
