package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tethy/mulwiki/server/internal/middleware"
	"github.com/tethy/mulwiki/server/pkg/gitrepo"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// ── Source handlers (git-backed — no DB table) ──

// GET /api/workspaces/{slug}/sources
func (h *Handler) ListSources(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	repo, err := h.openRepo(r)
	if err != nil {
		writeJSON(w, http.StatusOK, []protocol.Source{})
		return
	}

	files, err := repo.ListFiles("sources/")
	if err != nil {
		writeJSON(w, http.StatusOK, []protocol.Source{})
		return
	}

	sources := make([]protocol.Source, 0, len(files))
	for _, f := range files {
		name := filepath.Base(f)
		var size int64
		if s, err2 := repo.FileSize(f); err2 == nil {
			size = s
		}
		sources = append(sources, protocol.Source{
			Name: name,
			Type: guessType(name),
			Path: f,
			Size: size,
		})
	}
	slog.Debug("listed sources", "workspace", slug, "count", len(sources))

	writeJSON(w, http.StatusOK, sources)
}

// GET /api/workspaces/{slug}/sources/{path:.*} — get source by path.
// If the path ends with "/raw", serve the raw file content instead of JSON metadata.
func (h *Handler) GetSource(w http.ResponseWriter, r *http.Request) {
	sourcePath := idParam(r, "*")
	if sourcePath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	// Check for /raw suffix.
	if strings.HasSuffix(sourcePath, "/raw") {
		h.serveSourceRaw(w, r, strings.TrimSuffix(sourcePath, "/raw"))
		return
	}

	if !strings.HasPrefix(sourcePath, "sources/") {
		sourcePath = "sources/" + sourcePath
	}

	repo, err := h.openRepo(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace repo not found")
		return
	}

	files, err := repo.ListFiles(sourcePath)
	if err != nil || len(files) == 0 {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}

	name := filepath.Base(sourcePath)
	var size int64
	if s, err2 := repo.FileSize(sourcePath); err2 == nil {
		size = s
	}

	writeJSON(w, http.StatusOK, protocol.Source{
		Name: name,
		Type: guessType(name),
		Path: sourcePath,
		Size: size,
	})
}

// serveSourceRaw handles GET .../sources/{path}/raw.
func (h *Handler) serveSourceRaw(w http.ResponseWriter, r *http.Request, sourcePath string) {
	if sourcePath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !strings.HasPrefix(sourcePath, "sources/") {
		sourcePath = "sources/" + sourcePath
	}

	repo, err := h.openRepo(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace repo not found")
		return
	}

	content, err := repo.ShowFile(sourcePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}

	contentType := detectMimeType(filepath.Base(sourcePath))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.Write(content)
}

// POST /api/workspaces/{slug}/sources — upload a file into git.
func (h *Handler) CreateSource(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	content := make([]byte, header.Size)
	n, err := file.Read(content)
	if err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}
	content = content[:n]

	relPath := "sources/" + header.Filename
	commitHash, err := h.commitFile(r, relPath, content, fmt.Sprintf("add: %s", header.Filename))
	if err != nil {
		slog.Error("git write file failed", "path", relPath, "error", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to store file: %v", err))
		return
	}

	slog.Info("source committed", "workspace", slug, "file", header.Filename, "commit", commitHash[:12])

	writeJSON(w, http.StatusCreated, protocol.Source{
		Name: header.Filename,
		Type: guessType(header.Filename),
		Path: relPath,
		Size: int64(n),
	})
}

// DELETE /api/workspaces/{slug}/sources/{path:.*}
func (h *Handler) DeleteSource(w http.ResponseWriter, r *http.Request) {
	sourcePath := idParam(r, "*")
	if sourcePath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !strings.HasPrefix(sourcePath, "sources/") {
		sourcePath = "sources/" + sourcePath
	}

	_, err := h.removeFile(r, sourcePath, fmt.Sprintf("remove: %s", filepath.Base(sourcePath)))
	if err != nil {
		slog.Error("git remove file failed", "path", sourcePath, "error", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to remove file: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /api/workspaces/{slug}/sources/{path:.*}/raw — serve raw file content from git.
func (h *Handler) GetSourceRaw(w http.ResponseWriter, r *http.Request) {
	// Chi wildcard captures "sources/filename/raw"
	rawPath := idParam(r, "*")
	sourcePath := strings.TrimSuffix(rawPath, "/raw")
	if sourcePath == rawPath || sourcePath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !strings.HasPrefix(sourcePath, "sources/") {
		sourcePath = "sources/" + sourcePath
	}

	repo, err := h.openRepo(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace repo not found")
		return
	}

	content, err := repo.ShowFile(sourcePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}

	contentType := detectMimeType(filepath.Base(sourcePath))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.Write(content)
}

// ── helpers ──

// openRepo opens the git repo for the workspace in the current request.
func (h *Handler) openRepo(r *http.Request) (*gitrepo.Repo, error) {
	wsID := getWorkspaceID(r)
	return gitrepo.Open(filepath.Join(h.reposDir(), wsID+".git"))
}

// commitFile writes a file to the git repo and commits it, then auto-pushes.
func (h *Handler) commitFile(r *http.Request, path string, content []byte, msg string) (string, error) {
	repo, err := h.openRepo(r)
	if err != nil {
		return "", err
	}
	hash, err := repo.WriteFile(path, content, msg)
	if err != nil {
		return "", err
	}
	_ = repo.Push() // best-effort
	return hash, nil
}

// removeFile removes a file from git and auto-pushes.
func (h *Handler) removeFile(r *http.Request, path string, msg string) (string, error) {
	repo, err := h.openRepo(r)
	if err != nil {
		return "", err
	}
	hash, err := repo.RemoveFile(path, msg)
	if err != nil {
		return "", err
	}
	_ = repo.Push() // best-effort
	return hash, nil
}

func guessType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".pdf":
		return "pdf"
	case ".txt":
		return "text"
	case ".csv":
		return "csv"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "image"
	case ".docx":
		return "docx"
	case ".pptx":
		return "pptx"
	case ".xlsx":
		return "xlsx"
	default:
		return "file"
	}
}

func detectMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// getWorkspaceID extracts workspace ID from request context.
func getWorkspaceID(r *http.Request) string {
	return middleware.GetWorkspaceID(r)
}
