# Mulwiki

Compile raw documents into a structured wiki knowledge base — multi-tenant, schema-driven, agent-powered.

[🇨🇳 中文文档](README.zh-CN.md)

## 🎯 Overview

Mulwiki is a platform that transforms arbitrary documents into structured, interconnected wiki knowledge bases. Unlike traditional RAG systems that provide temporary answers, Mulwiki compiles documents into persistent, searchable, and continuously growing knowledge bases.

### Key Differentiators

| Dimension | NotebookLM / RAG Tools | Mulwiki |
|-----------|----------------------|---------|
| **Core Interaction** | Upload documents → Q&A | Upload documents → Compile to wiki |
| **Output** | Temporary answers (disposable) | Persistent wiki knowledge base (grows over time) |
| **Knowledge Organization** | Unstructured (vector search) | User-selectable/definable Schema |
| **Agent Selection** | Platform-fixed | User-chosen (Claude Code / Codex / Kimi CLI / custom) |
| **Incremental Updates** | Full retrieval each time | Incremental compilation, only affected pages updated |

### Target Users

Researchers, engineers, and deep learners who need to maintain long-term knowledge bases in specific domains. Users should understand the concept of "Schema-defined knowledge structure" and be willing to choose or customize Schemas for their domains.

## 🏗️ Core Concepts

Mulwiki organizes knowledge around five core entities:

```
Workspace
├── Sources (Raw documents, read-only)
├── Schema (Knowledge organization definition)
├── Agent Runtime (Agent execution environment)
├── Agent (Configured agent with instructions, skills, environment)
└── Wiki (Compiled output)

Ingest Job: Sources + Schema → Agent → Wiki
```

### Workspace
An isolated knowledge base containing Sources, Schemas, Agents, and Wiki pages.

### Source
Raw documents uploaded by users. Read-only to agents. Supports PDF, DOCX, PPTX, XLSX, Markdown, plain text, images, and URLs.

### Schema
A Markdown file defining how knowledge is organized across five dimensions:
- **Types**: What kinds of wiki pages exist
- **Structure**: How pages connect (strict hierarchy / knowledge graph / free links)
- **Frontmatter**: Required YAML metadata per page type
- **Ingest Pipeline**: Step-by-step document → wiki workflow
- **Lint Rules**: Quality checks (orphans, contradictions, evidence chains)

6 built-in schemas are included. Users can create custom schemas or fork built-in ones.

### Agent Runtime
An execution environment for agents. Represents an installed agent CLI daemon on a physical machine. Each Runtime tracks: backend type (claude-code / codex / kimi / custom), CLI path, hostname, OS, version, daemon_id, last_heartbeat (liveness), and online/offline status.

### Agent
A configured agent bound to a Runtime with six configuration dimensions:
- **Runtime binding**: Which Runtime this agent executes on
- **Instructions**: System-level prompt defining agent behavior
- **Skills**: Reusable capability modules
- **Tasks**: Execution history (status, duration, output stats, errors)
- **Environment**: Environment variables (API keys, workdir paths, model names)
- **Custom Args**: Extra CLI arguments passed to the runtime
- **Settings**: Model selection, max concurrent tasks, visibility (private/public)

Agent and Schema are fully decoupled. An ingest job combines: Sources + Schema → Agent → Wiki.

### Wiki
The compiled output — a directory of interconnected Markdown pages organized by the Schema's type system. Exportable as a zip (Obsidian-compatible).

## 🛠️ Tech Stack

- **Frontend**: Next.js 15, React 19, Tailwind CSS (oklch), React Query
- **Backend**: Go, chi router, SQLite (go-sqlite3)
- **Agent**: Daemon pattern — polls server for jobs, forks configured agent CLI as subprocess
- **Monorepo**: pnpm workspaces + turborepo

## 🚀 Quick Start

### Prerequisites

- Node.js 18+
- Go 1.21+
- pnpm 9+

### Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/mulwiki.git
cd mulwiki

# Install dependencies
pnpm install
```

### Development

```bash
# Start both Go backend and Next.js frontend
make dev

