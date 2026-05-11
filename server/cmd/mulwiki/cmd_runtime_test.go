package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRuntimeCreatePostsRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/demo/agents/runtimes" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"runtime":{"id":"rt1","workspace_id":"ws1","name":"Codex","backend":"codex","path":"codex","hostname":"","os":"","version":"","status":"offline","daemon_id":"","last_heartbeat":"","created_at":"now"}}`))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := runtimeCreateTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	setFlags(t, cmd, map[string]string{"name": "Codex", "backend": "codex", "path": "codex"})

	if err := runRuntimeCreate(cmd, nil); err != nil {
		t.Fatalf("runtime create: %v", err)
	}
	if !strings.Contains(out.String(), `"id": "rt1"`) {
		t.Fatalf("output = %q", out.String())
	}
}

func runtimeCreateTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addRuntimeCreateFlags(cmd)
	return cmd
}
