# Mulwiki Architecture Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring Mulwiki into alignment with the useful parts of Multica's evolved architecture while preserving Mulwiki's product shape: SQLite-first, schema-driven wiki ingest, single web app, and daemon-based local agent execution.

**Architecture:** Keep `Handler` as the HTTP composition root, but move durable rules into middleware, `internal/store`, and domain services. Do not copy Multica's PostgreSQL, Redis, desktop shell, or large-platform abstractions until Mulwiki has the product pressure that justifies them.

**Tech Stack:** Go 1.23, chi, `database/sql` with SQLite WAL, gorilla/websocket, Next.js 15, React 19, TanStack Query, pnpm workspaces.

---

## Reference Judgment

Multica patterns to borrow now:

- Router-level auth grouping: public routes, user-auth routes, daemon-auth routes, workspace-member routes, role-gated routes.
- Workspace membership as the real tenant boundary, not just `workspace_id` filters.
- `Handler` as a dependency container, with domain services for lifecycle logic.
- Task lifecycle owned by a service, including atomic claim, start, complete, fail, retry classification, and event publication.
- React Query as the source of truth for server state, with websocket events as invalidation signals.
- `packages/core` for headless API/query/realtime logic and `packages/views` for reusable page-level components.

Multica patterns to defer:

- PostgreSQL and `FOR UPDATE SKIP LOCKED`. Mulwiki can implement correct SQLite claims with `BEGIN IMMEDIATE`, conditional updates, and `RowsAffected` checks.
- Redis-backed request stores and multi-node realtime relays.
- Electron/desktop platform bridge.
- GORM or a broad ORM. If type-safe SQL becomes necessary, introduce sqlc for SQLite after the domain boundaries are stable.

Important correction to previous reviews:

- Mulwiki already has `server/pkg/agent.Backend` and daemon integration in `server/internal/daemon/daemon.go`. The remaining gap is not "create an agent backend abstraction"; it is "persist and expose the structured messages/session data that the backend already emits."

## Target Shape

Backend target:

```text
server/cmd/server/
  main.go                Router, dependency wiring, timeouts, process lifecycle

server/internal/auth/
  session.go             Cookie/session helpers used by handlers and middleware
  daemon_token.go        Hashed daemon token helpers, scoped to workspace + daemon

server/internal/middleware/
  auth.go                Validates user session/token, injects user context
  daemon_auth.go         Validates daemon token, injects daemon + workspace context
  workspace.go           Resolves workspace and membership from chi params/header/query
  role.go                Checks role from membership context

server/internal/store/
  workspace.go           Workspace and membership SQL
  agent.go               Agent scan, JSON decode/redaction helpers, skills loading
  task.go                Agent task SQL and SQLite transaction helpers
  job.go                 Job SQL

server/internal/service/
  task.go                Queue/lifecycle/retry/session/message rules
  workspace.go           Create/delete workspace with repo + membership coordination
  agent.go               Agent visibility/manage rules and mutation events

server/internal/handler/
  *.go                   Decode request, call service/store, write response
```

Frontend target:

```text
packages/core/
  api/client.ts          Fetch wrapper, cookie credentials, typed error surface
  query-client.ts        Shared TanStack Query defaults
  realtime/              WS client/provider and invalidation wiring
  workspace/queries.ts   Workspace query keys/options
  agents/queries.ts      Agent/runtimes/skills/tasks query keys/options
  jobs/queries.ts        Job query keys/options

packages/views/
  agents/                Agent page, panels, tabs, dialogs
  jobs/                  Job list/detail/log views
  schemas/               Schema list/editor views
  sources/               Source upload/list views
  wiki/                  Wiki list/detail views

apps/web/app/
  [workspaceSlug]/...    Thin Next route wrappers only
```

Primary data flow:

```text
User creates ingest job
  -> Job row records product request
  -> TaskService creates agent_task execution row
  -> Daemon claims agent_task by runtime/agent capacity
  -> Daemon starts task and executes server/pkg/agent backend
  -> Daemon streams structured task messages and pins session/workdir early
  -> TaskService completes/fails task and updates job status
  -> EventBus publishes task/job events
  -> WebSocket invalidates React Query caches
```

## Execution Order

### Task 1: Lock The Architecture Rules

**Files:**

- Modify: `AGENTS.md`
- Create: `docs/architecture/multica-alignment.md`

