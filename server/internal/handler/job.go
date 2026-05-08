package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tethy/mulwiki/server/internal/service"
	"github.com/tethy/mulwiki/server/internal/store"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// DaemonClaimRequest is the request body for claiming a job.
type DaemonClaimRequest struct {
	DaemonID string `json:"daemon_id"`
}

// POST /api/workspaces/{slug}/jobs/claim — daemon claims next pending job
func (h *Handler) ClaimJob(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req DaemonClaimRequest
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

	j, err := service.NewJobService(h.DB).ClaimJob(workspaceID, req.DaemonID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to claim job")
		return
	}
	if j == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, j)
}

// POST /api/workspaces/{slug}/jobs/{id}/progress — daemon updates job progress
func (h *Handler) UpdateJobProgress(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
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

	if err := service.NewJobService(h.DB).UpdateJobProgress(workspaceID, id, body.Progress); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update progress")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/workspaces/{slug}/jobs/{id}/complete — daemon marks job complete
func (h *Handler) CompleteJob(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	if err := service.NewJobService(h.DB).CompleteJob(workspaceID, id, 100); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to complete job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/workspaces/{slug}/jobs/{id}/fail — daemon marks job failed
func (h *Handler) FailJob(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
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

	if err := service.NewJobService(h.DB).FailJob(workspaceID, id, body.Error); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fail job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/workspaces/{slug}/jobs — list jobs
func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	jobs, err := store.NewJobStore(h.DB).ListByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	writeJSON(w, http.StatusOK, jobs)
}

// POST /api/workspaces/{slug}/jobs — create a new job
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
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

	j, err := service.NewJobServiceWithBus(h.DB, h.EventBus).CreateJob(service.CreateJobInput{
		WorkspaceID:  workspaceID,
		AgentID:      req.AgentID,
		SourcePath:   req.SourcePath,
		SourcePaths:  req.SourcePaths,
		SchemaID:     req.SchemaID,
		InitialClaim: req.ClaimedBy,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "agent not found in workspace")
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create job: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, j)
}

// GET /api/workspaces/{slug}/jobs/{id} — get job status
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	j, err := store.NewJobStore(h.DB).GetInWorkspace(r.Context(), workspaceID, id)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to load job")
			return
		}
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	writeJSON(w, http.StatusOK, j)
}

// POST /api/workspaces/{slug}/jobs/{id}/log-line — daemon pushes a log line
func (h *Handler) AppendJobLog(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var body struct {
		Stream string `json:"stream"`
		Line   string `json:"line"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var exists int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM jobs WHERE id = ? AND workspace_id = ?`, id, workspaceID).Scan(&exists); err != nil || exists == 0 {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	if h.LogBuf != nil {
		h.LogBuf.Append(id, body.Stream, body.Line)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/workspaces/{slug}/jobs/{id}/logs — stream logs via SSE
func (h *Handler) StreamJobLogs(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")

	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	// Verify job exists
	if _, err := store.NewJobStore(h.DB).GetInWorkspace(r.Context(), workspaceID, id); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to load job")
			return
		}
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
			err := h.DB.QueryRowContext(
				ctx,
				`SELECT status, progress, error, completed_at FROM jobs WHERE id = ? AND workspace_id = ?`,
				id, workspaceID,
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
	workspaceID, err := h.workspaceIDForRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	var exists int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM jobs WHERE id = ? AND workspace_id = ?`, id, workspaceID).Scan(&exists); err != nil || exists == 0 {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

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
