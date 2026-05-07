package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

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
			password_hash TEXT NOT NULL
		);
	`); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// Seed a test workspace and user.
	db.Exec(`INSERT INTO users (id, email, password_hash) VALUES ('dev-user', 'dev@test.local', 'hash')`)
	db.Exec(`INSERT INTO workspaces (id, slug, name) VALUES ('ws1', 'test-workspace', 'Test')`)
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

func TestAuth_DevMode_NoAuthHeader(t *testing.T) {
	handler := Auth(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-User-ID") != "" {
		// Dev mode sets X-User-ID on the request header (not response header).
		// Check it was set on the incoming request side.
		t.Log("dev-mode auth passed through")
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	handler := Auth(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuth_MalformedHeader(t *testing.T) {
	handler := Auth(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "NotBearer token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuth_ValidToken(t *testing.T) {
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

	handler := Auth(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
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

// ---------------------------------------------------------------------------
// Role middleware
// ---------------------------------------------------------------------------

func TestRole_AllowsPassthrough(t *testing.T) {
	handler := RequireOwner()(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (placeholder), got %d", rr.Code)
	}
}

func TestRequireAdmin_AllowsPassthrough(t *testing.T) {
	handler := RequireAdmin()(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (placeholder), got %d", rr.Code)
	}
}

func TestRole_WithSpecificRoles(t *testing.T) {
	handler := Role("owner", "admin")(echoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (placeholder), got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Middleware chain integration
// ---------------------------------------------------------------------------

func TestMiddlewareChain(t *testing.T) {
	db := newTestDB(t)

	chain := Auth(Workspace(db)(Role("owner")(echoHandler(t))))

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/test-workspace/agents", nil)
	rr := httptest.NewRecorder()

	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
