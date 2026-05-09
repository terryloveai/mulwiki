package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	req.Header.Set("X-User-ID", "dev-user")
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

	var role string
	if err := h.DB.QueryRow(
		`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = 'dev-user'`,
		ws.ID,
	).Scan(&role); err != nil {
		t.Fatalf("expected workspace owner membership: %v", err)
	}
	if role != "owner" {
		t.Errorf("expected owner role, got %q", role)
	}
}

func TestCreateWorkspace_SeedsBuiltinSchemas(t *testing.T) {
	h := newTestHandler(t)
	dataDir := t.TempDir()
	h.ReposDir = filepath.Join(dataDir, "repos")
	h.BuiltinSchemasDir = writeBuiltinSchemaFixture(t, dataDir)

	body := `{"name":"Seeded Workspace","slug":"seeded-workspace"}`
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

	var count int
	if err := h.DB.QueryRow(
		`SELECT COUNT(*) FROM schemas WHERE workspace_id = ? AND source_type = 'builtin'`,
		ws.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count builtin schemas: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 builtin schema, got %d", count)
	}
	if content := h.readSchemaFromGit(ws.ID, "schemas/concept-wiki-schema.md"); !strings.Contains(content, "# Types") {
		t.Fatalf("expected builtin schema content in workspace repo, got %q", content)
	}
}

func TestCreateWorkspace_ActivatesSelectedBuiltinSchema(t *testing.T) {
	h := newTestHandler(t)
	dataDir := t.TempDir()
	h.ReposDir = filepath.Join(dataDir, "repos")
	h.BuiltinSchemasDir = writeBuiltinSchemaFixture(t, dataDir)

	body := `{"name":"Selected Workspace","slug":"selected-workspace","initial_schema_type":"builtin","initial_schema_path":"schemas/concept-wiki-schema.md"}`
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

	var activeSchemaID, activeSchemaPath, sourceType string
	if err := h.DB.QueryRow(
		`SELECT COALESCE(w.active_schema_id, ''), w.active_schema_path, s.source_type
		 FROM workspaces w
		 JOIN schemas s ON s.id = w.active_schema_id
		 WHERE w.id = ?`,
		ws.ID,
	).Scan(&activeSchemaID, &activeSchemaPath, &sourceType); err != nil {
		t.Fatalf("load active schema: %v", err)
	}
	if activeSchemaID == "" {
		t.Fatal("expected active schema id to be set")
	}
	if activeSchemaPath != "schemas/concept-wiki-schema.md" {
		t.Fatalf("expected active schema path to be selected builtin, got %q", activeSchemaPath)
	}
	if sourceType != "builtin" {
		t.Fatalf("expected active schema source_type builtin, got %q", sourceType)
	}
}

func TestCreateWorkspace_BlankSchemaStartsWithUserSchema(t *testing.T) {
	h := newTestHandler(t)
	dataDir := t.TempDir()
	h.ReposDir = filepath.Join(dataDir, "repos")
	h.BuiltinSchemasDir = writeBuiltinSchemaFixture(t, dataDir)

	body := `{"name":"Blank Workspace","slug":"blank-workspace","initial_schema_type":"blank"}`
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

	var activeSchemaPath, name, sourceType string
	if err := h.DB.QueryRow(
		`SELECT w.active_schema_path, s.name, s.source_type
		 FROM workspaces w
		 JOIN schemas s ON s.id = w.active_schema_id
		 WHERE w.id = ?`,
		ws.ID,
	).Scan(&activeSchemaPath, &name, &sourceType); err != nil {
		t.Fatalf("load blank active schema: %v", err)
	}
	if activeSchemaPath != "schemas/blank-schema.md" {
		t.Fatalf("expected blank active schema path, got %q", activeSchemaPath)
	}
	if name != "Blank Schema" {
		t.Fatalf("expected blank schema name, got %q", name)
	}
	if sourceType != "user" {
		t.Fatalf("expected blank schema source_type user, got %q", sourceType)
	}
	if content := h.readSchemaFromGit(ws.ID, activeSchemaPath); !strings.Contains(content, "# Blank Schema") {
		t.Fatalf("expected blank schema content in workspace repo, got %q", content)
	}

	var builtinCount int
	if err := h.DB.QueryRow(
		`SELECT COUNT(*) FROM schemas WHERE workspace_id = ? AND source_type = 'builtin'`,
		ws.ID,
	).Scan(&builtinCount); err != nil {
		t.Fatalf("count builtin schemas: %v", err)
	}
	if builtinCount != 1 {
		t.Fatalf("expected builtin schemas to still be seeded, got %d", builtinCount)
	}
}

func writeBuiltinSchemaFixture(t *testing.T, dataDir string) string {
	t.Helper()
	builtinSchemasDir := filepath.Join(dataDir, "builtin", "schemas")
	if err := os.MkdirAll(builtinSchemasDir, 0755); err != nil {
		t.Fatalf("create builtin schema dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(builtinSchemasDir, "concept-wiki-schema.md"),
		[]byte("# Types\n\n- Concept\n"),
		0644,
	); err != nil {
		t.Fatalf("write builtin schema fixture: %v", err)
	}
	return builtinSchemasDir
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
	h.DB.Exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws2', 'dev-user', 'member')`)

	req := chiRequest(http.MethodGet, "/api/workspaces", nil, nil)
	req.Header.Set("X-User-ID", "dev-user")
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

func TestListWorkspaces_FiltersByMembership(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO users (id, email, password_hash) VALUES ('other-user', 'other@mulwiki.local', 'hash')`)
	h.DB.Exec(`INSERT INTO workspaces (id, slug, name, description) VALUES ('ws2', 'workspace-2', 'WS2', 'desc2')`)
	h.DB.Exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws2', 'other-user', 'owner')`)

	req := chiRequest(http.MethodGet, "/api/workspaces", nil, nil)
	req.Header.Set("X-User-ID", "dev-user")
	rr := httptest.NewRecorder()

	h.ListWorkspaces(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var wss []protocol.Workspace
	if err := json.NewDecoder(rr.Body).Decode(&wss); err != nil {
		t.Fatalf("decode workspaces: %v", err)
	}
	for _, ws := range wss {
		if ws.Slug == "workspace-2" {
			t.Fatalf("expected workspace-2 to be filtered out for dev-user")
		}
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
