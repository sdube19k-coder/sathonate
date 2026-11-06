import { afterEach, expect, it, vi } from "vitest";
import { createElement } from "react";
import { renderToString } from "react-dom/server";
import {
  parseCustomerConfiguration,
  customerSummary,
} from "./custom-configuration";
import { api, setTokenProvider } from "./api";
import type { Network, NetworkSpec } from "./types";
import CustomNetworks from "./CustomNetworks";

const fixture = (): NetworkSpec => ({
  apiVersion: "fabric.sathonate.io/v1alpha1",
  kind: "FabricNetwork",
  metadata: { id: "client-defined", name: "Customer-defined supply network" },
  spec: {
    environment: "development",
    fabricVersion: "2.5.15",
    deployment: {
      engine: "bevel",
      runtime: "kubernetes",
      provider: "local",
      targetRef: "smp-dynamic-local-v1",
    },
    organizations: [
      {
        id: "supplier",
        mspId: "SupplierMSP",
        identityProviderRef: "supplier-ca",
      },
    ],
    nodes: [
      {
        id: "warehouse",
        organizationRef: "supplier",
        role: "peer",
        placementRef: "approved",
        failureDomain: "local",
      },
    ],
    channels: [
      {
        id: "shipments",
        organizationRefs: ["supplier"],
        peerRefs: ["warehouse"],
        ordererRefs: [],
      },
    ],
    configuration: {
      placements: {},
      nodes: {},
      authorities: [],
      anchors: {},
      chaincodes: [],
    },
  },
});
afterEach(() => {
  setTokenProvider(null);
  vi.unstubAllGlobals();
});

it("retains customer names and arbitrary topology while distinguishing design from execution", () => {
  const s = fixture();
  expect(parseCustomerConfiguration(JSON.stringify(s))).toEqual(s);
  expect(customerSummary(s)).toContain("1 peers");
  s.spec.nodes.push({ ...s.spec.nodes[0], id: "second-customer-peer" });
  expect(customerSummary(s)).toContain("2 peers");
  const html = renderToString(createElement(CustomNetworks, {}));
  expect(html).toContain("Dynamic execution is disabled");
  expect(html).toContain("desired configuration");
  expect(html).not.toContain("Deployment verification passed");
});
it("rejects credentials, executable templates and nonlocal targets before submission", () => {
  for (const key of [
    "private_key",
    "access_token",
    "password",
    "template",
    "command",
  ]) {
    const s = fixture();
    const modified = { ...s, unexpected: { [key]: "forbidden-input" } };
    expect(() => parseCustomerConfiguration(JSON.stringify(modified))).toThrow(
      /Credentials/,
    );
  }
  const s = fixture();
  s.spec.deployment.provider = "aws";
  expect(() => parseCustomerConfiguration(JSON.stringify(s))).toThrow(
    /local target/,
  );
  expect(() => parseCustomerConfiguration("not json")).toThrow(/valid JSON/);
  expect(() => parseCustomerConfiguration(" ".repeat(1024 * 1024 + 1))).toThrow(
    /1 MiB/,
  );
});
it("binds preview/apply to saved revision and reuses only the exact approved intent key", async () => {
  const values = new Map<string, string>();
  vi.stubGlobal("sessionStorage", {
    getItem: (k: string) => values.get(k) ?? null,
    setItem: (k: string, v: string) => values.set(k, v),
  });
  const calls: { path: string; options: RequestInit }[] = [];
  vi.stubGlobal("fetch", async (path: string, options: RequestInit) => {
    calls.push({ path, options });
    return new Response("{}", { status: 200 });
  });
  const token = crypto.randomUUID();
  setTokenProvider(() => token);
  const n = {
    id: "network-customer",
    revision: 4,
    desired: fixture(),
  } as Network;
  await api.previewDynamic(n, "op-previous");
  await api.applyDynamic(n, "op-previous", "approved-digest");
  await api.applyDynamic(n, "op-previous", "approved-digest");
  expect(new Headers(calls[0].options.headers).get("If-Match")).toBe('"4"');
  expect(new Headers(calls[1].options.headers).get("Idempotency-Key")).toBe(
    new Headers(calls[2].options.headers).get("Idempotency-Key"),
  );
  expect(JSON.parse(calls[1].options.body as string)).toEqual({
    parentOperationId: "op-previous",
    approval: "approved-digest",
  });
  expect(
    calls.every(
      (c) =>
        !c.path.includes(token) &&
        new Headers(c.options.headers).get("Authorization") ===
          "Bearer " + token,
    ),
  ).toBe(true);
});
