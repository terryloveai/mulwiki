package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListWorkspaceDaemonsFiltersByWorkspaceRuntime(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.DB.Exec(`INSERT INTO workspaces (id, slug, name) VALUES ('ws2', 'other-workspace', 'Other')`); err != nil {
		t.Fatalf("seed other workspace: %v", err)
	}
	if _, err := h.DB.Exec(`
		INSERT INTO daemon_registrations (id, hostname, pid, version, runtime_ids, max_concurrent_tasks, last_heartbeat, registered_at) VALUES
			('daemon-ws1', 'host-a', 111, '0.1.0', '[]', 3, '2026-05-09T14:00:00Z', '2026-05-09T14:00:00Z'),
			('daemon-ws2', 'host-b', 222, '0.1.0', '[]', 3, '2026-05-09T14:00:00Z', '2026-05-09T14:00:00Z');
		INSERT INTO agent_runtimes (id, workspace_id, name, backend, daemon_id, status, last_heartbeat) VALUES
			('rt-ws1', 'ws1', 'Codex', 'codex', 'daemon-ws1', 'online', '2026-05-09T14:00:00Z'),
			('rt-ws2', 'ws2', 'Codex', 'codex', 'daemon-ws2', 'online', '2026-05-09T14:00:00Z');
	`); err != nil {
		t.Fatalf("seed daemons: %v", err)
	}

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/daemons", map[string]string{"slug": "test-workspace"}, nil)
	rr := httptest.NewRecorder()

	h.ListWorkspaceDaemons(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Daemons []struct {
			ID string `json:"id"`
		} `json:"daemons"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Daemons) != 1 {
		t.Fatalf("expected 1 daemon, got %#v", resp.Daemons)
	}
	if resp.Daemons[0].ID != "daemon-ws1" {
		t.Fatalf("expected daemon-ws1, got %q", resp.Daemons[0].ID)
	}
}

func TestDaemonHeartbeatRefreshesRegisteredRuntimeHeartbeatsWhenRuntimeIDsOmitted(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.DB.Exec(`
		INSERT INTO daemon_registrations (id, hostname, pid, version, runtime_ids, max_concurrent_tasks, last_heartbeat, registered_at)
		VALUES ('daemon-1', 'host-a', 111, '0.1.0', '["rt-ws1"]', 3, '1970-01-01T00:00:00Z', '2026-05-09T14:00:00Z');
		INSERT INTO agent_runtimes (id, workspace_id, name, backend, daemon_id, status, last_heartbeat)
		VALUES ('rt-ws1', 'ws1', 'Codex', 'codex', 'daemon-1', 'offline', '1970-01-01T00:00:00Z');
	`); err != nil {
		t.Fatalf("seed daemon runtime: %v", err)
	}

	req := chiRequest(http.MethodPost, "/api/daemon/heartbeat", nil, strings.NewReader(`{"id":"daemon-1","max_concurrent_tasks":3}`))
	rr := httptest.NewRecorder()

	h.DaemonHeartbeat(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var status, lastHeartbeat string
	if err := h.DB.QueryRow(`SELECT status, last_heartbeat FROM agent_runtimes WHERE id = 'rt-ws1'`).Scan(&status, &lastHeartbeat); err != nil {
		t.Fatalf("query runtime: %v", err)
	}
	if status != "online" {
		t.Fatalf("expected runtime online, got %q", status)
	}
	if lastHeartbeat == "1970-01-01T00:00:00Z" || lastHeartbeat == "" {
		t.Fatalf("expected runtime heartbeat to refresh, got %q", lastHeartbeat)
	}
}

func TestStartDaemonFromWorkspaceRouteMintsTokenAndPassesDaemonIdentity(t *testing.T) {
	h := newTestHandler(t)
	argsPath := filepath.Join(t.TempDir(), "mulwiki-args.txt")
	t.Setenv("MULWIKI_CAPTURE_ARGS", argsPath)
	origLookPath := daemonLookPath
	origCommand := daemonCommand
	daemonLookPath = func(file string) (string, error) {
		if file != "mulwiki" {
			t.Fatalf("unexpected lookpath target %q", file)
		}
		return "mulwiki", nil
	}
	daemonCommand = func(name string, args ...string) *exec.Cmd {
		shellArgs := []string{"-c", `printf '%s\n' "$@" > "$MULWIKI_CAPTURE_ARGS"`, "capture", name}
		shellArgs = append(shellArgs, args...)
		return exec.Command("/bin/sh", shellArgs...)
	}
	t.Cleanup(func() {
		daemonLookPath = origLookPath
		daemonCommand = origCommand
	})

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/daemons/start", map[string]string{"slug": "test-workspace"}, strings.NewReader(`{"server_url":"http://localhost:8080"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.StartDaemon(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var argsText string
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		data, err := os.ReadFile(argsPath)
		if err == nil {
			argsText = string(data)
			break
		}
	}
	if argsText == "" {
		t.Fatal("fake mulwiki did not capture start args")
	}
	if !strings.Contains(argsText, "--daemon-id\n") || !strings.Contains(argsText, "--daemon-token\nmwd_") {
		t.Fatalf("expected daemon id and token to be passed, got args:\n%s", argsText)
	}

	var count int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM daemon_tokens WHERE workspace_id = 'ws1'`).Scan(&count); err != nil {
		t.Fatalf("count daemon tokens: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one minted daemon token, got %d", count)
	}
}
