package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and update CLI configuration",
	RunE:  runConfigShow,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show CLI configuration",
	RunE:  runConfigShow,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a CLI configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func init() {
	configShowCmd.Flags().String("output", "table", "Output format: table or json")
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	profile := resolveProfile(cmd)
	cfg, err := loadCLIConfigForProfile(profile)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	}
	path, _ := cliConfigPathForProfile(profile)
	fmt.Fprintf(os.Stdout, "Profile:   %s\n", profileLabel(profile))
	fmt.Fprintf(os.Stdout, "Path:      %s\n", path)
	fmt.Fprintf(os.Stdout, "Server:    %s\n", valueOrUnset(cfg.ServerURL))
	fmt.Fprintf(os.Stdout, "Workspace: %s\n", valueOrUnset(cfg.WorkspaceSlug))
	if cfg.SessionID != "" {
		fmt.Fprintln(os.Stdout, "Session:   stored")
	} else {
		fmt.Fprintln(os.Stdout, "Session:   unset")
	}
	if cfg.DaemonToken != "" || len(cfg.DaemonTokens) > 0 {
		fmt.Fprintln(os.Stdout, "Daemon:    token stored")
	} else {
		fmt.Fprintln(os.Stdout, "Daemon:    token unset")
	}
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	profile := resolveProfile(cmd)
	key := strings.TrimSpace(args[0])
	value := strings.TrimSpace(args[1])
	cfg, err := loadCLIConfigForProfile(profile)
	if err != nil {
		return err
	}
	switch key {
	case "server_url", "server-url", "server":
		cfg.ServerURL = strings.TrimRight(value, "/")
	case "workspace_slug", "workspace", "workspace-slug":
		cfg.WorkspaceSlug = value
	case "session_id", "session":
		cfg.SessionID = value
	case "daemon_token", "daemon-token":
		cfg.DaemonToken = value
	default:
		return fmt.Errorf("unsupported config key %q", key)
	}
	if err := saveCLIConfigForProfile(profile, cfg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Updated %s for profile %s.\n", key, profileLabel(profile))
	return nil
}

func profileLabel(profile string) string {
	if profile == "" || profile == "default" {
		return "default"
	}
	return profile
}

func valueOrUnset(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unset"
	}
	return value
}
