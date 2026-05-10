package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/tethy/mulwiki/server/internal/auth"
	"github.com/tethy/mulwiki/server/internal/events"
	"github.com/tethy/mulwiki/server/internal/middleware"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var (
	daemonLookPath = exec.LookPath
	daemonCommand  = exec.Command
)

// POST /api/daemon/register — register or re-register a daemon
// Upserts daemon registration record and auto-detected runtimes.
func (h *Handler) DaemonRegister(w http.ResponseWriter, r *http.Request) {
	var req protocol.DaemonRegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if ctxDaemonID := daemonID(r); ctxDaemonID != "" && req.ID != ctxDaemonID {
		writeError(w, http.StatusForbidden, "daemon id mismatch")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	runtimeIDsJSON, _ := json.Marshal(req.RuntimeIDs)
	if string(runtimeIDsJSON) == "null" {
		runtimeIDsJSON = []byte("[]")
	}

	// Upsert daemon registration.
	if _, err := h.DB.Exec(
		`INSERT INTO daemon_registrations (id, hostname, pid, version, runtime_ids, max_concurrent_tasks, last_heartbeat, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   hostname = excluded.hostname,
		   pid = excluded.pid,
		   version = excluded.version,
		   runtime_ids = excluded.runtime_ids,
		   max_concurrent_tasks = excluded.max_concurrent_tasks,
		   last_heartbeat = excluded.last_heartbeat`,
		req.ID, req.Hostname, req.PID, req.Version, string(runtimeIDsJSON), req.MaxConcurrentTasks, now, now,
	); err != nil {
		slog.Error("daemon register failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to register daemon")
		return
	}

	// Resolve workspace_id if provided.
	var workspaceID string
	if req.WorkspaceSlug != "" {
		h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, req.WorkspaceSlug).Scan(&workspaceID)
	}
	if workspaceID == "" {
		workspaceID = middleware.DaemonWorkspaceIDFromContext(r.Context())
	}
	if workspaceID != "" && !h.daemonCanAccessWorkspace(r, workspaceID) {
		writeError(w, http.StatusForbidden, "workspace access denied")
		return
	}

	// Upsert auto-detected runtimes.
	var resolvedIDs []string
	for _, ri := range req.Runtimes {
		var existingID string

		// Try to find an existing runtime by (workspace_id, backend, path).
		if workspaceID != "" {
			h.DB.QueryRow(
				`SELECT id FROM agent_runtimes WHERE workspace_id = ? AND backend = ? AND name = ?`,
				workspaceID, ri.Backend, ri.Name,
			).Scan(&existingID)
		}

		if existingID != "" {
			// Update existing runtime.
			h.DB.Exec(
				`UPDATE agent_runtimes SET path = ?, version = ?, hostname = ?, os = ?, status = 'online', daemon_id = ?, last_heartbeat = ?
				 WHERE id = ?`,
				ri.Path, ri.Version, req.Hostname, "", req.ID, now, existingID,
			)
			resolvedIDs = append(resolvedIDs, existingID)
		} else if workspaceID != "" {
			// Create new runtime.
			var newID string
			err := h.DB.QueryRow(
				`INSERT INTO agent_runtimes (workspace_id, name, backend, path, hostname, os, version, status, daemon_id, last_heartbeat, created_at)
				 VALUES (?, ?, ?, ?, ?, '', ?, 'online', ?, ?, ?)
				 RETURNING id`,
				workspaceID, ri.Name, ri.Backend, ri.Path, req.Hostname, ri.Version, req.ID, now, now,
			).Scan(&newID)
			if err == nil {
				resolvedIDs = append(resolvedIDs, newID)
			}
		}
	}

	// Also update explicitly provided runtime IDs.
	if len(req.RuntimeIDs) > 0 {
		resolvedIDs = append(resolvedIDs, req.RuntimeIDs...)
	}

	// Update all resolved runtimes: set daemon_id, last_heartbeat, status='online'.
	if len(resolvedIDs) > 0 {
		placeholders := make([]string, len(resolvedIDs))
		args := make([]any, 0, len(resolvedIDs)+3)
		args = append(args, req.ID, now)
		for i, rid := range resolvedIDs {
			placeholders[i] = "?"
			args = append(args, rid)
		}
		query := `UPDATE agent_runtimes SET daemon_id = ?, last_heartbeat = ?, status = 'online'
		          WHERE id IN (` + strings.Join(placeholders, ",") + `)`
		if tokenWorkspaceID := middleware.DaemonWorkspaceIDFromContext(r.Context()); tokenWorkspaceID != "" {
			query += ` AND workspace_id = ?`
			args = append(args, tokenWorkspaceID)
		}
		if _, err := h.DB.Exec(query, args...); err != nil {
			slog.Error("daemon register: failed to update runtimes", "error", err)
		}

		// Update the daemon_registrations runtime_ids to include all resolved IDs.
		allIDsJSON, _ := json.Marshal(dedupStrings(resolvedIDs))
		h.DB.Exec(`UPDATE daemon_registrations SET runtime_ids = ? WHERE id = ?`, string(allIDsJSON), req.ID)
		runtimeIDsJSON = allIDsJSON
	}

	// Publish event.
	if h.EventBus != nil {
		h.EventBus.Publish(events.Event{
			Type:    events.EventDaemonOnline,
			Payload: req,
		})
	}

	var reg protocol.DaemonRegistration
	h.DB.QueryRow(
		`SELECT id, hostname, pid, version, runtime_ids, max_concurrent_tasks, last_heartbeat, registered_at
		 FROM daemon_registrations WHERE id = ?`, req.ID,
	).Scan(&reg.ID, &reg.Hostname, &reg.PID, &reg.Version, &runtimeIDsJSON, &reg.MaxConcurrentTasks, &reg.LastHeartbeat, &reg.RegisteredAt)
	json.Unmarshal(runtimeIDsJSON, &reg.RuntimeIDs)
	if reg.RuntimeIDs == nil {
		reg.RuntimeIDs = []string{}
	}

	writeJSON(w, http.StatusOK, reg)
}