- [ ] **Step 1: Update architecture guidance**

  Replace claims that are not true yet. The new text must say:

  - `server/pkg/agent` already owns agent backend abstraction.
  - `server/internal/service` is the target home for lifecycle rules.
  - `server/internal/store` is the target home for handwritten SQLite access.
  - Auth, workspace membership, role checks, and daemon auth are required before routes mutate workspace-scoped data.
  - React Query owns server state; client stores are only for local UI state if introduced later.

- [ ] **Step 2: Add alignment doc**

  `docs/architecture/multica-alignment.md` should record the decisions in "Reference Judgment" and "Target Shape" above. It must explicitly reject a near-term PostgreSQL, Redis, desktop, GORM, or blanket Zustand migration.

- [ ] **Step 3: Verify no stale architecture claims remain**

  Run:

  ```bash
  rg "three-layer|Role chain|Backend abstraction|Repository|Zustand|PostgreSQL|Redis" AGENTS.md docs
  ```

  Expected: matches either describe implemented behavior accurately or mark it as future work.

- [ ] **Step 4: Commit checkpoint**

  ```bash
  git add AGENTS.md docs/architecture/multica-alignment.md
  git commit -m "docs: define mulwiki architecture alignment"
  ```

### Task 2: Make Tenant Boundaries Real

**Files:**

- Create: `server/internal/auth/session.go`
- Modify: `server/internal/handler/auth.go`
- Modify: `server/internal/middleware/auth.go`
- Modify: `server/internal/middleware/workspace.go`
- Modify: `server/internal/middleware/role.go`
- Modify: `server/internal/middleware/middleware_test.go`
- Modify: `server/internal/handler/workspace.go`
- Modify: `server/internal/handler/workspace_test.go`
- Modify: `server/pkg/db/schema.sql`
- Modify: `server/cmd/server/main.go`

- [ ] **Step 1: Add membership schema**

  Add this table to `server/pkg/db/schema.sql`:

  ```sql
  CREATE TABLE IF NOT EXISTS workspace_members (
      workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
      user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      role TEXT NOT NULL DEFAULT 'member'
          CHECK (role IN ('owner', 'admin', 'member')),
      created_at TEXT NOT NULL DEFAULT (datetime('now')),
      PRIMARY KEY (workspace_id, user_id)
  );

  CREATE INDEX IF NOT EXISTS idx_workspace_members_user ON workspace_members(user_id);
  CREATE INDEX IF NOT EXISTS idx_workspace_members_workspace ON workspace_members(workspace_id);
  ```

- [ ] **Step 2: Backfill local data safely**

  In `runMigrations`, after schema execution, add a one-time backfill:

  - If `workspace_members` is empty and at least one user exists, insert the oldest user as `owner` for every non-`builtin` workspace.
  - If no user exists, leave memberships empty. The first authenticated `CreateWorkspace` call will create the owner row.

- [ ] **Step 3: Centralize session auth**

  Move session cookie constants and helpers from `handler/auth.go` into `server/internal/auth/session.go`:

  ```go
  type SessionUser struct {
      ID        string
      Email     string
      CreatedAt string
  }

  func CreateSession(ctx context.Context, db *sql.DB, w http.ResponseWriter, userID string) error
  func UserFromRequest(ctx context.Context, db *sql.DB, r *http.Request) (*SessionUser, error)
  func ClearSession(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request)
  func UserIDFromContext(ctx context.Context) string
  func WithUser(ctx context.Context, user *SessionUser) context.Context
  ```

  `handler.Register`, `handler.Login`, `handler.Logout`, and `handler.Me` should call this package instead of duplicating session logic.

- [ ] **Step 4: Replace spoofable auth middleware**

  `middleware.Auth` must:

  - Read and validate the session cookie through `internal/auth.UserFromRequest`.
  - Inject user identity into request context.
  - Set `X-User-ID` only after validation for existing handler compatibility.
  - Never accept a user-supplied `X-User-ID` as authentication.
  - Return 401 when unauthenticated.

- [ ] **Step 5: Replace path-splitting workspace middleware**

  Replace global path parsing with route-scoped middleware functions:

  ```go
  func RequireWorkspaceMemberFromSlug(db *sql.DB, param string) func(http.Handler) http.Handler
  func RequireWorkspaceRoleFromSlug(db *sql.DB, param string, roles ...string) func(http.Handler) http.Handler
  func WorkspaceIDFromContext(ctx context.Context) string
  func WorkspaceSlugFromContext(ctx context.Context) string
  func MemberRoleFromContext(ctx context.Context) string
  ```

  These functions should use `chi.URLParam(r, param)` and query `workspace_members`.

