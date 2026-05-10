package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tethy/mulwiki/server/pkg/agent"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// Daemon executes jobs by forking agent CLIs via pkg/agent backends.
type AgentEntry struct {
	Path  string // absolute path to the agent CLI binary
	Model string // optional default model (MULTICA_<PROVIDER>_MODEL env var equivalent)
}

type Daemon struct {
	ServerURL              string
	WorkspaceSlug          string
	WorkspaceSlugs         []string
	AutoDiscoverWorkspaces bool
	DaemonID               string
	DaemonToken            string
	WorkDir                string
	HTTPClient             *http.Client
	ReposURL               string // git repo URL for workspace (http://backend/repos/ws-id.git)
	RepoURL                string // direct repo URL for the configured workspace
	HealthPort             int    // port for health HTTP server (0 = disabled)

	Agents       map[string]AgentEntry // provider → executable (populated from detected runtimes)
	AgentTimeout time.Duration         // per-job timeout (default: 30 min)

	agentVersions   map[string]string
	agentVersionsMu sync.RWMutex

	startTime    time.Time
	detected     []protocol.RuntimeInfo // auto-detected runtimes
	workspacesMu sync.RWMutex
}

// Config holds daemon configuration.
type Config struct {
	ServerURL              string
	WorkspaceSlug          string
	WorkspaceSlugs         []string
	AutoDiscoverWorkspaces bool
	DaemonID               string
	DaemonToken            string
	WorkDir                string
	ReposURL               string        // e.g. "http://localhost:8080/repos" or "/data/repos" for local
	HealthPort             int           // port for health HTTP server (0 = disabled)
	AgentTimeout           time.Duration // per-job timeout (default: 30 min)
}

func New(cfg Config) *Daemon {
	if cfg.WorkDir == "" {
		cfg.WorkDir = "./workdir"
	}
	// Default repos path: relative to server
	if cfg.ReposURL == "" {
		cfg.ReposURL = "./data/repos"
	}
	if cfg.AgentTimeout == 0 {
		cfg.AgentTimeout = 30 * time.Minute
	}
	daemonID := cfg.DaemonID
	if daemonID == "" {
		daemonID = uuid.New().String()
	}
	return &Daemon{
		ServerURL:              cfg.ServerURL,
		WorkspaceSlug:          cfg.WorkspaceSlug,
		WorkspaceSlugs:         normalizeWorkspaceSlugs(cfg.WorkspaceSlugs, cfg.WorkspaceSlug),
		AutoDiscoverWorkspaces: cfg.AutoDiscoverWorkspaces || (cfg.WorkspaceSlug == "" && len(cfg.WorkspaceSlugs) == 0),
		DaemonID:               daemonID,
		DaemonToken:            cfg.DaemonToken,
		WorkDir:                cfg.WorkDir,
		ReposURL:               cfg.ReposURL,
		HealthPort:             cfg.HealthPort,
		AgentTimeout:           cfg.AgentTimeout,
		startTime:              time.Now(),
		Agents:                 make(map[string]AgentEntry),
		agentVersions:          make(map[string]string),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func LoadOrCreateDaemonID(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	id := uuid.New().String()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func (d *Daemon) Run() error {
	return d.run(context.Background())
}

func (d *Daemon) RunContext(ctx context.Context) error {
	return d.run(ctx)
}

func (d *Daemon) run(ctx context.Context) error {
	slog.Info("daemon starting",
		"server_url", d.ServerURL,
		"daemon_id", d.DaemonID,
		"workspace_slugs", d.WorkspaceSlugs,
	)

	if err := os.MkdirAll(d.WorkDir, 0755); err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}

	d.detected = detectRuntimes()
	d.Agents = make(map[string]AgentEntry, len(d.detected))
	for _, rt := range d.detected {
		version := rt.Version
		d.Agents[rt.Backend] = AgentEntry{Path: rt.Path}
		d.setAgentVersion(rt.Backend, version)
	}
	slog.Info("detected runtimes", "count", len(d.detected))

	if err := d.syncWorkspacesAndRegister(d.detected); err != nil {
		slog.Error("initial registration failed, continuing", "error", err)
	}

	if d.HealthPort > 0 {
		go d.serveHealth(ctx)
	}

	go d.heartbeatLoop(d.detected)
	go d.workspaceSyncLoop(ctx, d.detected)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon shutting down")
			return ctx.Err()
		case <-ticker.C:
		}

		slog.Debug("polling for pending jobs")
		for _, slug := range d.currentWorkspaceSlugs() {
			job, err := d.claimNextJob(slug)
			if err != nil {
				slog.Error("claim job failed", "workspace", slug, "error", err)
				continue
			}

			if job == nil {
				continue
			}
			job.WorkspaceSlug = slug
			d.WorkspaceSlug = slug

			slog.Info("claimed job", "job_id", job.ID, "workspace", slug, "agent_id", job.AgentID, "schema_id", job.SchemaID)
			d.executeJob(*job)
		}
	}
}

