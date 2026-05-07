package protocol

import "encoding/json"

// Workspace models.

type Workspace struct {
	ID              string  `json:"id"`
	Slug            string  `json:"slug"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	ActiveSchemaPath *string `json:"active_schema_path,omitempty"`
	ActiveSchemaID   *string `json:"active_schema_id,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

type CreateWorkspaceRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	GitRemote   string `json:"git_remote,omitempty"` // optional GitHub/remote URL
}

type UpdateWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Schema models.

type Schema struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Version     string  `json:"version"`
	Path        string  `json:"path"`
	Content     string  `json:"content,omitempty"`
	SourceType  string  `json:"source_type"`
	DerivedFrom *string `json:"derived_from,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

type SchemaWithActive struct {
	Schema
	IsActive bool `json:"is_active"`
}

type CreateSchemaRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Version     string  `json:"version"`
	Content     string  `json:"content"`
	SourceType  string  `json:"source_type"`
	DerivedFrom *string `json:"derived_from,omitempty"`
}

type UpdateSchemaRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Version     *string `json:"version,omitempty"`
	Content     *string `json:"content,omitempty"`
}

type ForkSchemaRequest struct {
	SchemaID    string `json:"schema_id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type ActivateSchemaRequest struct {
	SchemaID string `json:"schema_id"`
}

type ValidateSchemaRequest struct {
	Content string `json:"content"`
}

type ValidateSchemaResponse struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// User models.

type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Source models (git-backed — file_path is the primary identifier).

type Source struct {
	Name    string `json:"name"`     // basename, e.g. "doc.pdf"
	Type    string `json:"type"`     // inferred from extension
	Path    string `json:"path"`     // git path, e.g. "sources/doc.pdf"
	Size    int64  `json:"size"`     // bytes from git cat-file -s
}

// Wiki page models (git-backed — path.md files with frontmatter).

type WikiPage struct {
	Path    string `json:"path"`     // e.g. "/concepts/ai"
	Title   string `json:"title"`
	Content string `json:"content"`  // body (without frontmatter)
	Type    string `json:"type"`
	Layer   string `json:"layer,omitempty"`
}

type CreateWikiPageRequest struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Type    string `json:"type"`
	Layer   string `json:"layer,omitempty"`
}

// WikiBacklink represents a page that links to another wiki page.
type WikiBacklink struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// Job models.

type Job struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspace_id"`
	Status      string   `json:"status"`
	AgentID     string   `json:"agent_id"`
	SourcePath  string   `json:"source_path"`
	SourcePaths []string `json:"source_paths"`
	SchemaID    string   `json:"schema_id"`
	Progress    int      `json:"progress"`
	Error       string   `json:"error"`
	ClaimedBy   string   `json:"claimed_by"`
	CreatedAt   string   `json:"created_at"`
	CompletedAt *string  `json:"completed_at,omitempty"`
}

type CreateJobRequest struct {
	AgentID   string   `json:"agent_id"`
	SourcePath  string   `json:"source_path"`
	SourcePaths []string `json:"source_paths"`
	SchemaID  string   `json:"schema_id"`
	ClaimedBy string   `json:"claimed_by,omitempty"`
}

// Agent models.

type AgentRuntime struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	Name          string `json:"name"`
	Backend       string `json:"backend"`
	Path          string `json:"path"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Version       string `json:"version"`
	Status        string `json:"status"`
	DaemonID      string `json:"daemon_id"`
	LastHeartbeat string `json:"last_heartbeat"`
	CreatedAt     string `json:"created_at"`
}

type CreateRuntimeRequest struct {
	Name     string `json:"name"`
	Backend  string `json:"backend"`
	Path     string `json:"path"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Version  string `json:"version"`
}

type UpdateRuntimeRequest struct {
	Name     *string `json:"name,omitempty"`
	Backend  *string `json:"backend,omitempty"`
	Path     *string `json:"path,omitempty"`
	Hostname *string `json:"hostname,omitempty"`
	OS       *string `json:"os,omitempty"`
	Version  *string `json:"version,omitempty"`
}

