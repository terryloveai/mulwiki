package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tethy/mulwiki/server/internal/events"
	"github.com/tethy/mulwiki/server/internal/store"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var ErrInvalidTaskTransition = errors.New("invalid task status transition")

type TaskService struct {
	DB  *sql.DB
	Bus *events.Bus
}

type taskRowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func NewTaskService(db *sql.DB, bus *events.Bus) *TaskService {
	return &TaskService{DB: db, Bus: bus}
}

func (s *TaskService) CreateTaskForJob(ctx context.Context, jobID, workspaceID, agentID, runtimeID, sourcePath, schemaID string) (*protocol.AgentTask, error) {
	task, err := createTaskForJob(ctx, s.DB, jobID, workspaceID, agentID, runtimeID, sourcePath, schemaID)
	if err != nil {
		return nil, fmt.Errorf("create task for job: %w", err)
	}
	return task, nil
}

func createTaskForJob(ctx context.Context, q taskRowQueryer, jobID, workspaceID, agentID, runtimeID, sourcePath, schemaID string) (*protocol.AgentTask, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var runtime any
	if runtimeID != "" {
		runtime = runtimeID
	}

	row := q.QueryRowContext(ctx,
		`INSERT INTO agent_tasks (job_id, agent_id, runtime_id, workspace_id, source_path, schema_id,
		                          status, priority, attempt, max_attempts, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'queued', 0, 1, 3, ?)
		 RETURNING `+store.AgentTaskColumns,
		jobID, agentID, runtime, workspaceID, sourcePath, schemaID, now,
	)
	return store.ScanAgentTask(row.Scan)
}

func (s *TaskService) ClaimNext(ctx context.Context, workspaceID, runtimeID, daemonID string) (*protocol.AgentTask, error) {
	return s.claimNext(ctx, workspaceID, runtimeID, "", daemonID)
}

func (s *TaskService) ClaimNextForAgent(ctx context.Context, workspaceID, agentID, daemonID string) (*protocol.AgentTask, error) {
	return s.claimNext(ctx, workspaceID, "", agentID, daemonID)
}

