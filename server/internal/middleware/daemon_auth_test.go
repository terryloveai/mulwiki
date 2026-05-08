package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tethy/mulwiki/server/internal/auth"
)

func seedDaemonToken(t *testing.T, db *sql.DB, workspaceID, daemonID string) string {
	t.Helper()

	raw, hash, err := auth.NewDaemonToken()
	if err != nil {
		t.Fatalf("new daemon token: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS daemon_tokens (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			workspace_id TEXT NOT NULL,
			daemon_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			revoked_at TEXT
		);
	`); err != nil {
		t.Fatalf("create daemon_tokens: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO daemon_tokens (workspace_id, daemon_id, token_hash) VALUES (?, ?, ?)`,
		workspaceID, daemonID, hash,
	); err != nil {
		t.Fatalf("insert daemon token: %v", err)
	}
	return raw
}

func TestDaemonAuthMissingToken(t *testing.T) {
	db := newTestDB(t)
	handler := DaemonAuth(db)(echoHandler(t))

	req := httptest.NewRequest(http.MethodPost, "/api/daemon/register", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestDaemonAuthValidTokenInjectsWorkspaceAndDaemon(t *testing.T) {
	db := newTestDB(t)
	raw := seedDaemonToken(t, db, "ws1", "daemon-1")

	var gotWorkspaceID string
	var gotDaemonID string
	handler := DaemonAuth(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWorkspaceID = GetWorkspaceID(r)
		gotDaemonID = GetDaemonID(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/daemon/register", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if gotWorkspaceID != "ws1" || gotDaemonID != "daemon-1" {
		t.Fatalf("expected ws1/daemon-1, got %s/%s", gotWorkspaceID, gotDaemonID)
	}
}

func TestDaemonAuthWorkspaceMismatchIsDenied(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`INSERT INTO workspaces (id, slug, name) VALUES ('ws2', 'other-workspace', 'Other')`); err != nil {
		t.Fatalf("seed other workspace: %v", err)
	}
	raw := seedDaemonToken(t, db, "ws1", "daemon-1")

	handler := DaemonAuth(db)(Workspace(db)(echoHandler(t)))
	req := httptest.NewRequest(http.MethodPost, "/api/daemon/workspaces/other-workspace/jobs/claim", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWithDaemonContext(t *testing.T) {
	ctx := WithDaemon(context.Background(), "ws1", "daemon-1")
	if WorkspaceIDFromContext(ctx) != "ws1" {
		t.Fatalf("expected workspace id from context")
	}
	if DaemonIDFromContext(ctx) != "daemon-1" {
		t.Fatalf("expected daemon id from context")
	}
}