- [ ] **Step 6: Wire routes like Multica**

  In `server/cmd/server/main.go`:

  - Keep `/api/auth/register`, `/api/auth/login`, `/api/auth/logout`, `/api/auth/me` public except that `/me` returns 401 without a valid session.
  - Put `/api/workspaces` list/create under `middleware.Auth`.
  - Put `/api/workspaces/{slug}` member-readable routes under `RequireWorkspaceMemberFromSlug`.
  - Put workspace update/delete and schema/agent/runtime mutations under `RequireWorkspaceRoleFromSlug(..., "owner", "admin")` unless a specific owner rule exists.
  - Remove the global `r.Use(middleware.Workspace(db))`.

- [ ] **Step 7: Create owner membership on workspace creation**

  `CreateWorkspace` must require an authenticated user and run workspace insert + `workspace_members` owner insert in one transaction. If bare repo initialization fails, rollback the DB transaction and remove any partially created repo directory.

- [ ] **Step 8: List only accessible workspaces**

  `ListWorkspaces` must return workspaces joined through `workspace_members` for the current user. Do not return all non-builtin workspaces.

- [ ] **Step 9: Verify**

  Run:

  ```bash
  cd server
  go test ./internal/middleware ./internal/handler -run 'Test(Auth|Workspace|Role|CreateWorkspace|ListWorkspaces)' -count=1
  ```

  Expected: tests cover unauthenticated 401, non-member 404/403, member read allowed, member admin mutation denied, owner mutation allowed.

- [ ] **Step 10: Commit checkpoint**

  ```bash
  git add server/internal/auth server/internal/middleware server/internal/handler server/pkg/db/schema.sql server/cmd/server/main.go
  git commit -m "feat: enforce workspace membership"
  ```

### Task 3: Add Daemon Authentication And Stable Identity

**Files:**

- Create: `server/internal/auth/daemon_token.go`
- Create: `server/internal/middleware/daemon_auth.go`
- Modify: `server/pkg/db/schema.sql`
- Modify: `server/internal/handler/daemon.go`
- Modify: `server/internal/handler/job.go`
- Modify: `server/internal/handler/agent.go`
- Modify: `server/internal/handler/job_test.go`
- Modify: `server/cmd/server/main.go`
- Modify: `server/cmd/mulwiki/cmd_daemon.go`
- Modify: `server/internal/daemon/daemon.go`

- [ ] **Step 1: Add daemon token schema**

  Add:

  ```sql
  CREATE TABLE IF NOT EXISTS daemon_tokens (
      id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
      workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
      daemon_id TEXT NOT NULL,
      token_hash TEXT NOT NULL UNIQUE,
      expires_at TEXT,
      created_at TEXT NOT NULL DEFAULT (datetime('now')),
      revoked_at TEXT
  );

  CREATE INDEX IF NOT EXISTS idx_daemon_tokens_workspace ON daemon_tokens(workspace_id);
  CREATE INDEX IF NOT EXISTS idx_daemon_tokens_daemon ON daemon_tokens(daemon_id);
  ```

- [ ] **Step 2: Persist daemon ID**

  Add a daemon ID file at `~/.mulwiki/daemon/daemon.id`. On daemon start:

  - Read existing ID if present.
  - Generate a UUID and save it if missing.
  - Use this ID in `DaemonRegister`, heartbeats, and task claim calls.

- [ ] **Step 3: Add token helpers**

  `internal/auth/daemon_token.go` should provide:

  ```go
  func NewDaemonToken() (raw string, hash string, err error)
  func HashDaemonToken(raw string) string
  func VerifyDaemonToken(ctx context.Context, db *sql.DB, raw string) (workspaceID, daemonID string, ok bool, err error)
  ```

  Token format should be `mwd_` + at least 32 random bytes encoded as hex or base64url.

- [ ] **Step 4: Add daemon auth middleware**

  `middleware.DaemonAuth(db)` should:

  - Require `Authorization: Bearer mwd_...`.
  - Resolve token to `workspace_id` and `daemon_id`.
  - Reject revoked or expired tokens.
  - Inject workspace and daemon identity into context.