func (s *TaskService) Dispatch(ctx context.Context, taskID, workspaceID, daemonID string) (*protocol.AgentTask, error) {
	if daemonID == "" {
		return nil, errors.New("daemon_id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET status = 'dispatched', daemon_id = ?, dispatched_at = ?
		 WHERE id = ? AND workspace_id = ? AND status = 'queued'`,
		daemonID, now, taskID, workspaceID,
	)
	if err := s.checkLifecycleUpdate(ctx, result, err, taskID, workspaceID); err != nil {
		return nil, err
	}
	task, err := store.NewTaskStore(s.DB).GetInWorkspace(ctx, workspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("fetch dispatched task: %w", err)
	}
	s.publish(events.EventTaskDispatched, task)
	return task, nil
}

func (s *TaskService) claimNext(ctx context.Context, workspaceID, runtimeID, agentID, daemonID string) (*protocol.AgentTask, error) {
	if daemonID == "" {
		return nil, errors.New("daemon_id is required")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET priority = priority
		 WHERE id = (
		   SELECT id FROM agent_tasks WHERE workspace_id = ? LIMIT 1
		 )`,
		workspaceID,
	); err != nil {
		return nil, fmt.Errorf("lock task queue: %w", err)
	}

	var taskID string
	err = tx.QueryRowContext(ctx,
		`SELECT t.id
		 FROM agent_tasks t
		 JOIN agents a ON a.id = t.agent_id AND a.workspace_id = t.workspace_id
		 WHERE t.workspace_id = ?
		   AND t.status = 'queued'
		   AND (? = '' OR t.runtime_id = ?)
		   AND (? = '' OR t.agent_id = ?)
		   AND (
		     SELECT COUNT(*)
		     FROM agent_tasks active
		     WHERE active.workspace_id = t.workspace_id
		       AND active.agent_id = t.agent_id
		       AND active.status IN ('dispatched', 'running')
		   ) < CASE WHEN a.max_concurrent_tasks <= 0 THEN 1 ELSE a.max_concurrent_tasks END
		 ORDER BY t.priority DESC, t.created_at ASC, t.id ASC
		 LIMIT 1`,
		workspaceID, runtimeID, runtimeID, agentID, agentID,
	).Scan(&taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("select claim candidate: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET status = 'dispatched', daemon_id = ?, dispatched_at = ?
		 WHERE id = ? AND workspace_id = ? AND status = 'queued'`,
		daemonID, now, taskID, workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("claim update: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("claim rows affected: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}

	task, err := store.NewTaskStore(s.DB).GetInWorkspace(ctx, workspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("fetch claimed task: %w", err)
	}
	s.publish(events.EventTaskDispatched, task)
	return task, nil
}

func (s *TaskService) Start(ctx context.Context, taskID, workspaceID string) (*protocol.AgentTask, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET status = 'running', started_at = ?
		 WHERE id = ? AND workspace_id = ? AND status = 'dispatched'`,
		now, taskID, workspaceID,
	)
	if err := s.checkLifecycleUpdate(ctx, result, err, taskID, workspaceID); err != nil {
		return nil, err
	}
	task, err := store.NewTaskStore(s.DB).GetInWorkspace(ctx, workspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("fetch started task: %w", err)
	}
	s.publish(events.EventTaskStarted, task)
	return task, nil
}

func (s *TaskService) Complete(ctx context.Context, taskID, workspaceID, resultText, sessionID, workDir string) (*protocol.AgentTask, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET status = 'completed',
		     result = ?,
		     session_id = CASE WHEN ? != '' THEN ? ELSE session_id END,
		     work_dir = CASE WHEN ? != '' THEN ? ELSE work_dir END,
		     completed_at = ?
		 WHERE id = ? AND workspace_id = ? AND status = 'running'`,
		resultText, sessionID, sessionID, workDir, workDir, now, taskID, workspaceID,
	)
	if err := s.checkLifecycleUpdate(ctx, result, err, taskID, workspaceID); err != nil {
		return nil, err
	}
	task, err := store.NewTaskStore(s.DB).GetInWorkspace(ctx, workspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("fetch completed task: %w", err)
	}
	s.publish(events.EventTaskCompleted, task)
	return task, nil
}

func (s *TaskService) Fail(ctx context.Context, taskID, workspaceID, reason, message, sessionID, workDir string) (*protocol.AgentTask, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET status = 'failed',
		     failure_reason = ?,
		     error = ?,
		     session_id = CASE WHEN ? != '' THEN ? ELSE session_id END,
		     work_dir = CASE WHEN ? != '' THEN ? ELSE work_dir END,
		     completed_at = ?
		 WHERE id = ? AND workspace_id = ? AND status IN ('dispatched', 'running')`,
		reason, message, sessionID, sessionID, workDir, workDir, now, taskID, workspaceID,
	)
	if err := s.checkLifecycleUpdate(ctx, result, err, taskID, workspaceID); err != nil {
		return nil, err
	}
	task, err := store.NewTaskStore(s.DB).GetInWorkspace(ctx, workspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("fetch failed task: %w", err)
	}
	s.publish(events.EventTaskFailed, task)
	return task, nil
}

func (s *TaskService) Cancel(ctx context.Context, taskID, workspaceID string) (*protocol.AgentTask, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET status = 'cancelled', completed_at = ?
		 WHERE id = ? AND workspace_id = ? AND status IN ('queued', 'dispatched', 'running')`,
		now, taskID, workspaceID,
	)
	if err := s.checkLifecycleUpdate(ctx, result, err, taskID, workspaceID); err != nil {
		return nil, err
	}
	task, err := store.NewTaskStore(s.DB).GetInWorkspace(ctx, workspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("fetch cancelled task: %w", err)
	}
	s.publish(events.EventTaskCancelled, task)
	return task, nil
}

func (s *TaskService) RecoverOrphans(ctx context.Context, workspaceID, daemonID string) ([]protocol.AgentTask, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id
		 FROM agent_tasks
		 WHERE workspace_id = ? AND daemon_id = ? AND status IN ('dispatched', 'running')
		 ORDER BY created_at ASC, id ASC`,
		workspaceID, daemonID,
	)
	if err != nil {
		return nil, fmt.Errorf("list orphan tasks: %w", err)
	}
	defer rows.Close()

	taskIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan orphan task: %w", err)
		}
		taskIDs = append(taskIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orphan tasks: %w", err)
	}

	recovered := make([]protocol.AgentTask, 0, len(taskIDs))
	for _, id := range taskIDs {
		task, err := s.Fail(ctx, id, workspaceID, "runtime_recovery", "task recovered after daemon restart", "", "")
		if err != nil {
			return nil, fmt.Errorf("recover orphan task %s: %w", id, err)
		}
		recovered = append(recovered, *task)
	}
	return recovered, nil
}

