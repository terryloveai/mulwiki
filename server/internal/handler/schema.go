package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tethy/mulwiki/server/pkg/gitrepo"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// GET /api/workspaces/{slug}/schemas
// Returns all builtin schemas + workspace's own custom schemas, with is_active flag.
func (h *Handler) ListSchemas(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	rows, err := h.DB.Query(`
		SELECT s.id, s.workspace_id, s.name, s.description, s.version, s.path,
		       s.source_type, COALESCE(s.derived_from, '') as derived_from, s.created_at,
		       COALESCE((s.id = w.active_schema_id), 0) as is_active
		FROM schemas s
		JOIN workspaces w ON w.id = s.workspace_id
		WHERE w.id = ?
		ORDER BY s.source_type, s.created_at DESC
	`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list schemas")
		return
	}
	defer rows.Close()

	schemas := make([]protocol.SchemaWithActive, 0)
	for rows.Next() {
		var s protocol.SchemaWithActive
		var derivedFrom string
		var isActive int
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.Name, &s.Description, &s.Version,
			&s.Path, &s.SourceType, &derivedFrom, &s.CreatedAt, &isActive); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan schema")
			return
		}
		if derivedFrom != "" {
			s.DerivedFrom = &derivedFrom
		}
		s.IsActive = isActive != 0
		schemas = append(schemas, s)
	}

	writeJSON(w, http.StatusOK, schemas)
}

// GET /api/workspaces/{slug}/schemas/{id}
func (h *Handler) GetSchema(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var s protocol.Schema
	var derivedFrom string
	err := h.DB.QueryRow(`
		SELECT s.id, s.workspace_id, s.name, s.description, s.version, s.path,
		       s.source_type, COALESCE(s.derived_from, '') as derived_from, s.created_at
		FROM schemas s
		WHERE s.id = ? AND s.workspace_id = ?
	`, id, workspaceID).Scan(&s.ID, &s.WorkspaceID, &s.Name, &s.Description, &s.Version,
		&s.Path, &s.SourceType, &derivedFrom, &s.CreatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "schema not found")
		return
	}
	if derivedFrom != "" {
		s.DerivedFrom = &derivedFrom
	}

	// Read content — all schemas from git (including builtin)
	s.Content = h.readSchemaFromGit(workspaceID, s.Path)

	writeJSON(w, http.StatusOK, s)
}

// POST /api/workspaces/{slug}/schemas
func (h *Handler) CreateSchema(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req protocol.CreateSchemaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.Version == "" {
		req.Version = "1.0"
	}
	if req.SourceType == "" {
		req.SourceType = "user"
	}
	if req.SourceType == "builtin" {
		writeError(w, http.StatusBadRequest, "cannot create builtin schemas via API")
		return
	}

	if r.URL.Query().Get("skip_validation") != "true" {
		result := validateSchemaConfig(req.Content, h)
		if !result.Valid {
			writeJSON(w, http.StatusBadRequest, result)
			return
		}
	}

	// Insert metadata into DB
	var s protocol.Schema
	var derivedFrom string
	var derivedFromPtr any
	if req.DerivedFrom != nil {
		derivedFromPtr = *req.DerivedFrom
	}
	err := h.DB.QueryRow(`
		INSERT INTO schemas (workspace_id, name, description, version, path, source_type, derived_from, created_at)
		VALUES (?, ?, ?, ?, '', ?, ?, datetime('now'))
		RETURNING id, workspace_id, name, description, version, path,
		          source_type, COALESCE(derived_from, '') as derived_from, created_at`,
		workspaceID, req.Name, req.Description, req.Version, req.SourceType, derivedFromPtr,
	).Scan(&s.ID, &s.WorkspaceID, &s.Name, &s.Description, &s.Version, &s.Path,
		&s.SourceType, &derivedFrom, &s.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create schema")
		return
	}
	if derivedFrom != "" {
		s.DerivedFrom = &derivedFrom
	}

	// Write content to git
	gitPath := "schemas/" + s.ID + ".md"
	if _, err := h.writeSchemaToGit(workspaceID, gitPath, req.Content, "schema: "+req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write schema to git")
		return
	}

	// Update path in DB
	h.DB.Exec("UPDATE schemas SET path = ? WHERE id = ?", gitPath, s.ID)
	s.Path = gitPath
	s.Content = req.Content

	writeJSON(w, http.StatusCreated, s)
}

