# CLI Parity Plan

## Goal

Make the Mulwiki CLI a first-class automation surface for almost every core product capability. Web and future app clients should remain excellent interactive clients, but the CLI should be able to create, inspect, update, execute, and export the same core resources through the same server APIs.

## Design Position

Mulwiki should follow this layering rule:

```text
Server API = authoritative capability layer
CLI        = automation, scripting, local daemon, and advanced-user layer
Web/App    = interactive visualization and low-friction UX layer
```

Every durable product capability should start as a server API contract. The CLI and Web should consume that same contract. Avoid CLI-only business logic and avoid Web-only business capabilities unless the feature is purely presentational.

## Multica CLI Philosophy To Reuse

The local `multica` reference suggests several useful CLI principles:

- **Setup is operational, not decorative.** `multica setup self-host` configures server URLs, authenticates, discovers workspaces, and starts the daemon. Mulwiki should keep `setup` as a high-leverage path rather than a thin config writer.
- **Daemon defaults to broad usefulness.** Multica login/setup discovers all accessible workspaces and the daemon watches them automatically. Mulwiki should keep `mulwiki daemon start` as multi-workspace by default, with `--workspace` only as an explicit restriction.
- **Output is scriptable.** Multica commands commonly support `--output table|json`. Mulwiki should standardize this across list/get/create/update commands.
- **Stdin flags matter.** Multica uses patterns like `--description-stdin` and `--content-stdin` for multi-line fields. Mulwiki should use the same pattern for schema, source, skill, agent instructions, and wiki page content.
- **CLI errors should teach the next command.** Missing auth/workspace/agent/runtime errors should say exactly which command fixes the state.
- **Resource commands are nouns with small verbs.** Prefer `workspace list`, `schema activate`, `source add`, `job logs` over broad overloaded commands.

## Current Mulwiki CLI Coverage

Implemented:

```text
mulwiki --profile <name>
mulwiki login
mulwiki auth status/logout
mulwiki setup self-host
mulwiki profile list/use/delete
mulwiki config show/set
mulwiki workspace list/use
mulwiki daemon start/stop/status/logs
mulwiki runtime list
mulwiki agent list
```

Not yet complete:

```text
workspace create/get/update/delete
schema list/get/create/update/delete/fork/activate/validate/builtin
source list/add/get/remove/raw
runtime get/create/update/delete/status
agent get/create/update/archive/restore/skills/tasks
skill list/get/create/update/delete/add-to-agent/remove-from-agent
job list/create/get/logs/cancel/retry
wiki list/get/create/delete/search/backlinks/resolve/export
```

## CLI Conventions

### Global Flags

All commands should respect:

```text
--profile <name>        Select CLI profile; overrides saved default profile.
--server-url <url>      Override profile server URL where relevant.
--workspace <slug>      Select workspace for workspace-scoped commands.
--output table|json     Output format; table for humans, json for automation.
```

Default output:

- `list`: `table`
- `get`: `json`
- `create/update/delete/action`: `json`, unless the command naturally prints a short success message.
- `logs`: text stream by default, `json` when requested.

### Input Flags

Use the same pair pattern for multi-line fields:

```text
--content <text>
--content-stdin
--description <text>
--description-stdin
--instructions <text>
--instructions-stdin
```

Rules:

- Inline values decode common backslash escapes such as `\n`.
- `*-stdin` preserves stdin verbatim except one trailing newline.
- Inline and stdin variants are mutually exclusive.
- Empty stdin is an error for required fields.

### ID And Slug Resolution

Mulwiki resources should accept human-friendly identifiers where possible:

- Workspace: slug.
- Schema: ID or path.
- Runtime: ID or backend/name when unambiguous.
- Agent: ID or name when unambiguous.
- Skill: ID or name when unambiguous.
- Job/task/wiki page/source: ID or path depending on resource.

Ambiguous names should return an error listing the matching IDs.

### Automation Guarantees

Every command intended for scripts should:

- Exit non-zero on failure.
- Print machine-readable JSON with `--output json`.
- Avoid prompts unless a `--interactive` or empty-sensitive flag explicitly asks for one.
- Support non-interactive auth through existing session/profile config.
- Avoid writing secrets to stdout except token creation commands that explicitly return one-time tokens.

## Target Command Surface

### Auth And Profile

Already mostly done:

