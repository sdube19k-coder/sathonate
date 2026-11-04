import { afterEach, expect, it, vi } from "vitest";
import { api, setTokenProvider, type OperationDiagnostics } from "./api";
import { downloadDiagnostics } from "./OperationDiagnostics";

afterEach(() => {
  setTokenProvider(null);
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("fetches diagnostics with authentication and encodes the operation identifier", async () => {
  const token = crypto.randomUUID();
  setTokenProvider(() => token);
  const fetcher = vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          evidenceType: "recorded-operation",
          liveHealthChecked: false,
        }),
        { status: 200 },
      ),
  );
  vi.stubGlobal("fetch", fetcher);
  const result = await api.operationDiagnostics("op/a");
  const [url, options] = fetcher.mock.calls[0] as unknown as [
    string,
    RequestInit,
  ];
  expect(url).toBe("/api/v1/operations/op%2Fa/diagnostics");
  expect(new Headers(options.headers).get("Authorization")).toBe(
    `Bearer ${token}`,
  );
  expect(result.liveHealthChecked).toBe(false);
});

it("downloads only the API report and releases its object URL", async () => {
  const click = vi.fn();
  const link = { href: "", download: "", click };
  vi.stubGlobal("document", { createElement: () => link });
  let payload: Blob | undefined;
  vi.spyOn(URL, "createObjectURL").mockImplementation((blob) => {
    payload = blob as Blob;
    return "blob:local-report";
  });
  const revoke = vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  const report = {
    evidenceType: "recorded-operation",
    liveHealthChecked: false,
  } as OperationDiagnostics;
  downloadDiagnostics(report);
  expect(click).toHaveBeenCalledOnce();
  expect(link.download).toBe("sathonate-operation-diagnostics.json");
  expect(JSON.parse(await payload!.text())).toEqual(report);
  await new Promise((resolve) => setTimeout(resolve, 10));
  expect(revoke).toHaveBeenCalledWith("blob:local-report");
});
