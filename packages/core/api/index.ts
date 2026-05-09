import type {
  Workspace, Schema, SchemaWithActive, Source, WikiPage, WikiBacklink, 
  ResolveWikiLinksRequest, ResolveWikiLinksResponse,
  Job, Agent, User,
  AgentRuntime, AgentSkill, AgentTask,
  RuntimesResponse, RuntimeResponse,
  AgentsResponse, AgentResponse,
  SkillsResponse, SkillResponse,
  TasksResponse, TaskResponse, TaskMessagesResponse,
  CreateRuntimeRequest, UpdateRuntimeRequest,
  CreateAgentRequest, UpdateAgentRequest,
  CreateSkillRequest, UpdateSkillRequest,
  CreateSchemaRequest, UpdateSchemaRequest, ForkSchemaRequest, ValidateSchemaResponse,
} from "../types";

const BASE = "/api";

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  // Don't force Content-Type for FormData — browser needs to set multipart boundary
  const headers: Record<string, string> = {};
  if (!(init?.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
  }
  if (init?.headers) {
    const h = init.headers as Record<string, string>;
    Object.assign(headers, h);
  }

  const res = await fetch(`${BASE}${url}`, {
    ...init,
    headers,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const api = {
  // Auth
  register: (data: { email: string; password: string }) =>
    fetchJSON<User>("/auth/register", { method: "POST", body: JSON.stringify(data) }),
  login: (data: { email: string; password: string }) =>
    fetchJSON<User>("/auth/login", { method: "POST", body: JSON.stringify(data) }),
  logout: () => fetchJSON<{ ok: boolean }>("/auth/logout", { method: "POST" }),
  me: () => fetchJSON<User>("/auth/me"),

  // Workspaces
  listWorkspaces: () => fetchJSON<Workspace[]>("/workspaces"),
  getWorkspace: (slug: string) => fetchJSON<Workspace>(`/workspaces/${slug}`),
  createWorkspace: (data: {
    name: string;
    slug: string;
    description?: string;
    initial_schema_type?: "blank" | "builtin";
    initial_schema_path?: string;
  }) =>
    fetchJSON<Workspace>("/workspaces", { method: "POST", body: JSON.stringify(data) }),
  updateWorkspace: (slug: string, data: { name: string; description: string }) =>
    fetchJSON<Workspace>(`/workspaces/${slug}`, { method: "PATCH", body: JSON.stringify(data) }),
  deleteWorkspace: (slug: string) =>
    fetchJSON<void>(`/workspaces/${slug}`, { method: "DELETE" }),

  // Schemas
  listBuiltinSchemas: () => fetchJSON<Schema[]>("/schemas/builtin"),
  listSchemas: (ws: string) => fetchJSON<SchemaWithActive[]>(`/workspaces/${ws}/schemas`),
  getSchema: (ws: string, id: string) => fetchJSON<Schema>(`/workspaces/${ws}/schemas/${id}`),
  createSchema: (ws: string, data: CreateSchemaRequest) =>
    fetchJSON<Schema>(`/workspaces/${ws}/schemas`, { method: "POST", body: JSON.stringify(data) }),
  updateSchema: (ws: string, id: string, data: UpdateSchemaRequest) =>
    fetchJSON<Schema>(`/workspaces/${ws}/schemas/${id}`, { method: "PUT", body: JSON.stringify(data) }),
  deleteSchema: (ws: string, id: string) =>
    fetchJSON<void>(`/workspaces/${ws}/schemas/${id}`, { method: "DELETE" }),
  forkSchema: (ws: string, data: ForkSchemaRequest) =>
    fetchJSON<Schema>(`/workspaces/${ws}/schemas/fork`, { method: "POST", body: JSON.stringify(data) }),
  activateSchema: (ws: string, schemaId: string) =>
    fetchJSON<{ active_schema_id: string }>(`/workspaces/${ws}/activate-schema`, {
      method: "PUT",
      body: JSON.stringify({ schema_id: schemaId }),
    }),
  validateSchema: (ws: string, content: string) =>
    fetchJSON<ValidateSchemaResponse>(`/workspaces/${ws}/schemas/validate`, {
      method: "POST",
      body: JSON.stringify({ content }),
    }),

  // Sources
  listSources: (ws: string) => fetchJSON<Source[]>(`/workspaces/${ws}/sources`),
  getSource: (ws: string, path: string) => fetchJSON<Source>(`/workspaces/${ws}/sources/${encodeURIComponent(path)}`),
  uploadSource: (ws: string, formData: FormData) =>
    fetchJSON<Source>(`/workspaces/${ws}/sources`, { method: "POST", body: formData }),
  deleteSource: (ws: string, path: string) =>
    fetchJSON<void>(`/workspaces/${ws}/sources/${encodeURIComponent(path)}`, { method: "DELETE" }),

  // Wiki
  listWikiPages: (ws: string) => fetchJSON<WikiPage[]>(`/workspaces/${ws}/wiki`),
  getWikiPage: (ws: string, path: string) => fetchJSON<WikiPage>(`/workspaces/${ws}/wiki/${path}`),
  searchWiki: (ws: string, q: string) => fetchJSON<WikiPage[]>(`/workspaces/${ws}/wiki/search?q=${encodeURIComponent(q)}`),
  resolveWikiLinks: (ws: string, paths: string[]) =>
    fetchJSON<ResolveWikiLinksResponse>(`/workspaces/${ws}/wiki/resolve-links`, {
      method: "POST",
      body: JSON.stringify({ paths }),
    }),
  getWikiBacklinks: (ws: string, path: string) =>
    fetchJSON<WikiBacklink[]>(`/workspaces/${ws}/wiki/backlinks?path=${encodeURIComponent(path)}`),

  // Jobs
  listJobs: (ws: string) => fetchJSON<Job[]>(`/workspaces/${ws}/jobs`),
  createJob: (ws: string, data: { source_path?: string; source_paths?: string[]; schema_id: string; agent_id: string }) =>
    fetchJSON<Job>(`/workspaces/${ws}/jobs`, { method: "POST", body: JSON.stringify(data) }),
  getJob: (ws: string, jobId: string) =>
    fetchJSON<Job>(`/workspaces/${ws}/jobs/${jobId}`),
  /** Stream job logs via SSE. Returns an EventSource that emits status/progress/done events. */
  streamJobLogs: (ws: string, jobId: string): EventSource => {
    const es = new EventSource(`${BASE}/workspaces/${ws}/jobs/${jobId}/logs`);
    return es;
  },

  /* ── Daemon ── */
  listDaemons: (ws: string) =>
    fetchJSON<{ daemons: Array<{ id: string; hostname: string; pid: number; version: string; runtime_ids: string; max_concurrent_tasks: number; last_heartbeat: string; registered_at: string }> }>(`/workspaces/${ws}/daemons`),
  getDaemonLogs: (ws: string, id: string, n?: number) =>
    fetchJSON<{ daemon_id: string; log_path: string; lines: string[]; total: number }>(`/workspaces/${ws}/daemons/${id}/logs${n ? `?n=${n}` : ""}`),
  stopDaemon: (ws: string, id: string) =>
    fetchJSON<{ daemon_id: string; status: string }>(`/workspaces/${ws}/daemons/${id}/stop`, { method: "POST" }),
  startDaemon: (workspace: string, serverUrl?: string) =>
    fetchJSON<{ status: string; pid: number }>(`/workspaces/${workspace}/daemons/start`, {
      method: "POST",
      body: JSON.stringify({ workspace, server_url: serverUrl }),
    }),

  /* ── Runtimes ── */
  listRuntimes: (ws: string) =>
    fetchJSON<RuntimesResponse>(`/workspaces/${ws}/agents/runtimes`),
  createRuntime: (ws: string, data: CreateRuntimeRequest) =>
    fetchJSON<RuntimeResponse>(`/workspaces/${ws}/agents/runtimes`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  getRuntime: (ws: string, runtimeId: string) =>
    fetchJSON<RuntimeResponse>(`/workspaces/${ws}/agents/runtimes/${runtimeId}`),
  updateRuntime: (ws: string, runtimeId: string, data: UpdateRuntimeRequest) =>
    fetchJSON<RuntimeResponse>(`/workspaces/${ws}/agents/runtimes/${runtimeId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  deleteRuntime: (ws: string, runtimeId: string) =>
    fetchJSON<void>(`/workspaces/${ws}/agents/runtimes/${runtimeId}`, {
      method: "DELETE",
    }),

  /* ── Agents ── */
  listAgents: (ws: string) =>
    fetchJSON<AgentsResponse>(`/workspaces/${ws}/agents`),
  createAgent: (ws: string, data: CreateAgentRequest) =>
    fetchJSON<AgentResponse>(`/workspaces/${ws}/agents`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  getAgent: (ws: string, agentId: string) =>
    fetchJSON<AgentResponse>(`/workspaces/${ws}/agents/${agentId}`),
  updateAgent: (ws: string, agentId: string, data: UpdateAgentRequest) =>
    fetchJSON<AgentResponse>(`/workspaces/${ws}/agents/${agentId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  archiveAgent: (ws: string, agentId: string) =>
    fetchJSON<void>(`/workspaces/${ws}/agents/${agentId}/archive`, {
      method: "POST",
    }),
  restoreAgent: (ws: string, agentId: string) =>
    fetchJSON<void>(`/workspaces/${ws}/agents/${agentId}/restore`, {
      method: "POST",
    }),

  /* ── Skills ── */
  listSkills: (ws: string) =>
    fetchJSON<SkillsResponse>(`/workspaces/${ws}/agents/skills`),
  createSkill: (ws: string, data: CreateSkillRequest) =>
    fetchJSON<SkillResponse>(`/workspaces/${ws}/agents/skills`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updateSkill: (ws: string, skillId: string, data: UpdateSkillRequest) =>
    fetchJSON<SkillResponse>(`/workspaces/${ws}/agents/skills/${skillId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  deleteSkill: (ws: string, skillId: string) =>
    fetchJSON<void>(`/workspaces/${ws}/agents/skills/${skillId}`, {
      method: "DELETE",
    }),
  assignSkill: (ws: string, agentId: string, skillId: string) =>
    fetchJSON<void>(`/workspaces/${ws}/agents/${agentId}/skills`, {
      method: "POST",
      body: JSON.stringify({ skill_id: skillId }),
    }),
  unassignSkill: (ws: string, agentId: string, skillId: string) =>
    fetchJSON<void>(`/workspaces/${ws}/agents/${agentId}/skills/${skillId}`, {
      method: "DELETE",
    }),

  /* ── Tasks ── */
  listAgentTasks: (ws: string, agentId: string) =>
    fetchJSON<TasksResponse>(`/workspaces/${ws}/agents/${agentId}/tasks`),
  getAgentTask: (ws: string, agentId: string, taskId: string) =>
    fetchJSON<TaskResponse>(`/workspaces/${ws}/agents/${agentId}/tasks/${taskId}`),
  listTaskMessages: (taskId: string, since = 0) =>
    fetchJSON<TaskMessagesResponse>(`/tasks/${taskId}/messages?since=${since}`),
};

export type ApiClient = typeof api;
