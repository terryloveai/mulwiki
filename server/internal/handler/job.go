package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// DaemonClaimRequest is the request body for claiming a job.
type DaemonClaimRequest struct {
	DaemonID string `json:"daemon_id"`
}

// POST /api/workspaces/{slug}/jobs/claim — daemon claims next pending job
func (h *Handler) ClaimJob(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req DaemonClaimRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DaemonID == "" {
		writeError(w, http.StatusBadRequest, "daemon_id is required")
		return
	}

	// Pick the oldest pending job in this workspace and atomically claim it.
	tx, err := h.DB.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	var jobID string
	err = tx.QueryRow(
		`SELECT id FROM jobs WHERE workspace_id = ? AND status = 'pending' ORDER BY created_at ASC LIMIT 1`,
		workspaceID,
	).Scan(&jobID)
	if err != nil {
		// No pending jobs.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec(
		`UPDATE jobs SET status = 'running', claimed_by = ? WHERE id = ? AND status = 'pending'`,
		req.DaemonID, jobID,
	)
	if err != nil {
		// Someone else claimed it first.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit")
		return
	}

	// Return the claimed job.
	var j protocol.Job
	var completedAt *string
	var sourcePathsRaw string
	err = h.DB.QueryRow(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE id = ?`, jobID,
	).Scan(&j.ID, &j.WorkspaceID, &j.Status, &j.AgentID,
		&j.SourcePath, &sourcePathsRaw, &j.SchemaID, &j.Progress, &j.Error, &j.ClaimedBy, &now, &completedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch job")
		return
	}
	j.CreatedAt = now
	j.CompletedAt = completedAt
	json.Unmarshal([]byte(sourcePathsRaw), &j.SourcePaths)
	if j.SourcePaths == nil {
		j.SourcePaths = []string{}
	}

	writeJSON(w, http.StatusOK, j)
}

// POST /api/workspaces/{slug}/jobs/{id}/progress — daemon updates job progress
func (h *Handler) UpdateJobProgress(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var body struct {
		Progress int `json:"progress"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_, err := h.DB.Exec(
		`UPDATE jobs SET progress = ? WHERE id = ? AND workspace_id = ?`,
		body.Progress, id, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update progress")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/workspaces/{slug}/jobs/{id}/complete — daemon marks job complete
func (h *Handler) CompleteJob(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := h.DB.Exec(
		`UPDATE jobs SET status = 'completed', progress = 100, completed_at = ? WHERE id = ? AND workspace_id = ?`,
		now, id, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/workspaces/{slug}/jobs/{id}/fail — daemon marks job failed
func (h *Handler) FailJob(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := h.DB.Exec(
		`UPDATE jobs SET status = 'failed', error = ?, completed_at = ? WHERE id = ? AND workspace_id = ?`,
		body.Error, now, id, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fail job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/workspaces/{slug}/jobs — list jobs
func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	rows, err := h.DB.Query(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE workspace_id = ? ORDER BY created_at DESC`, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}
	defer rows.Close()

	jobs := make([]protocol.Job, 0)
	for rows.Next() {
		var j protocol.Job
		var completedAt *string
		var sourcePathsRaw string
		if err := rows.Scan(&j.ID, &j.WorkspaceID, &j.Status, &j.AgentID,
			&j.SourcePath, &sourcePathsRaw, &j.SchemaID, &j.Progress, &j.Error, &j.ClaimedBy, &j.CreatedAt, &completedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan job")
			return
		}
		j.CompletedAt = completedAt
		json.Unmarshal([]byte(sourcePathsRaw), &j.SourcePaths)
		if j.SourcePaths == nil {
			j.SourcePaths = []string{}
		}
		jobs = append(jobs, j)
	}

	writeJSON(w, http.StatusOK, jobs)
}

// POST /api/workspaces/{slug}/jobs — create a new job
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req protocol.CreateJobRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required — jobs are now schema+agent driven")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	sourcePathsJSON, _ := json.Marshal(req.SourcePaths)
	if string(sourcePathsJSON) == "null" {
		sourcePathsJSON = []byte("[]")
	}

	var j protocol.Job
	var completedAt *string
	var sourcePathsRaw string
	err := h.DB.QueryRow(
		`INSERT INTO jobs (workspace_id, status, agent_id, source_path, source_paths, schema_id, claimed_by, created_at)
		 VALUES (?, 'pending', ?, ?, ?, ?, ?, ?)
		 RETURNING id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		           claimed_by, created_at, completed_at`,
		workspaceID, req.AgentID, req.SourcePath, string(sourcePathsJSON), req.SchemaID, req.ClaimedBy, now,
	).Scan(&j.ID, &j.WorkspaceID, &j.Status, &j.AgentID,
		&j.SourcePath, &sourcePathsRaw, &j.SchemaID, &j.Progress, &j.Error, &j.ClaimedBy, &j.CreatedAt, &completedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create job: %v", err))
		return
	}
	j.CompletedAt = completedAt
	json.Unmarshal([]byte(sourcePathsRaw), &j.SourcePaths)
	if j.SourcePaths == nil {
		j.SourcePaths = []string{}
	}

	writeJSON(w, http.StatusCreated, j)
}

// GET /api/workspaces/{slug}/jobs/{id} — get job status
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var j protocol.Job
	var completedAt *string
	var sourcePathsRaw string
	err := h.DB.QueryRow(
		`SELECT id, workspace_id, status, agent_id, source_path, source_paths, schema_id, progress, error,
		        claimed_by, created_at, completed_at
		 FROM jobs WHERE id = ? AND workspace_id = ?`, id, workspaceID,
	).Scan(&j.ID, &j.WorkspaceID, &j.Status, &j.AgentID,
		&j.SourcePath, &sourcePathsRaw, &j.SchemaID, &j.Progress, &j.Error, &j.ClaimedBy, &j.CreatedAt, &completedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	j.CompletedAt = completedAt
	json.Unmarshal([]byte(sourcePathsRaw), &j.SourcePaths)
	if j.SourcePaths == nil {
		j.SourcePaths = []string{}
	}

	writeJSON(w, http.StatusOK, j)
}

// POST /api/workspaces/{slug}/jobs/{id}/log-line — daemon pushes a log line
func (h *Handler) AppendJobLog(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	var body struct {
		Stream string `json:"stream"`
		Line   string `json:"line"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if h.LogBuf != nil {
		h.LogBuf.Append(id, body.Stream, body.Line)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/workspaces/{slug}/jobs/{id}/logs — stream logs via SSE
func (h *Handler) StreamJobLogs(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	// Verify job exists
	var status string
	err := h.DB.QueryRow(
		`SELECT status FROM jobs WHERE id = ? AND workspace_id = ?`, id, workspaceID,
	).Scan(&status)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Poll the job status every second, streaming status + log updates until terminal.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	lastStatus := ""
	logCursor := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Stream job status changes.
			var currentStatus string
			var progress int
			var jobError string
			var completedAt *string
			err := h.DB.QueryRow(
				`SELECT status, progress, error, completed_at FROM jobs WHERE id = ?`, id,
			).Scan(&currentStatus, &progress, &jobError, &completedAt)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", err.Error())
				flusher.Flush()
				return
			}

			if currentStatus != lastStatus {
				fmt.Fprintf(w, "event: status\ndata: {\"status\":\"%s\",\"progress\":%d,\"error\":\"%s\"}\n\n",
					currentStatus, progress, jobError)
				flusher.Flush()
				lastStatus = currentStatus
			}

			// Stream new log lines from the in-memory buffer.
			if h.LogBuf != nil {
				entries, nextCursor := h.LogBuf.Since(id, logCursor)
				for _, entry := range entries {
					data, _ := json.Marshal(entry)
					fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(data))
				}
				logCursor = nextCursor
				if len(entries) > 0 {
					flusher.Flush()
				}
			}

			if currentStatus == "completed" || currentStatus == "failed" {
				if h.LogBuf != nil {
					entries, _ := h.LogBuf.Since(id, logCursor)
					for _, entry := range entries {
						data, _ := json.Marshal(entry)
						fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(data))
					}
					if len(entries) > 0 {
						flusher.Flush()
					}
				}
				fmt.Fprintf(w, "event: done\ndata: {\"status\":\"%s\"}\n\n", currentStatus)
				flusher.Flush()
				return
			}
		}
	}
}

type pageOutput struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Type    string `json:"type"`
	Layer   string `json:"layer"`
	Content string `json:"content"`
}

func (h *Handler) SubmitJobOutput(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	repo, err := h.openRepo(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to open repo")
		return
	}

	var payload struct {
		JobID string       `json:"job_id"`
		Pages []pageOutput `json:"pages"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if payload.JobID != id {
		writeError(w, http.StatusBadRequest, "job_id mismatch")
		return
	}

	for _, p := range payload.Pages {
		wikiPath := "wiki/" + strings.TrimPrefix(p.Path, "/")
		content := p.Content
		if p.Title != "" && !strings.HasPrefix(content, "---") {
			content = fmt.Sprintf("---\ntitle: %s\ntype: %s\nlayer: %s\n---\n\n%s", p.Title, p.Type, p.Layer, p.Content)
		}
		if _, err := repo.WriteFile(wikiPath, []byte(content), fmt.Sprintf("job %s: %s", id, p.Path)); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("write %s: %v", p.Path, err))
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"pages_written": len(payload.Pages),
	})
}
