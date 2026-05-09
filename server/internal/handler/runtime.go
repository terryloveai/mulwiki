package handler

import (
	"net/http"
	"time"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// GET /api/workspaces/{slug}/agents/runtimes
func (h *Handler) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, workspace_id, name, backend, path, hostname, os, version, status, daemon_id, last_heartbeat, created_at
		 FROM agent_runtimes WHERE workspace_id = ? ORDER BY created_at DESC`, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runtimes")
		return
	}
	defer rows.Close()

	runtimes := make([]protocol.AgentRuntime, 0)
	for rows.Next() {
		var rt protocol.AgentRuntime
		if err := rows.Scan(&rt.ID, &rt.WorkspaceID, &rt.Name, &rt.Backend, &rt.Path,
			&rt.Hostname, &rt.OS, &rt.Version, &rt.Status, &rt.DaemonID, &rt.LastHeartbeat, &rt.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan runtime")
			return
		}
		runtimes = append(runtimes, rt)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to iterate runtimes")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"runtimes": runtimes})
}

// POST /api/workspaces/{slug}/agents/runtimes
func (h *Handler) CreateRuntime(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req protocol.CreateRuntimeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	var rt protocol.AgentRuntime
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO agent_runtimes (workspace_id, name, backend, path, hostname, os, version, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'offline', ?)
		 RETURNING id, workspace_id, name, backend, path, hostname, os, version, status, daemon_id, last_heartbeat, created_at`,
		workspaceID, req.Name, req.Backend, req.Path, req.Hostname, req.OS, req.Version, time.Now().UTC().Format(time.RFC3339),
	).Scan(&rt.ID, &rt.WorkspaceID, &rt.Name, &rt.Backend, &rt.Path,
		&rt.Hostname, &rt.OS, &rt.Version, &rt.Status, &rt.DaemonID, &rt.LastHeartbeat, &rt.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create runtime")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"runtime": rt})
}

// GET /api/workspaces/{slug}/agents/runtimes/{id}
func (h *Handler) GetRuntime(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var rt protocol.AgentRuntime
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT id, workspace_id, name, backend, path, hostname, os, version,
		        status, daemon_id, last_heartbeat, created_at
		 FROM agent_runtimes
		 WHERE workspace_id = ? AND id = ?`, workspaceID, id,
	).Scan(&rt.ID, &rt.WorkspaceID, &rt.Name, &rt.Backend, &rt.Path,
		&rt.Hostname, &rt.OS, &rt.Version, &rt.Status, &rt.DaemonID, &rt.LastHeartbeat, &rt.CreatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"runtime": rt})
}

// PATCH /api/workspaces/{slug}/agents/runtimes/{id}
func (h *Handler) UpdateRuntime(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	var exists bool
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM agent_runtimes WHERE workspace_id = ? AND id = ?)`,
		workspaceID, id,
	).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	var req protocol.UpdateRuntimeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Build dynamic SET clause
	cls := []string{}
	args := []any{}
	if req.Name != nil {
		cls = append(cls, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Backend != nil {
		cls = append(cls, "backend = ?")
		args = append(args, *req.Backend)
	}
	if req.Path != nil {
		cls = append(cls, "path = ?")
		args = append(args, *req.Path)
	}
	if req.Hostname != nil {
		cls = append(cls, "hostname = ?")
		args = append(args, *req.Hostname)
	}
	if req.OS != nil {
		cls = append(cls, "os = ?")
		args = append(args, *req.OS)
	}
	if req.Version != nil {
		cls = append(cls, "version = ?")
		args = append(args, *req.Version)
	}
	if len(cls) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	q := "UPDATE agent_runtimes SET " + joinClauses(cls, ", ") + " WHERE id = ? AND workspace_id = ?" +
		" RETURNING id, workspace_id, name, backend, path, hostname, os, version, status, daemon_id, last_heartbeat, created_at"
	args = append(args, id, workspaceID)

	var rt protocol.AgentRuntime
	err = h.DB.QueryRowContext(r.Context(), q, args...).Scan(
		&rt.ID, &rt.WorkspaceID, &rt.Name, &rt.Backend, &rt.Path,
		&rt.Hostname, &rt.OS, &rt.Version, &rt.Status, &rt.DaemonID, &rt.LastHeartbeat, &rt.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update runtime")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"runtime": rt})
}

// DELETE /api/workspaces/{slug}/agents/runtimes/{id}
func (h *Handler) DeleteRuntime(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM agent_runtimes WHERE id = ? AND workspace_id = ?`, id, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete runtime")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

func joinClauses(cls []string, sep string) string {
	r := ""
	for i, c := range cls {
		if i > 0 {
			r += sep
		}
		r += c
	}
	return r
}
