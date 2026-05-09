package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/tethy/mulwiki/server/internal/middleware"
	"github.com/tethy/mulwiki/server/internal/service"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

func (h *Handler) AppendTaskMessages(w http.ResponseWriter, r *http.Request) {
	taskID := idParam(r, "taskId")
	workspaceID := middleware.GetWorkspaceID(r)
	if workspaceID == "" {
		writeError(w, http.StatusForbidden, "workspace access denied")
		return
	}

	var body struct {
		Messages []protocol.AgentTaskMessage `json:"messages"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := service.NewTaskService(h.DB, h.EventBus).AppendMessages(r.Context(), workspaceID, taskID, body.Messages); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PinTaskSession(w http.ResponseWriter, r *http.Request) {
	taskID := idParam(r, "taskId")
	workspaceID := middleware.GetWorkspaceID(r)
	if workspaceID == "" {
		writeError(w, http.StatusForbidden, "workspace access denied")
		return
	}

	var body struct {
		SessionID string `json:"session_id"`
		WorkDir   string `json:"work_dir"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := service.NewTaskService(h.DB, h.EventBus).PinSession(r.Context(), workspaceID, taskID, body.SessionID, body.WorkDir); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to pin session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListTaskMessages(w http.ResponseWriter, r *http.Request) {
	taskID := idParam(r, "taskId")
	userID := userID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT workspace_id FROM agent_tasks WHERE id = ?`, taskID).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	var allowed int
	if err := h.DB.QueryRow(
		`SELECT COUNT(*) FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID,
	).Scan(&allowed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify membership")
		return
	}
	if allowed == 0 {
		writeError(w, http.StatusForbidden, "workspace access denied")
		return
	}

	sinceSeq := int64(0)
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid since")
			return
		}
		sinceSeq = parsed
	}

	messages, err := service.NewTaskService(h.DB, h.EventBus).ListMessages(r.Context(), workspaceID, taskID, sinceSeq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}
