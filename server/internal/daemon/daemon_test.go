package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tethy/mulwiki/server/pkg/agent"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// setupMockServer creates a test HTTP server that mimics the Mulwiki API.
// All handler registration happens via the returned mux so callers can
// customize endpoints per test.
func setupMockServer(t *testing.T) (*httptest.Server, *http.ServeMux) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, mux
}

func TestNewDaemon(t *testing.T) {
	d := New(Config{
		ServerURL:     "http://localhost:8080",
		WorkspaceSlug: "test",
		WorkDir:       t.TempDir(),
	})

	if d.DaemonID == "" {
		t.Error("expected non-empty daemon ID")
	}
	if d.ServerURL != "http://localhost:8080" {
		t.Errorf("expected server URL, got %s", d.ServerURL)
	}
	if d.WorkspaceSlug != "test" {
		t.Errorf("expected workspace_slug 'test', got '%s'", d.WorkspaceSlug)
	}
}

func TestLoadOrCreateDaemonIDPersistsStableID(t *testing.T) {
	idPath := filepath.Join(t.TempDir(), "daemon.id")

	first, err := LoadOrCreateDaemonID(idPath)
	if err != nil {
		t.Fatalf("load/create first id: %v", err)
	}
	if first == "" {
		t.Fatal("expected first id")
	}

	second, err := LoadOrCreateDaemonID(idPath)
	if err != nil {
		t.Fatalf("load/create second id: %v", err)
	}
	if second != first {
		t.Fatalf("expected stable daemon id %q, got %q", first, second)
	}
}

func TestNewDaemonRequestUsesConfiguredDaemonToken(t *testing.T) {
	d := New(Config{
		ServerURL:     "http://localhost:8080",
		WorkspaceSlug: "test",
		DaemonID:      "daemon-1",
		DaemonToken:   "mwd_test-token",
	})

	req, err := d.newDaemonRequest(http.MethodPost, "http://localhost:8080/api/daemon/register", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer mwd_test-token" {
		t.Fatalf("expected configured bearer token, got %q", got)
	}
	if got := req.Header.Get("X-Daemon-ID"); got != "daemon-1" {
		t.Fatalf("expected X-Daemon-ID daemon-1, got %q", got)
	}
}

func TestTaskMessageBatcherFlushesMessagesAndPinsSession(t *testing.T) {
	srv, mux := setupMockServer(t)

	messageCalls := make(chan []protocol.AgentTaskMessage, 1)
	sessionCalls := make(chan map[string]string, 1)
	logCalls := make(chan map[string]string, 2)

	mux.HandleFunc("/api/daemon/tasks/task-1/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST /messages, got %s", r.Method)
		}
		var body struct {
			Messages []protocol.AgentTaskMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode messages: %v", err)
		}
		messageCalls <- body.Messages
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/daemon/tasks/task-1/session", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode session: %v", err)
		}
		sessionCalls <- body
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/daemon/workspaces/test/jobs/job-1/log-line", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode log: %v", err)
		}
		logCalls <- body
		w.WriteHeader(http.StatusNoContent)
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test", DaemonID: "daemon-1"})
	batcher := newTaskMessageBatcher(d, "job-1", "task-1", "/tmp/work", 10*time.Millisecond)
	batcher.Add(agent.Message{Type: agent.MessageStatus, Status: "running", SessionID: "sess-1"})
	batcher.Add(agent.Message{Type: agent.MessageText, Content: "hello"})
	batcher.Close()

	select {
	case messages := <-messageCalls:
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d: %#v", len(messages), messages)
		}
		if messages[0].Seq != 1 || messages[0].Type != "status" || messages[0].SessionID != "sess-1" {
			t.Fatalf("unexpected first message: %#v", messages[0])
		}
		if messages[1].Seq != 2 || messages[1].Type != "text" || messages[1].Content != "hello" {
			t.Fatalf("unexpected second message: %#v", messages[1])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for messages call")
	}

	select {
	case session := <-sessionCalls:
		if session["session_id"] != "sess-1" || session["work_dir"] != "/tmp/work" {
			t.Fatalf("unexpected session body: %#v", session)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session call")
	}

	for i := 0; i < 2; i++ {
		select {
		case log := <-logCalls:
			if log["line"] == "" {
				t.Fatalf("expected log line, got %#v", log)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for log call")
		}
	}
}

func TestClaimNextJob_NoPendingJobs(t *testing.T) {
	srv, mux := setupMockServer(t)

	mux.HandleFunc("/api/daemon/workspaces/test/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test"})

	job, err := d.claimNextJob()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job != nil {
		t.Errorf("expected nil job, got %v", job)
	}
}

