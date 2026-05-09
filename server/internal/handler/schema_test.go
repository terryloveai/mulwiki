package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

func TestListBuiltinSchemasReadsCatalogFromBuiltinDir(t *testing.T) {
	h := newTestHandler(t)
	h.BuiltinSchemasDir = writeBuiltinSchemaFixture(t, t.TempDir())

	req := chiRequest(http.MethodGet, "/api/schemas/builtin", nil, nil)
	rr := httptest.NewRecorder()

	h.ListBuiltinSchemas(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var schemas []protocol.Schema
	if err := json.NewDecoder(rr.Body).Decode(&schemas); err != nil {
		t.Fatalf("decode schemas: %v", err)
	}
	if len(schemas) != 1 {
		t.Fatalf("expected 1 builtin schema from catalog, got %d", len(schemas))
	}
	if schemas[0].ID != "concept-wiki-schema.md" {
		t.Fatalf("expected filename id, got %q", schemas[0].ID)
	}
	if schemas[0].Path != "schemas/concept-wiki-schema.md" {
		t.Fatalf("expected workspace schema path, got %q", schemas[0].Path)
	}
	if schemas[0].SourceType != "builtin" {
		t.Fatalf("expected source_type builtin, got %q", schemas[0].SourceType)
	}
}

func TestUpdateSchemaContentOnlyUpdatesGitAndReturnsSchema(t *testing.T) {
	h := newTestHandler(t)
	dataDir := t.TempDir()
	h.ReposDir = filepath.Join(dataDir, "repos")
	h.BuiltinSchemasDir = writeBuiltinSchemaFixture(t, dataDir)

	createBody := `{"name":"Content Only","slug":"content-only","initial_schema_type":"blank"}`
	createReq := chiRequest(http.MethodPost, "/api/workspaces", nil, strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	h.CreateWorkspace(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create workspace: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var ws protocol.Workspace
	if err := json.NewDecoder(createRR.Body).Decode(&ws); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	if ws.ActiveSchemaID == nil || *ws.ActiveSchemaID == "" {
		t.Fatal("expected blank schema to be active")
	}

	content := "# Types\n\n- Updated\n\n## Structure\n\nFree links\n"
	updateBody := `{"content":` + strconv.Quote(content) + `}`
	req := chiRequest(http.MethodPut, "/api/workspaces/content-only/schemas/"+*ws.ActiveSchemaID, map[string]string{
		"slug": "content-only",
		"id":   *ws.ActiveSchemaID,
	}, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.UpdateSchema(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var schema protocol.Schema
	if err := json.NewDecoder(rr.Body).Decode(&schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if schema.ID != *ws.ActiveSchemaID {
		t.Fatalf("expected schema %q, got %q", *ws.ActiveSchemaID, schema.ID)
	}
	if schema.Content != content {
		t.Fatalf("expected updated content, got %q", schema.Content)
	}
	if stored := h.readSchemaFromGit(ws.ID, schema.Path); stored != content {
		t.Fatalf("expected updated content in git, got %q", stored)
	}
}
