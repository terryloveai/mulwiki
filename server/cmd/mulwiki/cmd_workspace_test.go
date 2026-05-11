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

func TestWorkspaceListSupportsJSONOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"ws1","slug":"demo","name":"Demo","description":"","created_at":"now"}]`))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := workspaceListTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("set output: %v", err)
	}

	if err := runWorkspaceList(cmd, nil); err != nil {
		t.Fatalf("workspace list: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json output %q: %v", out.String(), err)
	}
	if len(got) != 1 || got[0]["slug"] != "demo" {
		t.Fatalf("workspace output = %#v", got)
	}
}

func TestWorkspaceCreatePostsInitialSchemaAndSavesDefaultWhenUseIsSet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["name"] != "Demo" || req["slug"] != "demo" || req["initial_schema_type"] != "blank" {
			t.Fatalf("request = %#v", req)
		}
		sawRequest = true
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ws1","slug":"demo","name":"Demo","description":"","created_at":"now"}`))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := workspaceCreateTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	setFlags(t, cmd, map[string]string{
		"name":   "Demo",
		"slug":   "demo",
		"schema": "blank",
		"use":    "true",
	})

	if err := runWorkspaceCreate(cmd, nil); err != nil {
		t.Fatalf("workspace create: %v", err)
	}

	cfg, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !sawRequest || cfg.WorkspaceSlug != "demo" {
		t.Fatalf("request=%v workspace slug=%q", sawRequest, cfg.WorkspaceSlug)
	}
	if !strings.Contains(out.String(), `"slug": "demo"`) {
		t.Fatalf("expected json response, got %q", out.String())
	}
}

func TestWorkspaceDeleteRequiresYes(t *testing.T) {
	cmd := workspaceDeleteTestCmd()
	if err := runWorkspaceDelete(cmd, []string{"demo"}); err == nil {
		t.Fatal("expected --yes requirement")
	}
}

func workspaceListTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addOutputFlag(cmd, outputTable)
	return cmd
}

func workspaceCreateTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addWorkspaceCreateFlags(cmd)
	return cmd
}

func workspaceDeleteTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addWorkspaceDeleteFlags(cmd)
	return cmd
}

func setFlags(t *testing.T, cmd *cobra.Command, values map[string]string) {
	t.Helper()
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
}
