package store

import (
	"context"
	"database/sql"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

type WorkspaceStore struct {
	DB *sql.DB
}

func NewWorkspaceStore(db *sql.DB) *WorkspaceStore {
	return &WorkspaceStore{DB: db}
}

func (s *WorkspaceStore) GetIDBySlug(ctx context.Context, slug string) (string, error) {
	var workspaceID string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID)
	return workspaceID, err
}

func (s *WorkspaceStore) GetBySlug(ctx context.Context, slug string) (*protocol.Workspace, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT id, slug, name, description, active_schema_id, active_schema_path, created_at
		 FROM workspaces
		 WHERE slug = ?`,
		slug,
	)
	return scanWorkspace(row.Scan)
}

func (s *WorkspaceStore) ListForUser(ctx context.Context, userID string) ([]protocol.Workspace, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT w.id, w.slug, w.name, w.description, w.active_schema_id, w.active_schema_path, w.created_at
		 FROM workspaces w
		 JOIN workspace_members wm ON wm.workspace_id = w.id
		 WHERE wm.user_id = ? AND w.slug <> 'builtin'
		 ORDER BY w.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	workspaces := make([]protocol.Workspace, 0)
	for rows.Next() {
		workspace, err := scanWorkspace(rows.Scan)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, *workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return workspaces, nil
}

func (s *WorkspaceStore) GetMemberRole(ctx context.Context, workspaceID, userID string) (string, error) {
	var role string
	err := s.DB.QueryRowContext(ctx,
		`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID,
	).Scan(&role)
	return role, err
}

func scanWorkspace(scan func(dest ...any) error) (*protocol.Workspace, error) {
	var workspace protocol.Workspace
	var activeSchemaID, activeSchemaPath sql.NullString
	if err := scan(
		&workspace.ID,
		&workspace.Slug,
		&workspace.Name,
		&workspace.Description,
		&activeSchemaID,
		&activeSchemaPath,
		&workspace.CreatedAt,
	); err != nil {
		return nil, err
	}
	if activeSchemaID.Valid {
		workspace.ActiveSchemaID = &activeSchemaID.String
	}
	if activeSchemaPath.Valid {
		workspace.ActiveSchemaPath = &activeSchemaPath.String
	}
	return &workspace, nil
}
