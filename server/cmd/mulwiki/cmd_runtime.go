package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Work with agent runtimes",
}

var runtimeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List runtimes registered by daemons in the workspace",
	RunE:  runRuntimeList,
}

func init() {
	runtimeListCmd.Flags().String("server-url", "", "Mulwiki server URL (env: MULWIKI_SERVER_URL)")
	runtimeListCmd.Flags().String("workspace", "", "Workspace slug (env: MULWIKI_WORKSPACE)")
	runtimeListCmd.Flags().String("output", "table", "Output format: table or json")

	runtimeCmd.AddCommand(runtimeListCmd)
	rootCmd.AddCommand(runtimeCmd)
}

func runRuntimeList(cmd *cobra.Command, _ []string) error {
	serverURL := flagOrEnv(cmd, "server-url", "MULWIKI_SERVER_URL", "http://localhost:8080")
	workspace := flagOrEnv(cmd, "workspace", "MULWIKI_WORKSPACE", "")
	outputFormat, _ := cmd.Flags().GetString("output")

	if workspace == "" {
		return fmt.Errorf("workspace is required (--workspace or MULWIKI_WORKSPACE)")
	}

	client := newAPIClient(serverURL)

	type runtimeItem struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Backend       string `json:"backend"`
		Version       string `json:"version"`
		Hostname      string `json:"hostname"`
		Status        string `json:"status"`
		DaemonID      string `json:"daemon_id"`
		LastHeartbeat string `json:"last_heartbeat"`
		Path          string `json:"path"`
	}

	var resp struct {
		Runtimes []runtimeItem `json:"runtimes"`
	}
	if err := client.get(fmt.Sprintf("/api/workspaces/%s/agents/runtimes", workspace), &resp); err != nil {
		return fmt.Errorf("list runtimes: %w", err)
	}

	runtimes := resp.Runtimes

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(runtimes)
	}

	if len(runtimes) == 0 {
		fmt.Println("No runtimes registered. Start a daemon to auto-register runtimes.")
		return nil
	}

	// Table output.
	fmt.Printf("%-20s %-14s %-26s %-12s %s\n", "NAME", "BACKEND", "DAEMON", "STATUS", "VERSION")
	for _, rt := range runtimes {
		shortDaemon := rt.DaemonID
		if len(shortDaemon) > 12 {
			shortDaemon = shortDaemon[:12]
		}
		status := rt.Status
		lastBeat, err := time.Parse(time.RFC3339, rt.LastHeartbeat)
		if err == nil && status == "online" {
			ago := time.Since(lastBeat).Truncate(time.Second)
			if ago > 5*time.Minute {
				status = "stale"
			}
		}
		statusIcon := map[string]string{
			"online": "●", "offline": "○", "stale": "◐",
		}[status]
		if statusIcon == "" {
			statusIcon = "?"
		}
		fmt.Printf("%-20s %-14s %-26s %s %-7s %s\n",
			truncate(rt.Name, 20), rt.Backend, shortDaemon, statusIcon, status, rt.Version)
	}
	fmt.Printf("\n%d runtime(s)\n", len(runtimes))
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func flagOrEnv(cmd *cobra.Command, flagName, envKey, defaultVal string) string {
	if v, _ := cmd.Flags().GetString(flagName); v != "" {
		return v
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return defaultVal
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
