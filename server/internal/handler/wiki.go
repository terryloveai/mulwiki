package handler

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// ── Wiki page handlers (git-backed — markdown files with frontmatter) ──
//
// Wiki pages are stored as wiki/{path}.md in the git repo.
// Frontmatter (YAML-like) carries title, type, layer:
//
//   ---
//   title: Getting Started
//   type: concept
//   layer: 1
//   ---
//   # Getting Started
//   Content here...

// GET /api/workspaces/{slug}/wiki — list all wiki pages.
func (h *Handler) ListWikiPages(w http.ResponseWriter, r *http.Request) {
	repo, err := h.openRepo(r)
	if err != nil {
		writeJSON(w, http.StatusOK, []protocol.WikiPage{})
		return
	}

	files, err := repo.ListFiles("wiki/")
	if err != nil {
		writeJSON(w, http.StatusOK, []protocol.WikiPage{})
		return
	}

	pages := make([]protocol.WikiPage, 0, len(files))
	for _, f := range files {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		// Path from git: "wiki/concepts/hello.md" → "/concepts/hello"
		wikiPath := strings.TrimPrefix(strings.TrimSuffix(f, ".md"), "wiki")
		if wikiPath == "" {
			wikiPath = "/"
		}

		content, err := repo.ShowFile(f)
		if err != nil {
			slog.Warn("failed to read wiki page", "path", f, "error", err)
			continue
		}

		fm, body := parseFrontmatter(string(content))
		title := fm["title"]
		if title == "" {
			title = filepath.Base(wikiPath)
		}
		pageType := fm["type"]
		if pageType == "" {
			pageType = "page"
		}
		layer := fm["layer"]

		pages = append(pages, protocol.WikiPage{
			Path:    wikiPath,
			Title:   title,
			Content: body,
			Type:    pageType,
			Layer:   layer,
		})
	}

	writeJSON(w, http.StatusOK, pages)
}

// GET /api/workspaces/{slug}/wiki/* — get page by path.
func (h *Handler) GetWikiPage(w http.ResponseWriter, r *http.Request) {
	pagePath := idParam(r, "*")
	if pagePath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !strings.HasPrefix(pagePath, "/") {
		pagePath = "/" + pagePath
	}

	gitPath := "wiki" + pagePath + ".md"

	repo, err := h.openRepo(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace repo not found")
		return
	}

	content, err := repo.ShowFile(gitPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "wiki page not found")
		return
	}

	fm, body := parseFrontmatter(string(content))
	title := fm["title"]
	if title == "" {
		title = filepath.Base(pagePath)
	}
	pageType := fm["type"]
	if pageType == "" {
		pageType = "page"
	}
	layer := fm["layer"]

	writeJSON(w, http.StatusOK, protocol.WikiPage{
		Path:    pagePath,
		Title:   title,
		Content: body,
		Type:    pageType,
		Layer:   layer,
	})
}

// POST /api/workspaces/{slug}/wiki — create a wiki page.
func (h *Handler) CreateWikiPage(w http.ResponseWriter, r *http.Request) {
	var req protocol.CreateWikiPageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
	}

	// Normalize path.
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}
	req.Path = strings.TrimSuffix(req.Path, "/")

	// Build markdown with frontmatter.
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("title: %s\n", req.Title))
	buf.WriteString(fmt.Sprintf("type: %s\n", req.Type))
	if req.Layer != "" {
		buf.WriteString(fmt.Sprintf("layer: %s\n", req.Layer))
	}
	buf.WriteString("---\n")
	if req.Content != "" {
		buf.WriteString("\n")
		buf.WriteString(req.Content)
		if !strings.HasSuffix(req.Content, "\n") {
			buf.WriteString("\n")
		}
	}

	gitPath := "wiki" + req.Path + ".md"
	commitHash, err := h.commitFile(r, gitPath, buf.Bytes(), fmt.Sprintf("wiki: %s", req.Path))
	if err != nil {
		slog.Error("git write wiki failed", "path", gitPath, "error", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to store wiki page: %v", err))
		return
	}

	slog.Info("wiki page committed", "path", req.Path, "commit", commitHash[:12])

	writeJSON(w, http.StatusCreated, protocol.WikiPage{
		Path:    req.Path,
		Title:   req.Title,
		Content: req.Content,
		Type:    req.Type,
		Layer:   req.Layer,
	})
}

