package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

const JobSelectColumns = `id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
	claimed_by, created_at, completed_at`

type JobStore struct {
	DB *sql.DB
}

func NewJobStore(db *sql.DB) *JobStore {
	return &JobStore{DB: db}
}

func (s *JobStore) ListByWorkspace(ctx context.Context, workspaceID string) ([]protocol.Job, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+JobSelectColumns+`
		 FROM jobs
		 WHERE workspace_id = ?
		 ORDER BY created_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]protocol.Job, 0)
	for rows.Next() {
		job, err := ScanJob(rows.Scan)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *JobStore) GetInWorkspace(ctx context.Context, workspaceID, jobID string) (*protocol.Job, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+JobSelectColumns+`
		 FROM jobs
		 WHERE id = ? AND workspace_id = ?`,
		jobID, workspaceID,
	)
	return ScanJob(row.Scan)
}

func ScanJob(scan func(dest ...any) error) (*protocol.Job, error) {
	var job protocol.Job
	var sourcePathsRaw string
	var completedAt sql.NullString
	if err := scan(
		&job.ID,
		&job.WorkspaceID,
		&job.Status,
		&job.AgentID,
		&job.SourcePath,
		&sourcePathsRaw,
		&job.SchemaID,
		&job.Progress,
		&job.Error,
		&job.ClaimedBy,
		&job.CreatedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.String
	}
	if sourcePathsRaw == "" {
		sourcePathsRaw = "[]"
	}
	if err := json.Unmarshal([]byte(sourcePathsRaw), &job.SourcePaths); err != nil {
		return nil, fmt.Errorf("decode source_paths: %w", err)
	}
	if job.SourcePaths == nil {
		job.SourcePaths = []string{}
	}
	return &job, nil
}