# Or start them separately:
# Terminal 1: Go backend (http://localhost:8080)
cd server && go run ./cmd/server

# Terminal 2: Frontend (http://localhost:3000)
cd apps/web && pnpm dev
```

### Build

```bash
make build
```

### Clean

```bash
make clean
```

## 📁 Project Structure

```
mulwiki/
├── apps/web/          Next.js 15 frontend (App Router)
├── packages/
│   ├── core/          Shared types, API client, hooks
│   ├── ui/            Design system: tokens, components, hooks
│   └── views/         Page-level components
├── server/
│   ├── cmd/server/    HTTP server (chi router)
│   ├── cmd/mulwiki/   CLI (daemon subcommand)
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
├── schemas/           Built-in schema definitions (Markdown)
├── scripts/           Dev helpers
├── LICENSE            MIT License
├── Makefile           Build automation
└── README.md          This file
```

## ✨ Features

### Multi-Tenant Workspace
- Isolated knowledge bases with full data separation
- Workspace-level settings and configurations
- Member management (owner/admin/member roles)

### Document Management
- Support for multiple file formats (PDF, DOCX, PPTX, XLSX, Markdown, plain text, images)
- URL ingestion with automatic content extraction
- Document preview and management

### Schema System
- 6 built-in schemas for different knowledge organization patterns
- Custom schema creation with Markdown editor
- Schema forking from built-in templates
- Pure Markdown definitions (no agent references, no pipeline routing config)

### Agent Runtime
- Support for multiple agent backends (Claude Code, Codex, Kimi CLI, custom)
- Runtime registration with health monitoring
- Automatic heartbeat tracking (30s interval)
- Stale detection (5min timeout)

### Agent Configuration
- Six-dimensional configuration (Runtime, Instructions, Skills, Tasks, Environment, Settings)
- Task execution history with detailed logs
- Environment variable management for sensitive data
- Model selection and concurrency control

### Ingest Pipeline
- Job creation with Schema + Agent + Sources selection
- Real-time task monitoring via WebSocket
- Incremental compilation for existing wikis
- Automatic retry with failure classification

### Wiki Output
- Structured Markdown pages organized by Schema types
- Full-text search across wiki content
- Source tracing back to original documents
- Obsidian-compatible export (zip format)

### Real-time Updates
- WebSocket-based real-time task status
- Event bus for internal service communication
- Room-based publish/subscribe pattern

## 🔧 Configuration

### Environment Variables

See `.env.example` for required environment variables:

```bash
# Server
SERVER_PORT=8080
DATABASE_URL=sqlite:///data/mulwiki.db

# Frontend
NEXT_PUBLIC_API_URL=http://localhost:8080
```

### Built-in Schemas

Located in `server/data/builtin-schemas/`:
- `concept-wiki-schema.md` - Strict 9-type hierarchy
- `karpathy-llm-wiki-schema.md` - Loose, free-link structure
- `nashsu-llm-wiki-schema.md` - 7-type knowledge graph
- `llm-knowledge-base-schema.md` - Minimal 3-type system
- `paper-spec-wiki-schema.md` - Academic 7-type structure
- `paper-spec-paper-schema.md` - Paper-level YAML structured profile

## 📖 API Routes

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/daemon/register` | Daemon registers itself and its runtimes |
| `POST` | `/api/daemon/heartbeat` | Periodic liveness heartbeat |
| `GET` | `/api/daemon/stale` | Mark stale runtimes offline |
| `POST` | `/api/workspaces/{slug}/agents/{id}/tasks/claim` | Atomic claim available task |

## 🎨 Frontend Routes

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

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Built with [Next.js](https://nextjs.org/)
- Backend powered by [Go](https://golang.org/)
- Styling with [Tailwind CSS](https://tailwindcss.com/)
- Monorepo management with [Turborepo](https://turbo.build/)

## 📞 Support

For support, please open an issue on GitHub or contact the maintainers.

---

**Note**: Mulwiki is currently in active development. APIs and features may change as we evolve the platform.