- [ ] **Step 5: Add token issuance route**

  Add an authenticated route:

  ```text
  POST /api/workspaces/{slug}/daemon-tokens
  ```

  It requires owner/admin membership and returns the raw token once. The row stores only the hash.

- [ ] **Step 6: Protect daemon routes**

  Apply `middleware.DaemonAuth(db)` to `/api/daemon/*` and daemon-facing task endpoints. Handlers must verify that any runtime, job, or task they touch belongs to the daemon token workspace.

- [ ] **Step 7: Send token from CLI/daemon**

  `mulwiki daemon start` should read token from:

  - `--daemon-token`
  - `MULWIKI_DAEMON_TOKEN`
  - `~/.mulwiki/daemon/token`

  `daemon.HTTPClient` calls should set `Authorization`.

- [ ] **Step 8: Verify**

  Run:

  ```bash
  cd server
  go test ./internal/middleware ./internal/handler ./internal/daemon -run 'Test(Daemon|Token|Claim)' -count=1
  ```

  Expected: missing token is 401, wrong workspace token is denied, valid token can register and claim only same-workspace tasks.

- [ ] **Step 9: Commit checkpoint**

  ```bash
  git add server/internal/auth server/internal/middleware server/internal/handler server/internal/daemon server/cmd server/pkg/db/schema.sql
  git commit -m "feat: add workspace-scoped daemon auth"
  ```

### Task 4: Move Queue Rules Into TaskService

**Files:**

- Create: `server/internal/store/task.go`
- Create: `server/internal/service/task.go`
- Modify: `server/internal/service/job.go`
- Modify: `server/internal/handler/job.go`
- Modify: `server/internal/handler/agent.go`
- Modify: `server/internal/handler/handler.go`
- Modify: `server/internal/service/job_test.go`
- Create: `server/internal/service/task_test.go`
- Modify: `server/internal/handler/job_test.go`
- Modify: `server/internal/handler/agent_test.go`

- [ ] **Step 1: Define TaskService responsibilities**

  `TaskService` owns:

  ```go
  type TaskService struct {
      DB *sql.DB
      Bus *events.Bus
  }

  func (s *TaskService) CreateTaskForJob(ctx context.Context, jobID, workspaceID, agentID, runtimeID, sourcePath, schemaID string) (*protocol.AgentTask, error)
  func (s *TaskService) ClaimNext(ctx context.Context, workspaceID, runtimeID, daemonID string) (*protocol.AgentTask, error)
  func (s *TaskService) Start(ctx context.Context, taskID, workspaceID string) (*protocol.AgentTask, error)
  func (s *TaskService) Complete(ctx context.Context, taskID, workspaceID, result, sessionID, workDir string) (*protocol.AgentTask, error)
  func (s *TaskService) Fail(ctx context.Context, taskID, workspaceID, reason, message, sessionID, workDir string) (*protocol.AgentTask, error)
  func (s *TaskService) Cancel(ctx context.Context, taskID, workspaceID string) (*protocol.AgentTask, error)
  func (s *TaskService) RecoverOrphans(ctx context.Context, workspaceID, daemonID string) ([]protocol.AgentTask, error)
  ```

- [ ] **Step 2: Implement SQLite atomic claim**

  Use a transaction with `BEGIN IMMEDIATE` semantics. With `database/sql`, use:

  ```go
  tx, err := db.BeginTx(ctx, &sql.TxOptions{})
  ```

  Then execute a dummy write or claim update as the first write in the transaction. The claim should:

  - Select the oldest queued task matching workspace/runtime capacity.
  - Update only where `status = 'queued'`.
  - Check `RowsAffected == 1`.
  - Commit before returning the task.
  - Return `nil, nil` when no claim is available.

  Do not return 204 for unexpected SQL errors.

- [ ] **Step 3: Make jobs product records, tasks execution records**

  `CreateJob` should create a job and an agent task together. The daemon should claim `agent_tasks`, not the old `jobs/claim` path. Keep `jobs/claim` only as a compatibility wrapper that calls `TaskService.ClaimNext` if existing daemon code still uses it during the same branch.

- [ ] **Step 4: Publish lifecycle events in service**

  `TaskService` should publish `task.dispatched`, `task.started`, `task.completed`, `task.failed`, and `task.cancelled`. Handlers should not duplicate event publication.