// serveHealth starts a minimal HTTP server that reports daemon health.
func (d *Daemon) serveHealth(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"status":     "running",
			"pid":        os.Getpid(),
			"version":    "0.1.0",
			"uptime":     time.Since(d.startTime).Truncate(time.Second).String(),
			"workspaces": d.currentWorkspaceSlugs(),
		}
		runtimes := make([]map[string]any, 0, len(d.detected))
		for _, ri := range d.detected {
			runtimes = append(runtimes, map[string]any{
				"name":    ri.Name,
				"backend": ri.Backend,
				"version": ri.Version,
				"status":  "online",
			})
		}
		resp["runtimes"] = runtimes
		json.NewEncoder(w).Encode(resp)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", d.HealthPort)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("health server started", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("health server error", "error", err)
	}
}

func normalizeWorkspaceSlugs(values []string, fallback string) []string {
	out := make([]string, 0, len(values)+1)
	seen := map[string]bool{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, v := range values {
		add(v)
	}
	add(fallback)
	return out
}

func (d *Daemon) currentWorkspaceSlugs() []string {
	d.workspacesMu.RLock()
	defer d.workspacesMu.RUnlock()
	return append([]string(nil), d.WorkspaceSlugs...)
}

func (d *Daemon) setWorkspaceSlugs(slugs []string) {
	d.workspacesMu.Lock()
	d.WorkspaceSlugs = normalizeWorkspaceSlugs(slugs, "")
	if len(d.WorkspaceSlugs) == 1 {
		d.WorkspaceSlug = d.WorkspaceSlugs[0]
	}
	d.workspacesMu.Unlock()
}

// ---------------------------------------------------------------------------
// Runtime auto-detection & registration (Multica pattern)
// ---------------------------------------------------------------------------

// knownAgents maps backend names to (executable name, display name).
// Covers all providers in pkg/agent/ — detection via exec.LookPath on daemon start.
var knownAgents = map[string]struct{ exe, display string }{
	"claude-code": {"claude", "Claude Code"},
	"codex":       {"codex", "Codex"},
	"kimi":        {"kimi", "Kimi"},
	"gemini":      {"gemini", "Gemini CLI"},
	"cursor":      {"cursor-agent", "Cursor"},
	"opencode":    {"opencode", "OpenCode"},
	"openclaw":    {"openclaw", "OpenClaw"},
	"kiro":        {"kiro-cli", "Kiro"},
	"pi":          {"pi", "Pi"},
	"copilot":     {"copilot", "GitHub Copilot"},
	"hermes":      {"hermes", "Hermes"},
}

// setAgentVersion records the detected CLI version for an agent provider.
func (d *Daemon) setAgentVersion(provider, version string) {
	d.agentVersionsMu.Lock()
	d.agentVersions[provider] = version
	d.agentVersionsMu.Unlock()
}

// agentVersion returns the last-detected CLI version for an agent provider.
func (d *Daemon) agentVersion(provider string) string {
	d.agentVersionsMu.RLock()
	v := d.agentVersions[provider]
	d.agentVersionsMu.RUnlock()
	return v
}

// detectRuntimes scans $PATH for installed agent CLIs and returns their metadata.
func detectRuntimes() []protocol.RuntimeInfo {
	var runtimes []protocol.RuntimeInfo

	for backend, info := range knownAgents {
		path, err := exec.LookPath(info.exe)
		if err != nil {
			slog.Debug("agent CLI not found", "backend", backend, "exe", info.exe)
			continue
		}

		version := detectVersion(path)
		runtimes = append(runtimes, protocol.RuntimeInfo{
			Name:    info.display,
			Backend: backend,
			Version: version,
			Path:    path,
		})
		slog.Info("detected agent CLI", "backend", backend, "path", path, "version", version)
	}

	return runtimes
}

// detectVersion runs {binary} --version and returns a trimmed version string.
func detectVersion(binaryPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("--version failed", "binary", binaryPath, "error", err)
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func (d *Daemon) syncWorkspacesAndRegister(runtimes []protocol.RuntimeInfo) error {
	slugs := d.currentWorkspaceSlugs()
	if d.AutoDiscoverWorkspaces || len(slugs) == 0 {
		discovered, err := d.discoverWorkspaces()
		if err != nil {
			return err
		}
		for _, ws := range discovered {
			slugs = append(slugs, ws.Slug)
		}
		d.setWorkspaceSlugs(slugs)
	}
	if len(slugs) == 0 {
		return fmt.Errorf("no workspaces available for daemon")
	}
	var firstErr error
	for _, slug := range slugs {
		if err := d.registerWorkspace(slug, runtimes); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Error("workspace registration failed", "workspace", slug, "error", err)
		}
	}
	return firstErr
}

func (d *Daemon) workspaceSyncLoop(ctx context.Context, runtimes []protocol.RuntimeInfo) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if d.AutoDiscoverWorkspaces || len(d.currentWorkspaceSlugs()) == 0 {
				if err := d.syncWorkspacesAndRegister(runtimes); err != nil {
					slog.Debug("workspace sync failed", "error", err)
				}
			}
		}
	}
}

func (d *Daemon) discoverWorkspaces() ([]protocol.DaemonWorkspace, error) {
	url := fmt.Sprintf("%s/api/daemon/workspaces", d.ServerURL)
	resp, err := d.getDaemonJSON(url)
	if err != nil {
		return nil, fmt.Errorf("http get daemon workspaces: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discover workspaces returned status %d: %s", resp.StatusCode, string(body))
	}
	var out protocol.DaemonWorkspacesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode daemon workspaces: %w", err)
	}
	return out.Workspaces, nil
}

