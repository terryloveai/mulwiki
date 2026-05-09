package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/tethy/mulwiki/server/internal/auth"
)

func (h *Handler) CreateDaemonToken(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req struct {
		DaemonID  string `json:"daemon_id"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.DaemonID = strings.TrimSpace(req.DaemonID)
	if req.DaemonID == "" {
		req.DaemonID = uuid.New().String()
	}
	req.ExpiresAt = strings.TrimSpace(req.ExpiresAt)

	raw, hash, err := auth.NewDaemonToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create daemon token")
		return
	}

	var resp struct {
		ID          string  `json:"id"`
		WorkspaceID string  `json:"workspace_id"`
		DaemonID    string  `json:"daemon_id"`
		Token       string  `json:"token"`
		ExpiresAt   *string `json:"expires_at,omitempty"`
		CreatedAt   string  `json:"created_at"`
	}

	var expiresAt sql.NullString
	err = h.DB.QueryRow(
		`INSERT INTO daemon_tokens (workspace_id, daemon_id, token_hash, expires_at)
		 VALUES (?, ?, ?, ?)
		 RETURNING id, workspace_id, daemon_id, expires_at, created_at`,
		workspaceID, req.DaemonID, hash, nullStr(req.ExpiresAt),
	).Scan(&resp.ID, &resp.WorkspaceID, &resp.DaemonID, &expiresAt, &resp.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store daemon token")
		return
	}
	if expiresAt.Valid {
		resp.ExpiresAt = &expiresAt.String
	}
	resp.Token = raw

	writeJSON(w, http.StatusCreated, resp)
}