- [ ] **Step 5: Verify concurrency**

  Add a test that starts 10 goroutines calling `ClaimNext` against one queued task. Expected result: exactly one goroutine gets the task; the rest get nil; the task status is `dispatched`.

- [ ] **Step 6: Verify lifecycle**

  Add tests:

  - `Start` only moves `dispatched -> running`.
  - `Complete` only moves `running -> completed`.
  - `Fail` preserves `session_id` and `work_dir` when provided.
  - `RecoverOrphans` marks this daemon's `dispatched/running` tasks failed with `failure_reason = 'runtime_recovery'`.

- [ ] **Step 7: Run tests**

  ```bash
  cd server
  go test ./internal/service ./internal/handler -run 'Test(Task|Job|Claim)' -count=1
  ```

- [ ] **Step 8: Commit checkpoint**

  ```bash
  git add server/internal/store/task.go server/internal/service server/internal/handler
  git commit -m "feat: centralize task lifecycle"
  ```

### Task 5: Persist Agent Messages And Session Pointers

**Files:**

- Modify: `server/pkg/db/schema.sql`
- Modify: `server/internal/service/task.go`
- Modify: `server/internal/handler/daemon.go`
- Modify: `server/internal/handler/agent.go`
- Modify: `server/internal/daemon/daemon.go`
- Modify: `server/pkg/protocol/models.go`
- Modify: `server/pkg/protocol/models_test.go`
- Create: `server/internal/service/task_message_test.go`
- Modify: `apps/web/app/[workspaceSlug]/jobs/page.tsx`

- [ ] **Step 1: Add task messages table**

  Add:

  ```sql
  CREATE TABLE IF NOT EXISTS agent_task_messages (
      id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
      task_id TEXT NOT NULL REFERENCES agent_tasks(id) ON DELETE CASCADE,
      seq INTEGER NOT NULL,
      type TEXT NOT NULL,
      content TEXT NOT NULL DEFAULT '',
      tool TEXT NOT NULL DEFAULT '',
      call_id TEXT NOT NULL DEFAULT '',
      input TEXT NOT NULL DEFAULT '{}',
      output TEXT NOT NULL DEFAULT '',
      status TEXT NOT NULL DEFAULT '',
      level TEXT NOT NULL DEFAULT '',
      session_id TEXT NOT NULL DEFAULT '',
      created_at TEXT NOT NULL DEFAULT (datetime('now')),
      UNIQUE(task_id, seq)
  );

  CREATE INDEX IF NOT EXISTS idx_agent_task_messages_task_seq ON agent_task_messages(task_id, seq);
  ```

- [ ] **Step 2: Add service APIs**

  Add:

  ```go
  func (s *TaskService) AppendMessages(ctx context.Context, workspaceID, taskID string, messages []protocol.AgentTaskMessage) error
  func (s *TaskService) ListMessages(ctx context.Context, workspaceID, taskID string, sinceSeq int64) ([]protocol.AgentTaskMessage, error)
  func (s *TaskService) PinSession(ctx context.Context, workspaceID, taskID, sessionID, workDir string) error
  ```

- [ ] **Step 3: Add daemon HTTP endpoints**

  Add protected daemon endpoints:

  ```text
  POST /api/daemon/tasks/{taskId}/messages
  POST /api/daemon/tasks/{taskId}/session
  GET  /api/tasks/{taskId}/messages?since=0
  ```

  The user-facing `GET` must verify workspace membership. Daemon `POST` must verify daemon-token workspace.

- [ ] **Step 4: Batch messages in daemon**

  In `runAgentAttempt`, replace one-log-line-per-message with a 500ms batcher:

  - Assign monotonic `seq` per task.
  - Append messages to the server in batches.
  - Still mirror text/error/log messages into the current job log buffer for SSE compatibility.
  - When `msg.SessionID != ""`, call `PinSession` once immediately.

- [ ] **Step 5: Complete/fail with session data**

  `markTaskCompleted` and `markTaskFailed` should send `session_id` and `work_dir` from the backend result when available. `TaskService.Fail` should preserve them for retry/resume.

- [ ] **Step 6: Verify**

  ```bash
  cd server
  go test ./internal/service ./internal/handler ./internal/daemon -run 'Test(TaskMessage|PinSession|Session)' -count=1
  ```

  Expected: messages persist in sequence, `since` filters correctly, session pin works before completion, and duplicate `seq` insertion is idempotent or returns a handled conflict.

