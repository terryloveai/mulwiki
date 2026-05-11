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

func TestWikiCreateReadsContentFromStdin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var req map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/demo/wiki" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"path":"/concepts/ai","title":"AI","content":"Body","type":"concept"}`))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := wikiCreateTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("Body\n"))
	setFlags(t, cmd, map[string]string{"title": "AI", "type": "concept", "content-stdin": "true"})

	if err := runWikiCreate(cmd, []string{"/concepts/ai"}); err != nil {
		t.Fatalf("wiki create: %v", err)
	}
	if req["content"] != "Body" || !strings.Contains(out.String(), `"/concepts/ai"`) {
		t.Fatalf("request=%#v output=%q", req, out.String())
	}
}

func wikiCreateTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addWikiCreateFlags(cmd)
	return cmd
}