func (s *TaskService) AppendMessages(ctx context.Context, workspaceID, taskID string, messages []protocol.AgentTaskMessage) error {
	var agentID string
	if err := s.DB.QueryRowContext(ctx,
		`SELECT agent_id FROM agent_tasks WHERE id = ? AND workspace_id = ?`,
		taskID, workspaceID,
	).Scan(&agentID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	var nextSeq int64
	if err := s.DB.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM agent_task_messages WHERE task_id = ?`,
		taskID,
	).Scan(&nextSeq); err != nil {
		return err
	}

	for _, msg := range messages {
		seq := msg.Seq
		if seq <= 0 {
			seq = nextSeq
			nextSeq++
		} else if seq >= nextSeq {
			nextSeq = seq + 1
		}

		role := msg.Role
		if role == "" {
			role = "agent"
		}
		input := defaultJSON(msg.Input)
		if !json.Valid(input) {
			return fmt.Errorf("invalid task message input")
		}
		metadata := defaultJSON(msg.Metadata)
		if !json.Valid(metadata) {
			return fmt.Errorf("invalid task message metadata")
		}

		if _, err := s.DB.ExecContext(ctx,
			`INSERT OR IGNORE INTO agent_task_messages
			 (task_id, workspace_id, agent_id, role, seq, type, content, tool, call_id, input, output, status, level, session_id, metadata)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			taskID, workspaceID, agentID, role, seq, msg.Type, msg.Content, msg.Tool, msg.CallID,
			string(input), msg.Output, msg.Status, msg.Level, msg.SessionID, string(metadata),
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *TaskService) ListMessages(ctx context.Context, workspaceID, taskID string, sinceSeq int64) ([]protocol.AgentTaskMessage, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT m.id, m.task_id, m.workspace_id, m.agent_id, m.role,
		        m.seq, m.type, m.content, m.tool, m.call_id, m.input, m.output,
		        m.status, m.level, m.session_id, m.metadata, m.created_at
		 FROM agent_task_messages m
		 JOIN agent_tasks t ON t.id = m.task_id AND t.workspace_id = m.workspace_id
		 WHERE m.task_id = ? AND t.workspace_id = ? AND m.seq > ?
		 ORDER BY m.seq ASC, m.created_at ASC, m.id ASC`,
		taskID, workspaceID, sinceSeq,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]protocol.AgentTaskMessage, 0)
	for rows.Next() {
		msg, err := scanTaskMessage(rows.Scan)
		if err != nil {
			return nil, err
		}
		messages = append(messages, *msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *TaskService) PinSession(ctx context.Context, workspaceID, taskID, sessionID, workDir string) error {
	if sessionID == "" && workDir == "" {
		return nil
	}
	result, err := s.DB.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET session_id = CASE WHEN ? != '' THEN ? ELSE session_id END,
		     work_dir = CASE WHEN ? != '' THEN ? ELSE work_dir END
		 WHERE id = ? AND workspace_id = ?`,
		sessionID, sessionID, workDir, workDir, taskID, workspaceID,
	)
	return rowsAffectedOrNotFound(result, err)
}

func (s *TaskService) checkLifecycleUpdate(ctx context.Context, result sql.Result, err error, taskID, workspaceID string) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}

	var exists int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_tasks WHERE id = ? AND workspace_id = ?`, taskID, workspaceID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	return ErrInvalidTaskTransition
}

func (s *TaskService) publish(eventType events.EventType, task *protocol.AgentTask) {
	if s.Bus == nil || task == nil {
		return
	}
	s.Bus.Publish(events.Event{
		Type:        eventType,
		WorkspaceID: task.WorkspaceID,
		AgentID:     task.AgentID,
		TaskID:      task.ID,
		Payload:     *task,
	})
}

func defaultJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`)
	}
	return raw
}

func scanTaskMessage(scan func(dest ...any) error) (*protocol.AgentTaskMessage, error) {
	var msg protocol.AgentTaskMessage
	var input, metadata string
	err := scan(
		&msg.ID,
		&msg.TaskID,
		&msg.WorkspaceID,
		&msg.AgentID,
		&msg.Role,
		&msg.Seq,
		&msg.Type,
		&msg.Content,
		&msg.Tool,
		&msg.CallID,
		&input,
		&msg.Output,
		&msg.Status,
		&msg.Level,
		&msg.SessionID,
		&metadata,
		&msg.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	msg.Input = json.RawMessage(input)
	msg.Metadata = json.RawMessage(metadata)
	return &msg, nil
}
