# Deployment

Mulwiki has three runtime parts:

- `mulwiki-server`: Go HTTP API server.
- `mulwiki`: CLI and local daemon entrypoint.
- Web UI: Next.js standalone server.

Use `go run` only for local development. For a third-party server, build or package first.

## Build A Package For The Current Machine

From the repository root:

```bash
pnpm install
make package-current
```

This creates:

```text
dist/
  mulwiki_<version>_<os>_<arch>/
    bin/
      mulwiki-server
      mulwiki
    web/
      apps/web/server.js
      apps/web/.next/static/
      apps/web/public/
    server/
      builtin/
      pkg/db/schema.sql
    .env.example
    server/.env.example
    docs/deployment.md
  mulwiki_<version>_<os>_<arch>.tar.gz
```

`make package-current` builds for the machine where it runs. For Linux deployment, run it on Linux or in a Linux build environment.

## Cross-Compilation

Mulwiki uses `github.com/mattn/go-sqlite3`, which depends on CGO. That means Go cross-compilation needs a target C compiler.

For example, Linux amd64 from a machine that has a Linux amd64 cross compiler:

```bash
cd server
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-gnu-gcc \
  go build -o bin/mulwiki-server-linux-amd64 ./cmd/server

CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-gnu-gcc \
  go build -o bin/mulwiki-linux-amd64 ./cmd/mulwiki
```

If you do not already have cross compilers installed, the reliable path is to build on the target OS or use a Linux CI runner.

## Server Environment

Create a production env file from `.env.example` and `server/.env.example`.

Important variables:

```bash
PORT=8080
DATABASE_URL=file:/var/lib/mulwiki/mulwiki.db
DATA_DIR=/var/lib/mulwiki/data
JWT_SECRET=replace-with-a-long-random-secret
```

The server loads built-in schemas and skills from `server/builtin/`, and database schema SQL from `server/pkg/db/schema.sql`, relative to the process working directory. When running from a release package, start `mulwiki-server` from the package root so these files are available.

## Start The API Server

```bash
cd /opt/mulwiki
DATABASE_URL=file:/var/lib/mulwiki/mulwiki.db \
DATA_DIR=/var/lib/mulwiki/data \
JWT_SECRET=replace-with-a-long-random-secret \
PORT=8080 \
./bin/mulwiki-server
```

## Start The Web UI

Set the API URL used by Next.js rewrites:

```bash
cd /opt/mulwiki
MULWIKI_API_URL=http://127.0.0.1:8080 \
PORT=3000 \
node web/apps/web/server.js
```

Open `http://<server-host>:3000`.

## Configure The CLI And Daemon

On the machine that will run local agent CLIs:

```bash
/opt/mulwiki/bin/mulwiki --profile prod login \
  --server-url http://127.0.0.1:8080 \
  --email you@example.com

/opt/mulwiki/bin/mulwiki --profile prod auth refresh
/opt/mulwiki/bin/mulwiki --profile prod daemon start
/opt/mulwiki/bin/mulwiki --profile prod doctor
```

When the daemon starts without `--workspace`, it discovers every workspace available to the logged-in user and registers local runtimes in each workspace.

## systemd Example

API server:

```ini
[Unit]
Description=Mulwiki API Server
After=network.target

[Service]
WorkingDirectory=/opt/mulwiki
Environment=PORT=8080
Environment=DATABASE_URL=file:/var/lib/mulwiki/mulwiki.db
Environment=DATA_DIR=/var/lib/mulwiki/data
Environment=JWT_SECRET=replace-with-a-long-random-secret
ExecStart=/opt/mulwiki/bin/mulwiki-server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Web UI:

```ini
[Unit]
Description=Mulwiki Web UI
After=network.target mulwiki-server.service

[Service]
WorkingDirectory=/opt/mulwiki
Environment=PORT=3000
Environment=MULWIKI_API_URL=http://127.0.0.1:8080
ExecStart=/usr/bin/node /opt/mulwiki/web/apps/web/server.js
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Daemon:

```ini
[Unit]
Description=Mulwiki Agent Runtime Daemon
After=network.target mulwiki-server.service

[Service]
Type=forking
WorkingDirectory=/opt/mulwiki
ExecStart=/opt/mulwiki/bin/mulwiki --profile prod daemon start
ExecStop=/opt/mulwiki/bin/mulwiki --profile prod daemon stop
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

## Verification

```bash
/opt/mulwiki/bin/mulwiki --profile prod doctor
/opt/mulwiki/bin/mulwiki --profile prod daemon status
curl -fsS http://127.0.0.1:8080/health
curl -fsS -I http://127.0.0.1:3000
```
