# Mulwiki

Compile raw documents into a structured wiki knowledge base — multi-tenant, schema-driven, agent-powered.

## Architecture

```
mulwiki/
├── apps/web/          Next.js 15 frontend (App Router)
├── packages/
│   ├── core/          Shared types, API client, hooks
│   ├── ui/            Design system: tokens, components, hooks
│   └── views/         Page-level components
├── server/
│   ├── cmd/server/    HTTP server (chi router)
│   ├── cmd/mulwiki/   CLI (login/setup + daemon subcommands)
│   ├── builtin/       Built-in schemas and skills shipped with the server
│   ├── internal/
│   │   ├── handler/   HTTP handlers
│   │   ├── daemon/    Daemon loop + agent job execution
│   │   ├── service/   Business logic
│   │   ├── middleware/ Auth → Workspace → Role chain
│   │   ├── events/    In-process event bus
│   │   └── realtime/  WebSocket hub (room-based pub/sub)
│   └── pkg/
│       ├── db/         SQL schema
│       └── protocol/   Shared types
└── scripts/           Dev helpers
```

## Architecture Guardrails

Mulwiki intentionally follows Multica's boundary shape, not its full stack:

- Design durable core business capabilities in this order: Server API, CLI, then Web UI. The server API is the authoritative capability layer; CLI is for automation, scripting, daemon workflows, and advanced users; Web UI is for interaction, visualization, and low-friction access.
- Before adding a feature, identify the server API that owns it, the CLI command that exposes it for scripts/CI/agents, and the Web UI that calls the same API. Do not create Web-only business capabilities unless the behavior is purely presentational.
- CLI commands for durable capabilities should be non-interactive by default, support `--output json` where automation needs it, and return non-zero on failures.
- Keep chi route groups as the backend composition boundary. Public auth and health routes are explicit; workspace routes must run through Auth, Workspace, then Role middleware.
- Treat `workspace_members` as the tenant boundary. A request is workspace-scoped only after membership has been resolved into context.
- Keep handlers thin. Handlers decode HTTP, call service/store code, publish events, and serialize responses.
- Put lifecycle rules in services. Claiming, dispatching, completing, failing, and retrying jobs or agent tasks should not be duplicated in handlers and daemon code.
- Keep schema Markdown decoupled from agents. Ingest combines Sources + Schema + Agent at job/task creation time.
- Use `packages/core` for API clients, query keys, query options, hooks, and shared types. Use `packages/views` for page-level React components. `apps/web/app` route files should be thin route adapters.
- Use React Query as the source of truth for server state. Do not introduce a separate client store for data fetched from the API.

Do not port Multica details that do not fit Mulwiki's current shape: PostgreSQL, Redis queues, desktop window orchestration, GORM, or model selection at runtime level.

## Core Concepts

### Workspace
Isolated knowledge base. Contains Sources, Schemas, Agents, Wiki pages.

### Source
Raw documents uploaded by the user. Read-only to agents. Supports PDF, DOCX, PPTX, XLSX, Markdown, plain text, images, and URLs.

### Schema
A Markdown file defining how knowledge is organized. Five dimensions:
- **Types**: What kinds of wiki pages exist
- **Structure**: How pages connect (strict hierarchy / knowledge graph / free links)
- **Frontmatter**: Required YAML metadata per page type
- **Ingest Pipeline**: Step-by-step document → wiki workflow
- **Lint Rules**: Quality checks (orphans, contradictions, evidence chains)

6 built-in schemas in `server/builtin/schemas/`. Users can create custom schemas or fork built-in ones. Schema files are pure Markdown — no agent references, no pipeline routing config.

### Agent Runtime
An execution environment for agents. Represents an installed agent CLI daemon on a physical machine. Each Runtime tracks: backend type (claude-code / codex / kimi / custom), CLI path, hostname, OS, version, daemon_id (which daemon manages it), last_heartbeat (liveness), online/offline status.

Model selection is at the Agent level, not Runtime — a single Runtime (daemon) can serve multiple Agents with different models.

Runtime lifecycle: daemon registers itself and its runtimes on startup, then sends periodic heartbeats. The server marks runtimes as offline when heartbeats stop.

### Agent
A configured agent bound to a Runtime. Six configuration dimensions:
- **Runtime binding**: Which Runtime this agent executes on
- **Instructions**: System-level prompt defining agent behavior
- **Skills**: Reusable capability modules (document parsing, concept extraction, etc.)
- **Tasks**: Execution history (status, duration, output stats, errors)
- **Environment**: Environment variables (API keys, workdir paths, model names)
- **Custom Args**: Extra CLI arguments passed to the runtime
- **Settings**: Model selection, max concurrent tasks, visibility (private/public)

