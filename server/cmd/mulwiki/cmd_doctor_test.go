package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCollectDoctorReportChecksServerAuthWorkspaceAndDaemon(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/auth/me":
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || cookie.Value != "sess-doctor" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"u1","email":"dev@example.com","created_at":"now"}`))
		case "/api/workspaces":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"ws1","slug":"demo","name":"Demo","description":"","created_at":"now"}]`))
		case "/api/daemon/workspaces":
			if r.Header.Get("Authorization") != "Bearer mwd_token" {
				http.Error(w, "invalid daemon token", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"workspaces":[{"id":"ws1","slug":"demo","name":"Demo"}]}`))
		case "/api/workspaces/demo/daemons":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"daemons":[{"id":"daemon-1","hostname":"host","pid":123,"version":"0.1.0","runtime_ids":"[]","workspace_slugs":["demo"],"max_concurrent_tasks":3,"last_heartbeat":"2099-01-01T00:00:00Z","registered_at":"now"}]}`))
		case "/api/workspaces/demo/agents/runtimes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"runtimes":[{"id":"rt1","name":"Codex","backend":"codex","status":"online","daemon_id":"daemon-1","last_heartbeat":"2099-01-01T00:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := saveCLIConfig(CLIConfig{
		ServerURL:     server.URL,
		WorkspaceSlug: "demo",
		SessionID:     "sess-doctor",
		DaemonToken:   "mwd_token",
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	report := collectDoctorReport("", 0)
	if report.Server.Status != "ok" {
		t.Fatalf("server check = %#v, want ok", report.Server)
	}
	if report.Auth.Status != "ok" || report.Auth.Detail != "dev@example.com" {
		t.Fatalf("auth check = %#v, want authenticated user", report.Auth)
	}
	if report.Workspace.Status != "ok" {
		t.Fatalf("workspace check = %#v, want ok", report.Workspace)
	}
	if report.DaemonToken.Status != "ok" {
		t.Fatalf("daemon token check = %#v, want ok", report.DaemonToken)
	}
	if report.ServerRegistration.Status != "ok" {
		t.Fatalf("server registration check = %#v, want ok", report.ServerRegistration)
	}
	if report.Runtimes.Status != "ok" {
		t.Fatalf("runtimes check = %#v, want ok", report.Runtimes)
	}
}