- [ ] **Step 7: Commit checkpoint**

  ```bash
  git add server apps/web/app/[workspaceSlug]/jobs/page.tsx
  git commit -m "feat: persist task messages and sessions"
  ```

### Task 6: Extract Store Helpers Before Broad Handler Refactors

**Files:**

- Create: `server/internal/store/workspace.go`
- Create: `server/internal/store/agent.go`
- Create: `server/internal/store/job.go`
- Modify: `server/internal/handler/handler.go`
- Modify: `server/internal/handler/agent.go`
- Modify: `server/internal/handler/job.go`
- Modify: `server/internal/handler/runtime.go`
- Modify: `server/internal/handler/skill.go`
- Create: `server/internal/store/agent_test.go`
- Create: `server/internal/store/workspace_test.go`

- [ ] **Step 1: Add small stores, not a generic repository framework**

  Create focused structs:

  ```go
  type WorkspaceStore struct { DB *sql.DB }
  type AgentStore struct { DB *sql.DB }
  type JobStore struct { DB *sql.DB }
  ```

  Each store should expose methods used by handlers/services. Do not add interfaces until tests need them.

- [ ] **Step 2: Centralize agent scanning**

  `AgentStore` should own:

  ```go
  func (s *AgentStore) ListByWorkspace(ctx context.Context, workspaceID string) ([]protocol.Agent, error)
  func (s *AgentStore) GetInWorkspace(ctx context.Context, workspaceID, agentID string) (*protocol.Agent, error)
  func (s *AgentStore) LoadSkillsForAgents(ctx context.Context, agentIDs []string) (map[string][]protocol.AgentSkill, error)
  ```

  JSON decode failures for `custom_env`, `custom_args`, and `mcp_config` must return errors. Do not silently replace corrupted JSON with empty maps/slices.

- [ ] **Step 3: Remove repeated workspace ID lookups**

  Handlers under workspace middleware should read `workspaceID` from context. Direct `SELECT id FROM workspaces WHERE slug = ?` in handlers should be removed unless the route is not middleware-protected.

- [ ] **Step 4: Check row iteration errors**

  Every `rows.Next()` loop moved into stores must check `rows.Err()` before returning.

- [ ] **Step 5: Run tests**

  ```bash
  cd server
  go test ./internal/store ./internal/handler -count=1
  ```

- [ ] **Step 6: Commit checkpoint**

  ```bash
  git add server/internal/store server/internal/handler
  git commit -m "refactor: extract sqlite store helpers"
  ```

### Task 7: Move Frontend Server State To Core Query Options

**Files:**

- Create: `packages/core/query-client.ts`
- Create: `packages/core/workspace/queries.ts`
- Create: `packages/core/agents/queries.ts`
- Create: `packages/core/jobs/queries.ts`
- Modify: `packages/core/api/index.ts`
- Modify: `packages/core/package.json`
- Modify: `apps/web/app/[workspaceSlug]/layout.tsx`
- Modify: `apps/web/app/[workspaceSlug]/agents/page.tsx`
- Modify: `apps/web/app/[workspaceSlug]/agents/[id]/page.tsx`
- Modify: `apps/web/app/[workspaceSlug]/jobs/page.tsx`

- [ ] **Step 1: Add shared QueryClient defaults**

  `packages/core/query-client.ts`:

  ```ts
  import { QueryClient } from "@tanstack/react-query";

  export function createQueryClient() {
    return new QueryClient({
      defaultOptions: {
        queries: {
          staleTime: 30_000,
          gcTime: 10 * 60 * 1000,
          refetchOnWindowFocus: true,
          refetchOnReconnect: true,
          retry: 1,
        },
        mutations: {
          retry: false,
        },
      },
    });
  }
  ```

  Keep `staleTime` finite until WebSocket invalidation is wired.

- [ ] **Step 2: Add domain query keys**

  Use workspace ID or slug consistently in keys:

  ```ts
  export const agentKeys = {
    all: (ws: string) => ["workspaces", ws, "agents"] as const,
    list: (ws: string) => [...agentKeys.all(ws), "list"] as const,
    detail: (ws: string, agentId: string) => [...agentKeys.all(ws), agentId] as const,
    tasks: (ws: string, agentId: string) => [...agentKeys.detail(ws, agentId), "tasks"] as const,
  };
  ```

  Repeat for jobs, schemas, sources, runtimes, and skills as they are touched.

