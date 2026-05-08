package service

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/tethy/mulwiki/server/internal/events"

	_ "github.com/mattn/go-sqlite3"
)

func newTestTaskService(t *testing.T) (*TaskService, *events.Bus, *sql.DB) {
	t.Helper()

	dbPath := t.TempDir() + "/task-service.db"
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(10)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY, slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT (datetime('now')));
		CREATE TABLE agent_runtimes (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			backend TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'offline',
			daemon_id TEXT NOT NULL DEFAULT '',
			last_heartbeat TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE agents (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			runtime_id TEXT REFERENCES agent_runtimes(id),
			name TEXT NOT NULL,
			max_concurrent_tasks INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE agent_tasks (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			job_id TEXT NOT NULL DEFAULT '',
			agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
			runtime_id TEXT REFERENCES agent_runtimes(id),
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			source_path TEXT NOT NULL DEFAULT '',
			schema_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'dispatched', 'running', 'completed', 'failed', 'cancelled')),
			priority INTEGER NOT NULL DEFAULT 0,
			parent_task_id TEXT REFERENCES agent_tasks(id),
			session_id TEXT NOT NULL DEFAULT '',
			work_dir TEXT NOT NULL DEFAULT '',
			failure_reason TEXT NOT NULL DEFAULT '',
			daemon_id TEXT NOT NULL DEFAULT '',
			dispatched_at TEXT,
			started_at TEXT,
			completed_at TEXT,
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			attempt INTEGER NOT NULL DEFAULT 1,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE agent_task_messages (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			task_id TEXT NOT NULL REFERENCES agent_tasks(id) ON DELETE CASCADE,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
			role TEXT NOT NULL DEFAULT 'agent',
			seq INTEGER NOT NULL DEFAULT 0,
			type TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			tool TEXT NOT NULL DEFAULT '',
			call_id TEXT NOT NULL DEFAULT '',
			input TEXT NOT NULL DEFAULT '{}',
			output TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			level TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE UNIQUE INDEX idx_agent_task_messages_task_seq_unique ON agent_task_messages(task_id, seq) WHERE seq > 0;
		INSERT INTO workspaces (id, slug, name) VALUES ('ws1', 'test', 'Test Workspace');
		INSERT INTO agent_runtimes (id, workspace_id, name, backend, path) VALUES ('rt1', 'ws1', 'Codex', 'codex', '/bin/codex');
		INSERT INTO agents (id, workspace_id, runtime_id, name, max_concurrent_tasks) VALUES ('a1', 'ws1', 'rt1', 'Agent', 1);
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	bus := events.NewBus()
	return NewTaskService(db, bus), bus, db
}

func TestTaskServiceCreateTaskForJob(t *testing.T) {
	s, _, db := newTestTaskService(t)

	task, err := s.CreateTaskForJob(context.Background(), "job1", "ws1", "a1", "rt1", "src1", "sch1")
	if err != nil {
		t.Fatalf("CreateTaskForJob: %v", err)
	}

	if task.JobID != "job1" {
		t.Errorf("expected job_id job1, got %q", task.JobID)
	}
	if task.Status != "queued" {
		t.Errorf("expected queued task, got %q", task.Status)
	}
	if task.RuntimeID == nil || *task.RuntimeID != "rt1" {
		t.Fatalf("expected runtime rt1, got %v", task.RuntimeID)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_tasks WHERE job_id = ? AND agent_id = ? AND status = 'queued'`, "job1", "a1").Scan(&count); err != nil {
		t.Fatalf("count task: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one queued task, got %d", count)
	}
}

func TestTaskServiceClaimNextConcurrent(t *testing.T) {
	s, _, db := newTestTaskService(t)
	task, err := s.CreateTaskForJob(context.Background(), "job1", "ws1", "a1", "rt1", "src1", "sch1")
	if err != nil {
		t.Fatalf("CreateTaskForJob: %v", err)
	}

	const claimers = 12
	var wg sync.WaitGroup
	results := make(chan string, claimers)
	errs := make(chan error, claimers)

	for i := 0; i < claimers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := s.ClaimNext(context.Background(), "ws1", "rt1", "daemon-1")
			if err != nil {
				errs <- err
				return
			}
			if claimed != nil {
				results <- claimed.ID
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Errorf("ClaimNext error: %v", err)
	}

	claimedIDs := make(map[string]int)
	for id := range results {
		claimedIDs[id]++
	}
	if len(claimedIDs) != 1 || claimedIDs[task.ID] != 1 {
		t.Fatalf("expected exactly one claim of %s, got %#v", task.ID, claimedIDs)
	}

	var status, daemonID string
	if err := db.QueryRow(`SELECT status, daemon_id FROM agent_tasks WHERE id = ?`, task.ID).Scan(&status, &daemonID); err != nil {
		t.Fatalf("fetch task: %v", err)
	}
	if status != "dispatched" || daemonID != "daemon-1" {
		t.Fatalf("expected dispatched by daemon-1, got status=%q daemon=%q", status, daemonID)
	}
}

func TestTaskServiceClaimNextRespectsCapacity(t *testing.T) {
	s, _, db := newTestTaskService(t)

	if _, err := db.Exec(`INSERT INTO agent_tasks (id, job_id, agent_id, runtime_id, workspace_id, status, daemon_id) VALUES ('running-1', 'job-running', 'a1', 'rt1', 'ws1', 'running', 'daemon-1')`); err != nil {
		t.Fatalf("seed running task: %v", err)
	}
	if _, err := s.CreateTaskForJob(context.Background(), "job2", "ws1", "a1", "rt1", "src2", "sch1"); err != nil {
		t.Fatalf("CreateTaskForJob: %v", err)
	}

	claimed, err := s.ClaimNext(context.Background(), "ws1", "rt1", "daemon-2")
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if claimed != nil {
		t.Fatalf("expected no claim while agent is at capacity, got %s", claimed.ID)
	}
}

func TestTaskServiceLifecyclePublishesEvents(t *testing.T) {
	s, bus, db := newTestTaskService(t)

	eventsSeen := make([]events.EventType, 0)
	for _, eventType := range []events.EventType{
		events.EventTaskStarted,
		events.EventTaskCompleted,
		events.EventTaskFailed,
		events.EventTaskCancelled,
	} {
		eventType := eventType
		bus.Subscribe(eventType, func(e events.Event) {
			eventsSeen = append(eventsSeen, e.Type)
		})
	}

	queued, err := s.CreateTaskForJob(context.Background(), "job-queued", "ws1", "a1", "rt1", "src0", "sch1")
	if err != nil {
		t.Fatalf("CreateTaskForJob queued: %v", err)
	}
	if _, err := s.Start(context.Background(), queued.ID, "ws1"); err == nil {
		t.Fatal("expected Start to reject queued task")
	}

	runningSeed, err := s.CreateTaskForJob(context.Background(), "job-run", "ws1", "a1", "rt1", "src1", "sch1")
	if err != nil {
		t.Fatalf("CreateTaskForJob running seed: %v", err)
	}
	if _, err := db.Exec(`UPDATE agent_tasks SET status = 'dispatched', daemon_id = 'daemon-1' WHERE id = ?`, runningSeed.ID); err != nil {
		t.Fatalf("seed dispatched: %v", err)
	}

	running, err := s.Start(context.Background(), runningSeed.ID, "ws1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if running.Status != "running" || running.StartedAt == nil {
		t.Fatalf("expected running with started_at, got %#v", running)
	}

	completed, err := s.Complete(context.Background(), running.ID, "ws1", "ok", "sess-1", "/tmp/work")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if completed.Status != "completed" || completed.Result != "ok" || completed.SessionID != "sess-1" || completed.WorkDir != "/tmp/work" {
		t.Fatalf("unexpected completed task: %#v", completed)
	}

	failSeed, err := s.CreateTaskForJob(context.Background(), "job-fail", "ws1", "a1", "rt1", "src2", "sch1")
	if err != nil {
		t.Fatalf("CreateTaskForJob fail seed: %v", err)
	}
	if _, err := db.Exec(`UPDATE agent_tasks SET status = 'running', daemon_id = 'daemon-1' WHERE id = ?`, failSeed.ID); err != nil {
		t.Fatalf("seed running: %v", err)
	}
	failed, err := s.Fail(context.Background(), failSeed.ID, "ws1", "agent_error", "boom", "sess-2", "/tmp/fail")
	if err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if failed.Status != "failed" || failed.FailureReason != "agent_error" || failed.Error != "boom" {
		t.Fatalf("unexpected failed task: %#v", failed)
	}

	cancelSeed, err := s.CreateTaskForJob(context.Background(), "job-cancel", "ws1", "a1", "rt1", "src3", "sch1")
	if err != nil {
		t.Fatalf("CreateTaskForJob cancel seed: %v", err)
	}
	cancelled, err := s.Cancel(context.Background(), cancelSeed.ID, "ws1")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != "cancelled" || cancelled.CompletedAt == nil {
		t.Fatalf("unexpected cancelled task: %#v", cancelled)
	}

	want := []events.EventType{
		events.EventTaskStarted,
		events.EventTaskCompleted,
		events.EventTaskFailed,
		events.EventTaskCancelled,
	}
	if len(eventsSeen) != len(want) {
		t.Fatalf("expected events %v, got %v", want, eventsSeen)
	}
	for i := range want {
		if eventsSeen[i] != want[i] {
			t.Fatalf("expected events %v, got %v", want, eventsSeen)
		}
	}
}

func TestTaskServiceRecoverOrphans(t *testing.T) {
	s, _, db := newTestTaskService(t)

	if _, err := db.Exec(`
		INSERT INTO agent_tasks (id, job_id, agent_id, runtime_id, workspace_id, status, daemon_id) VALUES
			('orphan-dispatched', 'job1', 'a1', 'rt1', 'ws1', 'dispatched', 'daemon-1'),
			('orphan-running', 'job2', 'a1', 'rt1', 'ws1', 'running', 'daemon-1'),
			('other-daemon', 'job3', 'a1', 'rt1', 'ws1', 'running', 'daemon-2'),
			('already-done', 'job4', 'a1', 'rt1', 'ws1', 'completed', 'daemon-1')
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	recovered, err := s.RecoverOrphans(context.Background(), "ws1", "daemon-1")
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(recovered) != 2 {
		t.Fatalf("expected 2 recovered tasks, got %d", len(recovered))
	}
	for _, task := range recovered {
		if task.Status != "failed" || task.FailureReason != "runtime_recovery" {
			t.Fatalf("unexpected recovered task: %#v", task)
		}
	}

	var otherStatus string
	if err := db.QueryRow(`SELECT status FROM agent_tasks WHERE id = 'other-daemon'`).Scan(&otherStatus); err != nil {
		t.Fatalf("fetch other daemon task: %v", err)
	}
	if otherStatus != "running" {
		t.Fatalf("expected other daemon task to remain running, got %q", otherStatus)
	}
}