// PUT /api/workspaces/{slug}/schemas/{id}
func (h *Handler) UpdateSchema(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var sourceType, currentPath string
	if err := h.DB.QueryRow(
		`SELECT source_type, path FROM schemas WHERE id = ? AND workspace_id = ?`, id, workspaceID,
	).Scan(&sourceType, &currentPath); err != nil {
		writeError(w, http.StatusNotFound, "schema not found or not owned by this workspace")
		return
	}
	if sourceType == "builtin" {
		writeError(w, http.StatusForbidden, "builtin schemas cannot be edited — fork to create a custom copy")
		return
	}

	var req protocol.UpdateSchemaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	setClauses := make([]string, 0)
	args := make([]any, 0)

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Version != nil {
		setClauses = append(setClauses, "version = ?")
		args = append(args, *req.Version)
	}

	// Content update → git first, then DB
	if req.Content != nil {
		if r.URL.Query().Get("skip_validation") != "true" {
			result := validateSchemaConfig(*req.Content, h)
			if !result.Valid {
				writeJSON(w, http.StatusBadRequest, result)
				return
			}
		}
		if currentPath != "" {
			if _, err := h.writeSchemaToGit(workspaceID, currentPath, *req.Content, "schema: update "+id); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to update schema in git")
				return
			}
		}
	}

	if len(setClauses) == 0 && req.Content == nil {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	var s protocol.Schema
	var derivedFrom string
	var err error
	if len(setClauses) > 0 {
		args = append(args, id, workspaceID)
		query := fmt.Sprintf(
			`UPDATE schemas SET %s WHERE id = ? AND workspace_id = ?
			 RETURNING id, workspace_id, name, description, version, path,
			           source_type, COALESCE(derived_from, '') as derived_from, created_at`,
			strings.Join(setClauses, ", "),
		)
		err = h.DB.QueryRow(query, args...).Scan(
			&s.ID, &s.WorkspaceID, &s.Name, &s.Description, &s.Version, &s.Path,
			&s.SourceType, &derivedFrom, &s.CreatedAt,
		)
	} else {
		err = h.DB.QueryRow(
			`SELECT id, workspace_id, name, description, version, path,
			        source_type, COALESCE(derived_from, '') as derived_from, created_at
			 FROM schemas WHERE id = ? AND workspace_id = ?`,
			id, workspaceID,
		).Scan(
			&s.ID, &s.WorkspaceID, &s.Name, &s.Description, &s.Version, &s.Path,
			&s.SourceType, &derivedFrom, &s.CreatedAt,
		)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update schema")
		return
	}
	if derivedFrom != "" {
		s.DerivedFrom = &derivedFrom
	}

	if s.Path != "" {
		s.Content = h.readSchemaFromGit(workspaceID, s.Path)
	}

	writeJSON(w, http.StatusOK, s)
}