// registerWorkspace sends the daemon registration + auto-detected runtimes to the server.
func (d *Daemon) registerWorkspace(workspaceSlug string, runtimes []protocol.RuntimeInfo) error {
	hostname, _ := os.Hostname()

	req := protocol.DaemonRegisterRequest{
		ID:                 d.DaemonID,
		Hostname:           hostname,
		PID:                os.Getpid(),
		Version:            "0.1.0",
		WorkspaceSlug:      workspaceSlug,
		Runtimes:           runtimes,
		MaxConcurrentTasks: 3,
		RuntimeIDs:         []string{}, // server fills these from upserted runtimes
	}

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal register request: %w", err)
	}

	url := fmt.Sprintf("%s/api/daemon/register", d.ServerURL)
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("http post register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to get resolved runtime IDs.
	var reg protocol.DaemonRegistration
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return fmt.Errorf("decode register response: %w", err)
	}

	slog.Info("registered with server",
		"daemon_id", d.DaemonID,
		"workspace", workspaceSlug,
		"runtime_count", len(reg.RuntimeIDs),
	)

	return nil
}

// register preserves the single-workspace test/helper API.
func (d *Daemon) register(runtimes []protocol.RuntimeInfo) error {
	return d.syncWorkspacesAndRegister(runtimes)
}

func (d *Daemon) postJSON(url string, body io.Reader) (*http.Response, error) {
	req, err := d.newDaemonRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	return d.HTTPClient.Do(req)
}

func (d *Daemon) newDaemonRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Daemon-ID", d.DaemonID)
	if token := d.DaemonToken; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := os.Getenv("MULWIKI_DAEMON_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func (d *Daemon) getDaemonJSON(url string) (*http.Response, error) {
	req, err := d.newDaemonRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return d.HTTPClient.Do(req)
}

// heartbeatLoop sends periodic heartbeats to the server every 30 seconds.
func (d *Daemon) heartbeatLoop(runtimes []protocol.RuntimeInfo) {
	hostname, _ := os.Hostname()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Send first heartbeat immediately, then every 30s.
	send := func() {
		req := protocol.DaemonHeartbeatRequest{
			ID:                 d.DaemonID,
			MaxConcurrentTasks: 3,
			RuntimeIDs:         []string{}, // server resolves from daemon_registrations
		}
		jsonBody, _ := json.Marshal(req)
		url := fmt.Sprintf("%s/api/daemon/heartbeat", d.ServerURL)
		resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
		if err != nil {
			slog.Debug("heartbeat failed", "error", err, "hostname", hostname)
			return
		}
		resp.Body.Close()

		// If the server doesn't recognize us, re-register.
		if resp.StatusCode == http.StatusNotFound {
			slog.Warn("daemon not found on server, re-registering")
			d.register(runtimes)
			return
		}
	}

	send()
	for range ticker.C {
		send()
	}
}

// claimNextJob claims the next pending job from the server.
func (d *Daemon) claimNextJob(workspaceSlug ...string) (*protocol.Job, error) {
	slug := d.workspaceSlugForCall(workspaceSlug...)
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/claim", d.ServerURL, slug)

	body := map[string]string{"daemon_id": d.DaemonID}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claim returned status %d", resp.StatusCode)
	}

	var j protocol.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	j.WorkspaceSlug = slug
	return &j, nil
}

func (d *Daemon) workspaceSlugForCall(slug ...string) string {
	if len(slug) > 0 && strings.TrimSpace(slug[0]) != "" {
		return strings.TrimSpace(slug[0])
	}
	if d.WorkspaceSlug != "" {
		return d.WorkspaceSlug
	}
	slugs := d.currentWorkspaceSlugs()
	if len(slugs) > 0 {
		return slugs[0]
	}
	return ""
}

// executeJob runs a claimed job using the agent execution pipeline.
// All jobs are now schema+agent driven — the schema defines the pipeline,
// the agent executes it. There are no hardcoded job types.
func (d *Daemon) executeJob(job protocol.Job) {
	jobDir := filepath.Join(d.WorkDir, job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		d.failJob(job, fmt.Sprintf("create job dir: %v", err))
		return
	}
	defer os.RemoveAll(jobDir)

	slog.Info("executing job", "job_id", job.ID, "agent_id", job.AgentID, "schema_id", job.SchemaID)

	if job.AgentID == "" {
		d.failJob(job, "no agent assigned — jobs require an agent to execute the schema pipeline")
		return
	}

	d.runAgentJob(job, jobDir)
}

// ---------------------------------------------------------------------------
// Multica Agent execution pipeline
// ---------------------------------------------------------------------------

