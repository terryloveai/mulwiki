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

func TestAgentCreateResolvesRuntimeNameAndPostsRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var sawRuntimeID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/workspaces/demo/agents/runtimes":
			_, _ = w.Write([]byte(`{"runtimes":[{"id":"rt1","workspace_id":"ws1","name":"Codex","backend":"codex","path":"codex","hostname":"","os":"","version":"","status":"online","daemon_id":"","last_heartbeat":"","created_at":"now"}]}`))
		case "/api/workspaces/demo/agents":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			sawRuntimeID, _ = req["runtime_id"].(string)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"agent":{"id":"agent1","workspace_id":"ws1","runtime_id":"rt1","name":"Writer","description":"","instructions":"Be useful.","runtime_mode":"codex","runtime_config":{},"custom_env":{},"custom_args":[],"mcp_config":{},"visibility":"private","status":"offline","max_concurrent_tasks":6,"model":"gpt-5.4","owner_id":"","skills":[],"created_at":"now","updated_at":"now"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := agentCreateTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	setFlags(t, cmd, map[string]string{
		"name":         "Writer",
		"runtime":      "Codex",
		"instructions": "Be useful.",
		"model":        "gpt-5.4",
	})

	if err := runAgentCreate(cmd, nil); err != nil {
		t.Fatalf("agent create: %v", err)
	}

	if sawRuntimeID != "rt1" || !strings.Contains(out.String(), `"id": "agent1"`) {
		t.Fatalf("runtime=%q output=%q", sawRuntimeID, out.String())
	}
}

func agentCreateTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addAgentCreateFlags(cmd)
	return cmd
}
