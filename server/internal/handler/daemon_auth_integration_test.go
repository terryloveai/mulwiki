package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/tethy/mulwiki/server/internal/auth"
	"github.com/tethy/mulwiki/server/internal/middleware"
)

func seedHandlerDaemonToken(t *testing.T, h *Handler, workspaceID, daemonID string) string {
	t.Helper()

	raw, hash, err := auth.NewDaemonToken()
	if err != nil {
		t.Fatalf("new daemon token: %v", err)
	}
	if _, err := h.DB.Exec(
		`INSERT INTO daemon_tokens (workspace_id, daemon_id, token_hash) VALUES (?, ?, ?)`,
		workspaceID, daemonID, hash,
	); err != nil {
		t.Fatalf("insert daemon token: %v", err)
	}
	return raw
}

func newDaemonClaimRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/daemon", func(r chi.Router) {
		r.Use(middleware.DaemonAuth(h.DB))
		r.Route("/workspaces/{slug}", func(r chi.Router) {
			r.Use(middleware.Workspace(h.DB))
			r.Post("/jobs/claim", h.ClaimJob)
		})
	})
	return r
}

func TestDaemonTokenRequiredForClaimRoute(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status) VALUES ('job1', 'ws1', 'pending')`)
	router := newDaemonClaimRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/daemon/workspaces/test-workspace/jobs/claim", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDaemonTokenCanClaimOnlySameWorkspaceJob(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.DB.Exec(`INSERT INTO workspaces (id, slug, name) VALUES ('ws2', 'other-workspace', 'Other')`); err != nil {
		t.Fatalf("seed other workspace: %v", err)
	}
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status) VALUES ('job1', 'ws1', 'pending')`)
	router := newDaemonClaimRouter(h)

	wrongToken := seedHandlerDaemonToken(t, h, "ws2", "daemon-wrong")
	req := httptest.NewRequest(http.MethodPost, "/api/daemon/workspaces/test-workspace/jobs/claim", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+wrongToken)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong workspace token, got %d: %s", rr.Code, rr.Body.String())
	}

	validToken := seedHandlerDaemonToken(t, h, "ws1", "daemon-valid")
	req = httptest.NewRequest(http.MethodPost, "/api/daemon/workspaces/test-workspace/jobs/claim", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+validToken)
	rr = httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid workspace token, got %d: %s", rr.Code, rr.Body.String())
	}

	var claimedBy string
	if err := h.DB.QueryRow(`SELECT claimed_by FROM jobs WHERE id = 'job1'`).Scan(&claimedBy); err != nil {
		t.Fatalf("query claimed_by: %v", err)
	}
	if claimedBy != "daemon-valid" {
		t.Fatalf("expected daemon-valid claimed_by, got %q", claimedBy)
	}
}