// runAgentJob orchestrates the full agent execution flow:
//
//  1. Fetch agent + runtime configuration from the server
//  2. Create an agent_tasks execution record
//  3. Build an isolated workdir with schema, sources, wiki, and AGENTS.md
//  4. Fork the agent via agent.Backend.Execute (per-provider CLI args)
//  5. Stream output and monitor progress
//  6. On success: parse manifest.json, merge output into wiki
//  7. On failure: update agent_tasks with error, retry if attempts remain
func (d *Daemon) runAgentJob(job protocol.Job, jobDir string) {
	if job.WorkspaceSlug != "" {
		d.WorkspaceSlug = job.WorkspaceSlug
	}
	// 1. Fetch agent configuration.
	agentCfg, err := d.fetchAgent(job.WorkspaceSlug, job.AgentID)
	if err != nil {
		d.failJob(job, fmt.Sprintf("fetch agent: %v", err))
		return
	}

	if agentCfg.RuntimeID == "" {
		d.failJob(job, "agent has no runtime configured")
		return
	}

	// 2. Fetch runtime configuration.
	runtime, err := d.fetchRuntime(job.WorkspaceSlug, agentCfg.RuntimeID)
	if err != nil {
		d.failJob(job, fmt.Sprintf("fetch runtime: %v", err))
		return
	}

	// 3. Look up the agent entry (executable path) by provider/backend.
	entry, ok := d.Agents[runtime.Backend]
	if !ok {
		// Fall back to runtime's recorded path.
		entry = AgentEntry{Path: runtime.Path}
	}
	if entry.Path == "" {
		d.failJob(job, fmt.Sprintf("no executable path for provider %q", runtime.Backend))
		return
	}

	d.updateProgress(job, 5)

	// 4. Create agent task record.
	task, err := d.createTask(job, agentCfg, runtime)
	if err != nil {
		d.failJob(job, fmt.Sprintf("create task: %v", err))
		return
	}
	slog.Info("agent task created", "task_id", task.ID, "agent_id", agentCfg.ID)

	// 5. Run the subprocess with retry loop.
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		d.runAgentAttempt(job, agentCfg, runtime, entry, task, attempt, maxAttempts)

		if task.Status == "completed" {
			return
		}

		if attempt < maxAttempts {
			slog.Info("retrying agent task",
				"job_id", job.ID,
				"task_id", task.ID,
				"attempt", attempt+1,
				"max_attempts", maxAttempts,
			)
		}
	}

	// All attempts exhausted.
	d.failJob(job, "all agent attempts exhausted")
}

// runAgentAttempt executes a single attempt via the agent.Backend interface.
// Each provider (claude, codex, kimi, etc.) gets its own CLI-specific args.
func (d *Daemon) runAgentAttempt(job protocol.Job, agentCfg *protocol.Agent, runtime *protocol.AgentRuntime, entry AgentEntry, task *protocol.AgentTask, attempt, maxAttempts int) {
	// Build isolated workdir.
	workdir, err := d.buildWorkdir(job, agentCfg)
	if err != nil {
		d.failJob(job, fmt.Sprintf("build workdir: %v", err))
		d.markTaskFailed(agentCfg.ID, task.ID, attempt, fmt.Sprintf("build workdir: %v", err))
		return
	}
	defer os.RemoveAll(workdir)

	d.updateProgress(job, 15)

	// Mark task as started.
	d.markTaskStarted(job.WorkspaceSlug, agentCfg.ID, task.ID, workdir)
	d.updateProgress(job, 20)

	// Build agent environment.
	agentEnv := make(map[string]string)
	agentEnv["AGENT_WORKDIR"] = workdir
	agentEnv["AGENT_OUTPUT_DIR"] = filepath.Join(workdir, "output")
	for k, v := range agentCfg.CustomEnv {
		agentEnv[k] = v
	}

	// Resolve model: agent config wins, then entry default.
	model := agentCfg.Model
	if model == "" {
		model = entry.Model
	}

	provider := runtime.Backend

	// Normalize backend names with hyphens (claude-code → claude, gemini-cli → gemini, etc.)
	provider = strings.ReplaceAll(provider, "-code", "")
	provider = strings.ReplaceAll(provider, "-cli", "")

	// Create the provider-specific agent backend.
	backend, err := agent.New(provider, agent.Config{
		ExecutablePath: entry.Path,
		Env:            agentEnv,
		Logger:         slog.Default(),
	})
	if err != nil {
		d.failJob(job, fmt.Sprintf("create agent backend: %v", err))
		d.markTaskFailed(agentCfg.ID, task.ID, attempt, fmt.Sprintf("create agent backend: %v", err))
		return
	}

	// Build the prompt from source paths and instructions.
	prompt := d.buildPrompt(job, agentCfg)

	execCtx, cancel := context.WithTimeout(context.Background(), d.AgentTimeout)
	defer cancel()

	execOpts := agent.ExecOptions{
		Cwd:        workdir,
		Model:      model,
		Timeout:    d.AgentTimeout,
		CustomArgs: agentCfg.CustomArgs,
	}
	// Agent.Instructions → developerInstructions (system-level role, persisted across turns).
	// Most backends map this to --system-prompt / --append-system-prompt / developerInstructions.
	if agentCfg.Instructions != "" {
		execOpts.SystemPrompt = agentCfg.Instructions
	}
	if len(agentCfg.McpConfig) > 0 && string(agentCfg.McpConfig) != "{}" && string(agentCfg.McpConfig) != "" {
		execOpts.McpConfig = agentCfg.McpConfig
	}

	slog.Info("starting agent",
		"job_id", job.ID,
		"task_id", task.ID,
		"attempt", attempt,
		"provider", provider,
		"workdir", workdir,
		"model", model,
	)

	// Execute via the unified agent backend.
	session, err := backend.Execute(execCtx, prompt, execOpts)
	if err != nil {
		slog.Error("agent execution failed", "job_id", job.ID, "task_id", task.ID, "error", err)
		d.markTaskFailed(agentCfg.ID, task.ID, attempt, fmt.Sprintf("agent execution: %v", err))
		task.Status = "failed"
		return
	}

	d.updateProgress(job, 25)

	batcher := newTaskMessageBatcher(d, job.WorkspaceSlug, job.ID, task.ID, workdir, 500*time.Millisecond)
	messagesDone := make(chan struct{})

	// Drain messages while waiting for the final result.
	go func() {
		defer close(messagesDone)
		for msg := range session.Messages {
			slog.Debug("agent message", "job_id", job.ID, "type", msg.Type, "content", msg.Content[:min(len(msg.Content), 200)])
			batcher.Add(msg)
		}
	}()

	// Wait for the result.
	result, ok := <-session.Result
	waitForMessages(messagesDone)
	batcher.Close()
	if !ok {
		d.markTaskFailed(agentCfg.ID, task.ID, attempt, "result channel closed without result")
		task.Status = "failed"
		return
	}

	// Handle result status.
	switch result.Status {
	case "completed":
		d.updateProgress(job, 85)

		// Collect output from workdir and merge into wiki.
		outputStr, err := d.collectOutput(workdir, job, task)
		if err != nil {
			slog.Error("collect output failed", "job_id", job.ID, "task_id", task.ID, "error", err)
			d.markTaskFailed(agentCfg.ID, task.ID, attempt, fmt.Sprintf("collect output: %v", err))
			task.Status = "failed"
			return
		}

		d.markTaskCompleted(job.WorkspaceSlug, agentCfg.ID, task.ID, outputStr, result.SessionID, workdir)
		task.Status = "completed"
		d.updateProgress(job, 100)
		d.completeJob(job)
		slog.Info("agent job completed", "job_id", job.ID, "task_id", task.ID, "attempt", attempt,
			"duration_ms", result.DurationMs, "session_id", result.SessionID)

	case "failed", "timeout", "cancelled":
		errMsg := result.Error
		if errMsg == "" {
			errMsg = fmt.Sprintf("agent %s", result.Status)
		}
		slog.Error("agent process failed", "job_id", job.ID, "task_id", task.ID, "attempt", attempt,
			"status", result.Status, "error", result.Error)
		d.markTaskFailed(agentCfg.ID, task.ID, attempt, errMsg, result.SessionID, workdir)
		task.Status = "failed"

	default:
		slog.Error("unknown agent status", "job_id", job.ID, "task_id", task.ID, "status", result.Status)
		d.markTaskFailed(agentCfg.ID, task.ID, attempt, fmt.Sprintf("unknown status: %s", result.Status))
		task.Status = "failed"
	}
}

