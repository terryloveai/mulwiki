package daemon

import (
	"bufio"
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
	ServerURL     string
	WorkspaceSlug string
	DaemonID      string
	DaemonToken   string
	WorkDir       string
	HTTPClient    *http.Client
	ReposURL      string // git repo URL for workspace (http://backend/repos/ws-id.git)
	RepoURL       string // direct repo URL for the configured workspace
	HealthPort    int    // port for health HTTP server (0 = disabled)

	Agents       map[string]AgentEntry // provider → executable (populated from detected runtimes)
	AgentTimeout time.Duration         // per-job timeout (default: 30 min)

	agentVersions   map[string]string
	agentVersionsMu sync.RWMutex

	startTime time.Time
	detected  []protocol.RuntimeInfo // auto-detected runtimes
}

// Config holds daemon configuration.
type Config struct {
	ServerURL     string
	WorkspaceSlug string
	DaemonID      string
	DaemonToken   string
	WorkDir       string
	ReposURL      string        // e.g. "http://localhost:8080/repos" or "/data/repos" for local
	HealthPort    int           // port for health HTTP server (0 = disabled)
	AgentTimeout  time.Duration // per-job timeout (default: 30 min)
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
		ServerURL:     cfg.ServerURL,
		WorkspaceSlug: cfg.WorkspaceSlug,
		DaemonID:      daemonID,
		DaemonToken:   cfg.DaemonToken,
		WorkDir:       cfg.WorkDir,
		ReposURL:      cfg.ReposURL,
		HealthPort:    cfg.HealthPort,
		AgentTimeout:  cfg.AgentTimeout,
		startTime:     time.Now(),
		Agents:        make(map[string]AgentEntry),
		agentVersions: make(map[string]string),
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
		"workspace_slug", d.WorkspaceSlug,
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

	if err := d.register(d.detected); err != nil {
		slog.Error("initial registration failed, continuing", "error", err)
	}

	if d.HealthPort > 0 {
		go d.serveHealth(ctx)
	}

	go d.heartbeatLoop(d.detected)

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
		job, err := d.claimNextJob()
		if err != nil {
			slog.Error("claim job failed", "error", err)
			continue
		}

		if job == nil {
			continue
		}

		slog.Info("claimed job", "job_id", job.ID, "agent_id", job.AgentID, "schema_id", job.SchemaID)
		d.executeJob(*job)
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
			"workspaces": []string{d.WorkspaceSlug},
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

// register sends the daemon registration + auto-detected runtimes to the server.
func (d *Daemon) register(runtimes []protocol.RuntimeInfo) error {
	hostname, _ := os.Hostname()

	req := protocol.DaemonRegisterRequest{
		ID:                 d.DaemonID,
		Hostname:           hostname,
		PID:                os.Getpid(),
		Version:            "0.1.0",
		WorkspaceSlug:      d.WorkspaceSlug,
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
		"runtime_count", len(reg.RuntimeIDs),
	)

	return nil
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
func (d *Daemon) claimNextJob() (*protocol.Job, error) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/claim", d.ServerURL, d.WorkspaceSlug)

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
	return &j, nil
}

// executeJob runs a claimed job using the agent execution pipeline.
// All jobs are now schema+agent driven — the schema defines the pipeline,
// the agent executes it. There are no hardcoded job types.
func (d *Daemon) executeJob(job protocol.Job) {
	jobDir := filepath.Join(d.WorkDir, job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		d.failJob(job.ID, fmt.Sprintf("create job dir: %v", err))
		return
	}
	defer os.RemoveAll(jobDir)

	slog.Info("executing job", "job_id", job.ID, "agent_id", job.AgentID, "schema_id", job.SchemaID)

	if job.AgentID == "" {
		d.failJob(job.ID, "no agent assigned — jobs require an agent to execute the schema pipeline")
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
	// 1. Fetch agent configuration.
	agentCfg, err := d.fetchAgent(job.AgentID)
	if err != nil {
		d.failJob(job.ID, fmt.Sprintf("fetch agent: %v", err))
		return
	}

	if agentCfg.RuntimeID == "" {
		d.failJob(job.ID, "agent has no runtime configured")
		return
	}

	// 2. Fetch runtime configuration.
	runtime, err := d.fetchRuntime(agentCfg.RuntimeID)
	if err != nil {
		d.failJob(job.ID, fmt.Sprintf("fetch runtime: %v", err))
		return
	}

	// 3. Look up the agent entry (executable path) by provider/backend.
	entry, ok := d.Agents[runtime.Backend]
	if !ok {
		// Fall back to runtime's recorded path.
		entry = AgentEntry{Path: runtime.Path}
	}
	if entry.Path == "" {
		d.failJob(job.ID, fmt.Sprintf("no executable path for provider %q", runtime.Backend))
		return
	}

	d.updateProgress(job.ID, 5)

	// 4. Create agent task record.
	task, err := d.createTask(job, agentCfg, runtime)
	if err != nil {
		d.failJob(job.ID, fmt.Sprintf("create task: %v", err))
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
	d.failJob(job.ID, "all agent attempts exhausted")
}

// runAgentAttempt executes a single attempt via the agent.Backend interface.
// Each provider (claude, codex, kimi, etc.) gets its own CLI-specific args.
func (d *Daemon) runAgentAttempt(job protocol.Job, agentCfg *protocol.Agent, runtime *protocol.AgentRuntime, entry AgentEntry, task *protocol.AgentTask, attempt, maxAttempts int) {
	// Build isolated workdir.
	workdir, err := d.buildWorkdir(job, agentCfg)
	if err != nil {
		d.failJob(job.ID, fmt.Sprintf("build workdir: %v", err))
		d.markTaskFailed(agentCfg.ID, task.ID, attempt, fmt.Sprintf("build workdir: %v", err))
		return
	}
	defer os.RemoveAll(workdir)

	d.updateProgress(job.ID, 15)

	// Mark task as started.
	d.markTaskStarted(agentCfg.ID, task.ID, workdir)
	d.updateProgress(job.ID, 20)

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
		d.failJob(job.ID, fmt.Sprintf("create agent backend: %v", err))
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

	d.updateProgress(job.ID, 25)

	batcher := newTaskMessageBatcher(d, job.ID, task.ID, workdir, 500*time.Millisecond)
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
		d.updateProgress(job.ID, 85)

		// Collect output from workdir and merge into wiki.
		outputStr, err := d.collectOutput(workdir, job, task)
		if err != nil {
			slog.Error("collect output failed", "job_id", job.ID, "task_id", task.ID, "error", err)
			d.markTaskFailed(agentCfg.ID, task.ID, attempt, fmt.Sprintf("collect output: %v", err))
			task.Status = "failed"
			return
		}

		d.markTaskCompleted(agentCfg.ID, task.ID, outputStr, result.SessionID, workdir)
		task.Status = "completed"
		d.updateProgress(job.ID, 100)
		d.completeJob(job.ID)
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
func (d *Daemon) fetchAgent(agentID string) (*protocol.Agent, error) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/agents/%s", d.ServerURL, d.WorkspaceSlug, agentID)
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
func (d *Daemon) fetchRuntime(runtimeID string) (*protocol.AgentRuntime, error) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/agents/runtimes/%s", d.ServerURL, d.WorkspaceSlug, runtimeID)
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
func (d *Daemon) fetchSchema(schemaID string) (*protocol.Schema, error) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/schemas/%s", d.ServerURL, d.WorkspaceSlug, schemaID)
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
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/agents/%s/tasks", d.ServerURL, d.WorkspaceSlug, agent.ID)

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
func (d *Daemon) patchTask(agentID, taskID string, body map[string]interface{}) error {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/agents/%s/tasks/%s", d.ServerURL, d.WorkspaceSlug, agentID, taskID)

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

func (d *Daemon) updateTask(agentID, taskID string, status, result, errMsg string, attempt int, sessionID, workDir string) error {
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

	return d.patchTask(agentID, taskID, body)
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
	d        *Daemon
	jobID    string
	taskID   string
	workDir  string
	interval time.Duration

	mu            sync.Mutex
	nextSeq       int64
	pending       []protocol.AgentTaskMessage
	pinnedSession bool
	done          chan struct{}
	closeOnce     sync.Once
}

func newTaskMessageBatcher(d *Daemon, jobID, taskID, workDir string, interval time.Duration) *taskMessageBatcher {
	b := &taskMessageBatcher{
		d:        d,
		jobID:    jobID,
		taskID:   taskID,
		workDir:  workDir,
		interval: interval,
		nextSeq:  1,
		done:     make(chan struct{}),
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

	b.d.postLogLine(b.jobID, string(msg.Type), content)
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
func (d *Daemon) markTaskStarted(agentID, taskID, workDir string) {
	if err := d.updateTask(agentID, taskID, "running", "", "", 0, "", workDir); err != nil {
		slog.Error("mark task started failed", "task_id", taskID, "error", err)
	}
}

// markTaskCompleted marks the task as "completed" with a result summary.
func (d *Daemon) markTaskCompleted(agentID, taskID, result, sessionID, workDir string) {
	if err := d.updateTask(agentID, taskID, "completed", result, "", 0, sessionID, workDir); err != nil {
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

// ---------------------------------------------------------------------------
// Workdir construction
// ---------------------------------------------------------------------------

// buildWorkdir creates the isolated workdir for an agent job using git.
//
// Instead of copying individual files over HTTP, the daemon:
//  1. Clones the workspace's bare git repo (or fetches if already cached)
//  2. Creates a git worktree from the bare clone into the job's workdir
//  3. Writes schema.md (wiki type/layer taxonomy — from DB)
//  4. Writes AGENTS.md (platform protocol + agent identity — generated dynamically)
//
// The agent sees a complete file tree:
//
//	/tmp/mulwiki-job-{jobID}/
//	├── AGENTS.md          ← Platform protocol + Agent identity
//	├── schema.md          ← Wiki schema definition (type/layer taxonomy)
//	├── sources/           ← All source documents
//	├── wiki/              ← Existing wiki pages (as .md files)
//	├── schemas/           ← Schema definitions
//	└── output/            ← Agent writes output here
func (d *Daemon) buildWorkdir(job protocol.Job, agent *protocol.Agent) (string, error) {
	workdir := filepath.Join(os.TempDir(), fmt.Sprintf("mulwiki-job-%s", job.ID))

	// Clean up stale workdir.
	if _, err := os.Stat(workdir); err == nil {
		slog.Warn("removing stale workdir", "workdir", workdir)
		if err := os.RemoveAll(workdir); err != nil {
			return "", fmt.Errorf("cleanup stale workdir: %w", err)
		}
	}

	// Fetch workspace ID to build the repo path.
	wsInfo, err := d.fetchWorkspace()
	if err != nil {
		return "", fmt.Errorf("fetch workspace: %w", err)
	}

	// Clone or fetch the workspace git repo.
	barePath, err := d.ensureRepo(wsInfo.ID)
	if err != nil {
		return "", fmt.Errorf("ensure repo: %w", err)
	}

	// Create worktree from bare clone.
	if err := d.createWorktree(barePath, workdir, job.ID); err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}

	// Create output dir.
	if err := os.MkdirAll(filepath.Join(workdir, "output"), 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// --- schema.md: wiki taxonomy (separate from AGENTS.md) ---
	if job.SchemaID != "" {
		schema, err := d.fetchSchema(job.SchemaID)
		if err != nil {
			slog.Warn("failed to fetch schema, continuing without schema content", "schema_id", job.SchemaID, "error", err)
		} else {
			if err := os.WriteFile(filepath.Join(workdir, "schema.md"), []byte(schema.Content), 0644); err != nil {
				return "", fmt.Errorf("write schema.md: %w", err)
			}
		}
	}

	// --- AGENTS.md: platform protocol + agent identity ---
	agentsMD := d.buildAgentsMD(agent, job)
	if err := os.WriteFile(filepath.Join(workdir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		return "", fmt.Errorf("write AGENTS.md: %w", err)
	}

	return workdir, nil
}

// fetchWorkspace retrieves workspace metadata.
func (d *Daemon) fetchWorkspace() (*protocol.Workspace, error) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s", d.ServerURL, d.WorkspaceSlug)
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

// ensureRepo clones or fetches the workspace git repo into a local bare cache.
// Returns the path to the bare clone.
func (d *Daemon) ensureRepo(workspaceID string) (string, error) {
	cacheDir := filepath.Join(d.WorkDir, ".repos")
	barePath := filepath.Join(cacheDir, workspaceID+".git")

	repoURL := d.repoURL(workspaceID)

	if isBareRepo(barePath) {
		// Already cached — fetch latest.
		slog.Debug("repo cached, fetching", "path", barePath, "url", repoURL)
		cmd := exec.Command("git", "-C", barePath, "fetch", "origin")
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("git fetch failed (continuing with cached data)", "error", err, "output", string(out))
		}
	} else {
		// First time — clone.
		slog.Info("cloning workspace repo", "url", repoURL, "path", barePath)
		if err := os.MkdirAll(filepath.Dir(barePath), 0755); err != nil {
			return "", fmt.Errorf("create cache dir: %w", err)
		}
		os.RemoveAll(barePath) // Clean any partial.

		cmd := exec.Command("git", "clone", "--bare", repoURL, barePath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git clone --bare: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	return barePath, nil
}

// repoURL returns the git URL for a workspace's bare repo.
// If ReposURL starts with "http", it's a remote HTTP git server.
// Otherwise, it's a local filesystem path.
func (d *Daemon) repoURL(workspaceID string) string {
	if strings.HasPrefix(d.ReposURL, "http") {
		return strings.TrimRight(d.ReposURL, "/") + "/" + workspaceID + ".git"
	}
	return filepath.Join(d.ReposURL, workspaceID+".git")
}

// createWorktree creates a git worktree from a bare clone into the target directory.
func (d *Daemon) createWorktree(barePath, workdir, jobID string) error {
	branchName := fmt.Sprintf("job/%s", shortJobID(jobID))

	// Prune stale worktree references from previous attempts, then delete the branch.
	_ = exec.Command("git", "-C", barePath, "worktree", "prune").Run()
	_ = exec.Command("git", "-C", barePath, "branch", "-D", branchName).Run()

	cmd := exec.Command("git", "-C", barePath, "worktree", "add", "-b", branchName, workdir, "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	slog.Debug("worktree created", "bare", barePath, "workdir", workdir, "branch", branchName)
	return nil
}

func isBareRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, "HEAD"))
	return err == nil
}

func shortJobID(jobID string) string {
	s := strings.ReplaceAll(jobID, "-", "")
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// ---------------------------------------------------------------------------
// Output collection
// ---------------------------------------------------------------------------

// collectOutput reads output/manifest.json, bundles referenced files, and delivers to server.
// The daemon does not interpret page types, layers, or the output format — that is the server's concern.
func (d *Daemon) collectOutput(workdir string, job protocol.Job, _ *protocol.AgentTask) (string, error) {
	manifestPath := filepath.Join(workdir, "output", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("read manifest.json: %w (agent may not have written output)", err)
	}

	var manifest struct {
		Pages []struct {
			Path  string `json:"path"`
			Title string `json:"title"`
			Type  string `json:"type"`
			Layer string `json:"layer"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("parse manifest.json: %w", err)
	}

	if len(manifest.Pages) == 0 {
		return "no pages in manifest", nil
	}

	type pageOut struct {
		Path    string `json:"path"`
		Title   string `json:"title"`
		Type    string `json:"type"`
		Layer   string `json:"layer"`
		Content string `json:"content"`
	}
	pages := make([]pageOut, 0, len(manifest.Pages))
	for _, p := range manifest.Pages {
		content, err := os.ReadFile(filepath.Join(workdir, "output", p.Path))
		if err != nil {
			slog.Warn("failed to read output file, skipping", "path", p.Path, "error", err)
			continue
		}
		pages = append(pages, pageOut{p.Path, p.Title, p.Type, p.Layer, string(content)})
	}

	payload := struct {
		JobID string    `json:"job_id"`
		Pages []pageOut `json:"pages"`
	}{job.ID, pages}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/output", d.ServerURL, d.WorkspaceSlug, job.ID)
	resp, err := d.postJSON(url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("deliver output: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server rejected output: %d %s", resp.StatusCode, string(body))
	}

	return fmt.Sprintf("pages=%d", len(pages)), nil
}

// ---------------------------------------------------------------------------
// Subprocess helpers
// ---------------------------------------------------------------------------

// mergeEnv merges the parent environment with the agent's custom environment.
// Custom values override parent values. Values containing potential secrets
// with keys matching *_SECRET, *_KEY, *_TOKEN, *_PASSWORD are logged safely.
func (d *Daemon) mergeEnv(parent []string, custom map[string]string) []string {
	// Start with parent env.
	merged := make([]string, len(parent))
	copy(merged, parent)

	// Override or append custom env vars.
	for k, v := range custom {
		idx := -1
		prefix := k + "="
		for i, envVar := range merged {
			if len(envVar) >= len(prefix) && envVar[:len(prefix)] == prefix {
				idx = i
				break
			}
		}
		newVal := prefix + v
		if idx >= 0 {
			merged[idx] = newVal
		} else {
			merged = append(merged, newVal)
		}

		// Never log secret values in clear text.
		if isSecretKey(k) {
			slog.Debug("set env", "key", k, "value", "***")
		} else {
			slog.Debug("set env", "key", k, "value", v)
		}
	}

	return merged
}

// isSecretKey returns true if the key name suggests it contains a secret.
func isSecretKey(key string) bool {
	upper := strings.ToUpper(key)
	return strings.Contains(upper, "SECRET") ||
		strings.Contains(upper, "KEY") ||
		strings.Contains(upper, "TOKEN") ||
		strings.Contains(upper, "PASSWORD") ||
		strings.Contains(upper, "API_KEY")
}

// streamLogs reads from an io.Reader line-by-line and forwards each line as a
// server-side log entry via HTTP POST, tagged with the job and stream name.
func (d *Daemon) streamLogs(reader io.Reader, jobID, stream string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		slog.Debug("agent log", "job_id", jobID, "stream", stream, "line", line)
		// Stream to server for real-time SSE.
		d.postLogLine(jobID, stream, line)
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("agent log stream error", "job_id", jobID, "stream", stream, "error", err)
	}
}

// postLogLine sends a single log line to the server's log buffer for SSE streaming.
func (d *Daemon) postLogLine(jobID, stream, line string) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/log-line", d.ServerURL, d.WorkspaceSlug, jobID)
	body := map[string]string{
		"stream": stream,
		"line":   line,
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		slog.Debug("post log line failed", "error", err)
		return
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Progress and job lifecycle helpers
// ---------------------------------------------------------------------------

// updateProgress sends a progress update to the server.
func (d *Daemon) updateProgress(jobID string, progress int) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/progress", d.ServerURL, d.WorkspaceSlug, jobID)
	body := map[string]int{"progress": progress}
	jsonBody, _ := json.Marshal(body)
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		slog.Error("update progress failed", "job_id", jobID, "error", err)
		return
	}
	resp.Body.Close()
}

// completeJob marks the job as completed on the server.
func (d *Daemon) completeJob(jobID string) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/complete", d.ServerURL, d.WorkspaceSlug, jobID)
	resp, err := d.postJSON(url, nil)
	if err != nil {
		slog.Error("complete job failed", "job_id", jobID, "error", err)
		return
	}
	resp.Body.Close()
	slog.Info("job completed", "job_id", jobID)
}

// failJob marks the job as failed on the server.
func (d *Daemon) failJob(jobID, errMsg string) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/fail", d.ServerURL, d.WorkspaceSlug, jobID)
	body := map[string]string{"error": errMsg}
	jsonBody, _ := json.Marshal(body)
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		slog.Error("fail job failed", "job_id", jobID, "error", err)
		return
	}
	resp.Body.Close()
	slog.Error("job failed", "job_id", jobID, "error", errMsg)
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

// copyFile copies a file from src to dst, creating the destination directory
// if needed. Uses io.Copy for efficient streaming.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	// Preserve permissions from source.
	srcInfo, _ := srcFile.Stat()
	if srcInfo != nil {
		os.Chmod(dst, srcInfo.Mode())
	}

	return nil
}
