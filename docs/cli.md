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
go run ./cmd/mulwiki --profile dev workspace use demo
```

The selected workspace is used by commands such as `runtime list` and `agent list` when `--workspace` is not provided.

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
```

The web UI shows registered runtimes under each workspace's **Runtimes** page. A multi-workspace daemon appears in every workspace where it has registered runtimes.
