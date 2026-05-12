package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

type doctorCheck struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type doctorReport struct {
	Profile            string      `json:"profile,omitempty"`
	ServerURL          string      `json:"server_url"`
	WorkspaceSlug      string      `json:"workspace_slug,omitempty"`
	Server             doctorCheck `json:"server"`
	Auth               doctorCheck `json:"auth"`
	Workspace          doctorCheck `json:"workspace"`
	DaemonToken        doctorCheck `json:"daemon_token"`
	LocalDaemon        doctorCheck `json:"local_daemon"`
	ServerRegistration doctorCheck `json:"server_registration"`
	Runtimes           doctorCheck `json:"runtimes"`
}

type daemonListResponse struct {
	Daemons []daemonListItem `json:"daemons"`
}

type daemonListItem struct {
	ID                 string   `json:"id"`
	Hostname           string   `json:"hostname"`
	PID                int      `json:"pid"`
	Version            string   `json:"version"`
	RuntimeIDs         string   `json:"runtime_ids"`
	WorkspaceSlugs     []string `json:"workspace_slugs"`
	MaxConcurrentTasks int      `json:"max_concurrent_tasks"`
	LastHeartbeat      string   `json:"last_heartbeat"`
	RegisteredAt       string   `json:"registered_at"`
}

type runtimeListResponse struct {
	Runtimes []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Status        string `json:"status"`
		DaemonID      string `json:"daemon_id"`
		LastHeartbeat string `json:"last_heartbeat"`
	} `json:"runtimes"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check CLI, server, workspace, and daemon health",
	RunE:  runDoctor,
}

func init() {
	addOutputFlag(doctorCmd, outputTable)
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	report := collectDoctorReport(resolveProfile(cmd), daemonHealthPortForProfile(resolveProfile(cmd)))
	if format == outputJSON {
		return writeJSONOutput(cmd.OutOrStdout(), report)
	}
	writeDoctorReport(cmd, report)
	return nil
}

func collectDoctorReport(profile string, healthPort int) doctorReport {
	cfg, cfgErr := loadCLIConfigForProfile(profile)
	serverURL := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	report := doctorReport{
		Profile:       normalizeProfile(profile),
		ServerURL:     serverURL,
		WorkspaceSlug: cfg.WorkspaceSlug,
	}
	if cfgErr != nil {
		report.Auth = doctorCheck{Status: "error", Detail: cfgErr.Error()}
		return report
	}

	if err := checkServerHealth(serverURL); err != nil {
		report.Server = doctorCheck{Status: "error", Detail: err.Error()}
	} else {
		report.Server = doctorCheck{Status: "ok"}
	}

	client := newAPIClient(serverURL)
	client.setSessionID(cfg.SessionID)
	if cfg.SessionID == "" {
		report.Auth = doctorCheck{Status: "missing", Detail: "run 'mulwiki login'"}
	} else {
		var user protocol.User
		if err := client.get("/api/auth/me", &user); err != nil {
			report.Auth = doctorCheck{Status: "error", Detail: err.Error()}
		} else {
			report.Auth = doctorCheck{Status: "ok", Detail: user.Email}
		}
	}

	var workspaces []protocol.Workspace
	if cfg.SessionID == "" {
		report.Workspace = doctorCheck{Status: "skipped", Detail: "not authenticated"}
	} else if err := client.get("/api/workspaces", &workspaces); err != nil {
		report.Workspace = doctorCheck{Status: "error", Detail: err.Error()}
	} else if cfg.WorkspaceSlug == "" {
		report.Workspace = doctorCheck{Status: "warning", Detail: "no default workspace configured"}
	} else if workspaceInList(workspaces, cfg.WorkspaceSlug) {
		report.Workspace = doctorCheck{Status: "ok", Detail: cfg.WorkspaceSlug}
	} else {
		report.Workspace = doctorCheck{Status: "error", Detail: fmt.Sprintf("user is not a member of workspace %q", cfg.WorkspaceSlug)}
	}

	report.DaemonToken = checkDaemonToken(serverURL, cfg, cfg.WorkspaceSlug)

	if healthPort <= 0 {
		healthPort = daemonHealthPortForProfile(profile)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	health := checkDaemonHealth(ctx, healthPort)
	cancel()
	if health["status"] == "running" {
		report.LocalDaemon = doctorCheck{Status: "ok", Detail: fmt.Sprintf("pid %.0f", health["pid"])}
	} else {
		report.LocalDaemon = doctorCheck{Status: "warning", Detail: "local health endpoint is not reachable"}
	}

	if cfg.SessionID == "" || cfg.WorkspaceSlug == "" {
		report.ServerRegistration = doctorCheck{Status: "skipped", Detail: "auth or workspace missing"}
		report.Runtimes = doctorCheck{Status: "skipped", Detail: "auth or workspace missing"}
		return report
	}
	report.ServerRegistration = checkServerRegistration(client, cfg.WorkspaceSlug)
	report.Runtimes = checkWorkspaceRuntimes(client, cfg.WorkspaceSlug)
	return report
}

func checkServerHealth(serverURL string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(serverURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

func workspaceInList(workspaces []protocol.Workspace, slug string) bool {
	for _, ws := range workspaces {
		if ws.Slug == slug {
			return true
		}
	}
	return false
}

func checkServerRegistration(client *apiClient, workspaceSlug string) doctorCheck {
	var resp daemonListResponse
	if err := client.get(fmt.Sprintf("/api/workspaces/%s/daemons", workspaceSlug), &resp); err != nil {
		return doctorCheck{Status: "error", Detail: err.Error()}
	}
	if len(resp.Daemons) == 0 {
		return doctorCheck{Status: "warning", Detail: "no daemon registration for this workspace"}
	}
	live := 0
	for _, d := range resp.Daemons {
		if daemonHeartbeatFresh(d.LastHeartbeat, time.Now()) {
			live++
		}
	}
	if live == 0 {
		return doctorCheck{Status: "warning", Detail: fmt.Sprintf("%d daemon registration(s), all heartbeat stale", len(resp.Daemons))}
	}
	return doctorCheck{Status: "ok", Detail: fmt.Sprintf("%d live daemon(s)", live)}
}

func checkDaemonToken(serverURL string, cfg CLIConfig, workspaceSlug string) doctorCheck {
	token := strings.TrimSpace(cfg.DaemonToken)
	if token == "" && cfg.DaemonTokens != nil {
		token = strings.TrimSpace(cfg.DaemonTokens[workspaceSlug])
	}
	if token == "" {
		return doctorCheck{Status: "warning", Detail: "run 'mulwiki auth refresh' or 'mulwiki daemon start'"}
	}
	client := newAPIClient(serverURL)
	client.setBearerToken(token)
	var resp struct {
		Workspaces []protocol.DaemonWorkspace `json:"workspaces"`
	}
	if err := client.get("/api/daemon/workspaces", &resp); err != nil {
		return doctorCheck{Status: "error", Detail: "cached daemon token is invalid or expired"}
	}
	if workspaceSlug == "" {
		return doctorCheck{Status: "ok", Detail: fmt.Sprintf("%d accessible workspace(s)", len(resp.Workspaces))}
	}
	for _, ws := range resp.Workspaces {
		if ws.Slug == workspaceSlug {
			return doctorCheck{Status: "ok", Detail: fmt.Sprintf("valid for %s", workspaceSlug)}
		}
	}
	return doctorCheck{Status: "warning", Detail: fmt.Sprintf("token is valid but does not include workspace %q", workspaceSlug)}
}

func checkWorkspaceRuntimes(client *apiClient, workspaceSlug string) doctorCheck {
	var resp runtimeListResponse
	if err := client.get(fmt.Sprintf("/api/workspaces/%s/agents/runtimes", workspaceSlug), &resp); err != nil {
		return doctorCheck{Status: "error", Detail: err.Error()}
	}
	if len(resp.Runtimes) == 0 {
		return doctorCheck{Status: "warning", Detail: "no runtimes registered"}
	}
	online := 0
	for _, rt := range resp.Runtimes {
		if rt.Status == "online" && daemonHeartbeatFresh(rt.LastHeartbeat, time.Now()) {
			online++
		}
	}
	if online == 0 {
		return doctorCheck{Status: "warning", Detail: fmt.Sprintf("%d runtime(s), none online", len(resp.Runtimes))}
	}
	return doctorCheck{Status: "ok", Detail: fmt.Sprintf("%d online runtime(s)", online)}
}

func daemonHeartbeatFresh(value string, now time.Time) bool {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		parsed, err = time.Parse("2006-01-02 15:04:05", value)
	}
	if err != nil {
		return false
	}
	return now.Sub(parsed) <= 5*time.Minute
}

func writeDoctorReport(cmd *cobra.Command, report doctorReport) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Server:              %s", report.Server.Status)
	if report.Server.Detail != "" {
		fmt.Fprintf(out, " (%s)", report.Server.Detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Auth:                %s", report.Auth.Status)
	if report.Auth.Detail != "" {
		fmt.Fprintf(out, " (%s)", report.Auth.Detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Workspace:           %s", report.Workspace.Status)
	if report.Workspace.Detail != "" {
		fmt.Fprintf(out, " (%s)", report.Workspace.Detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Daemon token:        %s", report.DaemonToken.Status)
	if report.DaemonToken.Detail != "" {
		fmt.Fprintf(out, " (%s)", report.DaemonToken.Detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Local daemon:        %s", report.LocalDaemon.Status)
	if report.LocalDaemon.Detail != "" {
		fmt.Fprintf(out, " (%s)", report.LocalDaemon.Detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Server registration: %s", report.ServerRegistration.Status)
	if report.ServerRegistration.Detail != "" {
		fmt.Fprintf(out, " (%s)", report.ServerRegistration.Detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Runtimes:            %s", report.Runtimes.Status)
	if report.Runtimes.Detail != "" {
		fmt.Fprintf(out, " (%s)", report.Runtimes.Detail)
	}
	fmt.Fprintln(out)
}

func writeJSONOutput(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