// DELETE /api/workspaces/{slug}/schemas/{id}
func (h *Handler) DeleteSchema(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)
	id := idParam(r, "id")

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var sourceType, path string
	if err := h.DB.QueryRow(
		`SELECT source_type, path FROM schemas WHERE id = ? AND workspace_id = ?`, id, workspaceID,
	).Scan(&sourceType, &path); err != nil {
		writeError(w, http.StatusNotFound, "schema not found or not owned by this workspace")
		return
	}
	if sourceType == "builtin" {
		writeError(w, http.StatusForbidden, "builtin schemas cannot be deleted")
		return
	}

	// Clear active schema references
	h.DB.Exec(`UPDATE workspaces SET active_schema_id = NULL, active_schema_path = '' WHERE id = ? AND active_schema_id = ?`,
		workspaceID, id)

	// Remove from git
	if path != "" {
		repo, err := h.openRepoByWSID(workspaceID)
		if err == nil {
			_, _ = repo.RemoveFile(path, fmt.Sprintf("schema delete: %s", id))
		}
	}

	result, err := h.DB.Exec(`DELETE FROM schemas WHERE id = ? AND workspace_id = ?`, id, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete schema")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "schema not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// POST /api/workspaces/{slug}/schemas/fork
func (h *Handler) ForkSchema(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req protocol.ForkSchemaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SchemaID == "" {
		writeError(w, http.StatusBadRequest, "schema_id is required")
		return
	}

	// Read source schema metadata
	var srcName, srcDesc, srcVersion, srcPath, srcWorkspaceID string
	err := h.DB.QueryRow(
		`SELECT name, description, version, path, workspace_id FROM schemas WHERE id = ?`,
		req.SchemaID,
	).Scan(&srcName, &srcDesc, &srcVersion, &srcPath, &srcWorkspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "source schema not found")
		return
	}

	// Read source content from git
	content := h.readSchemaFromGit(srcWorkspaceID, srcPath)
	if content == "" {
		writeError(w, http.StatusNotFound, "source schema content not found in git")
		return
	}

	name := srcName
	if req.Name != "" {
		name = req.Name
	} else {
		name = name + " (fork)"
	}
	description := srcDesc
	if req.Description != "" {
		description = req.Description
	}

	// Insert metadata
	var s protocol.Schema
	var newDerivedFrom string
	err = h.DB.QueryRow(`
		INSERT INTO schemas (workspace_id, name, description, version, path, source_type, derived_from, created_at)
		VALUES (?, ?, ?, ?, '', 'user', ?, datetime('now'))
		RETURNING id, workspace_id, name, description, version, path,
		          source_type, COALESCE(derived_from, '') as derived_from, created_at`,
		workspaceID, name, description, srcVersion, req.SchemaID,
	).Scan(&s.ID, &s.WorkspaceID, &s.Name, &s.Description, &s.Version, &s.Path,
		&s.SourceType, &newDerivedFrom, &s.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fork schema")
		return
	}
	if newDerivedFrom != "" {
		s.DerivedFrom = &newDerivedFrom
	}

	// Write forked content to git
	gitPath := "schemas/" + s.ID + ".md"
	if _, err := h.writeSchemaToGit(workspaceID, gitPath, content, "schema fork: "+name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write forked schema to git")
		return
	}

	h.DB.Exec("UPDATE schemas SET path = ? WHERE id = ?", gitPath, s.ID)
	s.Path = gitPath
	s.Content = content

	writeJSON(w, http.StatusCreated, s)
}

// PUT /api/workspaces/{slug}/activate-schema
func (h *Handler) ActivateSchema(w http.ResponseWriter, r *http.Request) {
	slug := workspaceSlug(r)

	var workspaceID string
	if err := h.DB.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var req protocol.ActivateSchemaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SchemaID == "" {
		writeError(w, http.StatusBadRequest, "schema_id is required")
		return
	}

	var schemaPath string
	if err := h.DB.QueryRow(
		`SELECT path FROM schemas WHERE id = ? AND workspace_id = ?`,
		req.SchemaID, workspaceID,
	).Scan(&schemaPath); err != nil {
		writeError(w, http.StatusNotFound, "schema not found")
		return
	}

	_, err := h.DB.Exec(
		`UPDATE workspaces SET active_schema_id = ?, active_schema_path = ? WHERE id = ?`,
		req.SchemaID, schemaPath, workspaceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to activate schema")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"active_schema_id":   req.SchemaID,
		"active_schema_path": schemaPath,
	})
}

// POST /api/workspaces/{slug}/schemas/validate
func (h *Handler) ValidateSchema(w http.ResponseWriter, r *http.Request) {
	var req protocol.ValidateSchemaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeJSON(w, http.StatusOK, protocol.ValidateSchemaResponse{
			Valid:  false,
			Errors: []string{"content is empty"},
		})
		return
	}

	result := validateSchemaConfig(req.Content, h)
	writeJSON(w, http.StatusOK, result)
}

// GET /api/schemas/builtin
func (h *Handler) ListBuiltinSchemas(w http.ResponseWriter, r *http.Request) {
	schemas, err := h.loadBuiltinSchemaCatalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list builtin schemas")
		return
	}

	writeJSON(w, http.StatusOK, schemas)
}

