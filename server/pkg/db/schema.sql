-- Mulwiki SQLite Schema

CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    active_schema_id TEXT REFERENCES schemas(id),
    active_schema_path TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS schemas (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL DEFAULT '1.0',
    path TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT 'user',
    derived_from TEXT REFERENCES schemas(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS workspace_members (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (workspace_id, user_id)
);

-- sources and wiki_pages are gone — all content is stored in per-workspace bare git repos.

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    agent_id TEXT NOT NULL DEFAULT '',
    source_path TEXT NOT NULL DEFAULT '',
    source_paths TEXT NOT NULL DEFAULT '[]',
    schema_id TEXT NOT NULL DEFAULT '',
    progress INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    claimed_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at TEXT
);

DROP TABLE IF EXISTS agents;

CREATE TABLE IF NOT EXISTS agent_runtimes (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    backend TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL DEFAULT '',
    os TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'offline',
    daemon_id TEXT NOT NULL DEFAULT '',
    last_heartbeat TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    runtime_id TEXT REFERENCES agent_runtimes(id),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    instructions TEXT NOT NULL DEFAULT '',
    runtime_mode TEXT NOT NULL DEFAULT '',
    runtime_config TEXT NOT NULL DEFAULT '{}',
    custom_env TEXT NOT NULL DEFAULT '{}',
    custom_args TEXT NOT NULL DEFAULT '[]',
    mcp_config TEXT NOT NULL DEFAULT '{}',
    visibility TEXT NOT NULL DEFAULT 'private',
    status TEXT NOT NULL DEFAULT 'offline',
    model TEXT NOT NULL DEFAULT '',
    max_concurrent_tasks INTEGER NOT NULL DEFAULT 6,
    owner_id TEXT REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    archived_at TEXT,
    archived_by TEXT REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS agent_skills (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS agent_skills_agents (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    skill_id TEXT NOT NULL REFERENCES agent_skills(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, skill_id)
);

CREATE TABLE IF NOT EXISTS agent_tasks (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    job_id TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    runtime_id TEXT REFERENCES agent_runtimes(id),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    source_path TEXT NOT NULL DEFAULT '',
    schema_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'dispatched', 'running', 'completed', 'failed', 'cancelled')),
    priority INTEGER NOT NULL DEFAULT 0,
    parent_task_id TEXT REFERENCES agent_tasks(id),
    session_id TEXT NOT NULL DEFAULT '',
    work_dir TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    daemon_id TEXT NOT NULL DEFAULT '',
    dispatched_at TEXT,
    started_at TEXT,
    completed_at TEXT,
    result TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    attempt INTEGER NOT NULL DEFAULT 1,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS agent_task_messages (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    task_id TEXT NOT NULL REFERENCES agent_tasks(id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'daemon',
    seq INTEGER NOT NULL DEFAULT 0,
    type TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    tool TEXT NOT NULL DEFAULT '',
    call_id TEXT NOT NULL DEFAULT '',
    input TEXT NOT NULL DEFAULT '{}',
    output TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    level TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS daemon_registrations (
    id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL DEFAULT '',
    pid INTEGER NOT NULL DEFAULT 0,
    version TEXT NOT NULL DEFAULT '',
    runtime_ids TEXT NOT NULL DEFAULT '[]',
    max_concurrent_tasks INTEGER NOT NULL DEFAULT 10,
    last_heartbeat TEXT NOT NULL DEFAULT '',
    registered_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS daemon_tokens (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL DEFAULT '',
    scope TEXT NOT NULL DEFAULT 'workspace',
    daemon_id TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_schemas_workspace ON schemas(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspaces_active_schema ON workspaces(active_schema_id);
CREATE INDEX IF NOT EXISTS idx_schemas_derived_from ON schemas(derived_from);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_workspace_members_user ON workspace_members(user_id);
CREATE INDEX IF NOT EXISTS idx_workspace_members_workspace ON workspace_members(workspace_id);
CREATE INDEX IF NOT EXISTS idx_jobs_workspace ON jobs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_agent_runtimes_workspace ON agent_runtimes(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agents_workspace ON agents(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agents_runtime ON agents(runtime_id);
CREATE INDEX IF NOT EXISTS idx_agents_owner ON agents(owner_id);
CREATE INDEX IF NOT EXISTS idx_agent_skills_workspace ON agent_skills(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agent_tasks_agent ON agent_tasks(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_tasks_job ON agent_tasks(job_id);
CREATE INDEX IF NOT EXISTS idx_agent_tasks_workspace ON agent_tasks(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agent_tasks_status ON agent_tasks(status);
CREATE INDEX IF NOT EXISTS idx_agent_task_messages_task ON agent_task_messages(task_id);
CREATE INDEX IF NOT EXISTS idx_agent_task_messages_workspace ON agent_task_messages(workspace_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_task_messages_task_seq ON agent_task_messages(task_id, seq) WHERE seq > 0;
CREATE INDEX IF NOT EXISTS idx_daemon_tokens_workspace ON daemon_tokens(workspace_id);
CREATE INDEX IF NOT EXISTS idx_daemon_tokens_daemon ON daemon_tokens(daemon_id);