// DELETE /api/workspaces/{slug}/wiki/* — delete a wiki page.
func (h *Handler) DeleteWikiPage(w http.ResponseWriter, r *http.Request) {
	pagePath := idParam(r, "*")
	if !strings.HasPrefix(pagePath, "/") {
		pagePath = "/" + pagePath
	}
	gitPath := "wiki" + pagePath + ".md"

	_, err := h.removeFile(r, gitPath, fmt.Sprintf("wiki rm: %s", pagePath))
	if err != nil {
		slog.Error("git remove wiki failed", "path", gitPath, "error", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete wiki page: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /api/workspaces/{slug}/wiki/search?q=... — full-text search across wiki pages.
func (h *Handler) SearchWikiPages(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []protocol.WikiPage{})
		return
	}

	repo, err := h.openRepo(r)
	if err != nil {
		writeJSON(w, http.StatusOK, []protocol.WikiPage{})
		return
	}

	files, err := repo.ListFiles("wiki/")
	if err != nil {
		writeJSON(w, http.StatusOK, []protocol.WikiPage{})
		return
	}

	var results []protocol.WikiPage
	for _, f := range files {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		content, err := repo.ShowFile(f)
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(content))
		if !strings.Contains(lower, q) {
			continue
		}

		wikiPath := strings.TrimPrefix(strings.TrimSuffix(f, ".md"), "wiki")
		fm, body := parseFrontmatter(string(content))
		title := fm["title"]
		if title == "" {
			title = filepath.Base(wikiPath)
		}
		pageType := fm["type"]
		if pageType == "" {
			pageType = "page"
		}
		layer := fm["layer"]

		// Return snippet (first 300 chars of matching content).
		snippet := body
		idx := strings.Index(lower, q)
		if idx > 60 {
			snippet = "..." + body[max(0, idx-60):min(len(body), idx+len(q)+240)]
		} else if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}

		results = append(results, protocol.WikiPage{
			Path:    wikiPath,
			Title:   title,
			Content: snippet,
			Type:    pageType,
			Layer:   layer,
		})
	}

	writeJSON(w, http.StatusOK, results)
}

// GET /api/workspaces/{slug}/wiki/export — export wiki markdown as a zip.
func (h *Handler) ExportWiki(w http.ResponseWriter, r *http.Request) {
	repo, err := h.openRepo(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace repo not found")
		return
	}
	files, err := repo.ListFiles("wiki/")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list wiki files")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="wiki.zip"`)
	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, file := range files {
		if !strings.HasSuffix(file, ".md") {
			continue
		}
		content, err := repo.ShowFile(file)
		if err != nil {
			continue
		}
		entry, err := zw.Create(strings.TrimPrefix(file, "wiki/"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create zip entry")
			return
		}
		if _, err := entry.Write(content); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to write zip entry")
			return
		}
	}
}

// ── frontmatter parser ──

// ── Wikilinks & Backlinks ──

// ResolveWikiLinksRequest is the request body for POST /wiki/resolve-links.
type ResolveWikiLinksRequest struct {
	Paths []string `json:"paths"`
}

// WikiLinkResolve tells whether a wikilink target exists and its canonical path.
type WikiLinkResolve struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path,omitempty"` // canonical page path if exists
}

// ResolveWikiLinksResponse returns which wiki paths exist, with canonical paths.
type ResolveWikiLinksResponse struct {
	Resolved map[string]WikiLinkResolve `json:"resolved"` // request path → resolve info
}