// ── Git operations ──

func (h *Handler) openRepoByWSID(workspaceID string) (*gitrepo.Repo, error) {
	return gitrepo.Open(filepath.Join(h.reposDir(), workspaceID+".git"))
}

func (h *Handler) writeSchemaToGit(workspaceID, gitPath, content, msg string) (string, error) {
	repo, err := h.openRepoByWSID(workspaceID)
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}
	hash, err := repo.WriteFile(gitPath, []byte(content), msg)
	if err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return hash, nil
}

func (h *Handler) readSchemaFromGit(workspaceID, gitPath string) string {
	if gitPath == "" {
		return ""
	}
	repo, err := h.openRepoByWSID(workspaceID)
	if err != nil {
		slog.Warn("read schema from git: repo not found", "ws_id", workspaceID, "error", err)
		return ""
	}
	content, err := repo.ShowFile(gitPath)
	if err != nil {
		slog.Warn("read schema from git: show file failed", "path", gitPath, "error", err)
		return ""
	}
	return string(content)
}

// builtinSchemaMeta maps schema filenames to display names.
var builtinSchemaMeta = map[string]string{
	"karpathy-llm-wiki-schema.md":  "Karpathy 原始",
	"concept-wiki-schema.md":       "concept-wiki (9层本体)",
	"nashsu-llm-wiki-schema.md":    "nashsu (CoT + 知识图谱)",
	"llm-knowledge-base-schema.md": "llm-knowledge-base (极简3类)",
	"paper-spec-wiki-schema.md":    "paper-spec wiki (学术7类)",
	"paper-spec-paper-schema.md":   "paper-spec paper (论文剖面)",
}

const (
	initialSchemaTypeBlank   = "blank"
	initialSchemaTypeBuiltin = "builtin"
	blankSchemaPath          = "schemas/blank-schema.md"
)

const blankSchemaContent = `# Blank Schema

## Types

## Structure

## Frontmatter

## Ingest Pipeline

## Lint Rules
`

func (h *Handler) loadBuiltinSchemaCatalog() ([]protocol.Schema, error) {
	dir := h.builtinSchemasDir()
	if dir == "" {
		return []protocol.Schema{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []protocol.Schema{}, nil
		}
		return nil, fmt.Errorf("read builtin schemas dir: %w", err)
	}

	schemas := make([]protocol.Schema, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read builtin schema %s: %w", entry.Name(), err)
		}

		name := entry.Name()
		if label, ok := builtinSchemaMeta[entry.Name()]; ok {
			name = label
		}

		schemas = append(schemas, protocol.Schema{
			ID:          entry.Name(),
			WorkspaceID: "builtin",
			Name:        name,
			Description: "",
			Version:     "1.0",
			Path:        "schemas/" + entry.Name(),
			Content:     string(content),
			SourceType:  "builtin",
		})
	}

	return schemas, nil
}

