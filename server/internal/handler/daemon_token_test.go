package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tethy/mulwiki/server/internal/auth"
	"github.com/tethy/mulwiki/server/internal/middleware"
)

func TestCreateDaemonTokenStoresHashAndReturnsRawTokenOnce(t *testing.T) {
	h := newTestHandler(t)

	body := `{"daemon_id":"daemon-1"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/daemon-tokens", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req = req.WithContext(middleware.WithDaemon(req.Context(), "ws1", ""))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateDaemonToken(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID        string `json:"id"`
		DaemonID  string `json:"daemon_id"`
		Token     string `json:"token"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == "" || resp.Token == "" || resp.CreatedAt == "" {
		t.Fatalf("expected id/token/created_at in response: %#v", resp)
	}
	if resp.DaemonID != "daemon-1" {
		t.Fatalf("expected daemon-1, got %q", resp.DaemonID)
	}
	if !strings.HasPrefix(resp.Token, "mwd_") {
		t.Fatalf("expected raw mwd_ token, got %q", resp.Token)
	}

	var storedHash string
	if err := h.DB.QueryRow(
		`SELECT token_hash FROM daemon_tokens WHERE id = ?`,
		resp.ID,
	).Scan(&storedHash); err != nil {
		t.Fatalf("query stored token: %v", err)
	}
	if storedHash == "" || storedHash == resp.Token {
		t.Fatalf("expected only token hash to be stored, got hash=%q", storedHash)
	}

	workspaceID, daemonID, ok, err := auth.VerifyDaemonToken(context.Background(), h.DB, resp.Token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if !ok || workspaceID != "ws1" || daemonID != "daemon-1" {
		t.Fatalf("expected token to verify for ws1/daemon-1, got ok=%v workspace=%q daemon=%q", ok, workspaceID, daemonID)
	}
}

func TestCreateDaemonTokenGeneratesDaemonIDWhenMissing(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/daemon-tokens", map[string]string{"slug": "test-workspace"}, strings.NewReader(`{}`))
	req = req.WithContext(middleware.WithDaemon(req.Context(), "ws1", ""))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateDaemonToken(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		DaemonID string `json:"daemon_id"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.DaemonID == "" || resp.Token == "" {
		t.Fatalf("expected generated daemon id and token, got %#v", resp)
	}
}

func TestCreateDaemonTokenRejectsUnknownWorkspace(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodPost, "/api/workspaces/missing/daemon-tokens", map[string]string{"slug": "missing"}, strings.NewReader(`{"daemon_id":"daemon-1"}`))
	rr := httptest.NewRecorder()

	h.CreateDaemonToken(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}

	var count int
	err := h.DB.QueryRow(`SELECT COUNT(*) FROM daemon_tokens`).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("count tokens: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no token rows, got %d", count)
	}
}
