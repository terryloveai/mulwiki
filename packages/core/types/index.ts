export interface Workspace {
  id: string;
  slug: string;
  name: string;
  description: string;
  active_schema_id?: string;
  active_schema_path?: string;
  created_at: string;
}

export interface Schema {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  version: string;
  path: string;
  content: string;
  source_type: "builtin" | "user";
  derived_from?: string;
  created_at: string;
}

/** Schema as returned by ListSchemas — includes is_active flag */
export interface SchemaWithActive extends Schema {
  is_active: boolean;
}

export interface CreateSchemaRequest {
  name: string;
  description?: string;
  version?: string;
  content: string;
  source_type?: "user" | "builtin";
  derived_from?: string;
}

export interface UpdateSchemaRequest {
  name?: string;
  description?: string;
  version?: string;
  content?: string;
}

export interface ForkSchemaRequest {
  schema_id: string;
  name?: string;
  description?: string;
}

export interface ValidateSchemaResponse {
  valid: boolean;
  errors?: string[];
  warnings?: string[];
}

export interface User {
  id: string;
  email: string;
  created_at: string;
}

/** Source — git-backed. Path is the canonical identifier. */
export interface Source {
  name: string;
  type: string;
  path: string;   // e.g. "sources/doc.pdf"
  size: number;
}

/** WikiPage — git-backed markdown with frontmatter. */
export interface WikiPage {
  path: string;
  title: string;
  content: string;
  type: string;
  layer?: string;
}

/** WikiBacklink — a page that references another page via [[wikilink]]. */
export interface WikiBacklink {
  path: string;
  title: string;
  snippet: string;
}

/** ResolveWikiLinks request/response for batch checking [[wikilink]] targets. */
export interface ResolveWikiLinksRequest {
  paths: string[];
}

export interface WikiLinkResolve {
  exists: boolean;
  path?: string;
}

export interface ResolveWikiLinksResponse {
  resolved: Record<string, WikiLinkResolve>;
}

export interface Job {
  id: string;
  workspace_id: string;
  status: "pending" | "running" | "completed" | "failed";
  agent_id?: string;
  source_path?: string;
  source_paths: string[];
  schema_id: string;
  progress: number;
  error?: string;
  claimed_by?: string;
  created_at: string;
  completed_at?: string;
}

/* ── Agent Runtime ── */
export interface AgentRuntime {
  id: string;
  workspace_id?: string;
  name: string;
  backend: string;
  path: string;
  hostname: string;
  os: string;
  version: string;
  status: "online" | "offline";
  daemon_id: string;
  last_heartbeat: string;
  created_at: string;
}

/* ── Agent Skill ── */
export interface AgentSkill {
  id: string;
  name: string;
  description: string;
}

/* ── Agent (Multica model) ── */
export interface Agent {
  id: string;
  workspace_id?: string;
  runtime_id: string;
  name: string;
  description: string;
  instructions: string;
  runtime_mode: string;
  runtime_config: string;
  custom_env: Record<string, string>;
  custom_args: string[];
  mcp_config: string;
  visibility: "private" | "public";
  status: "online" | "offline" | "archived";
  max_concurrent_tasks: number;
  model: string;
  skills: AgentSkill[];
  created_at: string;
  updated_at: string;
  archived_at?: string;
  archived_by?: string;
}

/* ── Agent Task ── */
export interface AgentTask {
  id: string;
  agent_id: string;
  runtime_id?: string;
  workspace_id?: string;
  source_path?: string;
  schema_id?: string;
  status: "queued" | "dispatched" | "started" | "running" | "completed" | "failed" | "cancelled";
  priority: number;
  parent_task_id?: string;
  session_id?: string;
  work_dir?: string;
  failure_reason?: string;
  daemon_id?: string;
  dispatched_at?: string;
  started_at?: string;
  completed_at?: string;
  result?: string;
  error?: string;
  attempt: number;
  max_attempts: number;
  created_at: string;
  messages?: AgentTaskMessage[];
}

export interface AgentTaskMessage {
  id: string;
  task_id: string;
  workspace_id: string;
  agent_id: string;
  role: string;
  seq: number;
  type: string;
  content: string;
  tool: string;
  call_id: string;
  input: Record<string, unknown>;
  output: string;
  status: string;
  level: string;
  session_id: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface TaskMessagesResponse {
  messages: AgentTaskMessage[];
}

/* ── Request / Response shapes ── */
export interface CreateRuntimeRequest {
  name: string;
  backend: string;
  path: string;
  hostname?: string;
  os?: string;
  version?: string;
}

export interface UpdateRuntimeRequest {
  name?: string;
  backend?: string;
  path?: string;
  hostname?: string;
  os?: string;
  version?: string;
}

export interface CreateAgentRequest {
  name: string;
  description?: string;
  instructions?: string;
  runtime_id: string;
  runtime_config?: string;
  custom_env?: Record<string, string>;
  custom_args?: string[];
  mcp_config?: string;
  visibility?: "private" | "public";
  max_concurrent_tasks?: number;
  model?: string;
}

export interface UpdateAgentRequest {
  name?: string;
  description?: string;
  instructions?: string;
  runtime_id?: string;
  runtime_config?: string;
  custom_env?: Record<string, string>;
  custom_args?: string[];
  mcp_config?: string;
  visibility?: "private" | "public";
  max_concurrent_tasks?: number;
  model?: string;
}

export interface CreateSkillRequest {
  name: string;
  description: string;
}

export interface UpdateSkillRequest {
  name?: string;
  description?: string;
}

export interface RuntimesResponse {
  runtimes: AgentRuntime[];
}

export interface RuntimeResponse {
  runtime: AgentRuntime;
}

export interface AgentsResponse {
  agents: Agent[];
}

export interface AgentResponse {
  agent: Agent;
}

export interface SkillsResponse {
  skills: AgentSkill[];
}

export interface SkillResponse {
  skill: AgentSkill;
}

export interface TasksResponse {
  tasks: AgentTask[];
}

export interface TaskResponse {
  task: AgentTask;
}
