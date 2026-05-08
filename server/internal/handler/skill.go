package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// GET /api/workspaces/{slug}/agents/skills
func (h *Handler) ListSkills(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	rows, err := h.DB.Query(
		`SELECT id, workspace_id, name, description, created_at
		 FROM agent_skills WHERE workspace_id = ? ORDER BY created_at DESC`, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list skills")
		return
	}
	defer rows.Close()

	skills := make([]protocol.AgentSkill, 0)
	for rows.Next() {
		var sk protocol.AgentSkill
		if err := rows.Scan(&sk.ID, &sk.WorkspaceID, &sk.Name, &sk.Description, &sk.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan skill")
			return
		}
		skills = append(skills, sk)
	}

	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

// POST /api/workspaces/{slug}/agents/skills
func (h *Handler) CreateSkill(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req protocol.CreateSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	var sk protocol.AgentSkill
	err := h.DB.QueryRow(
		`INSERT INTO agent_skills (workspace_id, name, description, created_at)
		 VALUES (?, ?, ?, ?)
		 RETURNING id, workspace_id, name, description, created_at`,
		workspaceID, req.Name, req.Description, time.Now().UTC().Format(time.RFC3339),
	).Scan(&sk.ID, &sk.WorkspaceID, &sk.Name, &sk.Description, &sk.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create skill")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"skill": sk})
}

// PATCH /api/workspaces/{slug}/agents/skills/{id}
func (h *Handler) UpdateSkill(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var req protocol.UpdateSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Build dynamic update
	setClauses := []string{}
	args := []any{}

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *req.Description)
	}

	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	args = append(args, id, slug)
	query := `UPDATE agent_skills SET ` + strings.Join(setClauses, ", ") +
		` WHERE id = ? AND workspace_id = (SELECT id FROM workspaces WHERE slug = ?)`

	res, err := h.DB.Exec(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update skill")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}

	// Fetch updated
	var sk protocol.AgentSkill
	if err := h.DB.QueryRow(
		`SELECT id, workspace_id, name, description, created_at FROM agent_skills WHERE id = ?`, id,
	).Scan(&sk.ID, &sk.WorkspaceID, &sk.Name, &sk.Description, &sk.CreatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch updated skill")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"skill": sk})
}

// DELETE /api/workspaces/{slug}/agents/skills/{id}
func (h *Handler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	res, err := h.DB.Exec(
		`DELETE FROM agent_skills WHERE id = ? AND workspace_id = (SELECT id FROM workspaces WHERE slug = ?)`,
		id, slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete skill")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// POST /api/workspaces/{slug}/agents/{id}/skills
func (h *Handler) AddAgentSkill(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	agentID := idParam(r, "id")

	var req protocol.AddAgentSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SkillID == "" {
		writeError(w, http.StatusBadRequest, "skill_id is required")
		return
	}

	// Verify agent is in workspace
	var workspaceID string
	if err := h.DB.QueryRow(
		`SELECT a.workspace_id FROM agents a
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ?`, slug, agentID,
	).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Verify skill exists in same workspace
	var skillExists bool
	if err := h.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM agent_skills WHERE id = ? AND workspace_id = ?)`,
		req.SkillID, workspaceID,
	).Scan(&skillExists); err != nil || !skillExists {
		writeError(w, http.StatusBadRequest, "skill not found")
		return
	}

	if _, err := h.DB.Exec(
		`INSERT OR IGNORE INTO agent_skills_agents (agent_id, skill_id) VALUES (?, ?)`,
		agentID, req.SkillID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add skill to agent")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// DELETE /api/workspaces/{slug}/agents/{id}/skills/{skillId}
func (h *Handler) RemoveAgentSkill(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	agentID := idParam(r, "id")
	skillID := idParam(r, "skillId")

	// Verify agent is in workspace
	if err := h.DB.QueryRow(
		`SELECT a.workspace_id FROM agents a
		 JOIN workspaces w ON w.id = a.workspace_id
		 WHERE w.slug = ? AND a.id = ?`, slug, agentID,
	).Scan(new(string)); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	res, err := h.DB.Exec(
		`DELETE FROM agent_skills_agents WHERE agent_id = ? AND skill_id = ?`,
		agentID, skillID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove skill")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "skill association not found")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}
