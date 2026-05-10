package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/internal/daemon"
)

// ---------------------------------------------------------------------------
// Multi-level CLI
// ---------------------------------------------------------------------------

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Control the local agent runtime daemon",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the local agent runtime daemon",
	Long:  "Start the daemon process that polls for jobs and executes them using local agent CLIs (Claude, Codex, Kimi).\nRuns in the background by default. Use --foreground to run in the current terminal.",
	RunE:  runDaemonStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	RunE:  runDaemonStop,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE:  runDaemonStatus,
}

var daemonLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show daemon logs",
	RunE:  runDaemonLogs,
}

func init() {
	f := daemonStartCmd.Flags()
	f.Bool("foreground", false, "Run in the foreground instead of background")
	f.String("server-url", "", "Mulwiki server URL (env: MULWIKI_SERVER_URL)")
	f.String("workspace", "", "Workspace slug to watch (env: MULWIKI_WORKSPACE)")
	f.String("repos-path", "", "Path to bare git repos (env: MULWIKI_REPOS_PATH)")
	f.String("daemon-id", "", "Daemon identity (normally auto-generated)")
	f.String("daemon-token", "", "Daemon token (env: MULWIKI_DAEMON_TOKEN, file: ~/.mulwiki/daemon/token)")

	daemonLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	daemonLogsCmd.Flags().IntP("lines", "n", 50, "Number of lines to show")

	daemonStatusCmd.Flags().String("output", "table", "Output format: table or json")

	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonLogsCmd)
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

const (
	daemonHealthPort = 19515
)

func mulwikiDir() string {
	dir, _ := mulwikiProfileDir("")
	return dir
}

func mulwikiDirForProfile(profile string) string {
	dir, _ := mulwikiProfileDir(profile)
	return dir
}

func daemonDir() string {
	return daemonDirForProfile("")
}

func daemonDirForProfile(profile string) string {
	return filepath.Join(mulwikiDirForProfile(profile), "daemon")
}

func daemonPIDPath() string {
	return daemonPIDPathForProfile("")
}

func daemonPIDPathForProfile(profile string) string {
	return filepath.Join(daemonDirForProfile(profile), "daemon.pid")
}

func daemonLogPath() string {
	return daemonLogPathForProfile("")
}

func daemonLogPathForProfile(profile string) string {
	return filepath.Join(daemonDirForProfile(profile), "daemon.log")
}

func daemonIDPath() string {
	return daemonIDPathForProfile("")
}

func daemonIDPathForProfile(profile string) string {
	return filepath.Join(daemonDirForProfile(profile), "daemon.id")
}

func daemonTokenPath() string {
	return daemonTokenPathForProfile("")
}

func daemonTokenPathForProfile(profile string) string {
	return filepath.Join(daemonDirForProfile(profile), "token")
}

// ---------------------------------------------------------------------------
// daemon start
// ---------------------------------------------------------------------------

func runDaemonStart(cmd *cobra.Command, _ []string) error {
	foreground, _ := cmd.Flags().GetBool("foreground")
	if foreground {
		return runDaemonForeground(cmd)
	}
	return runDaemonBackground(cmd)
}

