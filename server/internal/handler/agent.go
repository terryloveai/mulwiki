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

func redactAgentCustomEnv(agent *protocol.Agent, currentUser string, isDaemon bool) {
	if agent.OwnerID != "" && agent.OwnerID != currentUser && !isDaemon {
		agent.CustomEnv = nil
	}
}

// GET /api/workspaces/{slug}/agents
func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	currentUser := userID(r)

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	agents, err := store.NewAgentStore(h.DB).ListByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	for i := range agents {
		redactAgentCustomEnv(&agents[i], currentUser, isDaemonRequest(r))
	}

	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// POST /api/workspaces/{slug}/agents
func (h *Handler) CreateAgent(w http.ResponseWriter, r *http.Request) {
	currentUser := userID(r)

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
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
		if err := h.DB.QueryRowContext(r.Context(),
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

	var agentID string
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO agents (workspace_id, runtime_id, name, description, instructions,
		                     runtime_mode, runtime_config, custom_env, custom_args, mcp_config,
		                     visibility, status, model, max_concurrent_tasks, owner_id,
		                     created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'offline', ?, ?, ?, ?, ?)
		 RETURNING id`,
		workspaceID, nullStr(req.RuntimeID), req.Name, req.Description, req.Instructions,
		runtimeMode, runtimeConfig, string(customEnv), string(customArgs), mcpConfig,
		visibility, req.Model, maxConcurrent, nullStr(ownerID),
		now, now,
	).Scan(&agentID)
	if err != nil {
		slog.Error("CreateAgent insert failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	a, err := store.NewAgentStore(h.DB).GetInWorkspace(r.Context(), workspaceID, agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch created agent")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"agent": a})
}

// GET /api/workspaces/{slug}/agents/{id}
func (h *Handler) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	currentUser := userID(r)

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	a, err := store.NewAgentStore(h.DB).GetInWorkspace(r.Context(), workspaceID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load agent")
		return
	}

	redactAgentCustomEnv(a, currentUser, isDaemonRequest(r))

	writeJSON(w, http.StatusOK, map[string]any{"agent": a})
}

// PATCH /api/workspaces/{slug}/agents/{id}
func (h *Handler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	currentUser := userID(r)

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	agentStore := store.NewAgentStore(h.DB)
	_, err = agentStore.GetInWorkspace(r.Context(), workspaceID, id)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to load agent")
			return
		}
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
			if err := h.DB.QueryRowContext(r.Context(),
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
		setClauses = append(setClauses, "runtime_mode = ?")
		args = append(args, runtimeMode)
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
	args = append(args, id, workspaceID)

	query := `UPDATE agents SET ` + strings.Join(setClauses, ", ") + ` WHERE id = ? AND workspace_id = ?`

	if _, err := h.DB.ExecContext(r.Context(), query, args...); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	a, err := agentStore.GetInWorkspace(r.Context(), workspaceID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch updated agent")
		return
	}

	redactAgentCustomEnv(a, currentUser, isDaemonRequest(r))

	writeJSON(w, http.StatusOK, map[string]any{"agent": a})

	// Check if there are tasks
}

// POST /api/workspaces/{slug}/agents/{id}/archive
func (h *Handler) ArchiveAgent(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	currentUser := userID(r)

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE agents SET archived_at = ?, archived_by = ?, updated_at = ?
		 WHERE id = ? AND workspace_id = ?`,
		now, currentUser, now, id, workspaceID,
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
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE agents SET archived_at = NULL, archived_by = NULL, updated_at = ?
		 WHERE id = ? AND workspace_id = ?`,
		now, id, workspaceID,
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
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if _, err := store.NewAgentStore(h.DB).GetInWorkspace(r.Context(), workspaceID, id); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT `+store.AgentTaskSelectColumns+`
		 FROM agent_tasks WHERE agent_id = ? AND workspace_id = ? ORDER BY created_at DESC`,
		id, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	defer rows.Close()

	tasks := make([]protocol.AgentTask, 0)
	for rows.Next() {
		t, err := store.ScanAgentTask(rows.Scan)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan task")
			return
		}
		tasks = append(tasks, *t)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to iterate tasks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// GET /api/workspaces/{slug}/agents/{id}/tasks/{taskId}
func (h *Handler) GetAgentTask(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	taskID := idParam(r, "taskId")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	t, err := store.NewTaskStore(h.DB).GetInWorkspace(r.Context(), workspaceID, taskID)
	if err != nil || t.AgentID != id {
		writeError(w, http.StatusNotFound, "task not found")
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

// POST /api/workspaces/{slug}/agents/{id}/tasks — daemon creates a task record
func (h *Handler) CreateAgentTask(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if _, err := store.NewAgentStore(h.DB).GetInWorkspace(r.Context(), workspaceID, id); err != nil {
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
		row := h.DB.QueryRowContext(r.Context(),
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
	id := idParam(r, "id")
	taskID := idParam(r, "taskId")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	existingTask, err := store.NewTaskStore(h.DB).GetInWorkspace(r.Context(), workspaceID, taskID)
	if err != nil || existingTask.AgentID != id {
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
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if _, err := store.NewAgentStore(h.DB).GetInWorkspace(r.Context(), workspaceID, id); err != nil {
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
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE agents SET status = 'online', updated_at = ?
		 WHERE id = ? AND workspace_id = ?`,
		now, id, workspaceID,
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
