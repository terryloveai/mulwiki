package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// ---------------------------------------------------------------------------
// Workspace CRUD
// ---------------------------------------------------------------------------

func TestCreateWorkspace(t *testing.T) {
	h := newTestHandler(t)

	body := `{"name":"My Workspace","slug":"my-workspace","description":"Test workspace"}`
	req := chiRequest(http.MethodPost, "/api/workspaces", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateWorkspace(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var ws protocol.Workspace
	if err := json.NewDecoder(rr.Body).Decode(&ws); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	if ws.Name != "My Workspace" {
		t.Errorf("expected name 'My Workspace', got '%s'", ws.Name)
	}
	if ws.Slug != "my-workspace" {
		t.Errorf("expected slug 'my-workspace', got '%s'", ws.Slug)
	}
	if ws.Description != "Test workspace" {
		t.Errorf("expected description, got '%s'", ws.Description)
	}
}

func TestCreateWorkspace_AutoSlug(t *testing.T) {
	h := newTestHandler(t)

	body := `{"name":"My Workspace"}`
	req := chiRequest(http.MethodPost, "/api/workspaces", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateWorkspace(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var ws protocol.Workspace
	json.NewDecoder(rr.Body).Decode(&ws)
	if ws.Slug != "my-workspace" {
		t.Errorf("expected auto-generated slug 'my-workspace', got '%s'", ws.Slug)
	}
}

func TestCreateWorkspace_MissingName(t *testing.T) {
	h := newTestHandler(t)

	body := `{"slug":"no-name"}`
	req := chiRequest(http.MethodPost, "/api/workspaces", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateWorkspace(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCreateWorkspace_InvalidSlug(t *testing.T) {
	h := newTestHandler(t)

	body := `{"name":"Bad Slug","slug":"INVALID!!!"}`
	req := chiRequest(http.MethodPost, "/api/workspaces", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateWorkspace(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCreateWorkspace_DuplicateSlug(t *testing.T) {
	h := newTestHandler(t)

	// First creation succeeds.
	body := `{"name":"First","slug":"dup-slug"}`
	req := chiRequest(http.MethodPost, "/api/workspaces", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateWorkspace(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", rr.Code)
	}

	// Second creation with same slug fails.
	body2 := `{"name":"Second","slug":"dup-slug"}`
	req2 := chiRequest(http.MethodPost, "/api/workspaces", nil, strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	h.CreateWorkspace(rr2, req2)

	if rr2.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr2.Code)
	}
}

func TestListWorkspaces(t *testing.T) {
	h := newTestHandler(t)
	// Seed extra workspaces (one already exists in newTestHandler).
	h.DB.Exec(`INSERT INTO workspaces (id, slug, name, description) VALUES ('ws2', 'workspace-2', 'WS2', 'desc2')`)

	req := chiRequest(http.MethodGet, "/api/workspaces", nil, nil)
	rr := httptest.NewRecorder()

	h.ListWorkspaces(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var wss []protocol.Workspace
	if err := json.NewDecoder(rr.Body).Decode(&wss); err != nil {
		t.Fatalf("decode workspaces: %v", err)
	}

	// Should have ws2 + test-workspace (but builtin is filtered).
	if len(wss) < 1 {
		t.Errorf("expected at least 1 workspace, got %d", len(wss))
	}
}

func TestGetWorkspace(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace", map[string]string{"slug": "test-workspace"}, nil)
	rr := httptest.NewRecorder()

	h.GetWorkspace(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var ws protocol.Workspace
	json.NewDecoder(rr.Body).Decode(&ws)
	if ws.Slug != "test-workspace" {
		t.Errorf("expected slug 'test-workspace', got '%s'", ws.Slug)
	}
}

func TestGetWorkspace_NotFound(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodGet, "/api/workspaces/nonexistent", map[string]string{"slug": "nonexistent"}, nil)
	rr := httptest.NewRecorder()

	h.GetWorkspace(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestUpdateWorkspace(t *testing.T) {
	h := newTestHandler(t)

	body := `{"name":"Updated WS","description":"Updated desc"}`
	req := chiRequest(http.MethodPatch, "/api/workspaces/test-workspace", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.UpdateWorkspace(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var ws protocol.Workspace
	json.NewDecoder(rr.Body).Decode(&ws)
	if ws.Name != "Updated WS" {
		t.Errorf("expected name 'Updated WS', got '%s'", ws.Name)
	}
	if ws.Description != "Updated desc" {
		t.Errorf("expected description, got '%s'", ws.Description)
	}
}

func TestUpdateWorkspace_NotFound(t *testing.T) {
	h := newTestHandler(t)

	body := `{"name":"Nope"}`
	req := chiRequest(http.MethodPatch, "/api/workspaces/nonexistent", map[string]string{"slug": "nonexistent"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.UpdateWorkspace(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDeleteWorkspace(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO workspaces (id, slug, name) VALUES ('ws-del', 'delete-me', 'Delete Me')`)

	req := chiRequest(http.MethodDelete, "/api/workspaces/delete-me", map[string]string{"slug": "delete-me"}, nil)
	rr := httptest.NewRecorder()

	h.DeleteWorkspace(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}

	// Verify deleted.
	var count int
	h.DB.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE slug = 'delete-me'`).Scan(&count)
	if count != 0 {
		t.Error("workspace should be deleted")
	}
}

func TestDeleteWorkspace_NotFound(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodDelete, "/api/workspaces/nonexistent", map[string]string{"slug": "nonexistent"}, nil)
	rr := httptest.NewRecorder()

	h.DeleteWorkspace(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Slug utilities
// ---------------------------------------------------------------------------

func TestIsValidSlug(t *testing.T) {
	tests := []struct {
		slug  string
		valid bool
	}{
		{"hello", true},
		{"hello-world", true},
		{"a-b-c", true},
		{"my-project-123", true},
		{"", false},
		{"-leading", false},
		{"trailing-", false},
		{"has spaces", false},
		{"UPPERCASE", false},
		{"slash/thing", false},
	}

	for _, tt := range tests {
		got := isValidSlug(tt.slug)
		if got != tt.valid {
			t.Errorf("isValidSlug(%q) = %v, want %v", tt.slug, got, tt.valid)
		}
	}
}

func TestCleanSlug(t *testing.T) {
	tests := []struct {
		raw, expected string
	}{
		{"My Workspace", "my-workspace"},
		{"  Hello  World  ", "hello-world"},
		{"UPPERCASE", "uppercase"},
		{"a--b", "a-b"},
		{"a---b", "a-b"},
	}

	for _, tt := range tests {
		got := CleanSlug(tt.raw)
		if got != tt.expected {
			t.Errorf("CleanSlug(%q) = %q, want %q", tt.raw, got, tt.expected)
		}
	}
}