// buildPrompt assembles the task prompt for the agent.
// Sends a /goal-style instruction: the agent should plan, execute, check, and iterate
// until complete. Schema-driven phases are defined in schema.md. Platform protocol in AGENTS.md.
func (d *Daemon) buildPrompt(job protocol.Job, agentCfg *protocol.Agent) string {
	var b strings.Builder

	b.WriteString("## Goal\n\n")
	b.WriteString("Build a comprehensive wiki from source documents. This is an encyclopedia, not a summary.\n")
	b.WriteString("Cover EVERY section, concept, entity, and insight in the source material. Leave nothing behind.\n\n")

	b.WriteString("## Instructions\n\n")
	b.WriteString("1. Read AGENTS.md (platform protocol) and schema.md (business rules) first.\n")
	b.WriteString("2. Read ALL source files in sources/.\n")
	b.WriteString("3. Follow schema.md pipeline phases in order: Source Assessment → Structure Extraction → Per-Unit Processing → Synthesis → Self-Review → Manifest.\n")
	b.WriteString("4. Write output files progressively to output/ — do NOT batch everything at the end.\n")
	b.WriteString("5. Complete the self-review phase. If any check fails, go back and fill gaps.\n")
	b.WriteString("6. When ALL self-review checks pass, write output/manifest.json and stop.\n\n")

	b.WriteString("## Strategy\n\n")
	b.WriteString("Adapt to source size:\n")
	b.WriteString("- Short article (< 5000 words): process in one pass.\n")
	b.WriteString("- Long document / book: break into chapters/sections. Process each unit fully before moving on.\n")
	b.WriteString("- Video transcripts / OCR text: same adaptive logic.\n\n")

	b.WriteString("## Quality Threshold (non-negotiable)\n\n")
	b.WriteString("- Every section/chapter/unit of the source must produce output pages.\n")
	b.WriteString("- Concept pages must be >= 200 words with sources cited. One-sentence stubs are not acceptable.\n")
	b.WriteString("- All [[wikilinks]] must resolve to real pages. No dangling links.\n")
	b.WriteString("- Use multiple types from schema.md (not just Summary). Minimum: Entity + Concept.\n\n")

	if len(job.SourcePaths) > 0 {
		b.WriteString("## Source Files\n\n")
		for _, p := range job.SourcePaths {
			b.WriteString(fmt.Sprintf("- %s\n", p))
		}
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Server communication helpers (HTTP API)
// ---------------------------------------------------------------------------

// fetchAgent retrieves an agent's full configuration from the server.
func (d *Daemon) fetchAgent(workspaceSlugOrAgentID string, agentID ...string) (*protocol.Agent, error) {
	slug := d.workspaceSlugForCall()
	id := workspaceSlugOrAgentID
	if len(agentID) > 0 {
		slug = d.workspaceSlugForCall(workspaceSlugOrAgentID)
		id = agentID[0]
	}
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/agents/%s", d.ServerURL, slug, id)
	resp, err := d.getDaemonJSON(url)
	if err != nil {
		return nil, fmt.Errorf("http get agent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch agent returned status %d", resp.StatusCode)
	}

	var wrap struct {
		Agent protocol.Agent `json:"agent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return nil, fmt.Errorf("decode agent: %w", err)
	}
	return &wrap.Agent, nil
}

// fetchRuntime retrieves a runtime's configuration from the server.
func (d *Daemon) fetchRuntime(workspaceSlugOrRuntimeID string, runtimeID ...string) (*protocol.AgentRuntime, error) {
	slug := d.workspaceSlugForCall()
	id := workspaceSlugOrRuntimeID
	if len(runtimeID) > 0 {
		slug = d.workspaceSlugForCall(workspaceSlugOrRuntimeID)
		id = runtimeID[0]
	}
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/agents/runtimes/%s", d.ServerURL, slug, id)
	resp, err := d.getDaemonJSON(url)
	if err != nil {
		return nil, fmt.Errorf("http get runtime: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch runtime returned status %d", resp.StatusCode)
	}

	var rtWrap struct {
		Runtime protocol.AgentRuntime `json:"runtime"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rtWrap); err != nil {
		return nil, fmt.Errorf("decode runtime: %w", err)
	}
	return &rtWrap.Runtime, nil
}

// fetchSchema retrieves schema configuration from the server.
func (d *Daemon) fetchSchema(workspaceSlugOrSchemaID string, schemaID ...string) (*protocol.Schema, error) {
	slug := d.workspaceSlugForCall()
	id := workspaceSlugOrSchemaID
	if len(schemaID) > 0 {
		slug = d.workspaceSlugForCall(workspaceSlugOrSchemaID)
		id = schemaID[0]
	}
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/schemas/%s", d.ServerURL, slug, id)
	resp, err := d.getDaemonJSON(url)
	if err != nil {
		return nil, fmt.Errorf("http get schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch schema returned status %d", resp.StatusCode)
	}

	var schema protocol.Schema
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	return &schema, nil
}

// buildAgentsMD generates the agent's runtime configuration file.
// Follows Multica's buildMetaSkillContent pattern: platform protocol and
// agent identity go here; business rules (type/layer taxonomy) are in schema.md.
func (d *Daemon) buildAgentsMD(agent *protocol.Agent, job protocol.Job) string {
	var b strings.Builder

	// --- Identity ---
	b.WriteString("# Mulwiki Agent Runtime\n\n")
	if agent.Name != "" {
		b.WriteString("## Agent Identity\n\n")
		fmt.Fprintf(&b, "**You are: %s**", agent.Name)
		if agent.ID != "" {
			fmt.Fprintf(&b, " (ID: `%s`)", agent.ID)
		}
		b.WriteString("\n\n")
		if agent.Instructions != "" {
			b.WriteString(agent.Instructions)
			b.WriteString("\n\n")
		}
	} else if agent.Instructions != "" {
		b.WriteString(agent.Instructions)
		b.WriteString("\n\n")
	}

	// --- Platform Protocol ---
	b.WriteString("## Platform Protocol\n\n")

	b.WriteString("### Directory Structure\n\n")
	b.WriteString("- `sources/` — Source documents to process (read-only)\n")
	b.WriteString("- `output/` — **Your delivery zone.** Write all produced files here.\n")
	b.WriteString("- `schema.md` — Wiki schema defining type and layer taxonomy. **Read this first.**\n")
	b.WriteString("- `wiki/` — Existing wiki pages (reference only, do not modify directly)\n\n")

	b.WriteString("### Workflow\n\n")
	b.WriteString("1. Read `schema.md` to understand the wiki type/layer taxonomy\n")
	b.WriteString("2. Read source documents from `sources/`\n")
	b.WriteString("3. Read existing wiki pages from `wiki/` for context (if any)\n")
	b.WriteString("4. Write new wiki pages as `.md` files to `output/`, preserving directory structure.\n")
	b.WriteString("   Example: `output/concept/transformer-attention.md` → wiki path `/concept/transformer-attention`\n")
	b.WriteString("5. Create `output/manifest.json` listing all produced pages\\n\\n")
	b.WriteString("The daemon handles the rest: it reads manifest.json, commits pages to the wiki, and completes the job.\n")
	b.WriteString("You do NOT need to commit or push anything yourself.\n\n")

	b.WriteString("### manifest.json Specification\n\n")
	b.WriteString("Write `output/manifest.json` with this structure:\n\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "version": "1.0",` + "\n")
	fmt.Fprintf(&b, `  "job_id": "%s",`+"\n", job.ID)
	b.WriteString(`  "pages": [` + "\n")
	b.WriteString(`    {` + "\n")
	b.WriteString(`      "path": "<relative-path>"` + ",\n")
	b.WriteString(`      "title": "<page-title>"` + ",\n")
	b.WriteString(`      "type": "<type-from-schema.md>"` + ",\n")
	b.WriteString(`      "layer": "<layer-from-schema.md>"` + ",\n")
	b.WriteString(`      "hash": ""` + "\n")
	b.WriteString(`    }` + "\n")
	b.WriteString(`  ],` + "\n")
	b.WriteString(`  "file_count": <N>,` + "\n")
	b.WriteString(`  "total_size": <bytes>` + "\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")
	b.WriteString("**IMPORTANT**: Use the exact type and layer values defined in `schema.md`. ")
	b.WriteString("Do not guess or use placeholder values.\n")

	return b.String()
}

// createTask creates an agent_tasks record via the server API.
func (d *Daemon) createTask(job protocol.Job, agent *protocol.Agent, runtime *protocol.AgentRuntime) (*protocol.AgentTask, error) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/agents/%s/tasks", d.ServerURL, d.workspaceSlugForJob(job), agent.ID)

	body := map[string]interface{}{
		"job_id":       job.ID,
		"source_path":  job.SourcePath,
		"schema_id":    job.SchemaID,
		"runtime_id":   runtime.ID,
		"priority":     0,
		"max_attempts": 3,
		"daemon_id":    d.DaemonID,
		"messages": []map[string]interface{}{
			{
				"role":    "daemon",
				"content": "task queued",
				"metadata": map[string]interface{}{
					"job_id":     job.ID,
					"agent_id":   agent.ID,
					"runtime_id": runtime.ID,
				},
			},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal task body: %w", err)
	}

	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("http post create task: %w", err)
	}
	defer resp.Body.Close()

	// Read entire body once to avoid double-read issues.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read create task response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		var errBody struct {
			Error string `json:"error"`
		}
		json.Unmarshal(bodyBytes, &errBody)
		return nil, fmt.Errorf("create task returned status %d: %s", resp.StatusCode, errBody.Error)
	}

	var taskWrap struct {
		Task protocol.AgentTask `json:"task"`
	}
	if err := json.Unmarshal(bodyBytes, &taskWrap); err != nil {
		return nil, fmt.Errorf("decode task: %w", err)
	}
	return &taskWrap.Task, nil
}

