package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create minimal schema needed for workspace middleware.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			slug TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE IF NOT EXISTS workspace_members (
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (workspace_id, user_id)
		);
	`); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// Seed a test workspace and user.
	db.Exec(`INSERT INTO users (id, email, password_hash) VALUES ('dev-user', 'dev@test.local', 'hash')`)
	db.Exec(`INSERT INTO users (id, email, password_hash) VALUES ('outsider', 'outsider@test.local', 'hash')`)
	db.Exec(`INSERT INTO workspaces (id, slug, name) VALUES ('ws1', 'test-workspace', 'Test')`)
	db.Exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws1', 'dev-user', 'owner')`)
	return db
}

func echoHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

func TestAuth_MissingCredentials(t *testing.T) {
	db := newTestDB(t)
	handler := Auth(db)(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	db := newTestDB(t)
	handler := Auth(db)(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuth_MalformedHeader(t *testing.T) {
	db := newTestDB(t)
	handler := Auth(db)(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "NotBearer token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuth_ValidToken(t *testing.T) {
	db := newTestDB(t)
	// Generate a valid JWT using the dev secret.
	secret := []byte("dev-secret-change-me")
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "dev-user",
		"exp": 9999999999,
	})
	tokenString, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	var capturedUserID string
	handler := Auth(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = GetUserID(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if capturedUserID != "dev-user" {
		t.Errorf("expected user id dev-user, got %q", capturedUserID)
	}
}

func TestAuth_ValidSessionCookie(t *testing.T) {
	db := newTestDB(t)
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES ('sess1', 'dev-user', ?)`, expires); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	handler := Auth(db)(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "sess1"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Workspace middleware
// ---------------------------------------------------------------------------

func TestWorkspace_ValidSlug(t *testing.T) {
	db := newTestDB(t)
	handler := Workspace(db)(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/test-workspace/agents", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWorkspace_NotFound(t *testing.T) {
	db := newTestDB(t)
	handler := Workspace(db)(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/nonexistent/agents", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestWorkspace_NonWorkspaceRoute(t *testing.T) {
	db := newTestDB(t)
	handler := Workspace(db)(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (passthrough), got %d", rr.Code)
	}
}

func TestWorkspace_ContextInjection(t *testing.T) {
	db := newTestDB(t)
	var capturedID string
	var capturedSlug string

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetWorkspaceID(r)
		if slug, ok := r.Context().Value(WorkspaceSlugKey).(string); ok {
			capturedSlug = slug
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := Workspace(db)(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/test-workspace/agents", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedID == "" {
		t.Error("expected non-empty workspace ID in context")
	}
	if capturedSlug != "test-workspace" {
		t.Errorf("expected slug 'test-workspace', got '%s'", capturedSlug)
	}
}

func TestWorkspace_DaemonScopedPath(t *testing.T) {
	db := newTestDB(t)
	var capturedID string

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetWorkspaceID(r)
		w.WriteHeader(http.StatusOK)
	})
	handler := Workspace(db)(testHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/daemon/workspaces/test-workspace/agents/a1/tasks", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedID != "ws1" {
		t.Errorf("expected workspace ws1, got %q", capturedID)
	}
}

func TestRequireWorkspaceMember_AllowsMember(t *testing.T) {
	db := newTestDB(t)
	var capturedRole string

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRole = GetWorkspaceRole(r)
		w.WriteHeader(http.StatusOK)
	})
	handler := Auth(db)(Workspace(db)(RequireWorkspaceMember(db)(testHandler)))

	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES ('sess-member', 'dev-user', ?)`, expires)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/test-workspace/agents", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "sess-member"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if capturedRole != "owner" {
		t.Errorf("expected owner role, got %q", capturedRole)
	}
}

func TestRequireWorkspaceMember_DeniesNonMember(t *testing.T) {
	db := newTestDB(t)
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES ('sess-outsider', 'outsider', ?)`, expires)
	handler := Auth(db)(Workspace(db)(RequireWorkspaceMember(db)(echoHandler(t))))

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/test-workspace/agents", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "sess-outsider"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Role middleware
// ---------------------------------------------------------------------------

func TestRole_AllowsMatchingRole(t *testing.T) {
	handler := WorkspaceRole("owner")(RequireOwner()(echoHandler(t)))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAdmin_AllowsOwner(t *testing.T) {
	handler := WorkspaceRole("owner")(RequireAdmin()(echoHandler(t)))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRole_DeniesNonMatchingRole(t *testing.T) {
	handler := WorkspaceRole("member")(RequireOwner()(echoHandler(t)))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Middleware chain integration
// ---------------------------------------------------------------------------

func TestMiddlewareChain(t *testing.T) {
	db := newTestDB(t)

	chain := Auth(db)(Workspace(db)(RequireWorkspaceMember(db)(Role("owner")(echoHandler(t)))))

	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES ('sess-chain', 'dev-user', ?)`, expires)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/test-workspace/agents", strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "sess-chain"})
	rr := httptest.NewRecorder()

	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
