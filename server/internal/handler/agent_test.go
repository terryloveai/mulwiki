package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// ---------------------------------------------------------------------------
// Agent CRUD
// ---------------------------------------------------------------------------

func TestCreateAgent(t *testing.T) {
	h := newTestHandler(t)
	body := `{"name":"My Agent","description":"Test agent","instructions":"You are a helpful assistant","runtime_id":"","visibility":"public","model":"claude-sonnet-4"}`

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateAgent(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrap struct { Agent protocol.Agent `json:"agent"` }
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	a := wrap.Agent

	if a.Name != "My Agent" {
		t.Errorf("expected name 'My Agent', got '%s'", a.Name)
	}
	if a.Visibility != "public" {
		t.Errorf("expected visibility 'public', got '%s'", a.Visibility)
	}
	if a.Status != "offline" {
		t.Errorf("expected status 'offline', got '%s'", a.Status)
	}
	if a.MaxConcurrentTasks != 6 {
		t.Errorf("expected max_concurrent_tasks 6, got %d", a.MaxConcurrentTasks)
	}
	if a.CustomEnv == nil {
		t.Error("expected non-nil CustomEnv")
	}
	if a.CustomArgs == nil {
		t.Error("expected non-nil CustomArgs")
	}
	if a.Skills == nil {
		t.Error("expected non-nil Skills")
	}
}

func TestCreateAgentWithRuntime(t *testing.T) {
	h := newTestHandler(t)

	// First create a runtime.
	h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name, backend, path) VALUES ('rt1', 'ws1', 'Claude Code', 'claude-code', '/usr/local/bin/claude')`)

	body := `{"name":"Claude Agent","runtime_id":"rt1"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateAgent(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrap struct { Agent protocol.Agent `json:"agent"` }
	json.NewDecoder(rr.Body).Decode(&wrap)
	a := wrap.Agent

	if a.RuntimeID != "rt1" {
		t.Errorf("expected runtime_id 'rt1', got '%s'", a.RuntimeID)
	}
	if a.RuntimeMode != "claude-code" {
		t.Errorf("expected runtime_mode 'claude-code', got '%s'", a.RuntimeMode)
	}
}

func TestCreateAgentInvalidRuntime(t *testing.T) {
	h := newTestHandler(t)
	body := `{"name":"Bad Agent","runtime_id":"nonexistent"}`

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateAgent(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCreateAgentMissingName(t *testing.T) {
	h := newTestHandler(t)
	body := `{"description":"no name"}`

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateAgent(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListAgents(t *testing.T) {
	h := newTestHandler(t)

	// Seed two agents.
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name, description) VALUES ('a1', 'ws1', 'Agent 1', 'First'), ('a2', 'ws1', 'Agent 2', 'Second')`)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/agents", map[string]string{"slug": "test-workspace"}, nil)
	rr := httptest.NewRecorder()

	h.ListAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Agents []protocol.Agent `json:"agents"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode agents: %v", err)
	}

	agents := resp.Agents

	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestGetAgent(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name, description, instructions, visibility) VALUES ('a1', 'ws1', 'Test Agent', 'Desc', 'Instructions here', 'private')`)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/agents/a1", map[string]string{"slug": "test-workspace", "id": "a1"}, nil)
	rr := httptest.NewRecorder()

	h.GetAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrap struct { Agent protocol.Agent `json:"agent"` }
	json.NewDecoder(rr.Body).Decode(&wrap)
	a := wrap.Agent

	if a.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got '%s'", a.Name)
	}
	if a.Instructions != "Instructions here" {
		t.Errorf("expected instructions, got '%s'", a.Instructions)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	h := newTestHandler(t)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/agents/nonexistent", map[string]string{"slug": "test-workspace", "id": "nonexistent"}, nil)
	rr := httptest.NewRecorder()

	h.GetAgent(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestUpdateAgent(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name) VALUES ('a1', 'ws1', 'Old Name')`)

	body := `{"name":"New Name","description":"Updated desc"}`
	req := chiRequest(http.MethodPatch, "/api/workspaces/test-workspace/agents/a1", map[string]string{"slug": "test-workspace", "id": "a1"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.UpdateAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrap struct { Agent protocol.Agent `json:"agent"` }
	json.NewDecoder(rr.Body).Decode(&wrap)
	a := wrap.Agent

	if a.Name != "New Name" {
		t.Errorf("expected name 'New Name', got '%s'", a.Name)
	}
}

func TestArchiveAndRestoreAgent(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO users (id, email, password_hash) VALUES ('user1', 'user1@test.local', 'hash')`)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name, owner_id) VALUES ('a1', 'ws1', 'To Archive', 'user1')`)

	// Archive.
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents/a1/archive", map[string]string{"slug": "test-workspace", "id": "a1"}, nil)
	req.Header.Set("X-User-ID", "user1")
	rr := httptest.NewRecorder()
	h.ArchiveAgent(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("archive: expected 200, got %d", rr.Code)
	}

	// Verify archived.
	var archivedAt, archivedBy string
	h.DB.QueryRow(`SELECT COALESCE(archived_at,''), COALESCE(archived_by,'') FROM agents WHERE id = 'a1'`).Scan(&archivedAt, &archivedBy)
	if archivedAt == "" {
		t.Error("expected archived_at to be set")
	}
	if archivedBy != "user1" {
		t.Errorf("expected archived_by 'user1', got '%s'", archivedBy)
	}

	// Restore.
	req2 := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents/a1/restore", map[string]string{"slug": "test-workspace", "id": "a1"}, nil)
	rr2 := httptest.NewRecorder()
	h.RestoreAgent(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d", rr2.Code)
	}

	// Verify restored.
	var restoredAt string
	h.DB.QueryRow(`SELECT COALESCE(archived_at,'') FROM agents WHERE id = 'a1'`).Scan(&restoredAt)
	if restoredAt != "" {
		t.Error("expected archived_at to be NULL after restore")
	}
}

// ---------------------------------------------------------------------------
// Agent Skills
// ---------------------------------------------------------------------------

func TestCreateSkill(t *testing.T) {
	h := newTestHandler(t)
	body := `{"name":"PDF Parser","description":"Parse PDF documents into markdown"}`

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents/skills", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateSkill(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrap struct { Skill protocol.AgentSkill `json:"skill"` }
	json.NewDecoder(rr.Body).Decode(&wrap); sk := wrap.Skill
	if sk.Name != "PDF Parser" {
		t.Errorf("expected name 'PDF Parser', got '%s'", sk.Name)
	}
}

func TestAddAgentSkill(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name) VALUES ('a1', 'ws1', 'Agent')`)
	h.DB.Exec(`INSERT INTO agent_skills (id, workspace_id, name) VALUES ('sk1', 'ws1', 'Parser')`)

	body := `{"skill_id":"sk1"}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents/a1/skills", map[string]string{"slug": "test-workspace", "id": "a1"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.AddAgentSkill(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify skill is listed on agent.
	req2 := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/agents/a1", map[string]string{"slug": "test-workspace", "id": "a1"}, nil)
	rr2 := httptest.NewRecorder()
	h.GetAgent(rr2, req2)

	var wrap struct { Agent protocol.Agent `json:"agent"` }
	json.NewDecoder(rr2.Body).Decode(&wrap)
	a := wrap.Agent

	if len(a.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(a.Skills))
	}
	if a.Skills[0].Name != "Parser" {
		t.Errorf("expected skill name 'Parser', got '%s'", a.Skills[0].Name)
	}
}

func TestRemoveAgentSkill(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name) VALUES ('a1', 'ws1', 'Agent')`)
	h.DB.Exec(`INSERT INTO agent_skills (id, workspace_id, name) VALUES ('sk1', 'ws1', 'Parser')`)
	h.DB.Exec(`INSERT INTO agent_skills_agents (agent_id, skill_id) VALUES ('a1', 'sk1')`)

	req := chiRequest(http.MethodDelete, "/api/workspaces/test-workspace/agents/a1/skills/sk1", map[string]string{"slug": "test-workspace", "id": "a1", "skillId": "sk1"}, nil)
	rr := httptest.NewRecorder()

	h.RemoveAgentSkill(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify skill removed.
	req2 := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/agents/a1", map[string]string{"slug": "test-workspace", "id": "a1"}, nil)
	rr2 := httptest.NewRecorder()
	h.GetAgent(rr2, req2)

	var wrap struct { Agent protocol.Agent `json:"agent"` }
	json.NewDecoder(rr2.Body).Decode(&wrap)
	a := wrap.Agent
	if len(a.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(a.Skills))
	}
}

// ---------------------------------------------------------------------------
// Agent Runtime CRUD
// ---------------------------------------------------------------------------

func TestCreateRuntime(t *testing.T) {
	h := newTestHandler(t)
	body := `{"name":"Claude Code","backend":"claude-code","path":"/usr/local/bin/claude"}`

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents/runtimes", map[string]string{"slug": "test-workspace"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateRuntime(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrap struct { Runtime protocol.AgentRuntime `json:"runtime"` }
	json.NewDecoder(rr.Body).Decode(&wrap); rt := wrap.Runtime

	if rt.Name != "Claude Code" {
		t.Errorf("expected name 'Claude Code', got '%s'", rt.Name)
	}
	if rt.Status != "offline" {
		t.Errorf("expected status 'offline', got '%s'", rt.Status)
	}
}

func TestListRuntimes(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name, backend, path) VALUES ('rt1', 'ws1', 'Claude', 'claude-code', '/bin/claude'), ('rt2', 'ws1', 'Codex', 'codex', '/bin/codex')`)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/agents/runtimes", map[string]string{"slug": "test-workspace"}, nil)
	rr := httptest.NewRecorder()

	h.ListRuntimes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Runtimes []protocol.AgentRuntime `json:"runtimes"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if len(resp.Runtimes) != 2 {
		t.Errorf("expected 2 runtimes, got %d", len(resp.Runtimes))
	}
}

func TestDeleteRuntime(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name) VALUES ('rt1', 'ws1', 'To Delete')`)

	req := chiRequest(http.MethodDelete, "/api/workspaces/test-workspace/agents/runtimes/rt1", map[string]string{"slug": "test-workspace", "id": "rt1"}, nil)
	rr := httptest.NewRecorder()

	h.DeleteRuntime(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Agent Tasks
// ---------------------------------------------------------------------------

func TestCreateAgentTask(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name) VALUES ('rt1', 'ws1', 'Claude')`)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name, runtime_id, runtime_mode) VALUES ('a1', 'ws1', 'Agent', 'rt1', 'claude-code')`)

	body := `{"source_path":"src1","schema_id":"sch1","runtime_id":"rt1","priority":0,"max_attempts":3}`
	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents/a1/tasks", map[string]string{"slug": "test-workspace", "id": "a1"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.CreateAgentTask(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrap struct { Task protocol.AgentTask `json:"task"` }
	json.NewDecoder(rr.Body).Decode(&wrap); task := wrap.Task

	if task.Status != "queued" {
		t.Errorf("expected status 'queued', got '%s'", task.Status)
	}
	if task.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", task.Attempt)
	}
	if task.MaxAttempts != 3 {
		t.Errorf("expected max_attempts 3, got %d", task.MaxAttempts)
	}
}

