package main

import (
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tethy/mulwiki/server/internal/handler"
)

func newMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func createLegacyUsersAndWorkspaces(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			active_schema_id TEXT
		);
		INSERT INTO users (id, email, password_hash, created_at) VALUES
			('user-newer', 'newer@example.com', 'hash', '2026-01-02T00:00:00Z'),
			('user-oldest', 'oldest@example.com', 'hash', '2026-01-01T00:00:00Z');
		INSERT INTO workspaces (id, slug, name) VALUES
			('ws-builtin', 'builtin', 'Built In'),
			('ws-alpha', 'alpha', 'Alpha'),
			('ws-beta', 'beta', 'Beta');
	`)
	if err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}
}

func TestRunMigrationsBackfillsExistingWorkspacesToOldestUser(t *testing.T) {
	db := newMigrationTestDB(t)
	createLegacyUsersAndWorkspaces(t, db)

	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	rows, err := db.Query(`
		SELECT w.slug, wm.user_id, wm.role
		FROM workspace_members wm
		JOIN workspaces w ON w.id = wm.workspace_id
		ORDER BY w.slug
	`)
	if err != nil {
		t.Fatalf("query memberships: %v", err)
	}
	defer rows.Close()

	got := make([][3]string, 0)
	for rows.Next() {
		var slug, userID, role string
		if err := rows.Scan(&slug, &userID, &role); err != nil {
			t.Fatalf("scan membership: %v", err)
		}
		got = append(got, [3]string{slug, userID, role})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate memberships: %v", err)
	}

	want := [][3]string{
		{"alpha", "user-oldest", "owner"},
		{"beta", "user-oldest", "owner"},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d memberships, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("membership %d: expected %#v, got %#v", i, want[i], got[i])
		}
	}
}

func TestRunMigrationsLeavesMembershipsEmptyWhenNoUsersExist(t *testing.T) {
	db := newMigrationTestDB(t)
	if _, err := db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			active_schema_id TEXT
		);
		INSERT INTO workspaces (id, slug, name) VALUES ('ws-alpha', 'alpha', 'Alpha');
	`); err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}

	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspace_members`).Scan(&count); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no memberships without users, got %d", count)
	}
}

func TestRunMigrationsDoesNotBackfillWhenMembershipsAlreadyExist(t *testing.T) {
	db := newMigrationTestDB(t)
	createLegacyUsersAndWorkspaces(t, db)
	if _, err := db.Exec(`
		CREATE TABLE workspace_members (
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (workspace_id, user_id)
		);
		INSERT INTO workspace_members (workspace_id, user_id, role)
		VALUES ('ws-alpha', 'user-newer', 'admin');
	`); err != nil {
		t.Fatalf("seed existing membership: %v", err)
	}

	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspace_members`).Scan(&count); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected existing memberships to be left unchanged, got %d rows", count)
	}

	var role string
	if err := db.QueryRow(`
		SELECT role FROM workspace_members
		WHERE workspace_id = 'ws-alpha' AND user_id = 'user-newer'
	`).Scan(&role); err != nil {
		t.Fatalf("query existing membership: %v", err)
	}
	if role != "admin" {
		t.Fatalf("expected existing role admin, got %q", role)
	}
}

func TestRunMigrationsAddsAgentTaskJobIDBeforeSchemaIndexes(t *testing.T) {
	db := newMigrationTestDB(t)
	if _, err := db.Exec(`
		CREATE TABLE agent_tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			runtime_id TEXT,
			source_path TEXT NOT NULL DEFAULT '',
			schema_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued',
			priority INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		INSERT INTO agent_tasks (id, workspace_id, agent_id)
		VALUES ('task-1', 'ws-1', 'agent-1');
	`); err != nil {
		t.Fatalf("create legacy agent_tasks: %v", err)
	}

	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	hasJobID, err := hasColumn(db, "agent_tasks", "job_id")
	if err != nil {
		t.Fatalf("check job_id: %v", err)
	}
	if !hasJobID {
		t.Fatalf("expected agent_tasks.job_id to be added")
	}

	var indexedColumn string
	if err := db.QueryRow(`
		SELECT ii.name
		FROM pragma_index_info('idx_agent_tasks_job') ii
		LIMIT 1
	`).Scan(&indexedColumn); err != nil {
		t.Fatalf("query idx_agent_tasks_job: %v", err)
	}
	if indexedColumn != "job_id" {
		t.Fatalf("index column = %q, want job_id", indexedColumn)
	}
}

func TestMountWorkspaceRoutesDoesNotDuplicateSlugRoute(t *testing.T) {
	db := newMigrationTestDB(t)
	h := &handler.Handler{DB: db}
	r := chi.NewRouter()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("mountWorkspaceRoutes panicked: %v", recovered)
		}
	}()

	mountWorkspaceRoutes(r, db, h)
}

func TestTimeoutHTTPServerConfiguresRuntimeTimeouts(t *testing.T) {
	srv := newHTTPServer("9090", http.NewServeMux())

	if srv.Addr != ":9090" {
		t.Fatalf("expected addr :9090, got %q", srv.Addr)
	}
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("expected ReadHeaderTimeout 10s, got %s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Fatalf("expected ReadTimeout 30s, got %s", srv.ReadTimeout)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Fatalf("expected IdleTimeout 60s, got %s", srv.IdleTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Fatalf("expected no global WriteTimeout for SSE/WebSocket, got %s", srv.WriteTimeout)
	}
}

func TestAllowedOriginsDefaultForLocalDevelopment(t *testing.T) {
	got := allowedOrigins("")
	want := []string{"http://localhost:3000", "http://localhost:5173"}

	if len(got) != len(want) {
		t.Fatalf("expected %d origins, got %#v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("origin %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestAllowedOriginsParsesCommaSeparatedValues(t *testing.T) {
	got := allowedOrigins(" https://mulwiki.example.com, http://127.0.0.1:3001 ,,http://localhost:3000 ")
	want := []string{"https://mulwiki.example.com", "http://127.0.0.1:3001", "http://localhost:3000"}

	if len(got) != len(want) {
		t.Fatalf("expected %d origins, got %#v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("origin %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestMigrateSchemasToGitSkipsWhenLegacyConfigColumnIsAbsent(t *testing.T) {
	db := newMigrationTestDB(t)
	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	if err := migrateSchemasToGit(db); err != nil {
		t.Fatalf("expected migration to skip clean schema without config column, got %v", err)
	}
}
