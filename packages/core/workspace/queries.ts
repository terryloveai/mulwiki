import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const workspaceKeys = {
  all: () => ["workspaces"] as const,
  detail: (workspace: string) => [...workspaceKeys.all(), workspace] as const,
  schemas: (workspace: string) => [...workspaceKeys.detail(workspace), "schemas"] as const,
  schemaList: (workspace: string) => [...workspaceKeys.schemas(workspace), "list"] as const,
  schemaDetail: (workspace: string, schemaId: string) => [...workspaceKeys.schemas(workspace), schemaId] as const,
  sources: (workspace: string) => [...workspaceKeys.detail(workspace), "sources"] as const,
  sourceList: (workspace: string) => [...workspaceKeys.sources(workspace), "list"] as const,
  sourceDetail: (workspace: string, sourcePath: string) => [...workspaceKeys.sources(workspace), sourcePath] as const,
  wiki: (workspace: string) => [...workspaceKeys.detail(workspace), "wiki"] as const,
  wikiList: (workspace: string) => [...workspaceKeys.wiki(workspace), "list"] as const,
  wikiDetail: (workspace: string, pagePath: string) => [...workspaceKeys.wiki(workspace), pagePath] as const,
  wikiBacklinks: (workspace: string, pagePath: string) => [...workspaceKeys.wikiDetail(workspace, pagePath), "backlinks"] as const,
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

export function schemaDetailOptions(workspace: string, schemaId: string) {
  return queryOptions({
    queryKey: workspaceKeys.schemaDetail(workspace, schemaId),
    queryFn: () => api.getSchema(workspace, schemaId),
    enabled: !!workspace && !!schemaId,
  });
}

export function sourceListOptions(workspace: string) {
  return queryOptions({
    queryKey: workspaceKeys.sourceList(workspace),
    queryFn: () => api.listSources(workspace),
    enabled: !!workspace,
  });
}

export function sourceDetailOptions(workspace: string, sourcePath: string) {
  return queryOptions({
    queryKey: workspaceKeys.sourceDetail(workspace, sourcePath),
    queryFn: () => api.getSource(workspace, sourcePath),
    enabled: !!workspace && !!sourcePath,
  });
}

export function wikiListOptions(workspace: string) {
  return queryOptions({
    queryKey: workspaceKeys.wikiList(workspace),
    queryFn: () => api.listWikiPages(workspace),
    enabled: !!workspace,
  });
}

export function wikiDetailOptions(workspace: string, pagePath: string) {
  return queryOptions({
    queryKey: workspaceKeys.wikiDetail(workspace, pagePath),
    queryFn: () => api.getWikiPage(workspace, pagePath),
    enabled: !!workspace && !!pagePath,
  });
}

export function wikiBacklinksOptions(workspace: string, pagePath: string) {
  return queryOptions({
    queryKey: workspaceKeys.wikiBacklinks(workspace, pagePath),
    queryFn: () => api.getWikiBacklinks(workspace, pagePath),
    enabled: !!workspace && !!pagePath,
  });
}
