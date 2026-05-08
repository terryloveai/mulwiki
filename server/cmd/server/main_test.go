package main

import (
	"database/sql"
	"testing"

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
