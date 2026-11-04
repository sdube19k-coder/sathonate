import { afterEach, expect, it, vi } from "vitest";
import { api, setTokenProvider } from "./api";
import { setWorkspace } from "./workspace";

afterEach(() => {
  setTokenProvider(null);
  setWorkspace(null);
  vi.unstubAllGlobals();
});
it("binds lifecycle retries to the project, parent, action and approved preview", async () => {
  const storage = new Map<string, string>();
  vi.stubGlobal("sessionStorage", {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
  });
  setWorkspace({ id: "project-a", tenantId: "tenant-a", developer: "test" });
  const token = crypto.randomUUID();
  setTokenProvider(() => token);
  const calls: { path: string; options: RequestInit }[] = [];
  vi.stubGlobal("fetch", async (path: string, options: RequestInit) => {
    calls.push({ path, options });
    return new Response("{}", { status: 200 });
  });
  await api.lifecyclePreview("op/a", "add-peer");
  await api.lifecycleApply("op/a", "add-peer", "digest-a");
  await api.lifecycleApply("op/a", "add-peer", "digest-a");
  await api.lifecycleApply("op/a", "add-peer", "digest-b");
  expect(calls[0].path).toContain("op%2Fa/lifecycle/add-peer/preview");
  const key = (n: number) =>
    new Headers(calls[n].options.headers).get("Idempotency-Key");
  expect(key(1)).toBe(key(2));
  expect(key(1)).not.toBe(key(3));
  expect(JSON.parse(calls[1].options.body as string)).toEqual({
    approval: "digest-a",
  });
  expect(
    calls.every(
      (call) =>
        !call.path.includes(token) &&
        new Headers(call.options.headers).get("Authorization") ===
          "Bearer " + token,
    ),
  ).toBe(true);
});
