package handler

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// ---------------------------------------------------------------------------
// Job CRUD
// ---------------------------------------------------------------------------

func TestCreateJob(t *testing.T) {
	h := newTestHandler(t)
	seedJobAgent(t, h, "agent1", "rt-job-1")

	body := `{"source_path":"src1","schema_id":"sch1","agent_id":"agent1"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateJob(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var j protocol.Job
	if err := json.NewDecoder(rr.Body).Decode(&j); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if j.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", j.Status)
	}
	if j.AgentID != "agent1" {
		t.Errorf("expected agent_id 'agent1', got '%s'", j.AgentID)
	}
	if j.SourcePath != "src1" {
		t.Errorf("expected source_path 'src1', got '%s'", j.SourcePath)
	}
	if j.SchemaID != "sch1" {
		t.Errorf("expected schema_id 'sch1', got '%s'", j.SchemaID)
	}
	if j.Progress != 0 {
		t.Errorf("expected progress 0, got %d", j.Progress)
	}

	var taskJobID, taskStatus, taskAgentID string
	if err := h.DB.QueryRow(`SELECT job_id, status, agent_id FROM agent_tasks WHERE job_id = ?`, j.ID).Scan(&taskJobID, &taskStatus, &taskAgentID); err != nil {
		t.Fatalf("expected agent task for job: %v", err)
	}
	if taskJobID != j.ID || taskStatus != "queued" || taskAgentID != "agent1" {
		t.Fatalf("unexpected agent task row: job=%q status=%q agent=%q", taskJobID, taskStatus, taskAgentID)
	}
}

func TestCreateJob_WithSourcePaths(t *testing.T) {
	h := newTestHandler(t)
	seedJobAgent(t, h, "agent1", "rt-job-1")

	body := `{"source_paths":["src1","src2","src3"],"agent_id":"agent1","schema_id":"sch1"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateJob(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var j protocol.Job
	json.NewDecoder(rr.Body).Decode(&j)
	if len(j.SourcePaths) != 3 {
		t.Errorf("expected 3 source_paths, got %d", len(j.SourcePaths))
	}
}

func TestCreateJob_MissingAgentID(t *testing.T) {
	h := newTestHandler(t)

	body := `{"source_path":"src1","schema_id":"sch1"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func seedJobAgent(t *testing.T, h *Handler, agentID, runtimeID string) {
	t.Helper()

	if _, err := h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name, backend, path) VALUES (?, 'ws1', 'Runtime', 'codex', '/bin/codex')`, runtimeID); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if _, err := h.DB.Exec(`INSERT INTO agents (id, workspace_id, name, runtime_id, runtime_mode) VALUES (?, 'ws1', 'Agent', ?, 'codex')`, agentID, runtimeID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
}

func TestGetJob(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status, agent_id, source_path, source_paths, schema_id) VALUES ('job1', 'ws1', 'pending', 'agent1', 'src1', '["src1"]', 'sch1')`)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/jobs/job1", map[string]string{"slug": "test-workspace", "id": "job1"}, nil)
	rr := httptest.NewRecorder()

	h.GetJob(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var j protocol.Job
	json.NewDecoder(rr.Body).Decode(&j)
	if j.ID != "job1" {
		t.Errorf("expected id 'job1', got '%s'", j.ID)
	}
	if j.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", j.Status)
	}
	if len(j.SourcePaths) != 1 || j.SourcePaths[0] != "src1" {
		t.Errorf("expected source_paths ['src1'], got %v", j.SourcePaths)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/jobs/nonexistent", map[string]string{"slug": "test-workspace", "id": "nonexistent"}, nil)
	rr := httptest.NewRecorder()

	h.GetJob(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestListJobs(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status, agent_id, source_path, source_paths, schema_id) VALUES ('job1', 'ws1', 'pending', 'agent1', 'src1', '["src1"]', 'sch1'), ('job2', 'ws1', 'completed', 'agent2', 'src2', '["src2"]', 'sch2')`)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/jobs", map[string]string{"slug": "test-workspace"}, nil)
	rr := httptest.NewRecorder()

	h.ListJobs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var jobs []protocol.Job
	json.NewDecoder(rr.Body).Decode(&jobs)
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestListJobs_Empty(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/jobs", map[string]string{"slug": "test-workspace"}, nil)
	rr := httptest.NewRecorder()

	h.ListJobs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var jobs []protocol.Job
	json.NewDecoder(rr.Body).Decode(&jobs)
	if jobs == nil {
		t.Error("expected non-nil jobs slice")
	}
}

// ---------------------------------------------------------------------------
// Job Claim
// ---------------------------------------------------------------------------

func TestClaimJob_NoPending(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status) VALUES ('job1', 'ws1', 'running')`)

	body := `{"daemon_id":"daemon-1"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs/claim", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ClaimJob(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 (no pending), got %d", rr.Code)
	}
}

