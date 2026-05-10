# Mulwiki

Compile raw documents into durable, schema-driven wiki knowledge bases.

[中文文档](README.zh-CN.md)

Mulwiki is for people who want a long-lived knowledge base, not one-off chat answers. It takes source documents, applies a Markdown schema, runs a configured local agent, and writes the result back as interconnected Markdown wiki pages.

## What It Does

Mulwiki organizes work around five concepts:

```text
Workspace
├── Sources       Raw documents, read-only to agents
├── Schemas       Markdown definitions for wiki structure
├── Runtimes      Local agent CLI environments registered by daemon
├── Agents        Runtime binding plus instructions, skills, env, settings
└── Wiki          Compiled Markdown output

Ingest Job = Sources + Schema + Agent -> Wiki
```

Unlike a RAG chat tool, the main output is not a temporary answer. The output is a versionable wiki that can be inspected, edited, searched, and incrementally updated.

## Current Capabilities

- Authenticated users and workspace membership with owner/admin/member roles.
- Workspace creation from either a blank schema or a built-in schema.
- Built-in schemas and skills shipped with the server in `server/builtin/`.
- Git-backed workspace content for wiki pages, sources, schemas, and task output.
- Local daemon registration for available agent CLIs such as Codex, Claude Code, Kimi, or custom runtimes.
- Agent configuration for runtime binding, instructions, skills, environment variables, custom args, model settings, and task history.
- Persistent jobs, agent tasks, runtime state, session pointers, logs, and agent messages.
- Realtime UI updates through WebSocket events.

## Quick Start

### Prerequisites

- Node.js 20+ or current LTS
- pnpm 9.15+
- Go 1.23+
- Git
- Optional: one or more agent CLIs installed locally

### Run The App

```bash
git clone https://github.com/terryloveai/mulwiki.git
cd mulwiki
pnpm install
```

Start the backend:

```bash
cd server
go run ./cmd/server
```

Start the frontend in another terminal:

```bash
cd apps/web
pnpm dev
```

Open [http://localhost:3000](http://localhost:3000), register or sign in, then create a workspace. During workspace creation you can choose a built-in schema or start from a blank schema.

### CLI And Daemon

After creating a user and workspace in the web UI, configure the CLI and start the local runtime daemon:

```bash
cd server

go run ./cmd/mulwiki setup self-host \
  --server-url http://localhost:8080 \
  --workspace demo

go run ./cmd/mulwiki daemon start
go run ./cmd/mulwiki daemon status
go run ./cmd/mulwiki runtime list
```

You can also run `go run ./cmd/mulwiki login --server-url http://localhost:8080 --workspace demo` if you want to authenticate the CLI separately. The CLI stores its local session in `~/.mulwiki/config.json`; the daemon uses that session to mint a workspace-scoped daemon token and register detected runtimes with the server.

## Common Commands

```bash
pnpm typecheck
pnpm build

cd server
go test ./...
```

## Repository Map

```text
mulwiki/
├── apps/web/          Next.js app
├── packages/core/     Shared types, API client, hooks
├── packages/ui/       Design tokens, components, hooks
├── packages/views/    Page-level React views
├── server/
│   ├── cmd/server/    HTTP server
│   ├── cmd/mulwiki/   CLI and daemon entrypoint
│   ├── builtin/       Built-in schemas and skills
│   ├── internal/      Handlers, services, middleware, daemon, realtime
│   └── pkg/           Database and protocol packages
├── docs/              Product, architecture, and implementation plans
└── scripts/           Development helpers
```

## Configuration

Use `.env.example` as the source of truth for local server and daemon environment variables. Built-in schemas and skills are loaded from:

- `server/builtin/schemas/`
- `server/builtin/skills/`

New workspaces copy or fork schema content into their own workspace-owned storage. Built-ins remain immutable templates owned by the server distribution.

## Documentation

Long-form product, architecture, design, and implementation planning documents live in [docs/](docs/README.md).
