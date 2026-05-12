package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadCLIConfigRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := CLIConfig{
		ServerURL:     "http://localhost:8080",
		WorkspaceSlug: "demo",
		SessionID:     "sess-123",
		DaemonTokens:  map[string]string{"demo": "mwd_demo-token"},
	}

	if err := saveCLIConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.ServerURL != cfg.ServerURL || loaded.WorkspaceSlug != cfg.WorkspaceSlug || loaded.SessionID != cfg.SessionID {
		t.Fatalf("loaded config mismatch: %#v", loaded)
	}
	if loaded.DaemonTokens["demo"] != "mwd_demo-token" {
		t.Fatalf("daemon token not preserved: %#v", loaded.DaemonTokens)
	}

	path, err := cliConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %v, want 0600", info.Mode().Perm())
	}
}

func TestAuthRefreshMintsUserDaemonTokenAndClearsOldTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var sawCookie bool
	var sawDaemonID bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/daemon-tokens" {
			http.NotFound(w, r)
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value == "sess-refresh" {
			sawCookie = true
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		if strings.Contains(string(body), `"daemon_id":"daemon-refresh"`) {
			sawDaemonID = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"mwd_refreshed"}`))
	}))
	defer server.Close()

	if err := saveCLIConfig(CLIConfig{
		ServerURL:     server.URL,
		WorkspaceSlug: "demo",
		SessionID:     "sess-refresh",
		DaemonToken:   "mwd_old",
		DaemonTokens:  map[string]string{"demo": "mwd_old_workspace"},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	daemonIDPath := daemonIDPath()
	if err := os.MkdirAll(filepath.Dir(daemonIDPath), 0o755); err != nil {
		t.Fatalf("mkdir daemon dir: %v", err)
	}
	if err := os.WriteFile(daemonIDPath, []byte("daemon-refresh\n"), 0o600); err != nil {
		t.Fatalf("write daemon id: %v", err)
	}
	if err := os.WriteFile(daemonTokenPath(), []byte("mwd_old_file\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	cmd := authRefreshCmd
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := runAuthRefresh(cmd, nil); err != nil {
		t.Fatalf("auth refresh: %v", err)
	}
	if !sawCookie || !sawDaemonID {
		t.Fatalf("request missing expected auth or daemon id, sawCookie=%v sawDaemonID=%v", sawCookie, sawDaemonID)
	}

	cfg, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DaemonToken != "mwd_refreshed" {
		t.Fatalf("daemon token = %q, want refreshed", cfg.DaemonToken)
	}
	if cfg.DaemonTokens["demo"] != "mwd_refreshed" {
		t.Fatalf("workspace daemon token = %#v, want refreshed", cfg.DaemonTokens)
	}
	data, err := os.ReadFile(daemonTokenPath())
	if err != nil {
		t.Fatalf("read daemon token file: %v", err)
	}
	if string(data) != "mwd_refreshed\n" {
		t.Fatalf("daemon token file = %q, want refreshed token", string(data))
	}
}

func TestLoginWithCredentialsStoresSessionCookie(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			if r.Method != http.MethodPost {
				t.Fatalf("login method = %s", r.Method)
			}
			http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "sess-login", Path: "/"})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"u1","email":"dev@example.com","created_at":"now"}`))
		case "/api/workspaces":
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || cookie.Value != "sess-login" {
				t.Fatalf("workspace list missing session cookie: %v %#v", err, cookie)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"ws1","slug":"demo","name":"Demo","description":"","created_at":"now"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	user, err := loginWithCredentials(server.URL, "dev@example.com", "password123", "")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if user.Email != "dev@example.com" {
		t.Fatalf("user email = %q", user.Email)
	}

	cfg, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ServerURL != server.URL || cfg.SessionID != "sess-login" || cfg.WorkspaceSlug != "demo" {
		t.Fatalf("saved config mismatch: %#v", cfg)
	}
}

func TestSetupSelfHostLogsInAndSavesConfigWithoutStartingDaemon(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "sess-setup", Path: "/"})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"u1","email":"dev@example.com","created_at":"now"}`))
		case "/api/workspaces":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"ws1","slug":"ignored","name":"Ignored","description":"","created_at":"now"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := setupSelfHost(server.URL, "demo", "dev@example.com", "password123", true); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ServerURL != server.URL || cfg.WorkspaceSlug != "demo" || cfg.SessionID != "sess-setup" {
		t.Fatalf("setup config mismatch: %#v", cfg)
	}
}

func TestAPIClientSendsSavedSessionCookie(t *testing.T) {
	var sawCookie bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value == "sess-client" {
			sawCookie = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := newAPIClient(server.URL)
	client.setSessionID("sess-client")

	var resp struct {
		OK bool `json:"ok"`
	}
	if err := client.get("/anything", &resp); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !resp.OK || !sawCookie {
		t.Fatalf("expected response ok and session cookie, ok=%v sawCookie=%v", resp.OK, sawCookie)
	}
}

func TestAuthLogoutClearsSessionAndDaemonTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveCLIConfig(CLIConfig{
		ServerURL:     "http://example.invalid",
		WorkspaceSlug: "demo",
		SessionID:     "sess-old",
		DaemonTokens:  map[string]string{"demo": "mwd_old"},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if err := clearCLIAuth(); err != nil {
		t.Fatalf("clear auth: %v", err)
	}

	cfg, err := loadCLIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SessionID != "" || len(cfg.DaemonTokens) != 0 {
		t.Fatalf("auth data not cleared: %#v", cfg)
	}
	if cfg.ServerURL != "http://example.invalid" || cfg.WorkspaceSlug != "demo" {
		t.Fatalf("non-auth config should be preserved: %#v", cfg)
	}
}

func TestPromptInputTrimsWhitespace(t *testing.T) {
	got, err := readLine(strings.NewReader(" dev@example.com \n"), "Email: ")
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	if got != "dev@example.com" {
		t.Fatalf("read line = %q", got)
	}
}

func TestCLIConfigPathUsesMulwikiDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := cliConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	want := filepath.Join(home, ".mulwiki", "config.json")
	if path != want {
		t.Fatalf("config path = %q, want %q", path, want)
	}
}