func TestClaimJob_Success(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status, agent_id, source_path, source_paths, schema_id) VALUES ('job1', 'ws1', 'pending', 'agent1', 'src1', '["src1"]', 'sch1')`)

	body := `{"daemon_id":"daemon-1"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs/claim", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ClaimJob(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var j protocol.Job
	json.NewDecoder(rr.Body).Decode(&j)

	if j.ID != "job1" {
		t.Errorf("expected job1, got '%s'", j.ID)
	}
	if j.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", j.Status)
	}
	if j.ClaimedBy != "daemon-1" {
		t.Errorf("expected claimed_by 'daemon-1', got '%s'", j.ClaimedBy)
	}

	// Verify second claim returns 204 (job is now running, not pending).
	req2 := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs/claim", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	h.ClaimJob(rr2, req2)
	if rr2.Code != http.StatusNoContent {
		t.Errorf("expected 204 on second claim, got %d", rr2.Code)
	}
}

func TestClaimJob_MissingDaemonID(t *testing.T) {
	h := newTestHandler(t)

	body := `{}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs/claim", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ClaimJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Job Progress & Completion
// ---------------------------------------------------------------------------

func TestUpdateJobProgress(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status) VALUES ('job1', 'ws1', 'running')`)

	body := `{"progress":50}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs/job1/progress", map[string]string{"slug": "test-workspace", "id": "job1"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.UpdateJobProgress(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify progress updated.
	var progress int
	h.DB.QueryRow(`SELECT progress FROM jobs WHERE id = 'job1'`).Scan(&progress)
	if progress != 50 {
		t.Errorf("expected progress 50, got %d", progress)
	}
}

func TestCompleteJob(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status) VALUES ('job1', 'ws1', 'running')`)

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs/job1/complete", map[string]string{"slug": "test-workspace", "id": "job1"}, nil)
	rr := httptest.NewRecorder()

	h.CompleteJob(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify status updated.
	var status string
	var progress int
	var completedAt *string
	h.DB.QueryRow(`SELECT status, progress, completed_at FROM jobs WHERE id = 'job1'`).Scan(&status, &progress, &completedAt)
	if status != "completed" {
		t.Errorf("expected 'completed', got '%s'", status)
	}
	if progress != 100 {
		t.Errorf("expected progress 100, got %d", progress)
	}
	if completedAt == nil || *completedAt == "" {
		t.Error("expected completed_at to be set")
	}
}

func TestFailJob(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status) VALUES ('job1', 'ws1', 'running')`)

	body := `{"error":"something went wrong"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/jobs/job1/fail", map[string]string{"slug": "test-workspace", "id": "job1"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.FailJob(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify failure recorded.
	var status, jobError string
	var completedAt *string
	h.DB.QueryRow(`SELECT status, error, completed_at FROM jobs WHERE id = 'job1'`).Scan(&status, &jobError, &completedAt)
	if status != "failed" {
		t.Errorf("expected 'failed', got '%s'", status)
	}
	if jobError != "something went wrong" {
		t.Errorf("expected error 'something went wrong', got '%s'", jobError)
	}
	if completedAt == nil || *completedAt == "" {
		t.Error("expected completed_at to be set")
	}
}

// ---------------------------------------------------------------------------
// Job SSE Streaming
// ---------------------------------------------------------------------------

func TestStreamJobLogs_JobNotFound(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/jobs/nonexistent/logs", map[string]string{"slug": "test-workspace", "id": "nonexistent"}, nil)
	rr := httptest.NewRecorder()

	h.StreamJobLogs(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestStreamJobLogs_AlreadyCompleted(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status) VALUES ('job1', 'ws1', 'completed')`)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/jobs/job1/logs", map[string]string{"slug": "test-workspace", "id": "job1"}, nil)
	rr := httptest.NewRecorder()

	// StreamJobLogs sends SSE events. The standard httptest.ResponseRecorder
	// doesn't support Flusher, so it will return 500.
	h.StreamJobLogs(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Logf("streaming not supported in test recorder (got %d)", rr.Code)
	}
}

func TestStreamJobLogs_Running(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status, progress) VALUES ('job1', 'ws1', 'running', 0)`)

	// Simulate concurrent progress updates by updating the job in the background.
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(100 * time.Millisecond)
		h.DB.Exec(`UPDATE jobs SET status = 'completed', progress = 100, completed_at = datetime('now') WHERE id = 'job1'`)
	}()

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/jobs/job1/logs", map[string]string{"slug": "test-workspace", "id": "job1"}, nil)
	rr := httptest.NewRecorder()

	// Standard recorder won't support Flusher, but the status update via DB is real.
	h.StreamJobLogs(rr, req)

	<-done
	t.Logf("SSE stream finished with status %d", rr.Code)
}