// updateTask sends a PATCH to update the task's status/result/error.
func (d *Daemon) patchTask(agentID, taskID string, body map[string]interface{}, workspaceSlug ...string) error {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/agents/%s/tasks/%s", d.ServerURL, d.workspaceSlugForCall(workspaceSlug...), agentID, taskID)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal update task: %w", err)
	}

	req, err := d.newDaemonRequest(http.MethodPatch, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("http patch task: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update task returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *Daemon) updateTask(agentID, taskID string, status, result, errMsg string, attempt int, sessionID, workDir string, workspaceSlug ...string) error {
	body := map[string]interface{}{}
	if status != "" {
		body["status"] = status
	}
	if result != "" {
		body["result"] = result
	}
	if errMsg != "" {
		body["error"] = errMsg
	}
	if attempt > 0 {
		body["attempt"] = attempt
	}
	if sessionID != "" {
		body["session_id"] = sessionID
	}
	if workDir != "" {
		body["work_dir"] = workDir
	}

	if msg := taskStatusMessage(status, result, errMsg); msg != "" {
		body["messages"] = []map[string]interface{}{
			{
				"role":    "daemon",
				"content": msg,
				"metadata": map[string]interface{}{
					"status":     status,
					"attempt":    attempt,
					"session_id": sessionID,
					"work_dir":   workDir,
				},
			},
		}
	}

	return d.patchTask(agentID, taskID, body, workspaceSlug...)
}

func taskStatusMessage(status, result, errMsg string) string {
	switch {
	case errMsg != "":
		return errMsg
	case result != "":
		return result
	case status != "":
		return "task " + status
	default:
		return ""
	}
}

type taskMessageBatcher struct {
	d             *Daemon
	workspaceSlug string
	jobID         string
	taskID        string
	workDir       string
	interval      time.Duration

	mu            sync.Mutex
	nextSeq       int64
	pending       []protocol.AgentTaskMessage
	pinnedSession bool
	done          chan struct{}
	closeOnce     sync.Once
}

func newTaskMessageBatcher(d *Daemon, args ...any) *taskMessageBatcher {
	workspaceSlug := ""
	jobID := ""
	taskID := ""
	workDir := ""
	interval := 0 * time.Millisecond
	if len(args) == 4 {
		jobID, _ = args[0].(string)
		taskID, _ = args[1].(string)
		workDir, _ = args[2].(string)
		interval, _ = args[3].(time.Duration)
	} else if len(args) >= 5 {
		workspaceSlug, _ = args[0].(string)
		jobID, _ = args[1].(string)
		taskID, _ = args[2].(string)
		workDir, _ = args[3].(string)
		interval, _ = args[4].(time.Duration)
	}
	b := &taskMessageBatcher{
		d:             d,
		workspaceSlug: workspaceSlug,
		jobID:         jobID,
		taskID:        taskID,
		workDir:       workDir,
		interval:      interval,
		nextSeq:       1,
		done:          make(chan struct{}),
	}
	if interval > 0 {
		go b.loop()
	}
	return b
}

func (b *taskMessageBatcher) loop() {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.Flush()
		case <-b.done:
			return
		}
	}
}