```text
mulwiki login
mulwiki auth status
mulwiki auth logout
mulwiki profile list
mulwiki profile use <name>
mulwiki profile delete <name>
mulwiki config show
mulwiki config set <key> <value>
```

Add later:

```text
mulwiki profile path [name]       # show config/daemon paths
mulwiki config unset <key>
mulwiki config edit               # optional; opens $EDITOR
```

Priority: low. The existing profile/config commands are enough for near-term feature parity.

### Workspace

Target:

```text
mulwiki workspace list [--output table|json]
mulwiki workspace get [slug] [--output table|json]
mulwiki workspace create --name <name> --slug <slug> [--description <text>] [--schema builtin:<path>|blank]
mulwiki workspace update [slug] [--name <name>] [--description <text>|--description-stdin]
mulwiki workspace delete <slug> --yes
mulwiki workspace use <slug>
```

Server API status:

- Existing: list, get, create, update, delete.
- Existing create supports blank/builtin schema.

Notes:

- `workspace create` should optionally run `workspace use <slug>` via `--use`.
- `workspace delete` must require `--yes` and refuse deleting the active workspace unless `--yes` is present.

Priority: P0. CLI onboarding and tests need workspace creation.

### Schema

Target:

```text
mulwiki schema list [--workspace <slug>] [--output table|json]
mulwiki schema builtin list [--output table|json]
mulwiki schema get <id-or-path> [--content] [--output table|json]
mulwiki schema create --name <name> --content <text>|--content-stdin [--description <text>] [--version <version>]
mulwiki schema update <id-or-path> [--name <name>] [--description <text>] [--content <text>|--content-stdin] [--version <version>]
mulwiki schema delete <id-or-path> --yes
mulwiki schema fork <builtin-or-schema-id> [--name <name>] [--description <text>]
mulwiki schema activate <id-or-path>
mulwiki schema validate --content <text>|--content-stdin
```

Server API status:

- Existing: list/get/create/update/delete/fork/activate/validate/builtin list.

Notes:

- `schema get --content` should print only Markdown content for shell pipelines.
- `schema validate` should exit non-zero when validation fails.
- `schema fork` should work for both existing workspace schemas and builtin schemas if server supports the target identifier.

Priority: P0. Schema is required before source ingestion can be reliable.

### Source

Target:

```text
mulwiki source list [--output table|json]
mulwiki source add <file-or-url> [--path <workspace-path>] [--type <type>]
mulwiki source get <path> [--raw] [--output json]
mulwiki source remove <path> --yes
```

Server API status:

- Existing: list, create, get, raw, delete.

Notes:

- `source add` should support local files first.
- URL ingestion can be a later enhancement if server-side URL fetch is not yet stable.
- For local files, default path should be `sources/<basename>`.

Priority: P0. Sources are the input side of the ingest loop.

### Runtime And Daemon

Existing:

```text
mulwiki daemon start/stop/status/logs
mulwiki runtime list
```

Target additions:

```text
mulwiki runtime get <id-or-name> [--output table|json]
mulwiki runtime create --name <name> --backend <backend> --path <path> [--hostname <host>] [--version <version>]
mulwiki runtime update <id> [--name <name>] [--backend <backend>] [--path <path>]
mulwiki runtime delete <id> --yes
mulwiki runtime status [--output table|json]
```

Server API status:

- Existing: runtime list/create/get/update/delete.
- Daemon registration covers auto-detected runtimes.

Notes:

- Manual runtime create/update/delete should remain available for custom runtimes, but the recommended path is daemon auto-registration.
- `runtime status` should summarize daemon-backed runtimes across all workspaces when using user-scoped daemon tokens.

Priority: P1. Needed for custom runtimes, but daemon auto-registration already covers the common path.

### Agent

Existing:

```text
mulwiki agent list
```

Target:

```text
mulwiki agent list [--output table|json]
mulwiki agent get <id-or-name> [--output table|json]
mulwiki agent create --name <name> --runtime <runtime-id-or-name> [--model <model>] [--instructions <text>|--instructions-stdin] [--env KEY=VALUE] [--arg <arg>] [--skill <skill-id-or-name>]
mulwiki agent update <id-or-name> [--name <name>] [--runtime <runtime>] [--model <model>] [--instructions <text>|--instructions-stdin] [--env KEY=VALUE] [--clear-env KEY] [--arg <arg>] [--clear-args]
mulwiki agent archive <id-or-name>
mulwiki agent restore <id-or-name>
mulwiki agent skill add <agent> <skill>
mulwiki agent skill remove <agent> <skill>
mulwiki agent task list <agent> [--output table|json]
mulwiki agent task get <agent> <task-id> [--messages] [--output json]
```

