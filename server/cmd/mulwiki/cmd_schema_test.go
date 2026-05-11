package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSchemaCreateReadsContentFromStdin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var sawContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/demo/schemas" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		sawContent = req["content"]
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"schema1","workspace_id":"ws1","name":"Demo Schema","description":"","version":"1.0","path":"schemas/schema1.md","content":"# Demo","source_type":"user","created_at":"now"}`))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := schemaCreateTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("# Demo\n"))
	setFlags(t, cmd, map[string]string{
		"name":          "Demo Schema",
		"content-stdin": "true",
	})

	if err := runSchemaCreate(cmd, nil); err != nil {
		t.Fatalf("schema create: %v", err)
	}

	if sawContent != "# Demo" {
		t.Fatalf("content = %q", sawContent)
	}
	if !strings.Contains(out.String(), `"id": "schema1"`) {
		t.Fatalf("expected json output, got %q", out.String())
	}
}

func TestSchemaGetContentResolvesPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/workspaces/demo/schemas":
			_, _ = w.Write([]byte(`[{"id":"schema1","workspace_id":"ws1","name":"Demo","description":"","version":"1.0","path":"schemas/schema1.md","source_type":"user","created_at":"now","is_active":true}]`))
		case "/api/workspaces/demo/schemas/schema1":
			_, _ = w.Write([]byte(`{"id":"schema1","workspace_id":"ws1","name":"Demo","description":"","version":"1.0","path":"schemas/schema1.md","content":"# Demo","source_type":"user","created_at":"now"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := schemaGetTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("content", "true"); err != nil {
		t.Fatalf("set content: %v", err)
	}

	if err := runSchemaGet(cmd, []string{"schemas/schema1.md"}); err != nil {
		t.Fatalf("schema get: %v", err)
	}

	if out.String() != "# Demo\n" {
		t.Fatalf("content output = %q", out.String())
	}
}

func schemaCreateTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addSchemaCreateFlags(cmd)
	return cmd
}

func schemaGetTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addSchemaGetFlags(cmd)
	return cmd
}
