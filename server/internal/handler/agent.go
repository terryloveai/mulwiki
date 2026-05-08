package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tethy/mulwiki/server/internal/service"
	"github.com/tethy/mulwiki/server/internal/store"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// GET /api/workspaces/{slug}/agents
func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	currentUser := userID(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	rows, err := h.DB.Query(
		`SELECT id, workspace_id, COALESCE(runtime_id,''), name, description, instructions,
		        runtime_mode, runtime_config, custom_env, custom_args, mcp_config,
		        visibility, status, model, max_concurrent_tasks,
		        COALESCE(owner_id,''), created_at, updated_at, archived_at, archived_by
		 FROM agents WHERE workspace_id = ? ORDER BY created_at DESC`, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	defer rows.Close()

	agents := make([]protocol.Agent, 0)
	agentIDs := make([]string, 0)
	for rows.Next() {
		var a protocol.Agent
		var runtimeConfig, customEnv, customArgs, mcpConfig string
		var archivedAt, archivedBy sql.NullString
		if err := rows.Scan(
			&a.ID, &a.WorkspaceID, &a.RuntimeID, &a.Name, &a.Description, &a.Instructions,
			&a.RuntimeMode, &runtimeConfig, &customEnv, &customArgs, &mcpConfig,
			&a.Visibility, &a.Status, &a.Model, &a.MaxConcurrentTasks,
			&a.OwnerID, &a.CreatedAt, &a.UpdatedAt, &archivedAt, &archivedBy,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan agent")
			return
		}
		a.RuntimeConfig = json.RawMessage(runtimeConfig)
		a.McpConfig = json.RawMessage(mcpConfig)
		json.Unmarshal([]byte(customEnv), &a.CustomEnv)
		json.Unmarshal([]byte(customArgs), &a.CustomArgs)
		if archivedAt.Valid {
			a.ArchivedAt = &archivedAt.String
		}
		if archivedBy.Valid {
			a.ArchivedBy = &archivedBy.String
		}
		if a.CustomEnv == nil {
			a.CustomEnv = make(map[string]string)
		}
		if a.CustomArgs == nil {
			a.CustomArgs = make([]string, 0)
		}
		a.Skills = make([]protocol.AgentSkill, 0)

		// Redact custom_env for non-owners
		if a.OwnerID != "" && a.OwnerID != currentUser && !isDaemonRequest(r) {
			a.CustomEnv = nil
		}

		agents = append(agents, a)
		agentIDs = append(agentIDs, a.ID)
	}

	// Batch-load skills for all agents
	if len(agentIDs) > 0 {
		skillMap := make(map[string][]protocol.AgentSkill)
		placeholders := make([]string, len(agentIDs))
		args := make([]any, len(agentIDs))
		for i, id := range agentIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		query := `SELECT asa.agent_id, s.id, s.name, s.description
			 FROM agent_skills_agents asa
			 JOIN agent_skills s ON s.id = asa.skill_id
			 WHERE asa.agent_id IN (` + strings.Join(placeholders, ",") + `)`
		skillRows, err := h.DB.Query(query, args...)
		if err == nil {
			defer skillRows.Close()
			for skillRows.Next() {
				var agentID string
				var sk protocol.AgentSkill
				if err := skillRows.Scan(&agentID, &sk.ID, &sk.Name, &sk.Description); err == nil {
					skillMap[agentID] = append(skillMap[agentID], sk)
				}
			}
		}
		for i := range agents {
			if skills, ok := skillMap[agents[i].ID]; ok {
				agents[i].Skills = skills
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// POST /api/workspaces/{slug}/agents
func (h *Handler) CreateAgent(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	currentUser := userID(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req protocol.AgentCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Validate runtime_id if provided
	runtimeMode := ""
	if req.RuntimeID != "" {
		var backend string
		if err := h.DB.QueryRow(
			`SELECT backend FROM agent_runtimes WHERE id = ? AND workspace_id = ?`,
			req.RuntimeID, workspaceID,
		).Scan(&backend); err != nil {
			writeError(w, http.StatusBadRequest, "runtime not found")
			return
		}
		runtimeMode = backend
	}

	// Defaults
	visibility := req.Visibility
	if visibility == "" {
		visibility = "private"
	}
	maxConcurrent := req.MaxConcurrentTasks
	if maxConcurrent <= 0 {
		maxConcurrent = 6
	}

	// Marshal JSON fields
	runtimeConfig := string(req.RuntimeConfig)
	if runtimeConfig == "" || runtimeConfig == "null" {
		runtimeConfig = "{}"
	}
	customEnv, _ := json.Marshal(req.CustomEnv)
	if string(customEnv) == "null" {
		customEnv = []byte("{}")
	}
	customArgs, _ := json.Marshal(req.CustomArgs)
	if string(customArgs) == "null" {
		customArgs = []byte("[]")
	}
	mcpConfig := string(req.McpConfig)
	if mcpConfig == "" || mcpConfig == "null" {
		mcpConfig = "{}"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	ownerID := currentUser

	var a protocol.Agent
	var dbRuntimeConfig, dbCustomEnv, dbCustomArgs, dbMcpConfig string
	var archivedAt, archivedBy sql.NullString
	err := h.DB.QueryRow(
		`INSERT INTO agents (workspace_id, runtime_id, name, description, instructions,
		                     runtime_mode, runtime_config, custom_env, custom_args, mcp_config,
		                     visibility, status, model, max_concurrent_tasks, owner_id,
		                     created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'offline', ?, ?, ?, ?, ?)
		 RETURNING id, workspace_id, COALESCE(runtime_id,''), name, description, instructions,
		           runtime_mode, runtime_config, custom_env, custom_args, mcp_config,
		           visibility, status, model, max_concurrent_tasks,
		           COALESCE(owner_id,''), created_at, updated_at, archived_at, archived_by`,
		workspaceID, nullStr(req.RuntimeID), req.Name, req.Description, req.Instructions,
		runtimeMode, runtimeConfig, string(customEnv), string(customArgs), mcpConfig,
		visibility, req.Model, maxConcurrent, nullStr(ownerID),
		now, now,
	).Scan(
		&a.ID, &a.WorkspaceID, &a.RuntimeID, &a.Name, &a.Description, &a.Instructions,
		&a.RuntimeMode, &dbRuntimeConfig, &dbCustomEnv, &dbCustomArgs, &dbMcpConfig,
		&a.Visibility, &a.Status, &a.Model, &a.MaxConcurrentTasks,
		&a.OwnerID, &a.CreatedAt, &a.UpdatedAt, &archivedAt, &archivedBy,
	)
	if err != nil {
		slog.Error("CreateAgent insert failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	a.RuntimeConfig = json.RawMessage(dbRuntimeConfig)
	a.McpConfig = json.RawMessage(dbMcpConfig)
	json.Unmarshal([]byte(dbCustomEnv), &a.CustomEnv)
	json.Unmarshal([]byte(dbCustomArgs), &a.CustomArgs)
	if a.CustomEnv == nil {
		a.CustomEnv = make(map[string]string)
	}
	if a.CustomArgs == nil {
		a.CustomArgs = make([]string, 0)
	}
	a.Skills = make([]protocol.AgentSkill, 0)
	if archivedAt.Valid {
		a.ArchivedAt = &archivedAt.String
	}
	if archivedBy.Valid {
		a.ArchivedBy = &archivedBy.String
	}

	writeJSON(w, http.StatusCreated, map[string]any{"agent": a})
}

// GET /api/workspaces/{slug}/agents/{id}
func (h *Handler) GetAgent(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")
	currentUser := userID(r)

	var a protocol.Agent
	var runtimeConfig, customEnv, customArgs, mcpConfig string
	var archivedAt, archivedBy sql.NullString
	err := h.DB.QueryRow(
		`SELECT a.id, a.workspace_id, COALESCE(a.runtime_id,''), a.name, a.description, a.instructions,
		        a.runtime_mode, a.runtime_config, a.custom_env, a.custom_args, a.mcp_config,
		        a.visibility, a.status, a.model, a.max_concurrent_tasks,
		        COALESCE(a.owner_id,''), a.created_at, a.updated_at, a.archived_at, a.archived_by
		 FROM agents a
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ?`, slug, id,
	).Scan(
		&a.ID, &a.WorkspaceID, &a.RuntimeID, &a.Name, &a.Description, &a.Instructions,
		&a.RuntimeMode, &runtimeConfig, &customEnv, &customArgs, &mcpConfig,
		&a.Visibility, &a.Status, &a.Model, &a.MaxConcurrentTasks,
		&a.OwnerID, &a.CreatedAt, &a.UpdatedAt, &archivedAt, &archivedBy,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	a.RuntimeConfig = json.RawMessage(runtimeConfig)
	a.McpConfig = json.RawMessage(mcpConfig)
	json.Unmarshal([]byte(customEnv), &a.CustomEnv)
	json.Unmarshal([]byte(customArgs), &a.CustomArgs)
	if a.CustomEnv == nil {
		a.CustomEnv = make(map[string]string)
	}
	if a.CustomArgs == nil {
		a.CustomArgs = make([]string, 0)
	}
	if archivedAt.Valid {
		a.ArchivedAt = &archivedAt.String
	}
	if archivedBy.Valid {
		a.ArchivedBy = &archivedBy.String
	}

	// Redact custom_env for non-owners
	if a.OwnerID != "" && a.OwnerID != currentUser && !isDaemonRequest(r) {
		a.CustomEnv = nil
	}

	// Load skills
	a.Skills = make([]protocol.AgentSkill, 0)
	skillRows, err := h.DB.Query(
		`SELECT s.id, s.name, s.description
		 FROM agent_skills_agents asa
		 JOIN agent_skills s ON s.id = asa.skill_id
		 WHERE asa.agent_id = ?`, a.ID,
	)
	if err == nil {
		defer skillRows.Close()
		for skillRows.Next() {
			var sk protocol.AgentSkill
			if err := skillRows.Scan(&sk.ID, &sk.Name, &sk.Description); err == nil {
				a.Skills = append(a.Skills, sk)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"agent": a})
}

// PATCH /api/workspaces/{slug}/agents/{id}
func (h *Handler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")
	currentUser := userID(r)

	// Verify agent exists and get workspace
	var workspaceID, ownerID string
	if err := h.DB.QueryRow(
		`SELECT a.workspace_id, COALESCE(a.owner_id,'') FROM agents a
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ?`, slug, id,
	).Scan(&workspaceID, &ownerID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var req protocol.AgentUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Build dynamic UPDATE
	now := time.Now().UTC().Format(time.RFC3339)
	var setClauses []string
	var args []any

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Instructions != nil {
		setClauses = append(setClauses, "instructions = ?")
		args = append(args, *req.Instructions)
	}
	if req.RuntimeID != nil {
		// Validate runtime if non-empty
		runtimeMode := ""
		if *req.RuntimeID != "" {
			var backend string
			if err := h.DB.QueryRow(
				`SELECT backend FROM agent_runtimes WHERE id = ? AND workspace_id = ?`,
				*req.RuntimeID, workspaceID,
			).Scan(&backend); err != nil {
				writeError(w, http.StatusBadRequest, "runtime not found")
				return
			}
			runtimeMode = backend
		}
		setClauses = append(setClauses, "runtime_id = ?")
		args = append(args, nullStr(*req.RuntimeID))
		if runtimeMode != "" {
			setClauses = append(setClauses, "runtime_mode = ?")
			args = append(args, runtimeMode)
		}
	}
	if len(req.RuntimeConfig) > 0 {
		setClauses = append(setClauses, "runtime_config = ?")
		args = append(args, string(req.RuntimeConfig))
	}
	if req.CustomEnv != nil {
		envJSON, _ := json.Marshal(*req.CustomEnv)
		setClauses = append(setClauses, "custom_env = ?")
		args = append(args, string(envJSON))
	}
	if req.CustomArgs != nil {
		argsJSON, _ := json.Marshal(*req.CustomArgs)
		setClauses = append(setClauses, "custom_args = ?")
		args = append(args, string(argsJSON))
	}
	if len(req.McpConfig) > 0 {
		setClauses = append(setClauses, "mcp_config = ?")
		args = append(args, string(req.McpConfig))
	}
	if req.Visibility != nil {
		setClauses = append(setClauses, "visibility = ?")
		args = append(args, *req.Visibility)
	}
	if req.MaxConcurrentTasks != nil {
		setClauses = append(setClauses, "max_concurrent_tasks = ?")
		args = append(args, *req.MaxConcurrentTasks)
	}
	if req.Model != nil {
		setClauses = append(setClauses, "model = ?")
		args = append(args, *req.Model)
	}

	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, now)
	args = append(args, id)

	query := `UPDATE agents SET ` + strings.Join(setClauses, ", ") + ` WHERE id = ?`

	if _, err := h.DB.Exec(query, args...); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	// Return updated agent
	var a protocol.Agent
	var runtimeConfig, customEnv, customArgs, mcpConfig string
	var archivedAt, archivedBy sql.NullString
	err := h.DB.QueryRow(
		`SELECT a.id, a.workspace_id, COALESCE(a.runtime_id,''), a.name, a.description, a.instructions,
		        a.runtime_mode, a.runtime_config, a.custom_env, a.custom_args, a.mcp_config,
		        a.visibility, a.status, a.model, a.max_concurrent_tasks,
		        COALESCE(a.owner_id,''), a.created_at, a.updated_at, a.archived_at, a.archived_by
		 FROM agents a WHERE a.id = ?`, id,
	).Scan(
		&a.ID, &a.WorkspaceID, &a.RuntimeID, &a.Name, &a.Description, &a.Instructions,
		&a.RuntimeMode, &runtimeConfig, &customEnv, &customArgs, &mcpConfig,
		&a.Visibility, &a.Status, &a.Model, &a.MaxConcurrentTasks,
		&a.OwnerID, &a.CreatedAt, &a.UpdatedAt, &archivedAt, &archivedBy,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch updated agent")
		return
	}

	a.RuntimeConfig = json.RawMessage(runtimeConfig)
	a.McpConfig = json.RawMessage(mcpConfig)
	json.Unmarshal([]byte(customEnv), &a.CustomEnv)
	json.Unmarshal([]byte(customArgs), &a.CustomArgs)
	if a.CustomEnv == nil {
		a.CustomEnv = make(map[string]string)
	}
	if a.CustomArgs == nil {
		a.CustomArgs = make([]string, 0)
	}
	if archivedAt.Valid {
		a.ArchivedAt = &archivedAt.String
	}
	if archivedBy.Valid {
		a.ArchivedBy = &archivedBy.String
	}

	// Redact custom_env for non-owners
	if a.OwnerID != "" && a.OwnerID != currentUser {
		a.CustomEnv = nil
	}

	// Load skills
	a.Skills = make([]protocol.AgentSkill, 0)
	skillRows, err := h.DB.Query(
		`SELECT s.id, s.name, s.description
		 FROM agent_skills_agents asa
		 JOIN agent_skills s ON s.id = asa.skill_id
		 WHERE asa.agent_id = ?`, a.ID,
	)
	if err == nil {
		defer skillRows.Close()
		for skillRows.Next() {
			var sk protocol.AgentSkill
			if err := skillRows.Scan(&sk.ID, &sk.Name, &sk.Description); err == nil {
				a.Skills = append(a.Skills, sk)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"agent": a})

	// Check if there are tasks
}

// POST /api/workspaces/{slug}/agents/{id}/archive
func (h *Handler) ArchiveAgent(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")
	currentUser := userID(r)

	now := time.Now().UTC().Format(time.RFC3339)

	res, err := h.DB.Exec(
		`UPDATE agents SET archived_at = ?, archived_by = ?, updated_at = ?
		 WHERE id = ? AND workspace_id = (SELECT id FROM workspaces WHERE slug = ?)`,
		now, currentUser, now, id, slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to archive agent")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// POST /api/workspaces/{slug}/agents/{id}/restore
func (h *Handler) RestoreAgent(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	now := time.Now().UTC().Format(time.RFC3339)

	res, err := h.DB.Exec(
		`UPDATE agents SET archived_at = NULL, archived_by = NULL, updated_at = ?
		 WHERE id = ? AND workspace_id = (SELECT id FROM workspaces WHERE slug = ?)`,
		now, id, slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to restore agent")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// GET /api/workspaces/{slug}/agents/{id}/tasks
func (h *Handler) ListAgentTasks(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	// Verify agent exists in workspace
	var workspaceID string
	if err := h.DB.QueryRow(
		`SELECT a.workspace_id FROM agents a
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ?`, slug, id,
	).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	rows, err := h.DB.Query(
		`SELECT id, agent_id, runtime_id, workspace_id, source_path, schema_id,
		        status, priority, parent_task_id, session_id, work_dir,
		        failure_reason, daemon_id, dispatched_at, started_at, completed_at,
		        result, error, attempt, max_attempts, created_at
		 FROM agent_tasks WHERE agent_id = ? ORDER BY created_at DESC`, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	defer rows.Close()

	tasks := make([]protocol.AgentTask, 0)
	for rows.Next() {
		var t protocol.AgentTask
		var runtimeID, dispatchedAt, startedAt, completedAt, parentTaskID sql.NullString
		if err := rows.Scan(
			&t.ID, &t.AgentID, &runtimeID, &t.WorkspaceID, &t.SourcePath, &t.SchemaID,
			&t.Status, &t.Priority, &parentTaskID, &t.SessionID, &t.WorkDir,
			&t.FailureReason, &t.DaemonID, &dispatchedAt, &startedAt, &completedAt,
			&t.Result, &t.Error, &t.Attempt, &t.MaxAttempts, &t.CreatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan task")
			return
		}
		if runtimeID.Valid {
			t.RuntimeID = &runtimeID.String
		}
		if dispatchedAt.Valid {
			t.DispatchedAt = &dispatchedAt.String
		}
		if startedAt.Valid {
			t.StartedAt = &startedAt.String
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.String
		}
		if parentTaskID.Valid {
			t.ParentTaskID = &parentTaskID.String
		}
		tasks = append(tasks, t)
	}

	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// GET /api/workspaces/{slug}/agents/{id}/tasks/{taskId}
func (h *Handler) GetAgentTask(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")
	taskID := idParam(r, "taskId")

	var t protocol.AgentTask
	var runtimeID, dispatchedAt, startedAt, completedAt, parentTaskID sql.NullString
	err := h.DB.QueryRow(
		`SELECT t.id, t.agent_id, t.runtime_id, t.workspace_id, t.source_path, t.schema_id,
		        t.status, t.priority, t.parent_task_id, t.session_id, t.work_dir,
		        t.failure_reason, t.daemon_id, t.dispatched_at, t.started_at, t.completed_at,
		        t.result, t.error, t.attempt, t.max_attempts, t.created_at
		 FROM agent_tasks t
		 JOIN agents a ON a.id = t.agent_id
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ? AND t.id = ?`, slug, id, taskID,
	).Scan(
		&t.ID, &t.AgentID, &runtimeID, &t.WorkspaceID, &t.SourcePath, &t.SchemaID,
		&t.Status, &t.Priority, &parentTaskID, &t.SessionID, &t.WorkDir,
		&t.FailureReason, &t.DaemonID, &dispatchedAt, &startedAt, &completedAt,
		&t.Result, &t.Error, &t.Attempt, &t.MaxAttempts, &t.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	if runtimeID.Valid {
		t.RuntimeID = &runtimeID.String
	}
	if dispatchedAt.Valid {
		t.DispatchedAt = &dispatchedAt.String
	}
	if startedAt.Valid {
		t.StartedAt = &startedAt.String
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.String
	}
	if parentTaskID.Valid {
		t.ParentTaskID = &parentTaskID.String
	}
	messages, err := h.loadAgentTaskMessages(t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load task messages")
		return
	}
	t.Messages = messages

	writeJSON(w, http.StatusOK, map[string]any{"task": t})
}

// POST /api/workspaces/{slug}/agents/{id}/tasks — daemon creates a task record
func (h *Handler) CreateAgentTask(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(
		`SELECT a.workspace_id FROM agents a
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ?`, slug, id,
	).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var req struct {
		JobID        string                           `json:"job_id"`
		SourcePath   string                           `json:"source_path"`
		SchemaID     string                           `json:"schema_id"`
		RuntimeID    string                           `json:"runtime_id"`
		Priority     int                              `json:"priority"`
		MaxAttempts  int                              `json:"max_attempts"`
		ParentTaskID string                           `json:"parent_task_id"`
		SessionID    string                           `json:"session_id"`
		WorkDir      string                           `json:"work_dir"`
		DaemonID     string                           `json:"daemon_id"`
		Messages     []protocol.AgentTaskMessageInput `json:"messages"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DaemonID == "" {
		req.DaemonID = daemonID(r)
	}
	if ctxDaemonID := daemonID(r); ctxDaemonID != "" && req.DaemonID != ctxDaemonID {
		writeError(w, http.StatusForbidden, "daemon id mismatch")
		return
	}

	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 3
	}

	taskStore := store.NewTaskStore(h.DB)
	taskService := service.NewTaskService(h.DB, h.EventBus)

	var t *protocol.AgentTask
	if req.JobID != "" {
		existing, err := taskStore.GetByJob(r.Context(), workspaceID, req.JobID, id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to load existing task")
			return
		}
		if err == nil {
			t = existing
		}
	}

	if t == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		row := h.DB.QueryRow(
			`INSERT INTO agent_tasks (job_id, agent_id, runtime_id, workspace_id, source_path, schema_id,
			                          status, priority, parent_task_id, session_id, work_dir,
			                          attempt, max_attempts, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'queued', ?, ?, ?, ?, 1, ?, ?)
			 RETURNING `+store.AgentTaskColumns,
			req.JobID, id, nullStr(req.RuntimeID), workspaceID, req.SourcePath, req.SchemaID,
			req.Priority, nullStr(req.ParentTaskID), req.SessionID, req.WorkDir,
			req.MaxAttempts, now,
		)
		var err error
		t, err = store.ScanAgentTask(row.Scan)
		if err != nil {
			slog.Error("CreateAgentTask insert failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create task")
			return
		}
	}

	if req.DaemonID != "" && t.Status == "queued" {
		var err error
		t, err = taskService.Dispatch(r.Context(), t.ID, workspaceID, req.DaemonID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to dispatch task")
			return
		}
	}

	if err := h.insertAgentTaskMessages(workspaceID, id, t.ID, req.Messages); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	messages, err := h.loadAgentTaskMessages(t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load task messages")
		return
	}
	t.Messages = messages

	writeJSON(w, http.StatusCreated, map[string]any{"task": t})
}

// PATCH /api/workspaces/{slug}/agents/{id}/tasks/{taskId} — daemon updates task status
func (h *Handler) UpdateAgentTask(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")
	taskID := idParam(r, "taskId")

	var workspaceID string
	if err := h.DB.QueryRow(
		`SELECT t.workspace_id FROM agent_tasks t
		 JOIN agents a ON a.id = t.agent_id
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ? AND t.id = ?`, slug, id, taskID,
	).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	var req struct {
		Status        string                           `json:"status"`
		Result        string                           `json:"result"`
		Error         string                           `json:"error"`
		Attempt       int                              `json:"attempt"`
		ParentTaskID  string                           `json:"parent_task_id"`
		SessionID     string                           `json:"session_id"`
		WorkDir       string                           `json:"work_dir"`
		FailureReason string                           `json:"failure_reason"`
		DaemonID      string                           `json:"daemon_id"`
		Messages      []protocol.AgentTaskMessageInput `json:"messages"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DaemonID == "" {
		req.DaemonID = daemonID(r)
	}
	if ctxDaemonID := daemonID(r); ctxDaemonID != "" && req.DaemonID != ctxDaemonID {
		writeError(w, http.StatusForbidden, "daemon id mismatch")
		return
	}

	hasMetadata := req.Result != "" || req.Error != "" || req.Attempt > 0 || req.ParentTaskID != "" ||
		req.SessionID != "" || req.WorkDir != "" || req.FailureReason != "" || req.DaemonID != ""
	if req.Status == "" && !hasMetadata && len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	taskService := service.NewTaskService(h.DB, h.EventBus)
	var t *protocol.AgentTask
	var err error

	switch req.Status {
	case "":
	case "running":
		t, err = taskService.Start(r.Context(), taskID, workspaceID)
	case "completed":
		t, err = taskService.Complete(r.Context(), taskID, workspaceID, req.Result, req.SessionID, req.WorkDir)
	case "failed":
		t, err = taskService.Fail(r.Context(), taskID, workspaceID, req.FailureReason, req.Error, req.SessionID, req.WorkDir)
	case "cancelled":
		t, err = taskService.Cancel(r.Context(), taskID, workspaceID)
	default:
		writeError(w, http.StatusBadRequest, "unsupported task status")
		return
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if errors.Is(err, service.ErrInvalidTaskTransition) {
			writeError(w, http.StatusConflict, "invalid task status transition")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update task")
		return
	}

	if err := h.updateAgentTaskMetadata(taskID, req.Status, req.Result, req.Error, req.Attempt, req.ParentTaskID, req.SessionID, req.WorkDir, req.FailureReason, req.DaemonID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update task metadata")
		return
	}

	t, err = store.NewTaskStore(h.DB).GetInWorkspace(r.Context(), workspaceID, taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found after update")
		return
	}
	if err := h.insertAgentTaskMessages(workspaceID, id, t.ID, req.Messages); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	messages, err := h.loadAgentTaskMessages(t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load task messages")
		return
	}
	t.Messages = messages

	writeJSON(w, http.StatusOK, map[string]any{"task": t})
}

func (h *Handler) updateAgentTaskMetadata(taskID, status, resultText, taskError string, attempt int, parentTaskID, sessionID, workDir, failureReason, daemonID string) error {
	var setClauses []string
	var args []any

	if status == "" {
		if resultText != "" {
			setClauses = append(setClauses, "result = ?")
			args = append(args, resultText)
		}
		if taskError != "" {
			setClauses = append(setClauses, "error = ?")
			args = append(args, taskError)
		}
		if failureReason != "" {
			setClauses = append(setClauses, "failure_reason = ?")
			args = append(args, failureReason)
		}
	}
	if status == "" || status == "running" {
		if sessionID != "" {
			setClauses = append(setClauses, "session_id = ?")
			args = append(args, sessionID)
		}
		if workDir != "" {
			setClauses = append(setClauses, "work_dir = ?")
			args = append(args, workDir)
		}
	}
	if attempt > 0 {
		setClauses = append(setClauses, "attempt = ?")
		args = append(args, attempt)
	}
	if parentTaskID != "" {
		setClauses = append(setClauses, "parent_task_id = ?")
		args = append(args, parentTaskID)
	}
	if daemonID != "" {
		setClauses = append(setClauses, "daemon_id = ?")
		args = append(args, daemonID)
	}
	if len(setClauses) == 0 {
		return nil
	}

	args = append(args, taskID)
	query := `UPDATE agent_tasks SET ` + strings.Join(setClauses, ", ") + ` WHERE id = ?`
	_, err := h.DB.Exec(query, args...)
	return err
}

func (h *Handler) insertAgentTaskMessages(workspaceID, agentID, taskID string, messages []protocol.AgentTaskMessageInput) error {
	if len(messages) == 0 {
		return nil
	}
	taskMessages := make([]protocol.AgentTaskMessage, 0, len(messages))
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "daemon"
		}
		metadata := msg.Metadata
		if len(metadata) == 0 || string(metadata) == "null" {
			metadata = json.RawMessage(`{}`)
		}
		if !json.Valid(metadata) {
			return errInvalidTaskMessageMetadata
		}
		taskMessages = append(taskMessages, protocol.AgentTaskMessage{
			TaskID:      taskID,
			WorkspaceID: workspaceID,
			AgentID:     agentID,
			Role:        role,
			Seq:         msg.Seq,
			Type:        msg.Type,
			Content:     content,
			Tool:        msg.Tool,
			CallID:      msg.CallID,
			Input:       msg.Input,
			Output:      msg.Output,
			Status:      msg.Status,
			Level:       msg.Level,
			SessionID:   msg.SessionID,
			Metadata:    metadata,
		})
	}
	return service.NewTaskService(h.DB, h.EventBus).AppendMessages(context.Background(), workspaceID, taskID, taskMessages)
}

func (h *Handler) loadAgentTaskMessages(taskID string) ([]protocol.AgentTaskMessage, error) {
	rows, err := h.DB.Query(
		`SELECT id, task_id, workspace_id, agent_id, role, seq, type, content,
		        tool, call_id, input, output, status, level, session_id, metadata, created_at
		 FROM agent_task_messages
		 WHERE task_id = ?
		 ORDER BY seq ASC, created_at ASC, id ASC`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]protocol.AgentTaskMessage, 0)
	for rows.Next() {
		var msg protocol.AgentTaskMessage
		var input, metadata string
		if err := rows.Scan(
			&msg.ID, &msg.TaskID, &msg.WorkspaceID, &msg.AgentID, &msg.Role,
			&msg.Seq, &msg.Type, &msg.Content, &msg.Tool, &msg.CallID, &input,
			&msg.Output, &msg.Status, &msg.Level, &msg.SessionID, &metadata, &msg.CreatedAt,
		); err != nil {
			return nil, err
		}
		msg.Input = json.RawMessage(input)
		msg.Metadata = json.RawMessage(metadata)
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

var errInvalidTaskMessageMetadata = errors.New("invalid task message metadata")

// ClaimAgentTaskRequest is the request body for claiming a task via the new atomic claim endpoint.
type ClaimAgentTaskRequest struct {
	DaemonID string `json:"daemon_id"`
}

// POST /api/workspaces/{slug}/agents/{id}/tasks/claim — daemon atomically claims the next queued task
func (h *Handler) ClaimAgentTask(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(
		`SELECT a.workspace_id FROM agents a
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ?`, slug, id,
	).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var req ClaimAgentTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DaemonID == "" {
		req.DaemonID = daemonID(r)
	}

	if req.DaemonID == "" {
		writeError(w, http.StatusBadRequest, "daemon_id is required")
		return
	}
	if ctxDaemonID := daemonID(r); ctxDaemonID != "" && req.DaemonID != ctxDaemonID {
		writeError(w, http.StatusForbidden, "daemon id mismatch")
		return
	}

	t, err := service.NewTaskService(h.DB, h.EventBus).ClaimNextForAgent(r.Context(), workspaceID, id, req.DaemonID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to claim task")
		return
	}
	if t == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"task": t})
}

// POST /api/workspaces/{slug}/agents/{id}/heartbeat — daemon heartbeat (keeps agent online)
func (h *Handler) AgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := h.DB.Exec(
		`UPDATE agents SET status = 'online', updated_at = ?
		 WHERE id = ? AND workspace_id = (SELECT id FROM workspaces WHERE slug = ?)`,
		now, id, slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
