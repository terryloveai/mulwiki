package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/tethy/mulwiki/server/internal/auth"
	"github.com/tethy/mulwiki/server/internal/middleware"
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

func (h *Handler) CreateUserDaemonToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
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

	var anchorWorkspaceID string
	err := h.DB.QueryRow(
		`SELECT workspace_id FROM workspace_members WHERE user_id = ? ORDER BY created_at ASC LIMIT 1`,
		userID,
	).Scan(&anchorWorkspaceID)
	if err != nil || anchorWorkspaceID == "" {
		writeError(w, http.StatusBadRequest, "user has no workspaces")
		return
	}

	raw, hash, err := auth.NewDaemonToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create daemon token")
		return
	}

	var resp struct {
		ID        string  `json:"id"`
		UserID    string  `json:"user_id"`
		Scope     string  `json:"scope"`
		DaemonID  string  `json:"daemon_id"`
		Token     string  `json:"token"`
		ExpiresAt *string `json:"expires_at,omitempty"`
		CreatedAt string  `json:"created_at"`
	}
	var expiresAt sql.NullString
	err = h.DB.QueryRow(
		`INSERT INTO daemon_tokens (workspace_id, user_id, scope, daemon_id, token_hash, expires_at)
		 VALUES (?, ?, 'user', ?, ?, ?)
		 RETURNING id, user_id, scope, daemon_id, expires_at, created_at`,
		anchorWorkspaceID, userID, req.DaemonID, hash, nullStr(req.ExpiresAt),
	).Scan(&resp.ID, &resp.UserID, &resp.Scope, &resp.DaemonID, &expiresAt, &resp.CreatedAt)
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
