package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tethy/mulwiki/server/internal/middleware"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

func seedTaskMessageHandlerTask(t *testing.T, h *Handler) {
	t.Helper()

	if _, err := h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name, backend, path) VALUES ('rt-msg', 'ws1', 'Runtime', 'codex', '/bin/codex')`); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if _, err := h.DB.Exec(`INSERT INTO agents (id, workspace_id, name, runtime_id, runtime_mode) VALUES ('agent-msg', 'ws1', 'Agent', 'rt-msg', 'codex')`); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := h.DB.Exec(`INSERT INTO agent_tasks (id, job_id, agent_id, runtime_id, workspace_id, status) VALUES ('task-msg', 'job-msg', 'agent-msg', 'rt-msg', 'ws1', 'running')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func TestTaskMessageDaemonAppendAndUserList(t *testing.T) {
	h := newTestHandler(t)
	seedTaskMessageHandlerTask(t, h)

	body := `{"messages":[{"seq":1,"type":"text","content":"hello","session_id":"sess-1"},{"seq":2,"type":"tool-result","tool":"exec_command","call_id":"call-1","output":"ok"}]}`
	req := chiRequest(http.MethodPost, "/api/daemon/tasks/task-msg/messages", map[string]string{"taskId": "task-msg"}, strings.NewReader(body))
	req = req.WithContext(middleware.WithDaemon(req.Context(), "ws1", "daemon-1"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.AppendTaskMessages(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	listReq := chiRequest(http.MethodGet, "/api/tasks/task-msg/messages?since=1", map[string]string{"taskId": "task-msg"}, nil)
	listReq.Header.Set("X-User-ID", "dev-user")
	listRR := httptest.NewRecorder()

	h.ListTaskMessages(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}
	var resp struct {
		Messages []protocol.AgentTaskMessage `json:"messages"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Messages) != 1 || resp.Messages[0].Seq != 2 || resp.Messages[0].Output != "ok" {
		t.Fatalf("expected since filter to return seq 2, got %#v", resp.Messages)
	}
}

func TestPinSessionDaemonEndpoint(t *testing.T) {
	h := newTestHandler(t)
	seedTaskMessageHandlerTask(t, h)

	req := chiRequest(http.MethodPost, "/api/daemon/tasks/task-msg/session", map[string]string{"taskId": "task-msg"}, strings.NewReader(`{"session_id":"sess-early","work_dir":"/tmp/work"}`))
	req = req.WithContext(middleware.WithDaemon(req.Context(), "ws1", "daemon-1"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.PinTaskSession(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	var sessionID, workDir string
	if err := h.DB.QueryRow(`SELECT session_id, work_dir FROM agent_tasks WHERE id = 'task-msg'`).Scan(&sessionID, &workDir); err != nil {
		t.Fatalf("fetch task: %v", err)
	}
	if sessionID != "sess-early" || workDir != "/tmp/work" {
		t.Fatalf("expected pinned session/workdir, got session=%q workdir=%q", sessionID, workDir)
	}
}

func TestTaskMessageUserListRejectsNonMember(t *testing.T) {
	h := newTestHandler(t)
	seedTaskMessageHandlerTask(t, h)
	if _, err := h.DB.Exec(`INSERT INTO users (id, email, password_hash) VALUES ('outsider', 'outsider@test.local', 'hash')`); err != nil {
		t.Fatalf("seed outsider: %v", err)
	}

	req := chiRequest(http.MethodGet, "/api/tasks/task-msg/messages", map[string]string{"taskId": "task-msg"}, nil)
	req.Header.Set("X-User-ID", "outsider")
	rr := httptest.NewRecorder()

	h.ListTaskMessages(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}
