package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestWorkspaceStoreGetBySlugAndIDBySlug(t *testing.T) {
	db := newStoreTestDB(t)
	ctx := context.Background()

	store := NewWorkspaceStore(db)

	workspaceID, err := store.GetIDBySlug(ctx, "alpha")
	if err != nil {
		t.Fatalf("get workspace id by slug: %v", err)
	}
	if workspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", workspaceID)
	}

	workspace, err := store.GetBySlug(ctx, "alpha")
	if err != nil {
		t.Fatalf("get workspace by slug: %v", err)
	}
	if workspace.ID != "ws1" || workspace.Slug != "alpha" || workspace.Name != "Alpha" {
		t.Fatalf("unexpected workspace: %#v", workspace)
	}
	if workspace.ActiveSchemaPath == nil {
		t.Fatal("expected active_schema_path pointer to be set from schema default")
	}
	if *workspace.ActiveSchemaPath != "" {
		t.Fatalf("expected empty active_schema_path, got %q", *workspace.ActiveSchemaPath)
	}
}

func TestWorkspaceStoreListForUserAndMemberRole(t *testing.T) {
	db := newStoreTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(
		`INSERT INTO users (id, email, password_hash) VALUES ('user2', 'user2@example.test', 'hash')`,
	); err != nil {
		t.Fatalf("seed user2: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO workspaces (id, slug, name, description) VALUES ('ws2', 'beta', 'Beta', 'workspace two')`,
	); err != nil {
		t.Fatalf("seed workspace beta: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws2', 'user2', 'member')`,
	); err != nil {
		t.Fatalf("seed workspace member: %v", err)
	}

	store := NewWorkspaceStore(db)
	workspaces, err := store.ListForUser(ctx, "user1")
	if err != nil {
		t.Fatalf("list workspaces for user: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID != "ws1" {
		t.Fatalf("expected only ws1 for user1, got %#v", workspaces)
	}

	role, err := store.GetMemberRole(ctx, "ws1", "user1")
	if err != nil {
		t.Fatalf("get member role: %v", err)
	}
	if role != "owner" {
		t.Fatalf("expected owner role, got %q", role)
	}

	_, err = store.GetMemberRole(ctx, "ws1", "user2")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for non-member, got %v", err)
	}
}