type Agent struct {
	ID                 string            `json:"id"`
	WorkspaceID        string            `json:"workspace_id"`
	RuntimeID          string            `json:"runtime_id"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Instructions       string            `json:"instructions"`
	RuntimeMode        string            `json:"runtime_mode"`
	RuntimeConfig      json.RawMessage   `json:"runtime_config"`
	CustomEnv          map[string]string `json:"custom_env"`
	CustomArgs         []string          `json:"custom_args"`
	McpConfig          json.RawMessage   `json:"mcp_config"`
	Visibility         string            `json:"visibility"`
	Status             string            `json:"status"`
	MaxConcurrentTasks int               `json:"max_concurrent_tasks"`
	Model              string            `json:"model"`
	OwnerID            string            `json:"owner_id"`
	Skills             []AgentSkill      `json:"skills"`
	CreatedAt          string            `json:"created_at"`
	UpdatedAt          string            `json:"updated_at"`
	ArchivedAt         *string           `json:"archived_at,omitempty"`
	ArchivedBy         *string           `json:"archived_by,omitempty"`
}

type AgentCreateRequest struct {
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Instructions       string            `json:"instructions"`
	RuntimeID          string            `json:"runtime_id"`
	RuntimeConfig      json.RawMessage   `json:"runtime_config"`
	CustomEnv          map[string]string `json:"custom_env"`
	CustomArgs         []string          `json:"custom_args"`
	McpConfig          json.RawMessage   `json:"mcp_config"`
	Visibility         string            `json:"visibility"`
	MaxConcurrentTasks int               `json:"max_concurrent_tasks"`
	Model              string            `json:"model"`
}

type AgentUpdateRequest struct {
	Name               *string            `json:"name,omitempty"`
	Description        *string            `json:"description,omitempty"`
	Instructions       *string            `json:"instructions,omitempty"`
	RuntimeID          *string            `json:"runtime_id,omitempty"`
	RuntimeConfig      json.RawMessage    `json:"runtime_config,omitempty"`
	CustomEnv          *map[string]string `json:"custom_env,omitempty"`
	CustomArgs         *[]string          `json:"custom_args,omitempty"`
	McpConfig          json.RawMessage    `json:"mcp_config,omitempty"`
	Visibility         *string            `json:"visibility,omitempty"`
	MaxConcurrentTasks *int               `json:"max_concurrent_tasks,omitempty"`
	Model              *string            `json:"model,omitempty"`
}

type AgentSkill struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type CreateSkillRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateSkillRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type AddAgentSkillRequest struct {
	SkillID string `json:"skill_id"`
}

type AgentTask struct {
	ID            string  `json:"id"`
	AgentID       string  `json:"agent_id"`
	RuntimeID     *string `json:"runtime_id,omitempty"`
	WorkspaceID   string  `json:"workspace_id"`
	SourcePath    string  `json:"source_path"`
	SchemaID      string  `json:"schema_id"`
	Status        string  `json:"status"`
	Priority      int     `json:"priority"`
	ParentTaskID  *string `json:"parent_task_id,omitempty"`
	SessionID     string  `json:"session_id"`
	WorkDir       string  `json:"work_dir"`
	FailureReason string  `json:"failure_reason"`
	DaemonID      string  `json:"daemon_id"`
	DispatchedAt  *string `json:"dispatched_at,omitempty"`
	StartedAt     *string `json:"started_at,omitempty"`
	CompletedAt   *string `json:"completed_at,omitempty"`
	Result        string  `json:"result"`
	Error         string  `json:"error"`
	Attempt       int     `json:"attempt"`
	MaxAttempts   int     `json:"max_attempts"`
	CreatedAt     string  `json:"created_at"`
}

// Daemon models.

type DaemonRegistration struct {
	ID                 string   `json:"id"`
	Hostname           string   `json:"hostname"`
	PID                int      `json:"pid"`
	Version            string   `json:"version"`
	RuntimeIDs         []string `json:"runtime_ids"`
	MaxConcurrentTasks int      `json:"max_concurrent_tasks"`
	LastHeartbeat      string   `json:"last_heartbeat"`
	RegisteredAt       string   `json:"registered_at"`
}

type DaemonRegisterRequest struct {
	ID                 string        `json:"id"`
	Hostname           string        `json:"hostname"`
	PID                int           `json:"pid"`
	Version            string        `json:"version"`
	WorkspaceSlug      string        `json:"workspace_slug"`
	RuntimeIDs         []string      `json:"runtime_ids"`
	Runtimes           []RuntimeInfo `json:"runtimes"` // auto-detected runtime details for upsert
	MaxConcurrentTasks int           `json:"max_concurrent_tasks"`
}

type RuntimeInfo struct {
	Name    string `json:"name"`
	Backend string `json:"backend"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

type DaemonHeartbeatRequest struct {
	ID                 string   `json:"id"`
	RuntimeIDs         []string `json:"runtime_ids"`
	MaxConcurrentTasks int      `json:"max_concurrent_tasks"`
}