func (b *taskMessageBatcher) Add(msg agent.Message) {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		content = strings.TrimSpace(msg.Status)
	}
	if content == "" && msg.Tool != "" {
		content = msg.Tool
	}
	if content == "" && msg.Output != "" {
		content = strings.TrimSpace(msg.Output)
	}
	if content == "" {
		return
	}

	b.d.postLogLine(b.jobID, string(msg.Type), content, b.workspaceSlug)
	if msg.SessionID != "" {
		b.pinSession(msg.SessionID)
	}

	input := json.RawMessage(`{}`)
	if msg.Input != nil {
		if raw, err := json.Marshal(msg.Input); err == nil {
			input = raw
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	seq := b.nextSeq
	b.nextSeq++
	b.pending = append(b.pending, protocol.AgentTaskMessage{
		Seq:       seq,
		Type:      string(msg.Type),
		Content:   content,
		Tool:      msg.Tool,
		CallID:    msg.CallID,
		Input:     input,
		Output:    msg.Output,
		Status:    msg.Status,
		Level:     msg.Level,
		SessionID: msg.SessionID,
	})
}

func (b *taskMessageBatcher) pinSession(sessionID string) {
	b.mu.Lock()
	if b.pinnedSession {
		b.mu.Unlock()
		return
	}
	b.pinnedSession = true
	b.mu.Unlock()

	if err := b.d.pinTaskSession(b.taskID, sessionID, b.workDir); err != nil {
		slog.Debug("pin task session failed", "task_id", b.taskID, "error", err)
	}
}

func (b *taskMessageBatcher) Flush() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	messages := append([]protocol.AgentTaskMessage(nil), b.pending...)
	b.pending = b.pending[:0]
	b.mu.Unlock()

	if err := b.d.appendTaskMessages(b.taskID, messages); err != nil {
		slog.Debug("append task messages failed", "task_id", b.taskID, "error", err)
	}
}

