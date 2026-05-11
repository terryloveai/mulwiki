# Mulwiki CLI

The `mulwiki` CLI manages local authentication, workspace selection, and the daemon that registers local agent runtimes with the server.

Run commands from the `server/` directory during development:

```bash
go run ./cmd/mulwiki --help
```

For an installed binary, replace `go run ./cmd/mulwiki` with `mulwiki`.

## Profiles

A profile isolates CLI config and daemon state. Each named profile has its own config, daemon ID, token, PID file, log file, and health port.

```bash
go run ./cmd/mulwiki --profile dev login --server-url http://localhost:8080
go run ./cmd/mulwiki --profile dev daemon start
go run ./cmd/mulwiki --profile dev daemon status
```

Manage the default profile:

```bash
go run ./cmd/mulwiki profile list
go run ./cmd/mulwiki profile use dev
go run ./cmd/mulwiki profile delete dev
```

`--profile` always wins over the saved default profile. The unnamed default profile still uses the legacy paths under `~/.mulwiki/`.

## Config

Show or update profile config:

```bash
go run ./cmd/mulwiki --profile dev config show
go run ./cmd/mulwiki --profile dev config set server-url http://localhost:8080
go run ./cmd/mulwiki --profile dev config set workspace demo
```

Important config fields:

- `server_url`: Mulwiki server URL.
- `workspace_slug`: default workspace for commands that operate on one workspace.
- `session_id`: local login session cookie.
- `daemon_token`: user-scoped token used by the daemon.

## Login And Setup

Login stores a local session in the selected profile:

```bash
go run ./cmd/mulwiki --profile dev login \
  --server-url http://localhost:8080 \
  --email you@example.com
```

`setup self-host` combines login and optional daemon startup:

```bash
go run ./cmd/mulwiki --profile dev setup self-host \
  --server-url http://localhost:8080 \
  --email you@example.com
```

Use `--no-start` when you only want to write config and authenticate.

## Workspaces

List workspaces available to the logged-in user and choose a default workspace for the profile:

```bash
go run ./cmd/mulwiki --profile dev workspace list
go run ./cmd/mulwiki --profile dev workspace create \
  --name Demo \
  --slug demo \
  --schema blank \
  --use
go run ./cmd/mulwiki --profile dev workspace get demo
go run ./cmd/mulwiki --profile dev workspace update demo --description "Demo workspace"
go run ./cmd/mulwiki --profile dev workspace use demo
go run ./cmd/mulwiki --profile dev workspace delete demo --yes
```

The selected workspace is used by commands such as `runtime list` and `agent list` when `--workspace` is not provided.

Most resource commands also accept `--workspace <slug>` to override the profile default for one command.

## Schemas

Schemas are Markdown definitions. Builtin schemas are immutable templates; workspace schemas are editable copies stored in workspace-owned storage.

```bash
go run ./cmd/mulwiki --profile dev schema builtin list
go run ./cmd/mulwiki --profile dev schema list
go run ./cmd/mulwiki --profile dev schema get schemas/concept-wiki-schema.md --content

go run ./cmd/mulwiki --profile dev schema create \
  --name "Research Schema" \
  --content-stdin < schema.md

go run ./cmd/mulwiki --profile dev schema validate --content-stdin < schema.md
go run ./cmd/mulwiki --profile dev schema activate schemas/custom.md
```

Use `--output json` on `list`, `get`, `create`, `update`, and action commands when scripting.

## Sources

Sources are raw input documents stored under `sources/` in the workspace repo.

```bash
go run ./cmd/mulwiki --profile dev source list
go run ./cmd/mulwiki --profile dev source add ./notes.md
go run ./cmd/mulwiki --profile dev source get sources/notes.md
go run ./cmd/mulwiki --profile dev source get sources/notes.md --raw
go run ./cmd/mulwiki --profile dev source remove sources/notes.md --yes
```

Local file upload is supported. URL ingestion and custom upload paths need server API support before the CLI can expose them cleanly.

