package main

import (
	"fmt"
	"net/url"
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

var workspaceGetCmd = &cobra.Command{
	Use:   "get [slug]",
	Short: "Show a workspace",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runWorkspaceGet,
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a workspace",
	RunE:  runWorkspaceCreate,
}

var workspaceUpdateCmd = &cobra.Command{
	Use:   "update [slug]",
	Short: "Update a workspace",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runWorkspaceUpdate,
}

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Delete a workspace",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceDelete,
}

var workspaceUseCmd = &cobra.Command{
	Use:   "use <slug>",
	Short: "Set the default workspace for this profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceUse,
}

func init() {
	addOutputFlag(workspaceListCmd, outputTable)
	addOutputFlag(workspaceGetCmd, outputJSON)
	addWorkspaceCreateFlags(workspaceCreateCmd)
	addWorkspaceUpdateFlags(workspaceUpdateCmd)
	addWorkspaceDeleteFlags(workspaceDeleteCmd)

	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceGetCmd)
	workspaceCmd.AddCommand(workspaceCreateCmd)
	workspaceCmd.AddCommand(workspaceUpdateCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
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
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), workspaces)
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No workspaces.")
		return nil
	}
	rows := make([][]string, 0, len(workspaces))
	for _, ws := range workspaces {
		marker := " "
		if ws.Slug == cfg.WorkspaceSlug {
			marker = "*"
		}
		rows = append(rows, []string{marker, ws.Slug, ws.Name, ws.Description})
	}
	printTable(cmd.OutOrStdout(), []string{"", "SLUG", "NAME", "DESCRIPTION"}, rows)
	return nil
}

func runWorkspaceGet(cmd *cobra.Command, args []string) error {
	cfg, client, err := authenticatedCLIClient(resolveProfile(cmd))
	if err != nil {
		return err
	}
	slug := cfg.WorkspaceSlug
	if len(args) > 0 {
		slug = strings.TrimSpace(args[0])
	}
	if slug == "" {
		return fmt.Errorf("workspace slug is required: pass a slug or run 'mulwiki workspace use <slug>'")
	}
	var ws protocol.Workspace
	if err := client.get("/api/workspaces/"+url.PathEscape(slug), &ws); err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), ws)
	}
	printTable(cmd.OutOrStdout(), []string{"SLUG", "NAME", "DESCRIPTION", "ACTIVE_SCHEMA"}, [][]string{
		{ws.Slug, ws.Name, ws.Description, optionalString(ws.ActiveSchemaPath)},
	})
	return nil
}

func addWorkspaceCreateFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "Workspace name")
	cmd.Flags().String("slug", "", "Workspace slug")
	addTextInputFlags(cmd, "description", "Workspace description")
	cmd.Flags().String("schema", "", "Initial schema: blank or builtin:<path>")
	cmd.Flags().Bool("use", false, "Set this workspace as the profile default after creating it")
	addOutputFlag(cmd, outputJSON)
}

func runWorkspaceCreate(cmd *cobra.Command, _ []string) error {
	profile := resolveProfile(cmd)
	cfg, client, err := authenticatedCLIClient(profile)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	slug, _ := cmd.Flags().GetString("slug")
	description, ok, err := resolveTextFlag(cmd, "description", cmd.InOrStdin())
	if err != nil {
		return err
	}
	if !ok {
		description = ""
	}
	schema, _ := cmd.Flags().GetString("schema")
	req := protocol.CreateWorkspaceRequest{
		Name:        strings.TrimSpace(name),
		Slug:        strings.TrimSpace(slug),
		Description: description,
	}
	switch {
	case schema == "":
	case schema == "blank":
		req.InitialSchemaType = "blank"
	case strings.HasPrefix(schema, "builtin:"):
		req.InitialSchemaType = "builtin"
		req.InitialSchemaPath = strings.TrimPrefix(schema, "builtin:")
	default:
		return fmt.Errorf("invalid --schema %q: use blank or builtin:<path>", schema)
	}
	if req.Name == "" {
		return fmt.Errorf("--name is required")
	}

	var ws protocol.Workspace
	if _, err := client.post("/api/workspaces", req, &ws); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	useWorkspace, _ := cmd.Flags().GetBool("use")
	if useWorkspace {
		cfg.WorkspaceSlug = ws.Slug
		if err := saveCLIConfigForProfile(profile, cfg); err != nil {
			return err
		}
	}
	return writeWorkspaceOutput(cmd, ws)
}

func addWorkspaceUpdateFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "Workspace name")
	addTextInputFlags(cmd, "description", "Workspace description")
	addOutputFlag(cmd, outputJSON)
}

func runWorkspaceUpdate(cmd *cobra.Command, args []string) error {
	cfg, client, err := authenticatedCLIClient(resolveProfile(cmd))
	if err != nil {
		return err
	}
	slug := cfg.WorkspaceSlug
	if len(args) > 0 {
		slug = strings.TrimSpace(args[0])
	}
	if slug == "" {
		return fmt.Errorf("workspace slug is required: pass a slug or run 'mulwiki workspace use <slug>'")
	}
	name, _ := cmd.Flags().GetString("name")
	description, ok, err := resolveTextFlag(cmd, "description", cmd.InOrStdin())
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		current, err := getWorkspace(client, slug)
		if err != nil {
			return err
		}
		name = current.Name
		if !ok {
			description = current.Description
		}
	}
	req := protocol.UpdateWorkspaceRequest{Name: strings.TrimSpace(name), Description: description}
	var ws protocol.Workspace
	if _, err := client.patch("/api/workspaces/"+url.PathEscape(slug), req, &ws); err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	return writeWorkspaceOutput(cmd, ws)
}

func addWorkspaceDeleteFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("yes", false, "Confirm deletion")
}

func runWorkspaceDelete(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("refusing to delete workspace without --yes")
	}
	_, client, err := authenticatedCLIClient(resolveProfile(cmd))
	if err != nil {
		return err
	}
	slug := strings.TrimSpace(args[0])
	if slug == "" {
		return fmt.Errorf("workspace slug is required")
	}
	if _, err := client.delete("/api/workspaces/" + url.PathEscape(slug)); err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted workspace %s\n", slug)
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

func getWorkspace(client *apiClient, slug string) (protocol.Workspace, error) {
	var ws protocol.Workspace
	if err := client.get("/api/workspaces/"+url.PathEscape(slug), &ws); err != nil {
		return protocol.Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	return ws, nil
}

func writeWorkspaceOutput(cmd *cobra.Command, ws protocol.Workspace) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), ws)
	}
	printTable(cmd.OutOrStdout(), []string{"SLUG", "NAME", "DESCRIPTION"}, [][]string{
		{ws.Slug, ws.Name, ws.Description},
	})
	return nil
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