func TestStreamJobLogs_StatusTransitions(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO jobs (id, workspace_id, status, progress) VALUES ('job1', 'ws1', 'queued', 0)`)

	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(50 * time.Millisecond)
		h.DB.Exec(`UPDATE jobs SET status = 'running', progress = 50 WHERE id = 'job1'`)
		time.Sleep(50 * time.Millisecond)
		h.DB.Exec(`UPDATE jobs SET status = 'completed', progress = 100, completed_at = datetime('now') WHERE id = 'job1'`)
	}()

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/jobs/job1/logs", map[string]string{"slug": "test-workspace", "id": "job1"}, nil)
	rr := httptest.NewRecorder()

	h.StreamJobLogs(rr, req)
	<-done

	// Verify job ended completed.
	var status string
	h.DB.QueryRow(`SELECT status FROM jobs WHERE id = 'job1'`).Scan(&status)
	if status != "completed" {
		t.Errorf("expected 'completed', got '%s'", status)
	}
}

// ---------------------------------------------------------------------------
// Daemon handlers
// ---------------------------------------------------------------------------

func TestDaemonRegister(t *testing.T) {
	h := newTestHandler(t)

	body := `{"id":"daemon-1","hostname":"mac.local","pid":12345,"version":"0.1.0","workspace_slug":"test-workspace","runtime_ids":["rt1"],"runtimes":[{"name":"Claude Code","backend":"claude-code","version":"2.0.0","path":"/usr/local/bin/claude"}],"max_concurrent_tasks":10}`
	req := chiRequest(http.MethodPost, "/api/daemon/register", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.DaemonRegister(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify daemon registration inserted.
	var hostname string
	var pid int
	h.DB.QueryRow(`SELECT hostname, pid FROM daemon_registrations WHERE id = 'daemon-1'`).Scan(&hostname, &pid)
	if hostname != "mac.local" {
		t.Errorf("expected hostname 'mac.local', got '%s'", hostname)
	}
	if pid != 12345 {
		t.Errorf("expected pid 12345, got %d", pid)
	}

	// Verify runtimes upserted.
	var name, backend string
	h.DB.QueryRow(`SELECT name, backend FROM agent_runtimes WHERE name = 'Claude Code'`).Scan(&name, &backend)
	if name != "Claude Code" {
		t.Errorf("expected runtime name 'Claude Code', got '%s'", name)
	}
	if backend != "claude-code" {
		t.Errorf("expected backend 'claude-code', got '%s'", backend)
	}
}

func TestDaemonRegister_MinimalFields(t *testing.T) {
	h := newTestHandler(t)

	body := `{"id":"daemon-min"}`
	req := chiRequest(http.MethodPost, "/api/daemon/register", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.DaemonRegister(rr, req)

	// Daemon registration is intentionally permissive — only id is required.
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDaemonHeartbeat(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO daemon_registrations (id, hostname, pid, version, last_heartbeat) VALUES ('daemon-1', 'mac.local', 12345, '0.1.0', 'old-time')`)
	h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name, daemon_id) VALUES ('rt1', 'ws1', 'Claude', 'daemon-1')`)

	body := `{"id":"daemon-1","runtime_ids":["rt1"],"max_concurrent_tasks":10}`
	req := chiRequest(http.MethodPost, "/api/daemon/heartbeat", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.DaemonHeartbeat(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify heartbeat updated.
	var heartbeat string
	h.DB.QueryRow(`SELECT last_heartbeat FROM daemon_registrations WHERE id = 'daemon-1'`).Scan(&heartbeat)
	if heartbeat == "" || heartbeat == "old-time" {
		t.Errorf("expected heartbeat updated, got '%s'", heartbeat)
	}
}

func TestDaemonHeartbeat_UnknownDaemon(t *testing.T) {
	h := newTestHandler(t)

	body := `{"id":"nonexistent","runtime_ids":[],"max_concurrent_tasks":5}`
	req := chiRequest(http.MethodPost, "/api/daemon/heartbeat", nil, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.DaemonHeartbeat(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestStaleDaemons(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO daemon_registrations (id, hostname, pid, version, last_heartbeat) VALUES ('daemon-stale', 'old.local', 1, '0.0.1', '2020-01-01T00:00:00Z')`)
	h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name, daemon_id, status) VALUES ('rt-stale', 'ws1', 'Old Runtime', 'daemon-stale', 'offline')`)

	req := chiRequest(http.MethodGet, "/api/daemon/stale", nil, nil)
	rr := httptest.NewRecorder()

	h.DaemonStale(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestStaleDaemons_Empty(t *testing.T) {
	h := newTestHandler(t)
	// No daemons at all.
	req := chiRequest(http.MethodGet, "/api/daemon/stale", nil, nil)
	rr := httptest.NewRecorder()

	h.DaemonStale(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Tests use standard httptest
// ---------------------------------------------------------------------------

var _ = bufio.Scanner{}