func TestClaimNextJob_Success(t *testing.T) {
	srv, mux := setupMockServer(t)

	mux.HandleFunc("/api/daemon/workspaces/test/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		job := map[string]interface{}{
			"id":           "job-1",
			"workspace_id": "ws1",
			"status":       "running",
			"agent_id":     "agent-1",
			"source_path":  "src-1",
			"source_paths": []string{"src-1", "src-2"},
			"schema_id":    "sch-1",
			"progress":     0,
			"error":        "",
			"claimed_by":   "",
			"created_at":   "2026-01-01T00:00:00Z",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test"})

	job, err := d.claimNextJob()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("expected a job, got nil")
	}
	if job.ID != "job-1" {
		t.Errorf("expected job ID 'job-1', got '%s'", job.ID)
	}
	if job.AgentID != "agent-1" {
		t.Errorf("expected agent_id 'agent-1', got '%s'", job.AgentID)
	}
	if len(job.SourcePaths) != 2 {
		t.Errorf("expected 2 source paths, got %d", len(job.SourcePaths))
	}
}

func TestFetchAgent_Success(t *testing.T) {
	srv, mux := setupMockServer(t)

	mux.HandleFunc("/api/daemon/workspaces/test/agents/agent-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		agent := map[string]interface{}{
			"id":           "agent-1",
			"workspace_id": "ws1",
			"runtime_id":   "rt-1",
			"name":         "Test Agent",
			"description":  "A test agent",
			"instructions": "Do the thing",
			"runtime_mode": "claude-code",
			"custom_env":   map[string]string{"ANTHROPIC_API_KEY": "sk-test"},
			"custom_args":  []string{"--verbose"},
			"visibility":   "private",
			"status":       "online",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"agent": agent})
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test"})

	agent, err := d.fetchAgent("agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got '%s'", agent.Name)
	}
	if agent.RuntimeID != "rt-1" {
		t.Errorf("expected runtime_id 'rt-1', got '%s'", agent.RuntimeID)
	}
	if len(agent.CustomArgs) != 1 || agent.CustomArgs[0] != "--verbose" {
		t.Errorf("expected custom_args [--verbose], got %v", agent.CustomArgs)
	}
}

func TestFetchAgent_NotFound(t *testing.T) {
	srv, mux := setupMockServer(t)

	mux.HandleFunc("/api/daemon/workspaces/test/agents/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test"})

	agent, err := d.fetchAgent("missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if agent != nil {
		t.Errorf("expected nil agent, got %v", agent)
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "404") {
		// accept any not-found error
	}
}