- [ ] **Step 3: Move hooks to query options**

  New code should prefer:

  ```ts
  export function agentListOptions(ws: string) {
    return queryOptions({
      queryKey: agentKeys.list(ws),
      queryFn: () => api.listAgents(ws),
      select: (data) => data.agents,
    });
  }
  ```

  The existing `packages/core/hooks/index.ts` can remain as compatibility wrappers that call `useQuery(agentListOptions(ws))`.

- [ ] **Step 4: Keep server data out of client stores**

  Do not add Zustand for agents, jobs, schemas, sources, wiki pages, tasks, runtimes, or skills. Use local React state for selected tabs, filters, dialog open state, and search text.

- [ ] **Step 5: Verify typecheck**

  ```bash
  pnpm typecheck
  ```

- [ ] **Step 6: Commit checkpoint**

  ```bash
  git add packages/core apps/web/app
  git commit -m "refactor: centralize frontend query options"
  ```

### Task 8: Split Large App Route Components Into Views

**Files:**

- Create: `packages/views/agents/agents-page.tsx`
- Create: `packages/views/agents/agent-list.tsx`
- Create: `packages/views/agents/agent-detail-panel.tsx`
- Create: `packages/views/agents/agent-create-panel.tsx`
- Create: `packages/views/agents/tabs/instructions-tab.tsx`
- Create: `packages/views/agents/tabs/skills-tab.tsx`
- Create: `packages/views/agents/tabs/tasks-tab.tsx`
- Create: `packages/views/agents/tabs/env-tab.tsx`
- Create: `packages/views/agents/tabs/settings-tab.tsx`
- Modify: `apps/web/app/[workspaceSlug]/agents/page.tsx`
- Modify: `apps/web/app/[workspaceSlug]/agents/[id]/page.tsx`
- Modify: `packages/views/package.json`

- [ ] **Step 1: Move component code without changing behavior**

  Extract the current panels from `apps/web/app/[workspaceSlug]/agents/page.tsx`. The Next route should only:

  - Resolve `workspaceSlug` from params.
  - Read/write URL search params if needed.
  - Render `<AgentsPage workspaceSlug={workspaceSlug} />`.

- [ ] **Step 2: Keep framework imports out of `packages/views`**

  `packages/views` must not import:

  - `next/navigation`
  - `next/link`
  - App route files

  Pass navigation callbacks from the app wrapper or use plain props.

- [ ] **Step 3: Use query options from `packages/core`**

  View components should read data with query options from `packages/core/agents/queries.ts`. They should not hand-build query keys.

- [ ] **Step 4: Verify UI behavior manually**

  Start the app:

  ```bash
  pnpm --filter @mulwiki/web dev
  ```

  Open `/[workspaceSlug]/agents`, create an agent, select an agent, update instructions, assign/unassign a skill, and view tasks.

- [ ] **Step 5: Verify build/typecheck**

  ```bash
  pnpm typecheck
  pnpm build
  ```

- [ ] **Step 6: Commit checkpoint**

  ```bash
  git add packages/views/agents apps/web/app/[workspaceSlug]/agents packages/core
  git commit -m "refactor: move agent views into shared package"
  ```

### Task 9: Wire WebSocket Invalidation

**Files:**

- Create: `packages/core/realtime/ws-client.ts`
- Create: `packages/core/realtime/provider.tsx`
- Create: `packages/core/realtime/use-realtime-sync.ts`
- Modify: `packages/core/package.json`
- Modify: `apps/web/app/[workspaceSlug]/layout.tsx`
- Modify: `server/internal/realtime/hub.go`
- Modify: `server/internal/events/bus.go`
- Modify: `server/internal/realtime/hub_test.go`

- [ ] **Step 1: Confirm event payload contract**

  Backend websocket events should include:

  ```json
  {
    "type": "task.completed",
    "workspace_id": "workspace-id",
    "agent_id": "agent-id",
    "task_id": "task-id",
    "payload": {}
  }
  ```

  If current event names use dots, keep dots for backend and map prefixes in frontend with `type.split(".")[0]`.

- [ ] **Step 2: Add minimal WS client**

  The client should connect to `/ws?workspace=${workspaceSlug}` and support:

  ```ts
  on(eventType: string, handler: (payload: unknown) => void): () => void
  onAny(handler: (event: RealtimeEvent) => void): () => void
  reconnect with backoff
  close on unmount
  ```

