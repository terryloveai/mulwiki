package service

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestJobService(t *testing.T) *JobService {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT (datetime('now')));
		CREATE TABLE jobs (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			status TEXT NOT NULL DEFAULT 'pending',
			agent_id TEXT NOT NULL DEFAULT '',
			source_path TEXT NOT NULL DEFAULT '',
			source_paths TEXT NOT NULL DEFAULT '[]',
			schema_id TEXT NOT NULL DEFAULT '',
			progress INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			claimed_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			completed_at TEXT
		);
		INSERT INTO workspaces (id, slug, name) VALUES ('ws1', 'test', 'Test Workspace');
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return &JobService{DB: db}
}

func TestCreateJob(t *testing.T) {
	s := newTestJobService(t)

	job, err := s.CreateJob(CreateJobInput{
		WorkspaceID: "ws1",
		AgentID:     "agent1",
		SourcePath:  "src1",
		SchemaID:    "sch1",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if job.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", job.Status)
	}
	if job.WorkspaceID != "ws1" {
		t.Errorf("expected workspace_id 'ws1', got '%s'", job.WorkspaceID)
	}
	if job.SourcePath != "src1" {
		t.Errorf("expected source_path 'src1', got '%s'", job.SourcePath)
	}
	if job.SchemaID != "sch1" {
		t.Errorf("expected schema_id 'sch1', got '%s'", job.SchemaID)
	}
	if job.AgentID != "agent1" {
		t.Errorf("expected agent_id 'agent1', got '%s'", job.AgentID)
	}
}

func TestCreateJobWithSourcePaths(t *testing.T) {
	s := newTestJobService(t)

	job, err := s.CreateJob(CreateJobInput{
		WorkspaceID:  "ws1",
		AgentID:      "agent1",
		SourcePaths:  []string{"src1", "src2"},
		SchemaID:     "sch1",
		InitialClaim: "daemon-seed",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if len(job.SourcePaths) != 2 {
		t.Fatalf("expected 2 source paths, got %d", len(job.SourcePaths))
	}
	if job.ClaimedBy != "daemon-seed" {
		t.Errorf("expected claimed_by daemon-seed, got %q", job.ClaimedBy)
	}
}

func TestClaimJob(t *testing.T) {
	s := newTestJobService(t)

	// Create two pending jobs.
	s.CreateJob(CreateJobInput{WorkspaceID: "ws1", AgentID: "agent1", SourcePath: "src1", SchemaID: "sch1"})
	s.CreateJob(CreateJobInput{WorkspaceID: "ws1", AgentID: "agent1", SourcePath: "src2", SchemaID: "sch2"})

	// Claim first.
	job1, err := s.ClaimJob("ws1", "daemon-1")
	if err != nil {
		t.Fatalf("ClaimJob 1: %v", err)
	}
	if job1 == nil {
		t.Fatal("expected a job, got nil")
	}
	if job1.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", job1.Status)
	}
	if job1.ClaimedBy != "daemon-1" {
		t.Errorf("expected claimed_by 'daemon-1', got '%s'", job1.ClaimedBy)
	}
	if job1.SourcePath != "src1" {
		t.Errorf("expected first job (src1), got '%s'", job1.SourcePath)
	}

	// Claim second.
	job2, err := s.ClaimJob("ws1", "daemon-2")
	if err != nil {
		t.Fatalf("ClaimJob 2: %v", err)
	}
	if job2 == nil {
		t.Fatal("expected a second job, got nil")
	}
	if job2.SourcePath != "src2" {
		t.Errorf("expected second job (src2), got '%s'", job2.SourcePath)
	}

	// No more jobs.
	job3, err := s.ClaimJob("ws1", "daemon-3")
	if err != nil {
		t.Fatalf("ClaimJob 3: %v", err)
	}
	if job3 != nil {
		t.Errorf("expected nil (no more jobs), got %v", job3)
	}
}

func TestClaimJobNoPendingJobs(t *testing.T) {
	s := newTestJobService(t)

	job, err := s.ClaimJob("ws1", "daemon-1")
	if err != nil {
		t.Fatalf("ClaimJob: %v", err)
	}
	if job != nil {
		t.Errorf("expected nil, got job %s", job.ID)
	}
}

func TestCompleteJob(t *testing.T) {
	s := newTestJobService(t)

	job, _ := s.CreateJob(CreateJobInput{WorkspaceID: "ws1", AgentID: "agent1", SourcePath: "src1", SchemaID: "sch1"})

	err := s.CompleteJob("ws1", job.ID, 100)
	if err != nil {
		t.Fatalf("CompleteJob: %v", err)
	}

	// Verify.
	updated, _ := s.GetJob(job.ID)
	if updated.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", updated.Status)
	}
	if updated.Progress != 100 {
		t.Errorf("expected progress 100, got %d", updated.Progress)
	}
	if updated.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestFailJob(t *testing.T) {
	s := newTestJobService(t)

	job, _ := s.CreateJob(CreateJobInput{WorkspaceID: "ws1", AgentID: "agent1", SourcePath: "src1", SchemaID: "sch1"})

	err := s.FailJob("ws1", job.ID, "something went wrong")
	if err != nil {
		t.Fatalf("FailJob: %v", err)
	}

	updated, _ := s.GetJob(job.ID)
	if updated.Status != "failed" {
		t.Errorf("expected status 'failed', got '%s'", updated.Status)
	}
	if updated.Error != "something went wrong" {
		t.Errorf("expected error 'something went wrong', got '%s'", updated.Error)
	}
}

func TestUpdateJobProgress(t *testing.T) {
	s := newTestJobService(t)

	job, _ := s.CreateJob(CreateJobInput{WorkspaceID: "ws1", AgentID: "agent1", SourcePath: "src1", SchemaID: "sch1"})

	err := s.UpdateJobProgress("ws1", job.ID, 50)
	if err != nil {
		t.Fatalf("UpdateJobProgress: %v", err)
	}

	updated, _ := s.GetJob(job.ID)
	if updated.Progress != 50 {
		t.Errorf("expected progress 50, got %d", updated.Progress)
	}
}

func TestListAndGetJobsAreWorkspaceScoped(t *testing.T) {
	s := newTestJobService(t)
	s.DB.Exec(`INSERT INTO workspaces (id, slug, name) VALUES ('ws2', 'other', 'Other')`)
	ws1Job, _ := s.CreateJob(CreateJobInput{WorkspaceID: "ws1", AgentID: "agent1", SourcePath: "src1", SchemaID: "sch1"})
	s.CreateJob(CreateJobInput{WorkspaceID: "ws2", AgentID: "agent1", SourcePath: "src2", SchemaID: "sch1"})

	jobs, err := s.ListJobs("ws1")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].WorkspaceID != "ws1" {
		t.Fatalf("expected only ws1 jobs, got %+v", jobs)
	}

	got, err := s.GetWorkspaceJob("ws1", ws1Job.ID)
	if err != nil {
		t.Fatalf("GetWorkspaceJob: %v", err)
	}
	if got.ID != ws1Job.ID {
		t.Errorf("expected %s, got %s", ws1Job.ID, got.ID)
	}
}