Server API status:

- Existing: agent list/create/get/update/archive/restore.
- Existing: agent skill association add/remove.
- Existing: agent task list/get/create/update.

Notes:

- `--env KEY=VALUE` should be repeatable.
- `--arg` should be repeatable and preserve order.
- `--skill` should be repeatable during create/update.
- Archive/restore matches current server model better than hard delete.

Priority: P0 for create/get/update; P1 for archive/restore/tasks/skill attachment.

### Skill

Target:

```text
mulwiki skill list [--output table|json]
mulwiki skill get <id-or-name> [--output json]
mulwiki skill create --name <name> [--description <text>|--description-stdin] [--content <text>|--content-stdin]
mulwiki skill update <id-or-name> [--name <name>] [--description <text>|--description-stdin] [--content <text>|--content-stdin]
mulwiki skill delete <id-or-name> --yes
```

Server API status:

- Existing: skill list/create/update/delete.
- Server model should be checked before adding `--content`; current handler may only store name/description.

Notes:

- If skills remain file-backed in `server/builtin/skills`, user skills need a clear workspace-owned persistence model before full CLI parity.
- Do not conflate builtin skills with workspace skills; use `skill builtin list` later if needed.

Priority: P2. Important for mature automation, but less urgent than ingest loop.

### Job

Target:

```text
mulwiki job list [--status <status>] [--output table|json]
mulwiki job create --agent <agent-id-or-name> --schema <schema-id-or-path> --source <path> [--source <path>...] [--wait] [--output json]
mulwiki job get <id> [--output table|json]
mulwiki job logs <id> [-f|--follow] [--output text|json]
mulwiki job cancel <id>
mulwiki job retry <id>
```

Server API status:

- Existing: list/create/get/logs stream.
- Missing: user-facing cancel.
- Missing: user-facing retry or clone/requeue.

Notes:

- `job create --wait` should poll until terminal status and return non-zero on failure.
- `job logs -f` should stream server-sent events.
- `job retry` should preserve source/schema/agent and create a new job or task retry chain depending on server model.

Priority: P0 for list/create/get/logs; P1 for cancel/retry.

### Wiki

Target:

```text
mulwiki wiki list [--type <type>] [--output table|json]
mulwiki wiki get <path> [--raw] [--output json]
mulwiki wiki create <path> --content <text>|--content-stdin [--title <title>] [--type <type>] [--layer <layer>]
mulwiki wiki delete <path> --yes
mulwiki wiki search <query> [--output table|json]
mulwiki wiki backlinks <path> [--output table|json]
mulwiki wiki resolve-links --path <path> [--path <path>...] [--output json]
mulwiki wiki export [--format zip] [--output-file <path>]
```

Server API status:

- Existing: list/get/create/delete/search/backlinks/resolve.
- Missing or unclear: export zip endpoint.

Notes:

- `wiki get --raw` should print only Markdown for shell use.
- Export should produce an Obsidian-compatible zip once server export exists.

Priority: P1 for list/get/search; P2 for create/delete/backlinks/resolve/export.

## Shared CLI Infrastructure Needed

Add `server/cmd/mulwiki/cli_output.go` or equivalent:

- `printJSON(w, v)`
- `printTable(w, headers, rows)`
- `outputFlag(cmd, defaultValue)`
- `writeJSONOrTable(cmd, tableFn, value)`

Add `server/cmd/mulwiki/cli_text.go`:

- `resolveTextFlag(cmd, "content")`
- `resolveKeyValueFlags(cmd, "env")`
- escaping behavior matching Multica's CLI convention.

Add `server/cmd/mulwiki/cli_resolve.go`:

- workspace slug resolution.
- schema ID/path resolution.
- runtime ID/name/backend resolution.
- agent ID/name resolution.
- skill ID/name resolution.

Add tests per helper before adding resource commands. These helpers should make every later command smaller and more predictable.

## Implementation Phases

### Phase 0: CLI Foundation

Deliver:

- Shared output helpers.
- Shared text/stdin helpers.
- Shared workspace-scoped client builder.
- Consistent `--output` behavior on existing `workspace list`, `runtime list`, and `agent list`.

