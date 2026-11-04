import { afterEach, expect, it, vi } from "vitest";
import { createElement } from "react";
import { renderToString } from "react-dom/server";
import SmpApp from "./SmpApp";
import { setWorkspace, WorkspaceContext } from "./workspace";
afterEach(() => {
  setWorkspace(null);
  vi.unstubAllGlobals();
});
it("renders the existing secure wizard with its topology controls locked", () => {
  const project = {
    id: "render-project",
    tenantId: "render-tenant",
    developer: "render-developer",
  };
  setWorkspace(project);
  vi.stubGlobal("sessionStorage", { getItem: () => null });
  vi.stubGlobal("location", { hash: "#/create/3" });
  const markup = renderToString(
    createElement(
      WorkspaceContext.Provider,
      { value: project },
      createElement(SmpApp),
    ),
  );
  expect(markup).toContain("Secure network name");
  expect(markup).toContain("qualified topology");
  expect(markup).toMatch(/<fieldset[^>]*disabled/);
  expect(markup.includes("\ufffd")).toBe(false);
  expect(markup).not.toContain("Deployment verification passed");
});
