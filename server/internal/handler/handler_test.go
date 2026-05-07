package handler

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
)

// newTestHandler creates a Handler backed by an in-memory SQLite database
// with the full schema applied and a test workspace seeded.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Run schema migration.
	schemaPath := findSchemaSQL()
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	// Seed a test user (required for foreign keys).
	if _, err := db.Exec(
		`INSERT INTO users (id, email, password_hash) VALUES ('dev-user', 'dev@mulwiki.local', 'hash')`,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Seed a test workspace.
	if _, err := db.Exec(
		`INSERT INTO workspaces (id, slug, name, description) VALUES ('ws1', 'test-workspace', 'Test Workspace', 'A test workspace')`,
	); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	return &Handler{DB: db}
}

// chiRequest creates an httptest request with chi route context parameters set.
// params is a map of chi URL parameter names to values.
func chiRequest(method, target string, params map[string]string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	return req
}

// findSchemaSQL locates the schema.sql file relative to common working directories.
func findSchemaSQL() string {
	candidates := []string{
		"../../pkg/db/schema.sql",         // from server/internal/handler/
		"../pkg/db/schema.sql",            // alternative
		filepath.Join("pkg", "db", "schema.sql"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// Fall back to the most likely path from go test's working directory.
	return "../../pkg/db/schema.sql"
}
