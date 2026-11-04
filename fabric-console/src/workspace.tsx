import { createContext, useContext } from "react";
export type Project = { id: string; tenantId: string; developer: string };
let workspace: Project | null = null;
export function setWorkspace(value: Project | null) {
  workspace = value;
}
export function workspaceKey() {
  return workspace
    ? "fabric-secure-v1:" + JSON.stringify([workspace.tenantId, workspace.id])
    : "fabric-smp-flow-v2";
}
export function secureWorkspace() {
  return workspace !== null;
}
export const WorkspaceContext = createContext<Project | null>(null);
export function useWorkspace() {
  return useContext(WorkspaceContext);
}
export function localAuthAllowed(env: {
  DEV: boolean;
  VITE_LOCAL_AUTH_ENABLED?: string;
  VITE_APP_ENV?: string;
}) {
  return (
    env.DEV === true &&
    env.VITE_LOCAL_AUTH_ENABLED === "true" &&
    env.VITE_APP_ENV === "local-development"
  );
}
