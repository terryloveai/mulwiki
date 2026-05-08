package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// GET /api/workspaces/{slug}/agents/skills
func (h *Handler) ListSkills(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
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
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to iterate skills")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

// POST /api/workspaces/{slug}/agents/skills
func (h *Handler) CreateSkill(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
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
	err = h.DB.QueryRowContext(r.Context(),
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
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

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

	args = append(args, id, workspaceID)
	query := `UPDATE agent_skills SET ` + strings.Join(setClauses, ", ") +
		` WHERE id = ? AND workspace_id = ?`

	res, err := h.DB.ExecContext(r.Context(), query, args...)
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
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, workspace_id, name, description, created_at FROM agent_skills WHERE id = ? AND workspace_id = ?`,
		id, workspaceID,
	).Scan(&sk.ID, &sk.WorkspaceID, &sk.Name, &sk.Description, &sk.CreatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch updated skill")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"skill": sk})
}

// DELETE /api/workspaces/{slug}/agents/skills/{id}
func (h *Handler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM agent_skills WHERE id = ? AND workspace_id = ?`, id, workspaceID)
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
	agentID := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

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
	var agentExists bool
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM agents WHERE workspace_id = ? AND id = ?)`,
		workspaceID, agentID,
	).Scan(&agentExists); err != nil || !agentExists {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Verify skill exists in same workspace
	var skillExists bool
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM agent_skills WHERE id = ? AND workspace_id = ?)`,
		req.SkillID, workspaceID,
	).Scan(&skillExists); err != nil || !skillExists {
		writeError(w, http.StatusBadRequest, "skill not found")
		return
	}

	if _, err := h.DB.ExecContext(r.Context(),
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
	agentID := idParam(r, "id")
	skillID := idParam(r, "skillId")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	// Verify agent is in workspace
	var agentExists bool
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM agents WHERE workspace_id = ? AND id = ?)`,
		workspaceID, agentID,
	).Scan(&agentExists); err != nil || !agentExists {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	res, err := h.DB.ExecContext(
		r.Context(),
		`DELETE FROM agent_skills_agents
		 WHERE agent_id = ? AND skill_id = ?
		   AND EXISTS (SELECT 1 FROM agent_skills WHERE id = ? AND workspace_id = ?)`,
		agentID, skillID, skillID, workspaceID,
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