## Daemon

Start a daemon for all workspaces available to the logged-in user:

```bash
go run ./cmd/mulwiki --profile dev daemon start
```

Start a daemon restricted to one workspace:

```bash
go run ./cmd/mulwiki --profile dev daemon start --workspace demo
```

Start a daemon restricted to multiple explicit workspaces:

```bash
go run ./cmd/mulwiki --profile dev daemon start --workspace demo,research
```

Useful daemon commands:

```bash
go run ./cmd/mulwiki --profile dev daemon status
go run ./cmd/mulwiki --profile dev daemon logs
go run ./cmd/mulwiki --profile dev daemon stop
```

When no `--workspace` is supplied, the daemon uses a user-scoped daemon token and periodically discovers all workspaces the user can access. Workspace-scoped daemon tokens still work, but they only register runtimes for their original workspace.

## Runtime Checks

After the daemon starts, check that detected local agent CLIs registered:

```bash
go run ./cmd/mulwiki --profile dev runtime list
go run ./cmd/mulwiki --profile dev runtime get Codex
```

The web UI shows registered runtimes under each workspace's **Runtimes** page. A multi-workspace daemon appears in every workspace where it has registered runtimes.

Manual runtime records are available for custom setups:

```bash
go run ./cmd/mulwiki --profile dev runtime create \
  --name Codex \
  --backend codex \
  --path codex

go run ./cmd/mulwiki --profile dev runtime update Codex --version dev
go run ./cmd/mulwiki --profile dev runtime delete Codex --yes
```

The recommended path remains daemon auto-registration.

## Agents And Skills

Create an agent bound to a runtime:

```bash
go run ./cmd/mulwiki --profile dev agent list
go run ./cmd/mulwiki --profile dev agent create \
  --name Writer \
  --runtime Codex \
  --model gpt-5.4 \
  --instructions-stdin < AGENTS.md

go run ./cmd/mulwiki --profile dev agent get Writer
go run ./cmd/mulwiki --profile dev agent update Writer --model gpt-5.5
go run ./cmd/mulwiki --profile dev agent archive Writer
go run ./cmd/mulwiki --profile dev agent restore Writer
```

Skills are lightweight workspace records today:

```bash
go run ./cmd/mulwiki --profile dev skill list
go run ./cmd/mulwiki --profile dev skill create --name "Document Parse" --description "Parse uploaded documents"
go run ./cmd/mulwiki --profile dev agent skill add Writer "Document Parse"
go run ./cmd/mulwiki --profile dev agent skill remove Writer "Document Parse"
```

Full file-backed skill content editing remains a product/API design item.

## Jobs

Create and inspect ingest jobs:

```bash
go run ./cmd/mulwiki --profile dev job create \
  --agent Writer \
  --schema schema-id \
  --source sources/notes.md

go run ./cmd/mulwiki --profile dev job list
go run ./cmd/mulwiki --profile dev job get job-id
go run ./cmd/mulwiki --profile dev job logs job-id
go run ./cmd/mulwiki --profile dev job cancel job-id
go run ./cmd/mulwiki --profile dev job retry job-id
```

`job create --wait` polls until a terminal status.

## Wiki

Inspect and edit compiled wiki output:

```bash
go run ./cmd/mulwiki --profile dev wiki list
go run ./cmd/mulwiki --profile dev wiki get /concepts/ai --raw
go run ./cmd/mulwiki --profile dev wiki create /concepts/ai \
  --title "AI" \
  --type concept \
  --content-stdin < page.md

go run ./cmd/mulwiki --profile dev wiki search "transformer"
go run ./cmd/mulwiki --profile dev wiki backlinks /concepts/ai
go run ./cmd/mulwiki --profile dev wiki resolve-links --path ai --path /concepts/ai
go run ./cmd/mulwiki --profile dev wiki export --output-file wiki.zip
go run ./cmd/mulwiki --profile dev wiki delete /concepts/ai --yes
```