func TestUpdateAgentTask(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name) VALUES ('a1', 'ws1', 'Agent')`)
	h.DB.Exec(`INSERT INTO agent_runtimes (id, workspace_id, name) VALUES ('rt1', 'ws1', 'Claude')`)
	h.DB.Exec(`INSERT INTO agent_tasks (id, agent_id, workspace_id, status) VALUES ('t1', 'a1', 'ws1', 'dispatched')`)

	body := `{"status":"completed","result":"pages_created=5 pages_skipped=0"}`
	req := chiRequest(http.MethodPatch, "/api/workspaces/test-workspace/agents/a1/tasks/t1", map[string]string{"slug": "test-workspace", "id": "a1", "taskId": "t1"}, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.UpdateAgentTask(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrap struct { Task protocol.AgentTask `json:"task"` }
	json.NewDecoder(rr.Body).Decode(&wrap); task := wrap.Task

	if task.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", task.Status)
	}
	if task.CompletedAt == nil || *task.CompletedAt == "" {
		t.Error("expected completed_at to be set")
	}
}

func TestListAgentTasks(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name) VALUES ('a1', 'ws1', 'Agent')`)
	h.DB.Exec(`INSERT INTO agent_tasks (id, agent_id, workspace_id, status) VALUES ('t1', 'a1', 'ws1', 'completed'), ('t2', 'a1', 'ws1', 'failed')`)

	req := chiRequest(http.MethodGet, "/api/workspaces/test-workspace/agents/a1/tasks", map[string]string{"slug": "test-workspace", "id": "a1"}, nil)
	rr := httptest.NewRecorder()

	h.ListAgentTasks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var wrap struct { Tasks []protocol.AgentTask `json:"tasks"` }
	json.NewDecoder(rr.Body).Decode(&wrap); tasks := wrap.Tasks

	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

// ---------------------------------------------------------------------------
// Agent Heartbeat
// ---------------------------------------------------------------------------

func TestAgentHeartbeat(t *testing.T) {
	h := newTestHandler(t)
	h.DB.Exec(`INSERT INTO agents (id, workspace_id, name, status) VALUES ('a1', 'ws1', 'Agent', 'offline')`)

	req := chiRequest(http.MethodPost, "/api/workspaces/test-workspace/agents/a1/heartbeat", map[string]string{"slug": "test-workspace", "id": "a1"}, nil)
	rr := httptest.NewRecorder()

	h.AgentHeartbeat(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify status changed to online.
	var status string
	h.DB.QueryRow(`SELECT status FROM agents WHERE id = 'a1'`).Scan(&status)
	if status != "online" {
		t.Errorf("expected status 'online', got '%s'", status)
	}
}
