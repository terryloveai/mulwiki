package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage CLI workspace selection",
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspaces available to the authenticated user",
	RunE:  runWorkspaceList,
}

var workspaceUseCmd = &cobra.Command{
	Use:   "use <slug>",
	Short: "Set the default workspace for this profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceUse,
}

func init() {
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceUseCmd)
	rootCmd.AddCommand(workspaceCmd)
}

func runWorkspaceList(cmd *cobra.Command, _ []string) error {
	profile := resolveProfile(cmd)
	cfg, client, err := authenticatedCLIClient(profile)
	if err != nil {
		return err
	}
	workspaces, err := listUserWorkspaces(client)
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stdout, "No workspaces.")
		return nil
	}
	for _, ws := range workspaces {
		marker := " "
		if ws.Slug == cfg.WorkspaceSlug {
			marker = "*"
		}
		fmt.Fprintf(os.Stdout, "%s %s\t%s\n", marker, ws.Slug, ws.Name)
	}
	return nil
}

func runWorkspaceUse(cmd *cobra.Command, args []string) error {
	profile := resolveProfile(cmd)
	slug := strings.TrimSpace(args[0])
	if slug == "" {
		return fmt.Errorf("workspace slug is required")
	}
	cfg, client, err := authenticatedCLIClient(profile)
	if err != nil {
		return err
	}
	workspaces, err := listUserWorkspaces(client)
	if err != nil {
		return err
	}
	found := false
	for _, ws := range workspaces {
		if ws.Slug == slug {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("workspace %q is not available to this user", slug)
	}
	cfg.WorkspaceSlug = slug
	if err := saveCLIConfigForProfile(profile, cfg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Default workspace for profile %s: %s\n", profileLabel(profile), slug)
	return nil
}

func authenticatedCLIClient(profile string) (CLIConfig, *apiClient, error) {
	cfg, err := loadCLIConfigForProfile(profile)
	if err != nil {
		return CLIConfig{}, nil, err
	}
	if cfg.SessionID == "" {
		return CLIConfig{}, nil, fmt.Errorf("not authenticated: run 'mulwiki login' first")
	}
	serverURL := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	client := newAPIClient(serverURL)
	client.setSessionID(cfg.SessionID)
	return cfg, client, nil
}

func listUserWorkspaces(client *apiClient) ([]protocol.Workspace, error) {
	var workspaces []protocol.Workspace
	if err := client.get("/api/workspaces", &workspaces); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return workspaces, nil
}