// POST /api/workspaces/{slug}/wiki/resolve-links
// Batch-check which [[wikilink]] targets resolve to existing pages.
func (h *Handler) ResolveWikiLinks(w http.ResponseWriter, r *http.Request) {
	var req ResolveWikiLinksRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	repo, err := h.openRepo(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace repo not found")
		return
	}

	// List all wiki/ .md files, build existence lookup.
	allFiles, err := repo.ListFiles("wiki/")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list wiki files")
		return
	}

	// Build lookup: each known path alias → canonical full path.
	type pageInfo struct {
		fullPath string // e.g. "/concepts/ai"
		baseName string // e.g. "ai"
	}
	pages := make([]pageInfo, 0, len(allFiles))
	canonicalForPath := make(map[string]string) // "dbtest" → "/concepts/dbtest"

	for _, f := range allFiles {
		canonical := "/" + strings.TrimSuffix(strings.TrimPrefix(f, "wiki/"), ".md")

		// Exact path
		canonicalForPath[canonical] = canonical
		// Without leading slash
		trimmed := strings.TrimPrefix(canonical, "/")
		if trimmed != canonical {
			canonicalForPath[trimmed] = canonical
		}

		// Basename (only if unique — we'll de-duplicate later)
		base := filepath.Base(canonical)
		pages = append(pages, pageInfo{fullPath: canonical, baseName: base})
	}

	// Deduplicate basename collisions: only add to canonicalForPath if unique.
	baseCount := make(map[string]int)
	for _, pi := range pages {
		baseCount[pi.baseName]++
	}
	for _, pi := range pages {
		if baseCount[pi.baseName] == 1 {
			canonicalForPath[pi.baseName] = pi.fullPath
		}
	}

	resolved := make(map[string]WikiLinkResolve, len(req.Paths))
	for _, p := range req.Paths {
		if cp, ok := canonicalForPath[p]; ok {
			resolved[p] = WikiLinkResolve{Exists: true, Path: cp}
		} else {
			resolved[p] = WikiLinkResolve{Exists: false}
		}
	}

	writeJSON(w, http.StatusOK, ResolveWikiLinksResponse{Resolved: resolved})
}

// GET /api/workspaces/{slug}/wiki/backlinks?path=...
// Returns pages whose content contains [[path]] references to the target page.
func (h *Handler) GetWikiBacklinks(w http.ResponseWriter, r *http.Request) {
	targetPath := r.URL.Query().Get("path")
	if targetPath == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}

	repo, err := h.openRepo(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace repo not found")
		return
	}

	allFiles, err := repo.ListFiles("wiki/")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list wiki files")
		return
	}

	// Patterns to search for: [[path]], [[path|...]], and basename variants
	normalized := strings.TrimPrefix(targetPath, "/")
	base := filepath.Base(normalized)
	patterns := []string{
		"[[" + targetPath + "]]",
		"[[" + targetPath + "|",
		"[[" + normalized + "]]",
		"[[" + normalized + "|",
	}
	if base != normalized {
		patterns = append(patterns,
			"[["+base+"]]",
			"[["+base+"|",
		)
	}

	var backlinks []protocol.WikiBacklink
	for _, f := range allFiles {
		pagePath := "/" + strings.TrimSuffix(strings.TrimPrefix(f, "wiki/"), ".md")
		if pagePath == targetPath || pagePath == "/"+normalized {
			continue // skip self-references
		}

		content, err := repo.ShowFile(f)
		if err != nil {
			continue
		}

		body := string(content)
		for _, p := range patterns {
			if idx := strings.Index(body, p); idx != -1 {
				// Extract a snippet around the match.
				start := idx
				if start > 40 {
					start = idx - 40
				}
				end := idx + len(p) + 60
				if end > len(body) {
					end = len(body)
				}
				snippet := body[start:end]
				if start > 0 {
					snippet = "…" + snippet
				}
				if end < len(body) {
					snippet = snippet + "…"
				}

				backlinks = append(backlinks, protocol.WikiBacklink{
					Path:    pagePath,
					Title:   extractTitle(body),
					Snippet: strings.TrimSpace(snippet),
				})
				break
			}
		}
	}

	writeJSON(w, http.StatusOK, backlinks)
}

// extractTitle pulls the title frontmatter key from markdown content, fast.
func extractTitle(content string) string {
	lines := strings.SplitN(content, "\n", 10)
	inFM := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "---" {
			inFM = !inFM
			continue
		}
		if inFM && strings.HasPrefix(t, "title:") {
			return strings.TrimSpace(strings.TrimPrefix(t, "title:"))
		}
	}
	return ""
}

// parseFrontmatter extracts YAML-like frontmatter from markdown content.
// Returns a map of key-value pairs and the remaining body.
func parseFrontmatter(content string) (map[string]string, string) {
	fm := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return fm, content
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if key != "" {
				fm[key] = val
			}
		}
	}

	// The remaining lines are the body.
	remaining := ""
	for scanner.Scan() {
		remaining += scanner.Text() + "\n"
	}
	return fm, strings.TrimLeft(remaining, "\n")
}