func (h *Handler) daemonCanAccessWorkspace(r *http.Request, workspaceID string) bool {
	if workspaceID == "" {
		return false
	}
	if middleware.DaemonIDFromContext(r.Context()) == "" && middleware.DaemonScopeFromContext(r.Context()) == "" {
		return true
	}
	if tokenWorkspaceID := middleware.DaemonWorkspaceIDFromContext(r.Context()); tokenWorkspaceID != "" {
		return tokenWorkspaceID == workspaceID
	}
	userID := middleware.DaemonUserIDFromContext(r.Context())
	if userID == "" {
		return false
	}
	var exists int
	err := h.DB.QueryRow(`SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&exists)
	return err == nil
}

func (h *Handler) ListDaemonWorkspaces(w http.ResponseWriter, r *http.Request) {
	if workspaceID := middleware.DaemonWorkspaceIDFromContext(r.Context()); workspaceID != "" {
		var ws protocol.DaemonWorkspace
		if err := h.DB.QueryRow(`SELECT id, slug, name FROM workspaces WHERE id = ?`, workspaceID).Scan(&ws.ID, &ws.Slug, &ws.Name); err != nil {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		writeJSON(w, http.StatusOK, protocol.DaemonWorkspacesResponse{Workspaces: []protocol.DaemonWorkspace{ws}})
		return
	}

	userID := middleware.DaemonUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusForbidden, "daemon token is not user-scoped")
		return
	}
	rows, err := h.DB.Query(
		`SELECT w.id, w.slug, w.name
		 FROM workspaces w
		 JOIN workspace_members wm ON wm.workspace_id = w.id
		 WHERE wm.user_id = ?
		 ORDER BY w.name ASC`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}
	defer rows.Close()

	resp := protocol.DaemonWorkspacesResponse{Workspaces: []protocol.DaemonWorkspace{}}
	for rows.Next() {
		var ws protocol.DaemonWorkspace
		if err := rows.Scan(&ws.ID, &ws.Slug, &ws.Name); err == nil {
			resp.Workspaces = append(resp.Workspaces, ws)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// dedupStrings removes duplicates while preserving order.
func dedupStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// POST /api/daemon/heartbeat — periodic heartbeat from daemon
func (h *Handler) DaemonHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req protocol.DaemonHeartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if ctxDaemonID := daemonID(r); ctxDaemonID != "" && req.ID != ctxDaemonID {
		writeError(w, http.StatusForbidden, "daemon id mismatch")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Update daemon_registrations last_heartbeat.
	res, err := h.DB.Exec(
		`UPDATE daemon_registrations SET last_heartbeat = ?
		 WHERE id = ?`,
		now, req.ID,
	)
	if err != nil {
		slog.Error("daemon heartbeat failed", "error", err)
		writeError(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "daemon not registered")
		return
	}

	// Update max_concurrent_tasks if provided.
	if req.MaxConcurrentTasks > 0 {
		h.DB.Exec(`UPDATE daemon_registrations SET max_concurrent_tasks = ? WHERE id = ?`, req.MaxConcurrentTasks, req.ID)
	}

	// Update all matching runtimes: set last_heartbeat, status='online'.
	if len(req.RuntimeIDs) > 0 {
		placeholders := make([]string, len(req.RuntimeIDs))
		args := make([]any, 0, len(req.RuntimeIDs)+2)
		args = append(args, now)
		for i, rid := range req.RuntimeIDs {
			placeholders[i] = "?"
			args = append(args, rid)
		}
		query := `UPDATE agent_runtimes SET last_heartbeat = ?, status = 'online'
		          WHERE id IN (` + strings.Join(placeholders, ",") + `)`
		if tokenWorkspaceID := middleware.DaemonWorkspaceIDFromContext(r.Context()); tokenWorkspaceID != "" {
			query += ` AND workspace_id = ?`
			args = append(args, tokenWorkspaceID)
		}
		if _, err := h.DB.Exec(query, args...); err != nil {
			slog.Error("daemon heartbeat: failed to update runtimes", "error", err)
		}
	} else {
		query := `UPDATE agent_runtimes SET last_heartbeat = ?, status = 'online' WHERE daemon_id = ?`
		args := []any{now, req.ID}
		if tokenWorkspaceID := middleware.DaemonWorkspaceIDFromContext(r.Context()); tokenWorkspaceID != "" {
			query += ` AND workspace_id = ?`
			args = append(args, tokenWorkspaceID)
		}
		if _, err := h.DB.Exec(query, args...); err != nil {
			slog.Error("daemon heartbeat: failed to update runtimes by daemon", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/daemon/stale — internal endpoint: marks runtimes as offline if heartbeat is stale
func (h *Handler) DaemonStale(w http.ResponseWriter, r *http.Request) {
	thresholdStr := r.URL.Query().Get("threshold")
	staleAfter := 5 * time.Minute
	if thresholdStr != "" {
		if d, err := time.ParseDuration(thresholdStr); err == nil {
			staleAfter = d
		}
	}

	cutoff := time.Now().UTC().Add(-staleAfter).Format(time.RFC3339)

	// Mark runtimes as offline where last_heartbeat is stale or never set.
	query := `UPDATE agent_runtimes SET status = 'offline'
		WHERE (last_heartbeat < ? OR last_heartbeat = '')
		  AND status = 'online'`
	args := []any{cutoff}
	if tokenWorkspaceID := middleware.DaemonWorkspaceIDFromContext(r.Context()); tokenWorkspaceID != "" {
		query += ` AND workspace_id = ?`
		args = append(args, tokenWorkspaceID)
	}
	result, err := h.DB.Exec(query, args...)
	if err != nil {
		slog.Error("daemon stale check failed", "error", err)
		writeError(w, http.StatusInternalServerError, "stale check failed")
		return
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		slog.Info("daemon stale: marked runtimes offline", "count", affected, "threshold", staleAfter)

		// Publish offline events for each affected runtime.
		// We could query to get specific IDs, but for simplicity we just note the count.
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"offline_count": affected,
		"threshold":     staleAfter.String(),
	})
}

// GET /api/daemon — list all registered daemons with derived status
func (h *Handler) ListDaemons(w http.ResponseWriter, r *http.Request) {
	h.listDaemons(w, r, "")
}

// GET /api/workspaces/{slug}/daemons — list daemons that have runtimes in this workspace.
func (h *Handler) ListWorkspaceDaemons(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	h.listDaemons(w, r, workspaceID)
}

func (h *Handler) listDaemons(w http.ResponseWriter, _ *http.Request, workspaceID string) {
	query := `SELECT dr.id, dr.hostname, dr.pid, dr.version, dr.runtime_ids, dr.max_concurrent_tasks, dr.last_heartbeat, dr.registered_at
		FROM daemon_registrations dr`
	args := []any{}
	if workspaceID != "" {
		query += ` WHERE EXISTS (
			SELECT 1 FROM agent_runtimes ar
			WHERE ar.daemon_id = dr.id AND ar.workspace_id = ?
		)`
		args = append(args, workspaceID)
	}
	query += ` ORDER BY dr.last_heartbeat DESC`

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list daemons")
		return
	}
	defer rows.Close()

	type daemonItem struct {
		ID                 string `json:"id"`
		Hostname           string `json:"hostname"`
		PID                int    `json:"pid"`
		Version            string `json:"version"`
		RuntimeIDs         string `json:"runtime_ids"`
		MaxConcurrentTasks int    `json:"max_concurrent_tasks"`
		LastHeartbeat      string `json:"last_heartbeat"`
		RegisteredAt       string `json:"registered_at"`
	}

	daemons := make([]daemonItem, 0)
	for rows.Next() {
		var d daemonItem
		if err := rows.Scan(&d.ID, &d.Hostname, &d.PID, &d.Version,
			&d.RuntimeIDs, &d.MaxConcurrentTasks, &d.LastHeartbeat, &d.RegisteredAt); err != nil {
			continue
		}
		daemons = append(daemons, d)
	}

	writeJSON(w, http.StatusOK, map[string]any{"daemons": daemons})
}

func (h *Handler) daemonBelongsToWorkspace(daemonID, workspaceID string) bool {
	if daemonID == "" || workspaceID == "" {
		return false
	}
	var exists int
	err := h.DB.QueryRow(
		`SELECT 1 FROM agent_runtimes WHERE daemon_id = ? AND workspace_id = ? LIMIT 1`,
		daemonID, workspaceID,
	).Scan(&exists)
	return err == nil && exists == 1
}

// GET /api/daemon/{id}/logs — tail daemon log
func (h *Handler) GetDaemonLogs(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	if ctxDaemonID := daemonID(r); ctxDaemonID != "" && id != ctxDaemonID {
		writeError(w, http.StatusForbidden, "daemon id mismatch")
		return
	}
	if daemonID(r) == "" {
		workspaceID, err := h.workspaceIDForRequest(r)
		if err != nil || workspaceID == "" || !h.daemonBelongsToWorkspace(id, workspaceID) {
			writeError(w, http.StatusNotFound, "daemon not found")
			return
		}
	}
	n := 50
	if nStr := r.URL.Query().Get("n"); nStr != "" {
		if parsed, err := strconv.Atoi(nStr); err == nil && parsed > 0 && parsed <= 500 {
			n = parsed
		}
	}

	// Read log file
	logPath := os.ExpandEnv("$HOME/.mulwiki/daemon/daemon.log")
	b, err := os.ReadFile(logPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "daemon log not found")
		return
	}

	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"daemon_id": id,
		"log_path":  logPath,
		"lines":     lines,
		"total":     len(lines),
	})
}

// POST /api/daemon/{id}/stop — stop daemon by PID
func (h *Handler) StopDaemon(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	if ctxDaemonID := daemonID(r); ctxDaemonID != "" && id != ctxDaemonID {
		writeError(w, http.StatusForbidden, "daemon id mismatch")
		return
	}
	if daemonID(r) == "" {
		workspaceID, err := h.workspaceIDForRequest(r)
		if err != nil || workspaceID == "" || !h.daemonBelongsToWorkspace(id, workspaceID) {
			writeError(w, http.StatusNotFound, "daemon not found")
			return
		}
	}

	var pid int
	err := h.DB.QueryRow(`SELECT pid FROM daemon_registrations WHERE id = ?`, id).Scan(&pid)
	if err != nil {
		writeError(w, http.StatusNotFound, "daemon not found")
		return
	}

	// Find and kill the process
	proc, err := os.FindProcess(pid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to find daemon process")
		return
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Try force kill
		if err := proc.Kill(); err != nil {
			// Process already dead — still clear heartbeat
			h.DB.Exec(`UPDATE daemon_registrations SET pid = 0, last_heartbeat = '1970-01-01T00:00:00Z' WHERE id = ?`, id)
			if tokenWorkspaceID := middleware.DaemonWorkspaceIDFromContext(r.Context()); tokenWorkspaceID != "" {
				h.DB.Exec(`UPDATE agent_runtimes SET status = 'offline', last_heartbeat = '1970-01-01T00:00:00Z' WHERE daemon_id = ? AND workspace_id = ?`, id, tokenWorkspaceID)
			} else {
				h.DB.Exec(`UPDATE agent_runtimes SET status = 'offline', last_heartbeat = '1970-01-01T00:00:00Z' WHERE daemon_id = ?`, id)
			}
			writeJSON(w, http.StatusOK, map[string]any{"daemon_id": id, "status": "already_stopped"})
			return
		}
	}

	// Mark as stopped in DB — clear heartbeat so UI immediately reflects
	h.DB.Exec(`UPDATE daemon_registrations SET pid = 0, last_heartbeat = '1970-01-01T00:00:00Z' WHERE id = ?`, id)

	// Mark runtimes as offline too
	if tokenWorkspaceID := middleware.DaemonWorkspaceIDFromContext(r.Context()); tokenWorkspaceID != "" {
		h.DB.Exec(`UPDATE agent_runtimes SET status = 'offline', last_heartbeat = '1970-01-01T00:00:00Z' WHERE daemon_id = ? AND workspace_id = ?`, id, tokenWorkspaceID)
	} else {
		h.DB.Exec(`UPDATE agent_runtimes SET status = 'offline', last_heartbeat = '1970-01-01T00:00:00Z' WHERE daemon_id = ?`, id)
	}

	writeJSON(w, http.StatusOK, map[string]any{"daemon_id": id, "status": "stopped"})
}

// POST /api/daemon/start — start a daemon process locally
func (h *Handler) StartDaemon(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Workspace string `json:"workspace"`
		ServerURL string `json:"server_url"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Workspace == "" {
		req.Workspace = workspaceSlug(r)
	}
	if req.Workspace == "" {
		writeError(w, http.StatusBadRequest, "workspace is required")
		return
	}
	if req.ServerURL == "" {
		req.ServerURL = "http://localhost:8080"
	}

	// Find mulwiki binary (try common locations)
	mulwikiPath, err := daemonLookPath("mulwiki")
	if err != nil {
		// Fall back to known build locations
		candidates := []string{"/tmp/mulwiki", "./bin/mulwiki", os.ExpandEnv("$HOME/.mulwiki/bin/mulwiki")}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				mulwikiPath = c
				break
			}
		}
	}
	if mulwikiPath == "" {
		writeError(w, http.StatusInternalServerError, "mulwiki binary not found in PATH")
		return
	}

	cmd := daemonCommand(mulwikiPath, "daemon", "start",
		"--workspace", req.Workspace,
		"--server-url", req.ServerURL,
	)
	daemonIDArg := middleware.GetDaemonID(r)
	token := middleware.DaemonTokenFromContext(r.Context())
	if token == "" {
		workspaceID, err := h.workspaceIDForRequest(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		daemonIDArg = uuid.New().String()
		raw, hash, err := auth.NewDaemonToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create daemon token")
			return
		}
		if _, err := h.DB.Exec(
			`INSERT INTO daemon_tokens (workspace_id, daemon_id, token_hash)
			 VALUES (?, ?, ?)`,
			workspaceID, daemonIDArg, hash,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store daemon token")
			return
		}
		token = raw
	}
	if daemonIDArg != "" {
		cmd.Args = append(cmd.Args, "--daemon-id", daemonIDArg)
	}
	if token != "" {
		cmd.Args = append(cmd.Args, "--daemon-token", token)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start daemon: "+err.Error())
		return
	}

	// Don't wait — daemon runs in background
	go cmd.Wait()

	writeJSON(w, http.StatusOK, map[string]any{"status": "started", "pid": cmd.Process.Pid})
}

// runDaemonSweeper periodically detects stale daemons and marks their runtimes as offline.
// It runs in a background goroutine.
func RunDaemonSweeper(h *Handler, checkInterval, staleAfter time.Duration) {
	slog.Info("daemon sweeper started",
		"check_interval", checkInterval,
		"stale_after", staleAfter,
	)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().UTC().Add(-staleAfter).Format(time.RFC3339)

		result, err := h.DB.Exec(
			`UPDATE agent_runtimes SET status = 'offline'
			 WHERE (last_heartbeat < ? OR last_heartbeat = '')
			   AND status = 'online'`,
			cutoff,
		)
		if err != nil {
			slog.Error("daemon sweeper: stale detection failed", "error", err)
			continue
		}

		affected, _ := result.RowsAffected()
		if affected > 0 {
			slog.Info("daemon sweeper: marked stale runtimes offline", "count", affected)
		}
	}
}
