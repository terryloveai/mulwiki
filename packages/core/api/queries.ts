import { queryOptions } from "@tanstack/react-query";
import { schemaListOptions, workspaceDetailOptions, workspaceKeys, workspaceListOptions } from "../workspace/queries";
import { agentKeys } from "../agents/queries";
import { jobKeys } from "../jobs/queries";
import { api } from "./index";

export * from "../workspace/queries";
export * from "../agents/queries";
export * from "../jobs/queries";

export const queryKeys = {
  workspaces: workspaceKeys.all,
  workspace: workspaceKeys.detail,
  builtinSchemas: () => ["schemas", "builtin"] as const,
  schemas: workspaceKeys.schemaList,
  sources: workspaceKeys.sourceList,
  wikiPages: (workspace: string) => ["wiki", workspace, "pages"] as const,
  jobs: jobKeys.list,
  agents: agentKeys.list,
  runtimes: agentKeys.runtimes,
  daemons: () => ["daemons"] as const,
};

export const workspaceQueries = {
  list: workspaceListOptions,
  detail: workspaceDetailOptions,
};

export const schemaQueries = {
  builtin: () =>
    queryOptions({
      queryKey: queryKeys.builtinSchemas(),
      queryFn: () => api.listBuiltinSchemas(),
    }),
  list: schemaListOptions,
};