func (b *taskMessageBatcher) Close() {
	b.closeOnce.Do(func() {
		close(b.done)
		b.Flush()
	})
}

func waitForMessages(done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		slog.Warn("timed out waiting for agent message drain")
	}
}

func (d *Daemon) appendTaskMessages(taskID string, messages []protocol.AgentTaskMessage) error {
	if len(messages) == 0 {
		return nil
	}
	url := fmt.Sprintf("%s/api/daemon/tasks/%s/messages", d.ServerURL, taskID)
	body := map[string]any{"messages": messages}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal task messages: %w", err)
	}
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("http post task messages: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("append task messages returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *Daemon) pinTaskSession(taskID, sessionID, workDir string) error {
	if sessionID == "" && workDir == "" {
		return nil
	}
	url := fmt.Sprintf("%s/api/daemon/tasks/%s/session", d.ServerURL, taskID)
	body := map[string]string{
		"session_id": sessionID,
		"work_dir":   workDir,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal task session: %w", err)
	}
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("http post task session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pin task session returned status %d", resp.StatusCode)
	}
	return nil
}

// markTaskRunning marks the task as "running" (valid per DB CHECK constraint).
func (d *Daemon) markTaskStarted(args ...string) {
	workspaceSlug := ""
	agentID := ""
	taskID := ""
	workDir := ""
	if len(args) == 3 {
		agentID, taskID, workDir = args[0], args[1], args[2]
	} else if len(args) >= 4 {
		workspaceSlug, agentID, taskID, workDir = args[0], args[1], args[2], args[3]
	}
	if err := d.updateTask(agentID, taskID, "running", "", "", 0, "", workDir, workspaceSlug); err != nil {
		slog.Error("mark task started failed", "task_id", taskID, "error", err)
	}
}

// markTaskCompleted marks the task as "completed" with a result summary.
func (d *Daemon) markTaskCompleted(args ...string) {
	workspaceSlug := ""
	agentID := ""
	taskID := ""
	result := ""
	sessionID := ""
	workDir := ""
	if len(args) == 5 {
		agentID, taskID, result, sessionID, workDir = args[0], args[1], args[2], args[3], args[4]
	} else if len(args) >= 6 {
		workspaceSlug, agentID, taskID, result, sessionID, workDir = args[0], args[1], args[2], args[3], args[4], args[5]
	}
	if err := d.updateTask(agentID, taskID, "completed", result, "", 0, sessionID, workDir, workspaceSlug); err != nil {
		slog.Error("mark task completed failed", "task_id", taskID, "error", err)
	}
}

// markTaskFailed marks the task as "failed" with an error message.
func (d *Daemon) markTaskFailed(agentID, taskID string, attempt int, errMsg string, sessionID ...string) {
	resumeID := ""
	workDir := ""
	if len(sessionID) > 0 {
		resumeID = sessionID[0]
	}
	if len(sessionID) > 1 {
		workDir = sessionID[1]
	}
	if err := d.updateTask(agentID, taskID, "failed", "", errMsg, attempt, resumeID, workDir); err != nil {
		slog.Error("mark task failed failed", "task_id", taskID, "error", err)
	}
}

// fetchWorkspace retrieves workspace metadata.
func (d *Daemon) fetchWorkspace(workspaceSlug ...string) (*protocol.Workspace, error) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s", d.ServerURL, d.workspaceSlugForCall(workspaceSlug...))
	resp, err := d.getDaemonJSON(url)
	if err != nil {
		return nil, fmt.Errorf("http get workspace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch workspace returned status %d", resp.StatusCode)
	}

	var ws protocol.Workspace
	if err := json.NewDecoder(resp.Body).Decode(&ws); err != nil {
		return nil, fmt.Errorf("decode workspace: %w", err)
	}
	return &ws, nil
}
