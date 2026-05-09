package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Work with agents",
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agents in the workspace",
	RunE:  runAgentList,
}

func init() {
	agentListCmd.Flags().String("server-url", "", "Mulwiki server URL (env: MULWIKI_SERVER_URL)")
	agentListCmd.Flags().String("workspace", "", "Workspace slug (env: MULWIKI_WORKSPACE)")
	agentListCmd.Flags().String("output", "table", "Output format: table or json")

	agentCmd.AddCommand(agentListCmd)
	rootCmd.AddCommand(agentCmd)
}

func runAgentList(cmd *cobra.Command, _ []string) error {
	cfg, _ := loadCLIConfig()
	serverURL := flagOrEnvConfig(cmd, "server-url", "MULWIKI_SERVER_URL", cfg.ServerURL, "http://localhost:8080")
	workspace := flagOrEnvConfig(cmd, "workspace", "MULWIKI_WORKSPACE", cfg.WorkspaceSlug, "")
	outputFormat, _ := cmd.Flags().GetString("output")

	if workspace == "" {
		return fmt.Errorf("workspace is required (--workspace, MULWIKI_WORKSPACE, or login config)")
	}

	client := newAPIClient(serverURL)
	client.setSessionID(cfg.SessionID)

	type agentItem struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Status      string `json:"status"`
		RuntimeID   string `json:"runtime_id"`
		Visibility  string `json:"visibility"`
		Description string `json:"description"`
		Model       string `json:"model"`
	}

	var resp struct {
		Agents []agentItem `json:"agents"`
	}
	if err := client.get(fmt.Sprintf("/api/workspaces/%s/agents", workspace), &resp); err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	agents := resp.Agents

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(agents)
	}

	if len(agents) == 0 {
		fmt.Println("No agents configured. Create agents in the web UI or API.")
		return nil
	}

	fmt.Printf("%-24s %-12s %-10s %s\n", "NAME", "STATUS", "VISIBILITY", "MODEL")
	for _, a := range agents {
		statusIcon := map[string]string{
			"online": "●", "offline": "○", "active": "●",
		}[a.Status]
		if statusIcon == "" {
			statusIcon = "○"
		}
		model := a.Model
		if model == "" {
			model = "(default)"
		}
		fmt.Printf("%-24s %s %-7s %-10s %s\n",
			truncate(a.Name, 24), statusIcon, a.Status, a.Visibility, model)
	}
	fmt.Printf("\n%d agent(s)\n", len(agents))
	return nil
}