Agent and Schema are fully decoupled. An ingest job combines: Sources + Schema → Agent → Wiki.

### Job
An ingest execution. Combines a set of Source documents, a Schema, and an Agent. The daemon creates an isolated workdir, writes the Schema as AGENTS.md, and forks the Agent's Runtime CLI as a subprocess. Status flow: queued → dispatched → running → completed / failed / cancelled.

### Agent Task
Execution record. Every time an Agent handles a job, a task record is created. Visible in the Agent's Tasks panel. Tracks status (queued → dispatched → running → completed/failed/cancelled), timing, source/schema used, pages created/updated, errors.

Key fields for reliability:
- **parent_task_id**: Links to the task that spawned this retry, forming a retry chain
- **failure_reason**: Structured failure classifier (timeout, runtime_offline, agent_error, etc.) — drives automatic retry decisions
- **session_id / work_dir**: Supports task resume across daemon restarts
- **daemon_id**: Which daemon instance claimed and is executing this task
- **attempt / max_attempts**: Retry count and limit

### Daemon Registration
When the daemon starts, it registers itself with the server: daemon ID, hostname, PID, version, which runtimes it manages, and max concurrent task capacity. The server stores this in `daemon_registrations` and links each runtime's `daemon_id` and `last_heartbeat`. Heartbeats every 30s keep the registration alive; stale daemons (no heartbeat for 5min) have their runtimes marked offline.

The local CLI stores its server URL, default workspace slug, and session cookie in `~/.mulwiki/config.json`.
`mulwiki daemon start` can use that login state to mint and cache a workspace-scoped daemon token automatically, so users do not have to pass `MULWIKI_DAEMON_TOKEN` manually during normal setup.

### EventBus
In-process event system (`internal/events/bus.go`). Handlers publish typed events (task.dispatched, task.started, task.completed, task.failed, daemon.online, daemon.offline). The EventBus fans out to all subscribers — primarily the WebSocket Hub for real-time UI updates, but also internal services (analytics, stale detection).

### WebSocket Hub
Room-based publish/subscribe (`internal/realtime/hub.go`). Clients connect via `GET /ws?workspace=slug` and auto-subscribe to `workspace:{slug}` and `agent:{id}` rooms. Non-blocking sends with buffer 256; slow clients are evicted. The Hub receives events from EventBus and fans out to connected WebSocket clients.

### Middleware Chain
Three-layer middleware stack (Auth → Workspace → Role):
- **RequireAuth**: Reads session cookie/token, injects user identity into request context
- **RequireWorkspace**: Resolves workspace from URL slug, validates membership
- **RequireRole**: Checks user's role (owner/admin/member) for access control

### Wiki
The compiled output. A directory of interconnected Markdown pages organized by the Schema's type system. Includes an index, a log, and typed subdirectories. Exportable as a zip (Obsidian-compatible).

## Stack

- **Frontend**: Next.js 15, React 19, Tailwind CSS (oklch), React Query
- **Backend**: Go, chi router, SQLite (go-sqlite3)
- **Agent**: Daemon pattern — polls server for jobs, forks configured agent CLI as subprocess
- **Monorepo**: pnpm workspaces + turborepo

## Quick Start

```bash
# Install dependencies
pnpm install

# Start Go backend
cd server && go run ./cmd/server

# Start frontend (separate terminal)
cd apps/web && pnpm dev
```

## Route Map

| Path | Purpose |
|------|---------|
| `/` | Landing / redirect |
| `/login` / `/register` | Auth |
| `/workspaces` | Workspace list + create |
| `/[slug]/wiki` | Wiki index |
| `/[slug]/wiki/[...path]` | Wiki page detail |
| `/[slug]/sources` | Source upload + list |
| `/[slug]/schemas` | Schema list + create/edit |
| `/[slug]/agents` | Agent list (Runtimes + Agents + Skills + Tasks) |
| `/[slug]/jobs` | Job list + create + logs |
| `/[slug]/settings` | Workspace settings |
| `/api/*` → Go `:8080` | Backend proxy (Next.js rewrites) |
| `GET /ws` | WebSocket (real-time task status, agent state) |

### Internal API Routes

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/daemon/register` | Daemon registers itself and its runtimes |
| `POST` | `/api/daemon/heartbeat` | Periodic liveness heartbeat |
| `GET` | `/api/daemon/stale` | Mark stale runtimes offline |
| `POST` | `/api/workspaces/{slug}/agents/{id}/tasks/claim` | Atomic claim available task |

## Design

Tokens in `packages/ui/styles/tokens.css` — oklch color space, dark/light mode via `.dark` class, thin scrollbars, CJK-aware typography. All components use these tokens exclusively.
