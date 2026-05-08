package store

import (
	"context"
	"database/sql"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

const AgentTaskColumns = `id, job_id, agent_id, runtime_id, workspace_id, source_path, schema_id,
	status, priority, parent_task_id, session_id, work_dir, failure_reason, daemon_id,
	dispatched_at, started_at, completed_at, result, error, attempt, max_attempts, created_at`

const AgentTaskSelectColumns = `id, COALESCE(job_id, ''), agent_id, runtime_id, workspace_id, source_path, schema_id,
	status, priority, parent_task_id, session_id, work_dir, failure_reason, daemon_id,
	dispatched_at, started_at, completed_at, result, error, attempt, max_attempts, created_at`

type TaskStore struct {
	DB *sql.DB
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{DB: db}
}

func (s *TaskStore) GetInWorkspace(ctx context.Context, workspaceID, taskID string) (*protocol.AgentTask, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+AgentTaskSelectColumns+` FROM agent_tasks WHERE id = ? AND workspace_id = ?`, taskID, workspaceID)
	return ScanAgentTask(row.Scan)
}

func (s *TaskStore) GetByJob(ctx context.Context, workspaceID, jobID, agentID string) (*protocol.AgentTask, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+AgentTaskSelectColumns+`
		 FROM agent_tasks
		 WHERE workspace_id = ? AND job_id = ? AND agent_id = ?
		 ORDER BY created_at ASC, id ASC
		 LIMIT 1`,
		workspaceID, jobID, agentID,
	)
	return ScanAgentTask(row.Scan)
}

func ScanAgentTask(scan func(dest ...any) error) (*protocol.AgentTask, error) {
	var task protocol.AgentTask
	var runtimeID, parentTaskID, dispatchedAt, startedAt, completedAt sql.NullString
	err := scan(
		&task.ID,
		&task.JobID,
		&task.AgentID,
		&runtimeID,
		&task.WorkspaceID,
		&task.SourcePath,
		&task.SchemaID,
		&task.Status,
		&task.Priority,
		&parentTaskID,
		&task.SessionID,
		&task.WorkDir,
		&task.FailureReason,
		&task.DaemonID,
		&dispatchedAt,
		&startedAt,
		&completedAt,
		&task.Result,
		&task.Error,
		&task.Attempt,
		&task.MaxAttempts,
		&task.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if runtimeID.Valid {
		task.RuntimeID = &runtimeID.String
	}
	if parentTaskID.Valid {
		task.ParentTaskID = &parentTaskID.String
	}
	if dispatchedAt.Valid {
		task.DispatchedAt = &dispatchedAt.String
	}
	if startedAt.Valid {
		task.StartedAt = &startedAt.String
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.String
	}
	return &task, nil
}