- [ ] **Step 3: Add query invalidation map**

  `useRealtimeSync` should invalidate:

  - `task.*` -> agent tasks, job list/detail, job logs/messages.
  - `daemon.*` -> runtimes and daemon list.
  - `agent.*` -> agent list/detail.
  - `schema.*` -> schema list/detail.
  - `source.*` -> source list.
  - `wiki.*` -> wiki list/detail.

- [ ] **Step 4: Replace polling where safe**

  Keep polling for daemon heartbeat/runtimes if useful, but remove redundant short polling from tasks once websocket invalidation is verified.

- [ ] **Step 5: Verify**

  Run:

  ```bash
  pnpm typecheck
  cd server
  go test ./internal/realtime ./internal/events -count=1
  ```

- [ ] **Step 6: Commit checkpoint**

  ```bash
  git add packages/core/realtime apps/web/app/[workspaceSlug]/layout.tsx server/internal/realtime server/internal/events
  git commit -m "feat: invalidate queries from realtime events"
  ```

### Task 10: Add Server Runtime Hardening Without Breaking Streams

**Files:**

- Modify: `server/cmd/server/main.go`
- Modify: `server/internal/handler/job.go`
- Modify: `server/internal/realtime/hub.go`
- Modify: `server/internal/realtime/hub_test.go`

- [ ] **Step 1: Add HTTP server timeouts carefully**

  Configure:

  ```go
  srv := &http.Server{
      Addr:              ":" + port,
      Handler:           r,
      ReadHeaderTimeout: 10 * time.Second,
      ReadTimeout:       30 * time.Second,
      IdleTimeout:       60 * time.Second,
  }
  ```

  Do not add a short global `WriteTimeout` while SSE logs and WebSocket connections are long-lived.

- [ ] **Step 2: Make SSE log streams context-aware**

  Ensure `StreamJobLogs` exits when `r.Context().Done()` closes and does not leak goroutines.

- [ ] **Step 3: Verify**

  ```bash
  cd server
  go test ./cmd/server ./internal/handler ./internal/realtime -run 'Test(Health|Stream|WebSocket|Timeout)' -count=1
  ```

- [ ] **Step 4: Commit checkpoint**

  ```bash
  git add server/cmd/server/main.go server/internal/handler/job.go server/internal/realtime
  git commit -m "chore: harden server timeouts and streams"
  ```

## Success Criteria

- Workspace list and all workspace-scoped reads/mutations are impossible without an authenticated user and workspace membership.
- Owner/admin/member role checks are enforced by middleware, not individual handler habits.
- Daemon endpoints require workspace-scoped daemon tokens.
- A queued task can be claimed by only one daemon attempt under concurrent claim pressure.
- Agent task messages and session pointers survive daemon failure.
- Handlers stop owning repeated SQL scan/JSON decode logic.
- `agents/page.tsx` and `agents/[id]/page.tsx` become thin route wrappers or focused view files.
- Frontend server state remains in TanStack Query, with websocket events invalidating query caches.
- The server has slow-client protection without breaking SSE or WebSocket behavior.

## Explicit Non-Goals

- No PostgreSQL migration in this plan.
- No Redis realtime relay in this plan.
- No desktop app or platform bridge in this plan.
- No ORM adoption in this plan.
- No Zustand for API/server data.
- No broad rewrite of schema/source/wiki handlers until auth, task lifecycle, and store extraction are stable.

## Suggested Milestones

- Milestone A: Tasks 1-3. Security boundary is real.
- Milestone B: Tasks 4-6. Backend lifecycle and data access are coherent.
- Milestone C: Tasks 7-9. Frontend follows Multica's core/view/query/realtime split.
- Milestone D: Task 10. Runtime hardening.

## Self-Review

- Scope check: This is a roadmap with independent milestones. Execute one milestone at a time; do not run all tasks in one branch unless the branch is explicitly dedicated to architecture alignment.
- Placeholder scan: No task depends on an unspecified future decision. Deferred systems are listed under non-goals.
- Type consistency: Backend names use `agent_tasks`, `agent_task_messages`, `workspace_members`, and `daemon_tokens`; frontend query keys consistently use workspace slug or ID through the calling API layer.
- Multica fit: The plan borrows Multica's proven boundaries and queue rules without copying infrastructure that Mulwiki does not currently need.
