import { afterEach, expect, it, vi } from "vitest";
import { api, setTokenProvider } from "./api";
import { setWorkspace } from "./workspace";
afterEach(() => {
  setTokenProvider(null);
  setWorkspace(null);
  vi.unstubAllGlobals();
});
it("reuses deploy/retry intent keys and never puts credentials in URLs", async () => {
  const data = new Map<string, string>();
  vi.stubGlobal("sessionStorage", {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => data.set(key, value),
  });
  setWorkspace({ id: "a", tenantId: "tenant", developer: "developer" });
  const token = Array.from(crypto.getRandomValues(new Uint8Array(32)), (b) =>
    b.toString(16).padStart(2, "0"),
  ).join("");
  setTokenProvider(() => token);
  const calls: { path: string; headers: Headers }[] = [];
  vi.stubGlobal("fetch", async (path: string, options: RequestInit) => {
    calls.push({ path, headers: new Headers(options.headers) });
    return new Response("{}", { status: 202 });
  });
  const network = { id: "network-a", revision: 1 } as Parameters<
    typeof api.deploy
  >[0];
  await api.deploy(network);
  await api.deploy(network);
  await api.retryOperation("operation-a");
  await api.retryOperation("operation-a");
  expect(
    calls[0].headers.get("Idempotency-Key") ===
      calls[1].headers.get("Idempotency-Key"),
  ).toBe(true);
  expect(
    calls[2].headers.get("Idempotency-Key") ===
      calls[3].headers.get("Idempotency-Key"),
  ).toBe(true);
  expect(
    calls.every(
      (call) =>
        !call.path.includes(token) &&
        call.headers.get("Authorization") === "Bearer " + token,
    ),
  ).toBe(true);
  expect([...data.values()].every((value) => !value.includes(token))).toBe(
    true,
  );
});
it("reports expired authentication and forbidden requests without echoing credentials", async () => {
  const target = new EventTarget();
  vi.stubGlobal("window", target);
  let expired = false;
  target.addEventListener("local-auth-expired", () => {
    expired = true;
  });
  vi.stubGlobal(
    "fetch",
    async () => new Response("unauthorized", { status: 401 }),
  );
  await expect(api.projects()).rejects.toThrow("Invalid or expired token");
  expect(expired).toBe(true);
  vi.stubGlobal(
    "fetch",
    async () => new Response("forbidden", { status: 403 }),
  );
  await expect(api.projects()).rejects.toThrow("does not permit");
});
it("binds recovery retries to the approved operation, node and digest", async () => {
  const data = new Map<string, string>();
  vi.stubGlobal("sessionStorage", {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => data.set(key, value),
  });
  setWorkspace({ id: "a", tenantId: "tenant", developer: "developer" });
  const token = crypto.randomUUID();
  setTokenProvider(() => token);
  const calls: { path: string; headers: Headers; body?: BodyInit | null }[] =
    [];
  vi.stubGlobal("fetch", async (path: string, options: RequestInit) => {
    calls.push({
      path,
      headers: new Headers(options.headers),
      body: options.body,
    });
    return new Response("{}", { status: 202 });
  });
  await api.applyNodeRecovery("parent", "peer", "digest-a");
  await api.applyNodeRecovery("parent", "peer", "digest-a");
  await api.applyNodeRecovery("parent", "peer", "digest-b");
  expect(calls[0].headers.get("Idempotency-Key")).toBe(
    calls[1].headers.get("Idempotency-Key"),
  );
  expect(calls[0].headers.get("Idempotency-Key")).not.toBe(
    calls[2].headers.get("Idempotency-Key"),
  );
  expect(JSON.parse(String(calls[0].body))).toEqual({
    node: "peer",
    approval: "digest-a",
  });
  expect(
    calls.every(
      (c) =>
        !c.path.includes(token) &&
        c.headers.get("Authorization") === "Bearer " + token,
    ),
  ).toBe(true);
  expect([...data.values()].every((v) => !v.includes(token))).toBe(true);
});