func runDaemonBackground(cmd *cobra.Command) error {
	profile := resolveProfile(cmd)
	healthPort := daemonHealthPortForProfile(profile)
	// Check if already running via health port.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	health := checkDaemonHealth(ctx, healthPort)
	if health["status"] == "running" {
		pid, _ := health["pid"].(float64)
		return fmt.Errorf("daemon is already running (pid %v)", int(pid))
	}

	// Resolve current executable.
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	// Build child args: daemon start --foreground + forwarded flags.
	args := buildDaemonStartArgs(cmd)

	// Ensure daemon directory exists.
	if err := os.MkdirAll(daemonDirForProfile(profile), 0o755); err != nil {
		return fmt.Errorf("create daemon directory: %w", err)
	}

	logFile, err := os.OpenFile(daemonLogPathForProfile(profile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	child := exec.Command(exePath, args...)
	child.Stdout = logFile
	child.Stderr = logFile
	child.SysProcAttr = daemonSysProcAttr(true)

	if err := child.Start(); err != nil {
		if isAccessDeniedSpawnErr(err) {
			child = exec.Command(exePath, args...)
			child.Stdout = logFile
			child.Stderr = logFile
			child.SysProcAttr = daemonSysProcAttr(false)
			if err := child.Start(); err != nil {
				logFile.Close()
				return fmt.Errorf("start daemon (no breakaway): %w", err)
			}
		} else {
			logFile.Close()
			return fmt.Errorf("start daemon: %w", err)
		}
	}
	logFile.Close()

	pid := child.Process.Pid
	// Detach: don't Wait() on the child.
	child.Process.Release()

	// Write PID file.
	if err := os.WriteFile(daemonPIDPathForProfile(profile), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write PID file: %v\n", err)
	}

	// Poll health endpoint until ready.
	deadline := time.Now().Add(15 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		hctx, hcancel := context.WithTimeout(context.Background(), 2*time.Second)
		health = checkDaemonHealth(hctx, healthPort)
		hcancel()
		if health["status"] == "running" {
			started = true
			break
		}
	}
	if !started {
		fmt.Fprintf(os.Stderr, "Daemon may not have started successfully. Check logs:\n  %s\n", daemonLogPathForProfile(profile))
		return nil
	}

	fmt.Fprintf(os.Stderr, "Daemon started (pid %d, version %s)\n", pid, version)
	fmt.Fprintf(os.Stderr, "Logs: %s\n", daemonLogPathForProfile(profile))
	return nil
}

func buildDaemonStartArgs(cmd *cobra.Command) []string {
	args := []string{"daemon", "start", "--foreground"}
	if profile := resolveProfile(cmd); profile != "" {
		args = append([]string{"--profile", profile}, args...)
	}
	if v, _ := cmd.Flags().GetString("server-url"); v != "" {
		args = append(args, "--server-url", v)
	}
	if v, _ := cmd.Flags().GetString("workspace"); v != "" {
		args = append(args, "--workspace", v)
	}
	if v, _ := cmd.Flags().GetString("repos-path"); v != "" {
		args = append(args, "--repos-path", v)
	}
	if v, _ := cmd.Flags().GetString("daemon-id"); v != "" {
		args = append(args, "--daemon-id", v)
	}
	if v, _ := cmd.Flags().GetString("daemon-token"); v != "" {
		args = append(args, "--daemon-token", v)
	}
	return args
}

func runDaemonForeground(cmd *cobra.Command) error {
	profile := resolveProfile(cmd)
	cliCfg, _ := loadCLIConfigForProfile(profile)

	serverURL, _ := cmd.Flags().GetString("server-url")
	if serverURL == "" {
		serverURL = os.Getenv("MULWIKI_SERVER_URL")
	}
	if serverURL == "" {
		serverURL = cliCfg.ServerURL
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	serverURL = strings.TrimRight(serverURL, "/")

	workspaceSlug, _ := cmd.Flags().GetString("workspace")
	if workspaceSlug == "" {
		workspaceSlug = os.Getenv("MULWIKI_WORKSPACE")
	}
	workspaceSlug = strings.TrimSpace(workspaceSlug)

	// Build configuration.
	reposPath, _ := cmd.Flags().GetString("repos-path")
	if reposPath == "" {
		reposPath = os.Getenv("MULWIKI_REPOS_PATH")
	}
	if reposPath == "" {
		// Default: repos live alongside the server
		reposPath = filepath.Join(os.Getenv("HOME"), "Documents/DevCode/github/mulwiki/server/data/repos")
	}

	daemonID, _ := cmd.Flags().GetString("daemon-id")
	if daemonID == "" {
		var err error
		daemonID, err = daemon.LoadOrCreateDaemonID(daemonIDPathForProfile(profile))
		if err != nil {
			return fmt.Errorf("load daemon id: %w", err)
		}
	}
	tokenFlag, _ := cmd.Flags().GetString("daemon-token")
	daemonToken, err := resolveDaemonTokenForStartForProfile(profile, tokenFlag, daemonTokenPathForProfile(profile), cliCfg, workspaceSlug, daemonID, serverURL)
	if err != nil {
		return fmt.Errorf("resolve daemon token: %w", err)
	}
	if daemonToken == "" {
		return fmt.Errorf("not authenticated for daemon start: run 'mulwiki login' or pass --daemon-token")
	}

	workspaceSlugs := splitWorkspaceSlugs(workspaceSlug)
	cfg := daemon.Config{
		ServerURL:              serverURL,
		WorkspaceSlug:          workspaceSlug,
		WorkspaceSlugs:         workspaceSlugs,
		AutoDiscoverWorkspaces: len(workspaceSlugs) == 0,
		DaemonID:               daemonID,
		DaemonToken:            daemonToken,
		WorkDir:                filepath.Join(os.TempDir(), "mulwiki-daemon", normalizeProfile(profile)),
		ReposURL:               reposPath,
		HealthPort:             daemonHealthPortForProfile(profile),
	}

	// Ensure daemon dir exists for PID file.
	os.MkdirAll(daemonDirForProfile(profile), 0o755)
	os.WriteFile(daemonPIDPathForProfile(profile), []byte(strconv.Itoa(os.Getpid())), 0o644)
	defer os.Remove(daemonPIDPathForProfile(profile))

	ctx, stop := signalContext()
	defer stop()

	d := daemon.New(cfg)
	return d.RunContext(ctx)
}

func resolveDaemonToken(flagValue, tokenPath string) (string, error) {
	if token := strings.TrimSpace(flagValue); token != "" {
		return token, nil
	}
	if token := strings.TrimSpace(os.Getenv("MULWIKI_DAEMON_TOKEN")); token != "" {
		return token, nil
	}
	data, err := os.ReadFile(tokenPath)
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if os.IsNotExist(err) {
		return "", nil
	}
	return "", err
}

func resolveDaemonTokenForStart(flagValue, tokenPath string, cfg CLIConfig, workspaceSlug, daemonID, serverURL string) (string, error) {
	return resolveDaemonTokenForStartForProfile("", flagValue, tokenPath, cfg, workspaceSlug, daemonID, serverURL)
}

func resolveDaemonTokenForStartForProfile(profile, flagValue, tokenPath string, cfg CLIConfig, workspaceSlug, daemonID, serverURL string) (string, error) {
	token, err := resolveDaemonToken(flagValue, tokenPath)
	if err != nil || token != "" {
		return token, err
	}
	if token := strings.TrimSpace(cfg.DaemonToken); token != "" {
		return token, nil
	}
	if cfg.DaemonTokens != nil {
		if token := strings.TrimSpace(cfg.DaemonTokens[workspaceSlug]); token != "" {
			return token, nil
		}
	}
	if cfg.SessionID == "" {
		return "", nil
	}

	token, err = mintUserDaemonToken(serverURL, cfg.SessionID, daemonID)
	if err != nil {
		if workspaceSlug == "" {
			return "", err
		}
		token, err = mintDaemonToken(serverURL, cfg.SessionID, workspaceSlug, daemonID)
		if err != nil {
			return "", err
		}
	}
	if token == "" {
		return "", nil
	}

	latest, err := loadCLIConfigForProfile(profile)
	if err != nil {
		return "", err
	}
	if latest.DaemonTokens == nil {
		latest.DaemonTokens = map[string]string{}
	}
	latest.ServerURL = serverURL
	latest.WorkspaceSlug = workspaceSlug
	latest.SessionID = cfg.SessionID
	latest.DaemonToken = token
	if workspaceSlug != "" {
		latest.DaemonTokens[workspaceSlug] = token
	}
	if err := saveCLIConfigForProfile(profile, latest); err != nil {
		return "", err
	}
	_ = os.MkdirAll(filepath.Dir(tokenPath), 0o755)
	_ = os.WriteFile(tokenPath, []byte(token+"\n"), 0o600)
	return token, nil
}

func mintDaemonToken(serverURL, sessionID, workspaceSlug, daemonID string) (string, error) {
	client := newAPIClient(serverURL)
	client.setSessionID(sessionID)

	var resp struct {
		Token string `json:"token"`
	}
	_, err := client.post(
		fmt.Sprintf("/api/workspaces/%s/daemon-tokens", url.PathEscape(workspaceSlug)),
		map[string]string{"daemon_id": daemonID},
		&resp,
	)
	if err != nil {
		return "", fmt.Errorf("create daemon token: %w", err)
	}
	return strings.TrimSpace(resp.Token), nil
}

func mintUserDaemonToken(serverURL, sessionID, daemonID string) (string, error) {
	client := newAPIClient(serverURL)
	client.setSessionID(sessionID)

	var resp struct {
		Token string `json:"token"`
	}
	_, err := client.post("/api/daemon-tokens", map[string]string{"daemon_id": daemonID}, &resp)
	if err != nil {
		return "", fmt.Errorf("create user daemon token: %w", err)
	}
	return strings.TrimSpace(resp.Token), nil
}

func splitWorkspaceSlugs(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !seen[part] {
			seen[part] = true
			out = append(out, part)
		}
	}
	return out
}

// signalContext returns a context that cancels on SIGINT or SIGTERM.
func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	// We use the daemon's own signal handling; this is just a backup.
	return ctx, cancel
}

// ---------------------------------------------------------------------------
// daemon stop
// ---------------------------------------------------------------------------

func runDaemonStop(cmd *cobra.Command, _ []string) error {
	profile := resolveProfile(cmd)
	pidData, err := os.ReadFile(daemonPIDPathForProfile(profile))
	if err != nil {
		return fmt.Errorf("daemon not running (no PID file)")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(daemonPIDPathForProfile(profile))
		return fmt.Errorf("daemon process not found (pid %d)", pid)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		// On Unix, FindProcess always succeeds; the error comes from Signal.
		// ESRCH means process is already gone.
		if errors.Is(err, os.ErrProcessDone) {
			os.Remove(daemonPIDPathForProfile(profile))
			fmt.Fprintln(os.Stderr, "Daemon was already stopped")
			return nil
		}
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	// Wait for process to exit.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			os.Remove(daemonPIDPathForProfile(profile))
			fmt.Fprintln(os.Stderr, "Daemon stopped")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Fprintln(os.Stderr, "Daemon did not stop gracefully, sending SIGKILL")
	process.Signal(syscall.SIGKILL)
	os.Remove(daemonPIDPathForProfile(profile))
	return nil
}

// ---------------------------------------------------------------------------
// daemon status
// ---------------------------------------------------------------------------

func runDaemonStatus(cmd *cobra.Command, _ []string) error {
	profile := resolveProfile(cmd)
	outputFormat, _ := cmd.Flags().GetString("output")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	health := checkDaemonHealth(ctx, daemonHealthPortForProfile(profile))

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(health)
		return nil
	}

	// Table format.
	status, _ := health["status"].(string)
	if status != "running" {
		fmt.Println("Daemon: not running")

		// Show PID file info if it exists but process is dead.
		if pidData, err := os.ReadFile(daemonPIDPathForProfile(profile)); err == nil {
			fmt.Printf("  Stale PID file: %s\n", strings.TrimSpace(string(pidData)))
		}
		return nil
	}

	pid, _ := health["pid"].(float64)
	uptime, _ := health["uptime"].(string)
	version, _ := health["version"].(string)
	workspaces, _ := health["workspaces"].([]any)
	runtimes, _ := health["runtimes"].([]any)

	fmt.Printf("Daemon: running\n")
	fmt.Printf("  PID:      %.0f\n", pid)
	fmt.Printf("  Version:  %s\n", version)
	fmt.Printf("  Uptime:   %s\n", uptime)
	if len(workspaces) > 0 {
		fmt.Printf("  Workspaces: %v\n", workspaces)
	}
	if len(runtimes) > 0 {
		fmt.Println("  Runtimes:")
		for _, r := range runtimes {
			if rm, ok := r.(map[string]any); ok {
				fmt.Printf("    %s (%s) — %s\n", rm["name"], rm["backend"], rm["status"])
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// daemon logs
// ---------------------------------------------------------------------------

func runDaemonLogs(cmd *cobra.Command, _ []string) error {
	profile := resolveProfile(cmd)
	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")

	logPath := daemonLogPathForProfile(profile)
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no daemon log file found at %s", logPath)
	}

	if follow {
		return tailFollow(logPath, lines)
	}

	return tailLines(logPath, lines)
}

func tailLines(path string, n int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	fmt.Print(strings.Join(lines[start:], "\n"))
	return nil
}

func tailFollow(path string, showLast int) error {
	// Show last N lines first.
	if err := tailLines(path, showLast); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Seek to end.
	f.Seek(0, 2)

	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			f.Seek(0, 2) // Re-seek in case log rotated.
			continue
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// Health check
// ---------------------------------------------------------------------------

func checkDaemonHealth(ctx context.Context, port ...int) map[string]any {
	healthPort := daemonHealthPort
	if len(port) > 0 && port[0] > 0 {
		healthPort = port[0]
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://localhost:%d/health", healthPort), nil)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"status": "stopped"}
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return map[string]any{"status": "error", "error": "invalid health response"}
	}
	return result
}
