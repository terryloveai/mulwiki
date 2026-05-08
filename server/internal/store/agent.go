package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

const AgentSelectColumns = `id, workspace_id, COALESCE(runtime_id,''), name, description, instructions,
	runtime_mode, runtime_config, custom_env, custom_args, mcp_config,
	visibility, status, model, max_concurrent_tasks,
	COALESCE(owner_id,''), created_at, updated_at, archived_at, archived_by`

type AgentStore struct {
	DB *sql.DB
}

func NewAgentStore(db *sql.DB) *AgentStore {
	return &AgentStore{DB: db}
}

func (s *AgentStore) ListByWorkspace(ctx context.Context, workspaceID string) ([]protocol.Agent, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+AgentSelectColumns+`
		 FROM agents
		 WHERE workspace_id = ?
		 ORDER BY created_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make([]protocol.Agent, 0)
	agentIDs := make([]string, 0)
	for rows.Next() {
		agent, err := ScanAgent(rows.Scan)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *agent)
		agentIDs = append(agentIDs, agent.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	skillsByAgent, err := s.LoadSkillsForAgents(ctx, agentIDs)
	if err != nil {
		return nil, err
	}
	for i := range agents {
		if skills, ok := skillsByAgent[agents[i].ID]; ok {
			agents[i].Skills = skills
		}
	}
	return agents, nil
}

func (s *AgentStore) GetInWorkspace(ctx context.Context, workspaceID, agentID string) (*protocol.Agent, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+AgentSelectColumns+`
		 FROM agents
		 WHERE workspace_id = ? AND id = ?`,
		workspaceID, agentID,
	)
	agent, err := ScanAgent(row.Scan)
	if err != nil {
		return nil, err
	}

	skillsByAgent, err := s.LoadSkillsForAgents(ctx, []string{agent.ID})
	if err != nil {
		return nil, err
	}
	if skills, ok := skillsByAgent[agent.ID]; ok {
		agent.Skills = skills
	}
	return agent, nil
}

func (s *AgentStore) LoadSkillsForAgents(ctx context.Context, agentIDs []string) (map[string][]protocol.AgentSkill, error) {
	skillsByAgent := make(map[string][]protocol.AgentSkill, len(agentIDs))
	if len(agentIDs) == 0 {
		return skillsByAgent, nil
	}

	placeholders := make([]string, len(agentIDs))
	args := make([]any, len(agentIDs))
	for i, id := range agentIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.DB.QueryContext(ctx,
		`SELECT asa.agent_id, s.id, s.workspace_id, s.name, s.description, s.created_at
		 FROM agent_skills_agents asa
		 JOIN agent_skills s ON s.id = asa.skill_id
		 WHERE asa.agent_id IN (`+strings.Join(placeholders, ",")+`)
		 ORDER BY s.created_at DESC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var agentID string
		var skill protocol.AgentSkill
		if err := rows.Scan(&agentID, &skill.ID, &skill.WorkspaceID, &skill.Name, &skill.Description, &skill.CreatedAt); err != nil {
			return nil, err
		}
		skillsByAgent[agentID] = append(skillsByAgent[agentID], skill)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return skillsByAgent, nil
}

func ScanAgent(scan func(dest ...any) error) (*protocol.Agent, error) {
	var agent protocol.Agent
	var runtimeConfig, customEnv, customArgs, mcpConfig string
	var archivedAt, archivedBy sql.NullString
	err := scan(
		&agent.ID,
		&agent.WorkspaceID,
		&agent.RuntimeID,
		&agent.Name,
		&agent.Description,
		&agent.Instructions,
		&agent.RuntimeMode,
		&runtimeConfig,
		&customEnv,
		&customArgs,
		&mcpConfig,
		&agent.Visibility,
		&agent.Status,
		&agent.Model,
		&agent.MaxConcurrentTasks,
		&agent.OwnerID,
		&agent.CreatedAt,
		&agent.UpdatedAt,
		&archivedAt,
		&archivedBy,
	)
	if err != nil {
		return nil, err
	}

	var errJSON error
	if agent.RuntimeConfig, errJSON = parseRawJSON("runtime_config", runtimeConfig, "{}"); errJSON != nil {
		return nil, errJSON
	}
	if agent.McpConfig, errJSON = parseRawJSON("mcp_config", mcpConfig, "{}"); errJSON != nil {
		return nil, errJSON
	}

	if strings.TrimSpace(customEnv) == "" {
		customEnv = "{}"
	}
	if err := json.Unmarshal([]byte(customEnv), &agent.CustomEnv); err != nil {
		return nil, fmt.Errorf("decode custom_env: %w", err)
	}
	if agent.CustomEnv == nil {
		agent.CustomEnv = make(map[string]string)
	}

	if strings.TrimSpace(customArgs) == "" {
		customArgs = "[]"
	}
	if err := json.Unmarshal([]byte(customArgs), &agent.CustomArgs); err != nil {
		return nil, fmt.Errorf("decode custom_args: %w", err)
	}
	if agent.CustomArgs == nil {
		agent.CustomArgs = make([]string, 0)
	}
	agent.Skills = make([]protocol.AgentSkill, 0)

	if archivedAt.Valid {
		agent.ArchivedAt = &archivedAt.String
	}
	if archivedBy.Valid {
		agent.ArchivedBy = &archivedBy.String
	}

	return &agent, nil
}

func parseRawJSON(field, raw, fallback string) (json.RawMessage, error) {
	if strings.TrimSpace(raw) == "" {
		raw = fallback
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	return json.RawMessage(raw), nil
}