func builtinSchemaWorkspacePath(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	name = strings.TrimPrefix(name, "schemas/")
	if name == "" || strings.ContainsAny(name, `/\`) || !strings.HasSuffix(name, ".md") {
		return "", false
	}
	return "schemas/" + name, true
}

func builtinSchemaFilename(workspacePath string) (string, bool) {
	path, ok := builtinSchemaWorkspacePath(workspacePath)
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(path, "schemas/"), true
}

func (h *Handler) hasBuiltinSchema(workspacePath string) bool {
	filename, ok := builtinSchemaFilename(workspacePath)
	if !ok {
		return false
	}
	info, err := os.Stat(filepath.Join(h.builtinSchemasDir(), filename))
	return err == nil && !info.IsDir()
}

// SeedBuiltinSchemas writes all builtin schema .md files into the workspace git repo
// and inserts corresponding DB records. Idempotent — skips if already seeded.
func (h *Handler) SeedBuiltinSchemas(workspaceID string) error {
	// Check if already seeded.
	var count int
	if err := h.DB.QueryRow(
		`SELECT COUNT(*) FROM schemas WHERE workspace_id = ? AND source_type = 'builtin'`,
		workspaceID,
	).Scan(&count); err != nil {
		return fmt.Errorf("check seeded: %w", err)
	}
	if count > 0 {
		return nil
	}

	repo, err := h.openRepoByWSID(workspaceID)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	schemas, err := h.loadBuiltinSchemaCatalog()
	if err != nil {
		return err
	}

	for _, schema := range schemas {
		if _, err := repo.WriteFile(schema.Path, []byte(schema.Content), fmt.Sprintf("seed builtin schema: %s", schema.ID)); err != nil {
			slog.Warn("write builtin schema to git", "file", schema.ID, "error", err)
			continue
		}

		if _, err := h.DB.Exec(
			`INSERT INTO schemas (workspace_id, name, description, version, path, source_type)
			 VALUES (?, ?, '', '1.0', ?, 'builtin')`,
			workspaceID, schema.Name, schema.Path,
		); err != nil {
			slog.Warn("insert builtin schema record", "file", schema.ID, "error", err)
		}
	}

	return nil
}

func (h *Handler) activateSchemaByPath(workspaceID, schemaPath string) error {
	var schemaID string
	if err := h.DB.QueryRow(
		`SELECT id FROM schemas WHERE workspace_id = ? AND path = ?`,
		workspaceID, schemaPath,
	).Scan(&schemaID); err != nil {
		return fmt.Errorf("find schema by path: %w", err)
	}
	if _, err := h.DB.Exec(
		`UPDATE workspaces SET active_schema_id = ?, active_schema_path = ? WHERE id = ?`,
		schemaID, schemaPath, workspaceID,
	); err != nil {
		return fmt.Errorf("activate schema: %w", err)
	}
	return nil
}

func (h *Handler) createBlankSchema(workspaceID string) error {
	if _, err := h.writeSchemaToGit(workspaceID, blankSchemaPath, blankSchemaContent, "create blank schema"); err != nil {
		return err
	}

	var schemaID string
	if err := h.DB.QueryRow(
		`INSERT INTO schemas (workspace_id, name, description, version, path, source_type)
		 VALUES (?, 'Blank Schema', '', '1.0', ?, 'user')
		 RETURNING id`,
		workspaceID, blankSchemaPath,
	).Scan(&schemaID); err != nil {
		return fmt.Errorf("insert blank schema: %w", err)
	}

	if _, err := h.DB.Exec(
		`UPDATE workspaces SET active_schema_id = ?, active_schema_path = ? WHERE id = ?`,
		schemaID, blankSchemaPath, workspaceID,
	); err != nil {
		return fmt.Errorf("activate blank schema: %w", err)
	}
	return nil
}

// ── Schema Validator ──

func validateSchemaConfig(content string, h *Handler) protocol.ValidateSchemaResponse {
	result := protocol.ValidateSchemaResponse{Valid: true}

	if !strings.Contains(content, "#") {
		result.Valid = false
		result.Errors = append(result.Errors, "No headings found — schema must have at least one section")
		return result
	}

	hasTypes := containsSection(content, "Types") || containsSection(content, "types") || containsSection(content, "节点类型")
	hasStructure := containsSection(content, "Structure") || containsSection(content, "structure") || containsSection(content, "层级")
	hasFrontmatter := containsSection(content, "Frontmatter") || containsSection(content, "frontmatter") || containsSection(content, "元数据")

	if !hasTypes {
		result.Errors = append(result.Errors, "Missing Types section — define node types for the schema")
		result.Valid = false
	}
	if !hasStructure {
		result.Errors = append(result.Errors, "Missing Structure section — define the hierarchy/layer rules")
		result.Valid = false
	}
	if !hasFrontmatter {
		result.Warnings = append(result.Warnings, "Missing Frontmatter section — consider adding metadata field specifications")
	}

	if strings.TrimSpace(content)[0] == '{' {
		result.Warnings = append(result.Warnings, "Content appears to be JSON — schema content should be Markdown")
	}

	return result
}

func containsSection(markdown, keyword string) bool {
	lines := strings.Split(markdown, "\n")
	kw := strings.ToLower(keyword)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			if strings.Contains(strings.ToLower(trimmed), kw) {
				return true
			}
		}
	}
	return false
}
