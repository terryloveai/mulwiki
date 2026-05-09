import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const workspaceKeys = {
  all: () => ["workspaces"] as const,
  detail: (workspace: string) => [...workspaceKeys.all(), workspace] as const,
  schemas: (workspace: string) => [...workspaceKeys.detail(workspace), "schemas"] as const,
  schemaList: (workspace: string) => [...workspaceKeys.schemas(workspace), "list"] as const,
  sources: (workspace: string) => [...workspaceKeys.detail(workspace), "sources"] as const,
  sourceList: (workspace: string) => [...workspaceKeys.sources(workspace), "list"] as const,
};

export function workspaceListOptions() {
  return queryOptions({
    queryKey: workspaceKeys.all(),
    queryFn: () => api.listWorkspaces(),
  });
}

export function workspaceDetailOptions(workspace: string) {
  return queryOptions({
    queryKey: workspaceKeys.detail(workspace),
    queryFn: () => api.getWorkspace(workspace),
    enabled: !!workspace,
  });
}

export function schemaListOptions(workspace: string) {
  return queryOptions({
    queryKey: workspaceKeys.schemaList(workspace),
    queryFn: () => api.listSchemas(workspace),
    enabled: !!workspace,
  });
}

export function sourceListOptions(workspace: string) {
  return queryOptions({
    queryKey: workspaceKeys.sourceList(workspace),
    queryFn: () => api.listSources(workspace),
    enabled: !!workspace,
  });
}
