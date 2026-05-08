package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newStoreTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schemaSQL, err := os.ReadFile(findStoreSchemaSQL())
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema.sql: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO users (id, email, password_hash) VALUES ('user1', 'user1@example.test', 'hash')`,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO workspaces (id, slug, name, description) VALUES ('ws1', 'alpha', 'Alpha', 'workspace one')`,
	); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws1', 'user1', 'owner')`,
	); err != nil {
		t.Fatalf("seed workspace member: %v", err)
	}

	return db
}

func findStoreSchemaSQL() string {
	candidates := []string{
		"../../pkg/db/schema.sql",
		filepath.Join("server", "pkg", "db", "schema.sql"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "../../pkg/db/schema.sql"
}

func TestAgentStoreListByWorkspaceLoadsJSONAndSkills(t *testing.T) {
	db := newStoreTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(
		`INSERT INTO agent_runtimes (id, workspace_id, name, backend, path)
		 VALUES ('rt1', 'ws1', 'Codex', 'codex', '/usr/local/bin/codex')`,
	); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO agents (
			id, workspace_id, runtime_id, name, description, instructions,
			runtime_mode, runtime_config, custom_env, custom_args, mcp_config,
			visibility, status, model, owner_id
		) VALUES (
			'a1', 'ws1', 'rt1', 'Agent One', 'desc', 'inst',
			'codex', '{"effort":"high"}', '{"OPENAI_API_KEY":"secret"}', '["--json"]', '{"servers":[]}',
			'private', 'offline', 'gpt-5', 'user1'
		)`,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO agent_skills (id, workspace_id, name, description)
		 VALUES ('sk1', 'ws1', 'Parse', 'parse docs')`,
	); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO agent_skills_agents (agent_id, skill_id) VALUES ('a1', 'sk1')`,
	); err != nil {
		t.Fatalf("seed agent skill: %v", err)
	}

	agents, err := NewAgentStore(db).ListByWorkspace(ctx, "ws1")
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	agent := agents[0]
	if agent.ID != "a1" || agent.RuntimeID != "rt1" || agent.RuntimeMode != "codex" {
		t.Fatalf("unexpected agent identity/runtime: %#v", agent)
	}
	if got := agent.CustomEnv["OPENAI_API_KEY"]; got != "secret" {
		t.Fatalf("expected custom env to decode, got %q", got)
	}
	if len(agent.CustomArgs) != 1 || agent.CustomArgs[0] != "--json" {
		t.Fatalf("expected custom args to decode, got %#v", agent.CustomArgs)
	}
	if got := strings.TrimSpace(string(agent.RuntimeConfig)); got != `{"effort":"high"}` {
		t.Fatalf("expected runtime config raw JSON, got %s", got)
	}
	if got := strings.TrimSpace(string(agent.McpConfig)); got != `{"servers":[]}` {
		t.Fatalf("expected mcp config raw JSON, got %s", got)
	}
	if len(agent.Skills) != 1 || agent.Skills[0].ID != "sk1" {
		t.Fatalf("expected skill sk1, got %#v", agent.Skills)
	}
}

func TestAgentStoreGetInWorkspaceRejectsCorruptJSON(t *testing.T) {
	db := newStoreTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(
		`INSERT INTO agents (id, workspace_id, name, custom_env)
		 VALUES ('a1', 'ws1', 'Broken Agent', '{not-json')`,
	); err != nil {
		t.Fatalf("seed corrupt agent: %v", err)
	}

	_, err := NewAgentStore(db).GetInWorkspace(ctx, "ws1", "a1")
	if err == nil {
		t.Fatal("expected corrupt custom_env JSON to return an error")
	}
}
