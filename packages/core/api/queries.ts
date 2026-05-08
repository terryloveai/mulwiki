import { queryOptions } from "@tanstack/react-query";
import { api } from "./index";

export const queryKeys = {
  workspaces: () => ["workspaces"] as const,
  workspace: (slug: string) => ["workspaces", slug] as const,
  builtinSchemas: () => ["schemas", "builtin"] as const,
  schemas: (workspace: string) => ["schemas", workspace] as const,
  sources: (workspace: string) => ["sources", workspace] as const,
  wikiPages: (workspace: string) => ["wiki", workspace, "pages"] as const,
  jobs: (workspace: string) => ["jobs", workspace] as const,
  agents: (workspace: string) => ["agents", workspace] as const,
  runtimes: (workspace: string) => ["agents", workspace, "runtimes"] as const,
  daemons: () => ["daemons"] as const,
};

export const workspaceQueries = {
  list: () =>
    queryOptions({
      queryKey: queryKeys.workspaces(),
      queryFn: () => api.listWorkspaces(),
    }),
  detail: (slug: string) =>
    queryOptions({
      queryKey: queryKeys.workspace(slug),
      queryFn: () => api.getWorkspace(slug),
      enabled: !!slug,
    }),
};

export const schemaQueries = {
  builtin: () =>
    queryOptions({
      queryKey: queryKeys.builtinSchemas(),
      queryFn: () => api.listBuiltinSchemas(),
    }),
  list: (workspace: string) =>
    queryOptions({
      queryKey: queryKeys.schemas(workspace),
      queryFn: () => api.listSchemas(workspace),
      enabled: !!workspace,
    }),
};
