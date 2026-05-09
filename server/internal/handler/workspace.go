package handler

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tethy/mulwiki/server/pkg/gitrepo"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// slugRegex is a simple slug validator — lowercase alphanumeric with hyphens.
func isValidSlug(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i, r := range s {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' && i > 0 && i < len(s)-1 {
			continue
		}
		return false
	}
	return true
}

// CleanSlug returns a normalized slug: lowercase, trimmed, no double hyphens.
func CleanSlug(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	// Replace spaces with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	// Collapse multiple hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

func (h *Handler) loadWorkspaceActiveSchema(ws *protocol.Workspace) {
	var activeSchemaID, activeSchemaPath string
	if err := h.DB.QueryRow(
		`SELECT COALESCE(active_schema_id, ''), active_schema_path FROM workspaces WHERE id = ?`,
		ws.ID,
	).Scan(&activeSchemaID, &activeSchemaPath); err != nil {
		return
	}
	if activeSchemaID != "" {
		ws.ActiveSchemaID = &activeSchemaID
	}
	if activeSchemaPath != "" {
		ws.ActiveSchemaPath = &activeSchemaPath
	}
}

// POST /api/workspaces
func (h *Handler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	currentUserID := userID(r)

	var req protocol.CreateWorkspaceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	slug := req.Slug
	if slug == "" {
		slug = CleanSlug(req.Name)
	} else {
		slug = CleanSlug(slug)
	}
	if !isValidSlug(slug) {
		writeError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	initialSchemaType := strings.TrimSpace(req.InitialSchemaType)
	initialSchemaPath := ""
	if initialSchemaType == "" && strings.TrimSpace(req.InitialSchemaPath) != "" {
		initialSchemaType = initialSchemaTypeBuiltin
	}
	switch initialSchemaType {
	case "":
	case initialSchemaTypeBlank:
	case initialSchemaTypeBuiltin:
		path, ok := builtinSchemaWorkspacePath(req.InitialSchemaPath)
		if !ok || !h.hasBuiltinSchema(path) {
			writeError(w, http.StatusBadRequest, "invalid initial schema")
			return
		}
		initialSchemaPath = path
	default:
		writeError(w, http.StatusBadRequest, "invalid initial schema type")
		return
	}

	ws := protocol.Workspace{}
	err := h.DB.QueryRow(
		`INSERT INTO workspaces (slug, name, description) VALUES (?, ?, ?)
		 RETURNING id, slug, name, description, created_at`,
		slug, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description),
	).Scan(&ws.ID, &ws.Slug, &ws.Name, &ws.Description, &ws.CreatedAt)
	if err != nil {
		if isUniqueConstraint(err) {
			writeError(w, http.StatusConflict, "workspace slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}

	if currentUserID != "" {
		if _, err := h.DB.Exec(
			`INSERT OR IGNORE INTO workspace_members (workspace_id, user_id, role) VALUES (?, ?, 'owner')`,
			ws.ID, currentUserID,
		); err != nil {
			slog.Error("failed to create workspace owner membership", "slug", slug, "user_id", currentUserID, "error", err)
			h.DB.Exec(`DELETE FROM workspaces WHERE id = ?`, ws.ID)
			writeError(w, http.StatusInternalServerError, "failed to create workspace membership")
			return
		}
	}

	// Init bare git repo for workspace storage.
	repoPath := filepath.Join(h.reposDir(), ws.ID+".git")
	repo, err := gitrepo.InitBare(repoPath)
	if err != nil {
		slog.Error("failed to init bare repo for workspace", "slug", slug, "error", err)
		// Rollback DB creation.
		h.DB.Exec(`DELETE FROM workspaces WHERE id = ?`, ws.ID)
		writeError(w, http.StatusInternalServerError, "failed to initialize workspace storage")
		return
	}

	// If a git remote URL was provided, configure it.
	if req.GitRemote != "" {
		if err := repo.SetRemote(req.GitRemote); err != nil {
			slog.Warn("failed to set git remote for workspace", "slug", slug, "remote", req.GitRemote, "error", err)
		}
	}

	// Seed builtin schemas into the workspace repo.
	if err := h.SeedBuiltinSchemas(ws.ID); err != nil {
		slog.Warn("failed to seed builtin schemas", "slug", slug, "error", err)
		if initialSchemaType == initialSchemaTypeBuiltin {
			h.DB.Exec(`DELETE FROM workspaces WHERE id = ?`, ws.ID)
			writeError(w, http.StatusInternalServerError, "failed to initialize workspace schema")
			return
		}
	}

	switch initialSchemaType {
	case initialSchemaTypeBuiltin:
		if err := h.activateSchemaByPath(ws.ID, initialSchemaPath); err != nil {
			slog.Error("failed to activate initial schema", "slug", slug, "schema_path", initialSchemaPath, "error", err)
			h.DB.Exec(`DELETE FROM workspaces WHERE id = ?`, ws.ID)
			writeError(w, http.StatusInternalServerError, "failed to initialize workspace schema")
			return
		}
	case initialSchemaTypeBlank:
		if err := h.createBlankSchema(ws.ID); err != nil {
			slog.Error("failed to create blank schema", "slug", slug, "error", err)
			h.DB.Exec(`DELETE FROM workspaces WHERE id = ?`, ws.ID)
			writeError(w, http.StatusInternalServerError, "failed to initialize workspace schema")
			return
		}
	}
	h.loadWorkspaceActiveSchema(&ws)

	writeJSON(w, http.StatusCreated, ws)
}

// GET /api/workspaces
func (h *Handler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	currentUserID := userID(r)
	query := `SELECT id, slug, name, description, created_at FROM workspaces WHERE slug <> 'builtin' ORDER BY created_at DESC`
	args := []any{}
	if currentUserID != "" {
		query = `SELECT w.id, w.slug, w.name, w.description, w.created_at
		 FROM workspaces w
		 JOIN workspace_members wm ON wm.workspace_id = w.id
		 WHERE wm.user_id = ? AND w.slug <> 'builtin'
		 ORDER BY w.created_at DESC`
		args = append(args, currentUserID)
	}

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}
	defer rows.Close()

	workspaces := make([]protocol.Workspace, 0)
	for rows.Next() {
		var ws protocol.Workspace
		if err := rows.Scan(&ws.ID, &ws.Slug, &ws.Name, &ws.Description, &ws.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan workspace")
			return
		}
		workspaces = append(workspaces, ws)
	}

	writeJSON(w, http.StatusOK, workspaces)
}

// GET /api/workspaces/{slug}
func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var ws protocol.Workspace
	var activeSchemaID *string
	var activeSchemaPath string
	err := h.DB.QueryRow(
		`SELECT id, slug, name, description, active_schema_id, active_schema_path, created_at FROM workspaces WHERE slug = ?`, slug,
	).Scan(&ws.ID, &ws.Slug, &ws.Name, &ws.Description, &activeSchemaID, &activeSchemaPath, &ws.CreatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	ws.ActiveSchemaID = activeSchemaID
	if activeSchemaPath != "" {
		ws.ActiveSchemaPath = &activeSchemaPath
	}

	writeJSON(w, http.StatusOK, ws)
}

// PATCH /api/workspaces/{slug}
func (h *Handler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var req protocol.UpdateWorkspaceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	var ws protocol.Workspace
	err := h.DB.QueryRow(
		`UPDATE workspaces SET name = ?, description = ? WHERE slug = ? AND slug <> 'builtin'
		 RETURNING id, slug, name, description, created_at`,
		strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), slug,
	).Scan(&ws.ID, &ws.Slug, &ws.Name, &ws.Description, &ws.CreatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	writeJSON(w, http.StatusOK, ws)
}

// DELETE /api/workspaces/{slug}
func (h *Handler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	res, err := h.DB.Exec(`DELETE FROM workspaces WHERE slug = ? AND slug <> 'builtin'`, slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workspace")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
