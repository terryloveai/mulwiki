package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tethy/mulwiki/server/internal/events"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// JobService handles job lifecycle operations.
type JobService struct {
	DB  *sql.DB
	Bus *events.Bus
}

type CreateJobInput struct {
	WorkspaceID  string
	AgentID      string
	SourcePath   string
	SourcePaths  []string
	SchemaID     string
	InitialClaim string
}

// NewJobService creates a new JobService.
func NewJobService(db *sql.DB) *JobService {
	return &JobService{DB: db}
}

func NewJobServiceWithBus(db *sql.DB, bus *events.Bus) *JobService {
	return &JobService{DB: db, Bus: bus}
}

// CreateJob inserts a new pending job and returns it.
func (s *JobService) CreateJob(input CreateJobInput) (*protocol.Job, error) {
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	sourcePathsJSON, _ := json.Marshal(input.SourcePaths)
	if string(sourcePathsJSON) == "null" {
		sourcePathsJSON = []byte("[]")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create job tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx,
		`INSERT INTO jobs (workspace_id, status, agent_id, source_path, source_paths, schema_id, claimed_by, created_at)
		 VALUES (?, 'pending', ?, ?, ?, ?, ?, ?)
		 RETURNING id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		           claimed_by, created_at, completed_at`,
		input.WorkspaceID, input.AgentID, input.SourcePath, string(sourcePathsJSON), input.SchemaID, input.InitialClaim, now,
	)
	j, err := scanJob(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	if input.AgentID != "" {
		runtimeID, err := resolveAgentRuntimeID(ctx, tx, input.WorkspaceID, input.AgentID)
		if err != nil {
			return nil, fmt.Errorf("resolve agent runtime: %w", err)
		}
		taskSourcePath := input.SourcePath
		if taskSourcePath == "" && len(input.SourcePaths) > 0 {
			taskSourcePath = input.SourcePaths[0]
		}
		if _, err := createTaskForJob(ctx, tx, j.ID, input.WorkspaceID, input.AgentID, runtimeID, taskSourcePath, input.SchemaID); err != nil {
			return nil, fmt.Errorf("create job task: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create job: %w", err)
	}
	return j, nil
}

// ClaimJob atomically claims the next pending job for the given workspace.
func (s *JobService) ClaimJob(workspaceID, daemonID string) (*protocol.Job, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Pick the oldest pending job in this workspace.
	var jobID string
	err = tx.QueryRow(
		`SELECT id FROM jobs
		 WHERE workspace_id = ? AND status = 'pending'
		 ORDER BY created_at ASC LIMIT 1`,
		workspaceID,
	).Scan(&jobID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // no pending jobs
		}
		return nil, fmt.Errorf("claim query: %w", err)
	}

	// Atomically claim it.
	result, err := tx.Exec(
		`UPDATE jobs SET status = 'running', claimed_by = ? WHERE id = ? AND status = 'pending'`,
		daemonID, jobID,
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
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Fetch the claimed job.
	row := s.DB.QueryRow(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE id = ?`, jobID,
	)
	j, err := scanJob(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("fetch claimed job: %w", err)
	}
	return j, nil
}

func resolveAgentRuntimeID(ctx context.Context, q taskRowQueryer, workspaceID, agentID string) (string, error) {
	var runtimeID sql.NullString
	err := q.QueryRowContext(ctx,
		`SELECT runtime_id FROM agents WHERE id = ? AND workspace_id = ?`,
		agentID, workspaceID,
	).Scan(&runtimeID)
	if err != nil {
		return "", err
	}
	if runtimeID.Valid {
		return runtimeID.String, nil
	}
	return "", nil
}

// CompleteJob marks a job as completed.
func (s *JobService) CompleteJob(workspaceID, jobID string, progress int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.Exec(
		`UPDATE jobs SET status = 'completed', progress = ?, completed_at = ? WHERE id = ? AND workspace_id = ?`,
		progress, now, jobID, workspaceID,
	)
	return rowsAffectedOrNotFound(result, err)
}

// FailJob marks a job as failed with an error message.
func (s *JobService) FailJob(workspaceID, jobID string, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.Exec(
		`UPDATE jobs SET status = 'failed', error = ?, completed_at = ? WHERE id = ? AND workspace_id = ?`,
		errMsg, now, jobID, workspaceID,
	)
	return rowsAffectedOrNotFound(result, err)
}

// UpdateJobProgress updates the progress of a running job.
func (s *JobService) UpdateJobProgress(workspaceID, jobID string, progress int) error {
	result, err := s.DB.Exec(
		`UPDATE jobs SET progress = ? WHERE id = ? AND workspace_id = ?`,
		progress, jobID, workspaceID,
	)
	return rowsAffectedOrNotFound(result, err)
}

// GetJob returns a job by ID.
func (s *JobService) GetJob(jobID string) (*protocol.Job, error) {
	row := s.DB.QueryRow(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE id = ?`, jobID,
	)
	j, err := scanJob(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	return j, nil
}

// GetWorkspaceJob returns a job by ID scoped to a workspace.
func (s *JobService) GetWorkspaceJob(workspaceID, jobID string) (*protocol.Job, error) {
	row := s.DB.QueryRow(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE id = ? AND workspace_id = ?`, jobID, workspaceID,
	)
	j, err := scanJob(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("get workspace job: %w", err)
	}
	return j, nil
}

// ListJobs lists jobs scoped to a workspace.
func (s *JobService) ListJobs(workspaceID string) ([]protocol.Job, error) {
	rows, err := s.DB.Query(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE workspace_id = ? ORDER BY created_at DESC`, workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]protocol.Job, 0)
	for rows.Next() {
		j, err := scanJob(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, *j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}
	return jobs, nil
}

func scanJob(scan func(dest ...any) error) (*protocol.Job, error) {
	var j protocol.Job
	var completedAt *string
	var sourcePathsRaw string
	err := scan(&j.ID, &j.WorkspaceID, &j.Status, &j.AgentID,
		&j.SourcePath, &sourcePathsRaw, &j.SchemaID, &j.Progress, &j.Error, &j.ClaimedBy, &j.CreatedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	j.CompletedAt = completedAt
	json.Unmarshal([]byte(sourcePathsRaw), &j.SourcePaths)
	if j.SourcePaths == nil {
		j.SourcePaths = []string{}
	}
	return &j, nil
}

func rowsAffectedOrNotFound(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
