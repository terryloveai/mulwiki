package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSkillCreatePostsNameAndDescription(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/demo/agents/skills" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"skill":{"id":"sk1","workspace_id":"ws1","name":"Parse","description":"Parse docs","created_at":"now"}}`))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := skillCreateTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	setFlags(t, cmd, map[string]string{"name": "Parse", "description": "Parse docs"})

	if err := runSkillCreate(cmd, nil); err != nil {
		t.Fatalf("skill create: %v", err)
	}
	if !strings.Contains(out.String(), `"id": "sk1"`) {
		t.Fatalf("output = %q", out.String())
	}
}

func skillCreateTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addSkillCreateFlags(cmd)
	return cmd
}