Verification:

```bash
cd server
go test ./cmd/mulwiki
```

### Phase 1: Workspace And Schema Parity

Deliver:

- `workspace get/create/update/delete/use/list`.
- `schema list/builtin/get/create/update/delete/fork/activate/validate`.
- JSON and table outputs.

Why first:

- Workspace and schema are prerequisites for automated ingest.
- They make local E2E setup deterministic without manual Web clicks.

### Phase 2: Source, Agent, And Job Ingest Loop

Deliver:

- `source list/add/get/remove`.
- `agent get/create/update/archive/restore`.
- `job list/create/get/logs`.
- A CLI smoke script that creates a workspace, creates a schema, adds a source, creates an agent bound to an existing runtime, creates a job, and observes the job state.

Why second:

- This forms the first real CLI ingest flow.

### Phase 3: Runtime, Skill, Wiki Depth

Deliver:

- Runtime manual CRUD.
- Skill CRUD and agent-skill association commands.
- Wiki list/get/search/backlinks/resolve.
- Server export endpoint and `wiki export` if export remains a product requirement.

Why third:

- These complete operator workflows but are not required for the first ingest loop.

### Phase 4: Automation Polish

Deliver:

- `--wait` for long-running commands.
- `--format` or `--template` only if there is a demonstrated need beyond JSON/table.
- Shell completion checks.
- CI smoke coverage for key CLI commands against a temporary server.

## API Gaps To Close

Known gaps before full CLI parity:

- Job cancel endpoint.
- Job retry endpoint or documented requeue model.
- Wiki export endpoint.
- Skill content/storage model if skills should be editable beyond name/description.
- Possibly source URL ingestion if `source add <url>` is required.

These should be added to server first, then exposed in CLI, then reused by Web.

## Testing Strategy

Unit tests:

- CLI helper tests for output, text/stdin, key/value parsing, and ID resolution.
- Command tests using `httptest.Server` for each resource command.

Integration tests:

- Temporary SQLite DB, temporary data dir, temporary HOME.
- Start server on a random or reserved port.
- Use compiled CLI binary with `--profile e2e`.
- Create workspace, schema, source, agent, job.
- Assert JSON output and DB/server state.

Manual smoke:

```bash
cd server
go run ./cmd/server
go run ./cmd/mulwiki --profile dev login --server-url http://localhost:8080
go run ./cmd/mulwiki --profile dev workspace create --name Demo --slug demo --schema blank --use
go run ./cmd/mulwiki --profile dev daemon start
go run ./cmd/mulwiki --profile dev runtime list
```

## Success Criteria

CLI parity is successful when:

- A new workspace can be created without opening Web.
- A schema can be selected or authored without opening Web.
- A source can be added without opening Web.
- An agent can be created and bound to a runtime without opening Web.
- A job can be created and monitored without opening Web.
- Wiki output can be inspected and exported without opening Web.
- Every command has `--output json` where automation needs it.
- Web and CLI use the same server APIs for the same capability.

## Recommended Next Task

Start with Phase 0 and Phase 1:

```text
Task 1: Add shared CLI output/text/client helpers.
Task 2: Complete workspace CLI parity.
Task 3: Complete schema CLI parity.
Task 4: Add a temporary-server CLI integration test for workspace + schema.
```

This gives Mulwiki a stable CLI foundation and makes later source/agent/job commands much faster to implement cleanly.

## Execution Status

Implemented in `codex/cli-parity`:

- Shared CLI JSON/table output helpers.
- Shared text/stdin helpers.
- Workspace CLI parity: `list/get/create/update/delete/use`.
- Schema CLI parity: `list/builtin list/get/create/update/delete/fork/activate/validate`.
- Source CLI parity for server-supported behavior: `list/add/get/remove`, including `get --raw`.
- Runtime CLI parity: `list/get/create/update/delete`.
- Agent CLI parity: `list/get/create/update/archive/restore`, plus `agent skill add/remove` and `agent task list/get`.
- Skill CLI parity for the current server model: `list/get/create/update/delete`.
- Job CLI parity: `list/create/get/logs/cancel/retry`, including `create --wait`.
- Wiki CLI parity: `list/get/create/delete/search/backlinks/resolve-links/export`.

Remaining API gaps:

- source URL ingestion.
- custom source upload paths.
- file-backed editable skill content beyond name/description.
