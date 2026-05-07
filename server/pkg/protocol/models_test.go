package protocol

import (
	"encoding/json"
	"testing"
)

// strPtr returns a pointer to the given string.
func strPtr(s string) *string { return &s }

func intPtr(i int) *int { return &i }

func TestWorkspaceJSON(t *testing.T) {
	ws := Workspace{
		ID:          "ws-1",
		Slug:        "test-workspace",
		Name:        "Test",
		Description: "A test workspace",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	data, err := json.Marshal(ws)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var ws2 Workspace
	if err := json.Unmarshal(data, &ws2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ws2.ID != ws.ID {
		t.Errorf("id mismatch: %q vs %q", ws2.ID, ws.ID)
	}
	if ws2.ActiveSchemaID != nil {
		t.Error("expected nil active_schema_id")
	}
}

func TestWorkspaceJSON_WithActiveSchema(t *testing.T) {
	ws := Workspace{
		ID:             "ws-1",
		Slug:           "test",
		Name:           "Test",
		ActiveSchemaID: strPtr("sch-1"),
	}
	data, _ := json.Marshal(ws)

	var ws2 Workspace
	json.Unmarshal(data, &ws2)

	if ws2.ActiveSchemaID == nil || *ws2.ActiveSchemaID != "sch-1" {
		t.Errorf("expected active_schema_id 'sch-1', got %v", ws2.ActiveSchemaID)
	}
}

func TestJobJSON(t *testing.T) {
	completedAt := "2026-01-02T00:00:00Z"
	j := Job{
		ID:          "job-1",
		WorkspaceID: "ws-1",
		Status:      "completed",
		AgentID:     "agent-1",
		SourcePath:  "sources/doc.pdf",
		SourcePaths: []string{"sources/doc.pdf", "sources/notes.md"},
		SchemaID:    "sch-1",
		Progress:    100,
		ClaimedBy:   "daemon-1",
		CreatedAt:   "2026-01-01T00:00:00Z",
		CompletedAt: &completedAt,
	}

	data, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var j2 Job
	if err := json.Unmarshal(data, &j2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if j2.SourcePath != "sources/doc.pdf" {
		t.Errorf("source_path mismatch: %q", j2.SourcePath)
	}
	if len(j2.SourcePaths) != 2 {
		t.Errorf("expected 2 source_paths, got %d", len(j2.SourcePaths))
	}
	if j2.SourcePaths[0] != "sources/doc.pdf" {
		t.Errorf("source_paths[0] mismatch: %q", j2.SourcePaths[0])
	}
	if j2.CompletedAt == nil || *j2.CompletedAt != completedAt {
		t.Errorf("completed_at mismatch: %v", j2.CompletedAt)
	}
}

func TestJobJSON_EmptySourcePaths(t *testing.T) {
	j := Job{
		ID:          "job-1",
		WorkspaceID: "ws-1",
		Status:      "pending",
		SourcePaths: []string{},
	}

	data, _ := json.Marshal(j)
	var j2 Job
	json.Unmarshal(data, &j2)

	if j2.SourcePaths == nil {
		t.Error("expected non-nil SourcePaths after unmarshal")
	}
	if len(j2.SourcePaths) != 0 {
		t.Errorf("expected empty slice, got %d items", len(j2.SourcePaths))
	}
}

func TestJobJSON_NilCompletedAt(t *testing.T) {
	j := Job{
		ID:     "job-1",
		Status: "running",
	}
	data, _ := json.Marshal(j)

	// completed_at should be omitted or null.
	raw := string(data)
	// It's fine either way; just verify we can unmarshal back.
	var j2 Job
	if err := json.Unmarshal(data, &j2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if j2.CompletedAt != nil {
		t.Errorf("expected nil completed_at, got %v", *j2.CompletedAt)
	}
	_ = raw // used
}

func TestAgentJSON_WithSkills(t *testing.T) {
	a := Agent{
		ID:          "agent-1",
		WorkspaceID: "ws-1",
		Name:        "Test Agent",
		RuntimeMode: "claude-code",
		CustomEnv:   map[string]string{"API_KEY": "secret"},
		CustomArgs:  []string{"--verbose", "--model", "sonnet"},
		Skills: []AgentSkill{
			{ID: "sk-1", Name: "Code Review", Description: "Reviews code"},
			{ID: "sk-2", Name: "Testing", Description: "Runs tests"},
		},
		Status:    "online",
		CreatedAt: "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var a2 Agent
	if err := json.Unmarshal(data, &a2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(a2.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(a2.Skills))
	}
	if a2.Skills[0].Name != "Code Review" {
		t.Errorf("skill name mismatch: %q", a2.Skills[0].Name)
	}
	if len(a2.CustomEnv) != 1 || a2.CustomEnv["API_KEY"] != "secret" {
		t.Errorf("custom_env mismatch: %v", a2.CustomEnv)
	}
	if len(a2.CustomArgs) != 3 {
		t.Errorf("expected 3 custom_args, got %d", len(a2.CustomArgs))
	}
}

func TestAgentJSON_NoSkills(t *testing.T) {
	a := Agent{
		ID:     "agent-1",
		Name:   "Bare Agent",
		Status: "offline",
	}
	data, _ := json.Marshal(a)

	var a2 Agent
	json.Unmarshal(data, &a2)

	if a2.Skills != nil {
		t.Errorf("expected nil Skills, got %d items", len(a2.Skills))
	}
}

func TestAgentTaskJSON(t *testing.T) {
	task := AgentTask{
		ID:          "task-1",
		AgentID:     "agent-1",
		WorkspaceID: "ws-1",
		SourcePath:  "sources/doc.pdf",
		SchemaID:    "sch-1",
		Status:      "queued",
		Priority:    5,
		Attempt:     1,
		MaxAttempts: 3,
		CreatedAt:   "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var t2 AgentTask
	if err := json.Unmarshal(data, &t2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if t2.SourcePath != "sources/doc.pdf" {
		t.Errorf("source_path mismatch: %q", t2.SourcePath)
	}
	if t2.Status != "queued" {
		t.Errorf("status mismatch: %q", t2.Status)
	}
	if t2.Attempt != 1 {
		t.Errorf("attempt mismatch: %d", t2.Attempt)
	}
}

func TestAgentTaskJSON_NullableFields(t *testing.T) {
	task := AgentTask{
		ID:     "task-1",
		Status: "completed",
		// RuntimeID, ParentTaskID, DispatchedAt, StartedAt, CompletedAt left nil
	}
	data, _ := json.Marshal(task)

	var t2 AgentTask
	if err := json.Unmarshal(data, &t2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if t2.RuntimeID != nil {
		t.Error("expected nil runtime_id")
	}
	if t2.ParentTaskID != nil {
		t.Error("expected nil parent_task_id")
	}
}

func TestDaemonRegisterRequestJSON(t *testing.T) {
	req := DaemonRegisterRequest{
		ID:                 "daemon-1",
		Hostname:           "macbook.local",
		PID:                12345,
		Version:            "1.0.0",
		WorkspaceSlug:      "test",
		RuntimeIDs:         []string{"rt-1", "rt-2"},
		MaxConcurrentTasks: 8,
		Runtimes: []RuntimeInfo{
			{Name: "Claude Code", Backend: "claude-code", Version: "2.0.0", Path: "/usr/local/bin/claude"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var req2 DaemonRegisterRequest
	if err := json.Unmarshal(data, &req2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req2.Hostname != "macbook.local" {
		t.Errorf("hostname mismatch: %q", req2.Hostname)
	}
	if len(req2.Runtimes) != 1 {
		t.Errorf("expected 1 runtime, got %d", len(req2.Runtimes))
	}
	if req2.Runtimes[0].Backend != "claude-code" {
		t.Errorf("runtime backend mismatch: %q", req2.Runtimes[0].Backend)
	}
	if req2.MaxConcurrentTasks != 8 {
		t.Errorf("max_concurrent_tasks mismatch: %d", req2.MaxConcurrentTasks)
	}
}

func TestDaemonRegisterRequestJSON_EmptyRuntimes(t *testing.T) {
	req := DaemonRegisterRequest{
		ID:        "daemon-min",
		Runtimes:  nil,
	}
	data, _ := json.Marshal(req)

	var req2 DaemonRegisterRequest
	json.Unmarshal(data, &req2)

	if req2.Runtimes != nil {
		t.Errorf("expected nil Runtimes, got %d items", len(req2.Runtimes))
	}
}

func TestSourceJSON(t *testing.T) {
	s := Source{
		Name: "report.pdf",
		Type: "pdf",
		Path: "sources/report.pdf",
		Size: 102400,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var s2 Source
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s2.Path != "sources/report.pdf" {
		t.Errorf("path mismatch: %q", s2.Path)
	}
	if s2.Type != "pdf" {
		t.Errorf("type mismatch: %q", s2.Type)
	}
	if s2.Size != 102400 {
		t.Errorf("size mismatch: %d", s2.Size)
	}
}

func TestWikiPageJSON(t *testing.T) {
	wp := WikiPage{
		Path:    "/concepts/ai",
		Title:   "Artificial Intelligence",
		Content: "AI is the simulation of human intelligence.",
		Type:    "Concept",
		Layer:   "core",
	}

	data, err := json.Marshal(wp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wp2 WikiPage
	if err := json.Unmarshal(data, &wp2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wp2.Path != "/concepts/ai" {
		t.Errorf("path mismatch: %q", wp2.Path)
	}
	if wp2.Title != "Artificial Intelligence" {
		t.Errorf("title mismatch: %q", wp2.Title)
	}
	if wp2.Type != "Concept" {
		t.Errorf("type mismatch: %q", wp2.Type)
	}
}

func TestWikiPageJSON_NoLayer(t *testing.T) {
	wp := WikiPage{
		Path:  "/misc/notes",
		Title: "Random Notes",
		Type:  "Note",
	}
	data, _ := json.Marshal(wp)

	var wp2 WikiPage
	json.Unmarshal(data, &wp2)

	if wp2.Layer != "" {
		t.Errorf("expected empty layer, got %q", wp2.Layer)
	}
}

func TestSchemaWithActiveJSON(t *testing.T) {
	s := SchemaWithActive{
		Schema: Schema{
			ID:          "sch-1",
			WorkspaceID: "ws-1",
			Name:        "Knowledge Graph",
			Version:     "1.0.0",
			Content:      "# Types\n- Fact\n- Concept",
		},
		IsActive: true,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var s2 SchemaWithActive
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !s2.IsActive {
		t.Error("expected is_active=true")
	}
	if s2.Name != "Knowledge Graph" {
		t.Errorf("name mismatch: %q", s2.Name)
	}
}

func TestCreateJobRequestJSON(t *testing.T) {
	req := CreateJobRequest{
		AgentID:     "agent-1",
		SourcePath:  "sources/doc.pdf",
		SourcePaths: []string{"sources/doc.pdf", "sources/notes.md"},
		SchemaID:    "sch-1",
	}
	data, _ := json.Marshal(req)

	var req2 CreateJobRequest
	json.Unmarshal(data, &req2)

	if req2.SourcePath != "sources/doc.pdf" {
		t.Errorf("source_path mismatch: %q", req2.SourcePath)
	}
	if len(req2.SourcePaths) != 2 {
		t.Errorf("expected 2 source_paths, got %d", len(req2.SourcePaths))
	}
}

func TestUpdateSchemaRequestJSON_Partial(t *testing.T) {
	// Only update description, leave name and version nil.
	req := UpdateSchemaRequest{
		Description: strPtr("Updated description"),
	}
	data, _ := json.Marshal(req)

	// Should only contain description.
	raw := string(data)
	if raw != `{"description":"Updated description"}` {
		// Note: order of JSON keys is unspecified. Check individually.
		var m map[string]interface{}
		json.Unmarshal(data, &m)
		if _, ok := m["name"]; ok {
			t.Errorf("expected 'name' to be omitted, got %s", raw)
		}
		if _, ok := m["version"]; ok {
			t.Errorf("expected 'version' to be omitted, got %s", raw)
		}
	}
}