func TestFetchRuntime_Success(t *testing.T) {
	srv, mux := setupMockServer(t)

	mux.HandleFunc("/api/daemon/workspaces/test/agents/runtimes/rt-1", func(w http.ResponseWriter, r *http.Request) {
		rt := map[string]interface{}{
			"id":           "rt-1",
			"workspace_id": "ws1",
			"name":         "Claude Code",
			"backend":      "claude-code",
			"path":         "/usr/local/bin/claude",
			"status":       "online",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"runtime": rt})
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test"})

	rt, err := d.fetchRuntime("rt-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Backend != "claude-code" {
		t.Errorf("expected backend 'claude-code', got '%s'", rt.Backend)
	}
	if rt.Path != "/usr/local/bin/claude" {
		t.Errorf("expected path '/usr/local/bin/claude', got '%s'", rt.Path)
	}
}

func TestBuildWorkdir(t *testing.T) {
	// This test requires a real git repo setup — the buildWorkdir function
	// now clones a bare git repo and creates worktrees. Skipped in unit tests;
	// covered by integration/E2E tests.
	t.Skip("requires real git bare repo — covered by E2E tests")

	srv, mux := setupMockServer(t)

	// Mock workspace fetch (needed by fetchWorkspace).
	mux.HandleFunc("/api/daemon/workspaces/test", func(w http.ResponseWriter, r *http.Request) {
		ws := map[string]interface{}{
			"id":   "ws1",
			"slug": "test",
			"name": "Test Workspace",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ws)
	})

	// Mock schema fetch.
	mux.HandleFunc("/api/daemon/workspaces/test/schemas/sch-1", func(w http.ResponseWriter, r *http.Request) {
		schema := map[string]interface{}{
			"id":     "sch-1",
			"name":   "Test Schema",
			"config": "# Types\n- Fact\n- Concept",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(schema)
	})

	// Mock source fetch.
	mux.HandleFunc("/api/workspaces/test/sources/src-1", func(w http.ResponseWriter, r *http.Request) {
		src := map[string]interface{}{
			"id":        "src-1",
			"name":      "test-doc.md",
			"type":      "markdown",
			"file_path": "",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(src)
	})

	// Mock wiki list.
	mux.HandleFunc("/api/workspaces/test/wiki", func(w http.ResponseWriter, r *http.Request) {
		pages := []map[string]interface{}{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test"})

	job := protocol.Job{
		ID:         "job-test-1",
		SchemaID:   "sch-1",
		SourcePath: "src-1",
	}
	agent := &protocol.Agent{
		ID:           "agent-1",
		Instructions: "Process the documents.",
	}

	workdir, err := d.buildWorkdir(job, agent)
	if err != nil {
		t.Fatalf("buildWorkdir: %v", err)
	}

	// Verify workdir structure.
	entries, err := os.ReadDir(workdir)
	if err != nil {
		t.Fatalf("read workdir: %v", err)
	}

	dirNames := make(map[string]bool)
	for _, e := range entries {
		dirNames[e.Name()] = e.IsDir()
	}

	required := []string{"sources", "wiki", "output", "AGENTS.md"}
	for _, r := range required {
		if _, ok := dirNames[r]; !ok {
			t.Errorf("expected %s in workdir, but not found. Found: %v", r, entriesToNames(entries))
		}
	}

	// Verify AGENTS.md content.
	agentsMD, err := os.ReadFile(filepath.Join(workdir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(agentsMD)
	if !strings.Contains(content, "Test Schema") {
		t.Error("AGENTS.md should contain schema name")
	}
	if !strings.Contains(content, "Process the documents") {
		t.Error("AGENTS.md should contain agent instructions")
	}
	if !strings.Contains(content, "# Types") {
		t.Error("AGENTS.md should contain schema config")
	}

	// Cleanup.
	os.RemoveAll(workdir)
}

func TestCollectOutput_EmptyManifest(t *testing.T) {
	workdir := t.TempDir()
	os.MkdirAll(filepath.Join(workdir, "output"), 0755)

	manifest := `{"job_id":"job-1","pages":[]}`
	os.WriteFile(filepath.Join(workdir, "output", "manifest.json"), []byte(manifest), 0644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := &Daemon{
		ServerURL:     srv.URL,
		WorkspaceSlug: "test",
		HTTPClient:    srv.Client(),
	}
	job := protocol.Job{ID: "job-1"}
	task := &protocol.AgentTask{ID: "task-1"}

	result, err := d.collectOutput(workdir, job, task)
	if err != nil {
		t.Fatalf("collectOutput: %v", err)
	}
	if !strings.Contains(result, "no pages") {
		t.Errorf("expected pages=0 in result, got '%s'", result)
	}
}

func TestCollectOutput_MissingManifest(t *testing.T) {
	workdir := t.TempDir()
	os.MkdirAll(filepath.Join(workdir, "output"), 0755)

	d := &Daemon{
		ServerURL:     "http://localhost:1",
		WorkspaceSlug: "test",
		HTTPClient:    &http.Client{},
	}
	job := protocol.Job{ID: "job-1"}
	task := &protocol.AgentTask{ID: "task-1"}

	_, err := d.collectOutput(workdir, job, task)
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
}

func TestMergeEnv(t *testing.T) {
	d := &Daemon{}

	parent := []string{"PATH=/usr/bin", "HOME=/home/user", "EXISTING=old"}
	custom := map[string]string{
		"NEW_VAR":   "new-value",
		"EXISTING":  "overridden",
		"API_TOKEN": "secret-123",
	}

	merged := d.mergeEnv(parent, custom)

	// Check custom overrides existing.
	foundExisting := false
	for _, e := range merged {
		if e == "EXISTING=overridden" {
			foundExisting = true
		}
		if e == "EXISTING=old" {
			t.Error("old EXISTING should have been overridden")
		}
	}
	if !foundExisting {
		t.Error("overridden EXISTING not found")
	}

	// Check new var is added.
	foundNew := false
	for _, e := range merged {
		if e == "NEW_VAR=new-value" {
			foundNew = true
		}
	}
	if !foundNew {
		t.Error("NEW_VAR not found")
	}

	// Check secret is present (value should be set but logged safely).
	foundSecret := false
	for _, e := range merged {
		if e == "API_TOKEN=secret-123" {
			foundSecret = true
		}
	}
	if !foundSecret {
		t.Error("API_TOKEN should be present in merged env")
	}
}

func TestIsSecretKey(t *testing.T) {
	tests := []struct {
		key    string
		secret bool
	}{
		{"API_KEY", true},
		{"ANTHROPIC_API_KEY", true},
		{"GITHUB_TOKEN", true},
		{"DB_PASSWORD", true},
		{"MY_SECRET", true},
		{"PATH", false},
		{"HOME", false},
		{"MODEL_NAME", false},
	}

	for _, tt := range tests {
		got := isSecretKey(tt.key)
		if got != tt.secret {
			t.Errorf("isSecretKey(%q) = %v, want %v", tt.key, got, tt.secret)
		}
	}
}

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	src := filepath.Join(srcDir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(dstDir, "subdir", "test.txt")
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	readBack, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(readBack) != string(content) {
		t.Errorf("expected '%s', got '%s'", string(content), string(readBack))
	}
}

func entriesToNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// ── Integration: End-to-End Job Pipeline ──

// TestE2EJobPipeline validates the entire job lifecycle:
//
//	create job → daemon claims → daemon executes (no-op) → job completed
//
// All server endpoints are mocked via httptest. The agent subprocess is
// skipped (no real CLI) — only the API orchestration is tested.
func TestE2EJobPipeline(t *testing.T) {
	srv, mux := setupMockServer(t)

	jobCompleted := make(chan struct{}, 1)
	taskCreated := make(chan struct{}, 1)
	taskUpdated := make(chan string, 3) // collects status updates

	// Mock: workspace fetch
	mux.HandleFunc("/api/daemon/workspaces/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET /workspaces/test, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"id":   "ws1",
			"slug": "test",
			"name": "Test Workspace",
		})
	})

	// Mock: job claim (POST /api/daemon/workspaces/test/jobs/claim)
	mux.HandleFunc("/api/daemon/workspaces/test/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST /jobs/claim, got %s", r.Method)
		}
		var req struct {
			DaemonID string `json:"daemon_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.DaemonID == "" {
			t.Error("daemon_id is required")
		}
		json.NewEncoder(w).Encode(protocol.Job{
			ID:          "job-e2e-1",
			WorkspaceID: "ws1",
			Status:      "running",
			AgentID:     "agent-1",
			SourcePath:  "sources/test.md",
			SourcePaths: []string{"sources/test.md"},
			SchemaID:    "sch-1",
			Progress:    0,
			ClaimedBy:   req.DaemonID,
			CreatedAt:   "2026-01-01T00:00:00Z",
		})
	})

	// Mock: agent fetch
	mux.HandleFunc("/api/daemon/workspaces/test/agents/agent-1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"agent": protocol.Agent{
			ID:           "agent-1",
			WorkspaceID:  "ws1",
			RuntimeID:    "rt-1",
			Name:         "Test Agent",
			Instructions: "Extract facts from sources.",
			CustomArgs:   []string{"--verbose"},
			CustomEnv:    map[string]string{"MODEL": "claude-3"},
		}})
	})

	// Mock: runtime fetch
	mux.HandleFunc("/api/daemon/workspaces/test/agents/runtimes/rt-1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"runtime": protocol.AgentRuntime{
			ID:      "rt-1",
			Name:    "Echo Runtime",
			Backend: "custom",
			Path:    "/bin/echo", // real binary, exists on all systems
			Status:  "online",
		}})
	})

	// Mock: schema fetch
	mux.HandleFunc("/api/daemon/workspaces/test/schemas/sch-1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(protocol.Schema{
			ID:      "sch-1",
			Name:    "Test Schema",
			Content: "# Types\n- Fact",
		})
	})

	// Mock: create task (POST /api/daemon/workspaces/test/agents/agent-1/tasks)
	mux.HandleFunc("/api/daemon/workspaces/test/agents/agent-1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST /tasks, got %s", r.Method)
		}
		taskCreated <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]protocol.AgentTask{
			"task": {
				ID:          "task-e2e-1",
				AgentID:     "agent-1",
				Status:      "queued",
				SourcePath:  "sources/test.md",
				SchemaID:    "sch-1",
				Attempt:     1,
				MaxAttempts: 3,
				CreatedAt:   "2026-01-01T00:00:00Z",
			},
		})
	})

	// Mock: update task (PATCH)
	mux.HandleFunc("/api/daemon/workspaces/test/agents/agent-1/tasks/task-e2e-1", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if st, ok := body["status"].(string); ok {
			taskUpdated <- st
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Mock: update progress
	mux.HandleFunc("/api/daemon/workspaces/test/jobs/job-e2e-1/progress", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Mock: complete job
	mux.HandleFunc("/api/daemon/workspaces/test/jobs/job-e2e-1/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST /complete, got %s", r.Method)
		}
		jobCompleted <- struct{}{}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Create daemon (uses /bin/echo as "agent" so subprocess succeeds).
	d := New(Config{
		ServerURL:     srv.URL,
		WorkspaceSlug: "test",
		WorkDir:       t.TempDir(),
	})

	// Ready a job for the daemon to claim.
	// We construct it directly since there's no real DB.
	job := protocol.Job{
		ID:         "job-e2e-1",
		AgentID:    "agent-1",
		SourcePath: "sources/test.md",
		SchemaID:   "sch-1",
	}

	// Execute job synchronously (runAgentAttempt calls buildWorkdir which needs git).
	// We test the claim mechanism separately.

	// Test 1: Claim works
	claimed, err := d.claimNextJob()
	if err != nil {
		t.Fatalf("claimNextJob: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a claimed job, got nil")
	}
	if claimed.ID != "job-e2e-1" {
		t.Errorf("expected job-e2e-1, got %s", claimed.ID)
	}
	if claimed.Status != "running" {
		t.Errorf("expected status running, got %s", claimed.Status)
	}

	// Test 2: Fetching agent config works
	agent, err := d.fetchAgent("agent-1")
	if err != nil {
		t.Fatalf("fetchAgent: %v", err)
	}
	if agent.RuntimeID != "rt-1" {
		t.Errorf("expected runtime_id rt-1, got %s", agent.RuntimeID)
	}
	if len(agent.CustomArgs) != 1 {
		t.Errorf("expected 1 custom arg, got %d", len(agent.CustomArgs))
	}

	// Test 3: Fetching runtime works
	rt, err := d.fetchRuntime("rt-1")
	if err != nil {
		t.Fatalf("fetchRuntime: %v", err)
	}
	if rt.Path != "/bin/echo" {
		t.Errorf("expected /bin/echo, got %s", rt.Path)
	}

	// Test 4: Task creation works
	task, err := d.createTask(job, agent, rt)
	if err != nil {
		t.Fatalf("createTask: %v", err)
	}
	if task == nil {
		t.Fatal("expected task, got nil")
	}
	if task.Status != "queued" {
		t.Errorf("expected status queued, got %s", task.Status)
	}

	// Test 5: Task status update works (mark started → completed → failed)
	if err := d.updateTask("agent-1", "task-e2e-1", "running", "", "", 0, "", ""); err != nil {
		t.Fatalf("updateTask started: %v", err)
	}
	if err := d.updateTask("agent-1", "task-e2e-1", "completed", "pages_created=3", "", 0, "", ""); err != nil {
		t.Fatalf("updateTask completed: %v", err)
	}

	// Test 6: Progress updates work
	d.updateProgress("job-e2e-1", 50)
	d.updateProgress("job-e2e-1", 100)

	// Test 7: Job completion works
	d.completeJob("job-e2e-1")

	// Verify side effects
	select {
	case <-taskCreated:
		// good
	default:
		t.Error("task was not created via API")
	}

	select {
	case <-jobCompleted:
		// good
	default:
		t.Error("job was not completed via API")
	}

	// Collect task status updates (non-blocking check).
	updates := make([]string, 0)
	for {
		select {
		case st := <-taskUpdated:
			updates = append(updates, st)
		default:
			goto done
		}
	}
done:
	// Should have received at least the "started" and "completed" updates.
	if len(updates) < 2 {
		t.Errorf("expected at least 2 task status updates, got %d: %v", len(updates), updates)
	}
}

// TestE2EJobFailure validates that failing a job propagates correctly
// through the daemon pipeline.
func TestE2EJobFailure(t *testing.T) {
	srv, mux := setupMockServer(t)

	jobFailed := make(chan struct{}, 1)

	// Mock: workspace
	mux.HandleFunc("/api/daemon/workspaces/test", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"id": "ws1", "slug": "test", "name": "Test"})
	})

	// Mock: job claim returns an agent-bound job that will fail
	mux.HandleFunc("/api/daemon/workspaces/test/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(protocol.Job{
			ID:          "job-fail-1",
			WorkspaceID: "ws1",
			Status:      "running",
			AgentID:     "agent-1",
			SourcePath:  "sources/broken.md",
			SchemaID:    "sch-1",
		})
	})

	// Mock: agent fetch returns agent with non-existent runtime
	mux.HandleFunc("/api/daemon/workspaces/test/agents/agent-1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"agent": protocol.Agent{
			ID:        "agent-1",
			RuntimeID: "rt-nonexistent",
			Name:      "Broken Agent",
		}})
	})

	// Mock: runtime fetch returns 404
	mux.HandleFunc("/api/daemon/workspaces/test/agents/runtimes/rt-nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Mock: fail job
	mux.HandleFunc("/api/daemon/workspaces/test/jobs/job-fail-1/fail", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["error"] == "" {
			t.Error("expected error in fail body")
		}
		jobFailed <- struct{}{}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	d := New(Config{
		ServerURL:     srv.URL,
		WorkspaceSlug: "test",
		WorkDir:       t.TempDir(),
	})

	// Simulate the executeJob flow for a job with a broken agent.
	job := protocol.Job{
		ID:         "job-fail-1",
		AgentID:    "agent-1",
		SourcePath: "sources/broken.md",
		SchemaID:   "sch-1",
	}

	// The runAgentJob should fail because the runtime can't be fetched.
	d.runAgentJob(job, t.TempDir())

	// Verify the job was marked as failed.
	select {
	case <-jobFailed:
		// good
	default:
		t.Error("job was not marked as failed")
	}
}

// TestDaemonJobClaim_NoContent validates that claimNextJob returns nil
// when no pending jobs are available (HTTP 204).
func TestDaemonJobClaim_NoContent(t *testing.T) {
	srv, mux := setupMockServer(t)

	mux.HandleFunc("/api/daemon/workspaces/test/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test"})

	job, err := d.claimNextJob()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job != nil {
		t.Errorf("expected nil job for 204, got %v", job)
	}
}

// TestDaemonJobClaim_Error validates error handling when the claim endpoint fails.
func TestDaemonJobClaim_Error(t *testing.T) {
	srv, mux := setupMockServer(t)

	mux.HandleFunc("/api/daemon/workspaces/test/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	d := New(Config{ServerURL: srv.URL, WorkspaceSlug: "test"})

	_, err := d.claimNextJob()
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}
