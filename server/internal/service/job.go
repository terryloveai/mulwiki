package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// JobService handles job lifecycle operations.
type JobService struct {
	DB *sql.DB
}

// NewJobService creates a new JobService.
func NewJobService(db *sql.DB) *JobService {
	return &JobService{DB: db}
}

// CreateJob inserts a new pending job and returns it.
func (s *JobService) CreateJob(workspaceID, sourceID, schemaID string) (*protocol.Job, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	var j protocol.Job
	var completedAt *string
	var sourceIDsRaw string
	err := s.DB.QueryRow(
		`INSERT INTO jobs (workspace_id, status, source_path, source_paths, schema_id, created_at)
		 VALUES (?, 'pending', ?, '[]', ?, ?)
		 RETURNING id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		           claimed_by, created_at, completed_at`,
		workspaceID, sourceID, schemaID, now,
	).Scan(&j.ID, &j.WorkspaceID, &j.Status, &j.AgentID,
		&j.SourcePath, &sourceIDsRaw, &j.SchemaID, &j.Progress, &j.Error, &j.ClaimedBy, &j.CreatedAt, &completedAt)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	j.CompletedAt = completedAt
	json.Unmarshal([]byte(sourceIDsRaw), &j.SourcePaths)
	if j.SourcePaths == nil {
		j.SourcePaths = []string{}
	}
	return &j, nil
}

// ClaimJob atomically claims the next pending job for the given workspace.
func (s *JobService) ClaimJob(workspaceID, daemonID string) (*protocol.Job, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Pick the oldest pending job in this workspace
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

	// Atomically claim it
	now := time.Now().Format(time.RFC3339)
	_, err = tx.Exec(
		`UPDATE jobs SET status = 'running', claimed_by = ? WHERE id = ? AND status = 'pending'`,
		daemonID, jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("claim update: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Fetch the claimed job
	var j protocol.Job
	var completedAt *string
	var sourceIDsRaw string
	err = s.DB.QueryRow(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE id = ?`, jobID,
	).Scan(&j.ID, &j.WorkspaceID, &j.Status, &j.AgentID,
		&j.SourcePath, &sourceIDsRaw, &j.SchemaID, &j.Progress, &j.Error, &j.ClaimedBy, &now, &completedAt)
	if err != nil {
		return nil, fmt.Errorf("fetch claimed job: %w", err)
	}
	j.CreatedAt = now
	j.CompletedAt = completedAt
	json.Unmarshal([]byte(sourceIDsRaw), &j.SourcePaths)
	if j.SourcePaths == nil {
		j.SourcePaths = []string{}
	}
	return &j, nil
}

// CompleteJob marks a job as completed.
func (s *JobService) CompleteJob(jobID string, progress int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(
		`UPDATE jobs SET status = 'completed', progress = ?, completed_at = ? WHERE id = ?`,
		progress, now, jobID,
	)
	return err
}

// FailJob marks a job as failed with an error message.
func (s *JobService) FailJob(jobID string, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(
		`UPDATE jobs SET status = 'failed', error = ?, completed_at = ? WHERE id = ?`,
		errMsg, now, jobID,
	)
	return err
}

// UpdateJobProgress updates the progress of a running job.
func (s *JobService) UpdateJobProgress(jobID string, progress int) error {
	_, err := s.DB.Exec(
		`UPDATE jobs SET progress = ? WHERE id = ?`,
		progress, jobID,
	)
	return err
}

// GetJob returns a job by ID.
func (s *JobService) GetJob(jobID string) (*protocol.Job, error) {
	var j protocol.Job
	var completedAt *string
	var sourceIDsRaw string
	err := s.DB.QueryRow(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE id = ?`, jobID,
	).Scan(&j.ID, &j.WorkspaceID, &j.Status, &j.AgentID,
		&j.SourcePath, &sourceIDsRaw, &j.SchemaID, &j.Progress, &j.Error, &j.ClaimedBy, &j.CreatedAt, &completedAt)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	j.CompletedAt = completedAt
	json.Unmarshal([]byte(sourceIDsRaw), &j.SourcePaths)
	if j.SourcePaths == nil {
		j.SourcePaths = []string{}
	}
	return &j, nil
}
