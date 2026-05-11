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

func TestJobCreatePostsAgentSchemaAndSources(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var req map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/demo/jobs" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"job1","workspace_id":"ws1","status":"queued","agent_id":"agent1","source_path":"sources/a.md","source_paths":["sources/a.md"],"schema_id":"schema1","progress":0,"error":"","claimed_by":"","created_at":"now"}`))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := jobCreateTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	setFlags(t, cmd, map[string]string{
		"agent":  "agent1",
		"schema": "schema1",
		"source": "sources/a.md",
	})

	if err := runJobCreate(cmd, nil); err != nil {
		t.Fatalf("job create: %v", err)
	}

	if req["agent_id"] != "agent1" || req["schema_id"] != "schema1" || !strings.Contains(out.String(), `"id": "job1"`) {
		t.Fatalf("request=%#v output=%q", req, out.String())
	}
}

func jobCreateTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addJobCreateFlags(cmd)
	return cmd
}
