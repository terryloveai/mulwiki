package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveDaemonTokenPrecedence(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("mwd_file-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	t.Setenv("MULWIKI_DAEMON_TOKEN", "mwd_env-token")
	token, err := resolveDaemonToken("mwd_flag-token", tokenPath)
	if err != nil {
		t.Fatalf("resolve flag token: %v", err)
	}
	if token != "mwd_flag-token" {
		t.Fatalf("expected flag token, got %q", token)
	}

	token, err = resolveDaemonToken("", tokenPath)
	if err != nil {
		t.Fatalf("resolve env token: %v", err)
	}
	if token != "mwd_env-token" {
		t.Fatalf("expected env token, got %q", token)
	}

	t.Setenv("MULWIKI_DAEMON_TOKEN", "")
	token, err = resolveDaemonToken("", tokenPath)
	if err != nil {
		t.Fatalf("resolve file token: %v", err)
	}
	if token != "mwd_file-token" {
		t.Fatalf("expected file token, got %q", token)
	}
}

func TestResolveDaemonTokenMissingIsEmpty(t *testing.T) {
	t.Setenv("MULWIKI_DAEMON_TOKEN", "")
	token, err := resolveDaemonToken("", filepath.Join(t.TempDir(), "missing-token"))
	if err != nil {
		t.Fatalf("resolve missing token: %v", err)
	}
	if token != "" {
		t.Fatalf("expected empty token, got %q", token)
	}
}

func TestResolveDaemonTokenForStartRefreshesCachedTokenWhenSessionExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MULWIKI_DAEMON_TOKEN", "")
	var mintCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/daemon-tokens" {
			http.NotFound(w, r)
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value != "sess-daemon" {
			t.Fatalf("missing session cookie: %v %#v", err, cookie)
		}
		mintCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"mwd_fresh"}`))
	}))
	defer server.Close()

	cfg := CLIConfig{
		ServerURL:     server.URL,
		WorkspaceSlug: "demo",
		SessionID:     "sess-daemon",
		DaemonToken:   "mwd_old-global",
		DaemonTokens:  map[string]string{"demo": "mwd_cached"},
	}
	if err := saveCLIConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("mwd_old-file\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	token, err := resolveDaemonTokenForStart("", tokenPath, cfg, "demo", "daemon-1", server.URL)
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "mwd_fresh" || mintCount != 1 {
		t.Fatalf("token=%q mintCount=%d, want fresh token minted once", token, mintCount)
	}

	loaded, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.DaemonToken != "mwd_fresh" || loaded.DaemonTokens["demo"] != "mwd_fresh" {
		t.Fatalf("fresh token was not cached: %#v", loaded)
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if string(data) != "mwd_fresh\n" {
		t.Fatalf("token file = %q, want fresh token", string(data))
	}
}

func TestResolveDaemonTokenForStartMintsAndCachesWithCLISession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MULWIKI_DAEMON_TOKEN", "")

	var sawCookie bool
	var sawDaemonID bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/demo/daemon-tokens" {
			http.NotFound(w, r)
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value == "sess-daemon" {
			sawCookie = true
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		if string(body) == `{"daemon_id":"daemon-1"}`+"\n" || string(body) == `{"daemon_id":"daemon-1"}` {
			sawDaemonID = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"tok1","workspace_id":"ws1","daemon_id":"daemon-1","token":"mwd_minted","created_at":"now"}`))
	}))
	defer server.Close()

	cfg := CLIConfig{
		ServerURL:     server.URL,
		WorkspaceSlug: "demo",
		SessionID:     "sess-daemon",
	}
	if err := saveCLIConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	token, err := resolveDaemonTokenForStart("", filepath.Join(t.TempDir(), "missing-token"), cfg, "demo", "daemon-1", server.URL)
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "mwd_minted" || !sawCookie || !sawDaemonID {
		t.Fatalf("unexpected mint result token=%q sawCookie=%v sawDaemonID=%v", token, sawCookie, sawDaemonID)
	}

	loaded, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.DaemonTokens["demo"] != "mwd_minted" {
		t.Fatalf("minted token was not cached: %#v", loaded.DaemonTokens)
	}
}

func TestResolveDaemonTokenForStartPreservesDefaultWorkspaceWhenAutoDiscovering(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MULWIKI_DAEMON_TOKEN", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/daemon-tokens" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"mwd_auto"}`))
	}))
	defer server.Close()

	cfg := CLIConfig{
		ServerURL:     server.URL,
		WorkspaceSlug: "demo",
		SessionID:     "sess-auto",
	}
	if err := saveCLIConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	token, err := resolveDaemonTokenForStart("", filepath.Join(t.TempDir(), "token"), cfg, "", "daemon-1", server.URL)
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "mwd_auto" {
		t.Fatalf("token = %q, want minted token", token)
	}

	loaded, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.WorkspaceSlug != "demo" {
		t.Fatalf("workspace slug was cleared: %#v", loaded)
	}
	if loaded.DaemonTokens["demo"] != "mwd_auto" {
		t.Fatalf("daemon token not cached for default workspace: %#v", loaded.DaemonTokens)
	}
}

func TestBuildDaemonStartArgsForwardsBackgroundFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("server-url", "", "")
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("repos-path", "", "")
	cmd.Flags().String("daemon-id", "", "")
	cmd.Flags().String("daemon-token", "", "")
	cmd.Flags().Set("server-url", "http://localhost:8080")
	cmd.Flags().Set("workspace", "demo")
	cmd.Flags().Set("repos-path", "/tmp/repos")
	cmd.Flags().Set("daemon-id", "daemon-1")
	cmd.Flags().Set("daemon-token", "mwd_test")

	got := buildDaemonStartArgs(cmd)
	want := []string{
		"daemon", "start", "--foreground",
		"--server-url", "http://localhost:8080",
		"--workspace", "demo",
		"--repos-path", "/tmp/repos",
		"--daemon-id", "daemon-1",
		"--daemon-token", "mwd_test",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("buildDaemonStartArgs() = %#v, want %#v", got, want)
	}
}
