package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Manage workspace schemas",
}

var schemaBuiltinCmd = &cobra.Command{
	Use:   "builtin",
	Short: "Manage builtin schemas",
}

var schemaBuiltinListCmd = &cobra.Command{
	Use:   "list",
	Short: "List builtin schemas",
	RunE:  runSchemaBuiltinList,
}

var schemaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspace schemas",
	RunE:  runSchemaList,
}

var schemaGetCmd = &cobra.Command{
	Use:   "get <id-or-path>",
	Short: "Show a schema",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchemaGet,
}

var schemaCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a schema",
	RunE:  runSchemaCreate,
}

var schemaUpdateCmd = &cobra.Command{
	Use:   "update <id-or-path>",
	Short: "Update a schema",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchemaUpdate,
}

var schemaDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-path>",
	Short: "Delete a schema",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchemaDelete,
}

var schemaForkCmd = &cobra.Command{
	Use:   "fork <id-or-path>",
	Short: "Fork a schema",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchemaFork,
}

var schemaActivateCmd = &cobra.Command{
	Use:   "activate <id-or-path>",
	Short: "Set the active workspace schema",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchemaActivate,
}

var schemaValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate schema Markdown",
	RunE:  runSchemaValidate,
}

func init() {
	addWorkspaceFlag(schemaListCmd)
	addOutputFlag(schemaListCmd, outputTable)
	addSchemaGetFlags(schemaGetCmd)
	addSchemaCreateFlags(schemaCreateCmd)
	addSchemaUpdateFlags(schemaUpdateCmd)
	addSchemaDeleteFlags(schemaDeleteCmd)
	addSchemaForkFlags(schemaForkCmd)
	addWorkspaceFlag(schemaActivateCmd)
	addOutputFlag(schemaActivateCmd, outputJSON)
	addSchemaValidateFlags(schemaValidateCmd)
	addOutputFlag(schemaBuiltinListCmd, outputTable)

	schemaBuiltinCmd.AddCommand(schemaBuiltinListCmd)
	schemaCmd.AddCommand(schemaListCmd)
	schemaCmd.AddCommand(schemaBuiltinCmd)
	schemaCmd.AddCommand(schemaGetCmd)
	schemaCmd.AddCommand(schemaCreateCmd)
	schemaCmd.AddCommand(schemaUpdateCmd)
	schemaCmd.AddCommand(schemaDeleteCmd)
	schemaCmd.AddCommand(schemaForkCmd)
	schemaCmd.AddCommand(schemaActivateCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
	rootCmd.AddCommand(schemaCmd)
}

func addWorkspaceFlag(cmd *cobra.Command) {
	cmd.Flags().String("workspace", "", "Workspace slug")
}

func addSchemaGetFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().Bool("content", false, "Print only Markdown content")
	addOutputFlag(cmd, outputJSON)
}

func addSchemaCreateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Schema name")
	addTextInputFlags(cmd, "description", "Schema description")
	cmd.Flags().String("version", "1.0", "Schema version")
	addTextInputFlags(cmd, "content", "Schema Markdown content")
	addOutputFlag(cmd, outputJSON)
}

func addSchemaUpdateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Schema name")
	addTextInputFlags(cmd, "description", "Schema description")
	cmd.Flags().String("version", "", "Schema version")
	addTextInputFlags(cmd, "content", "Schema Markdown content")
	addOutputFlag(cmd, outputJSON)
}

func addSchemaDeleteFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().Bool("yes", false, "Confirm deletion")
}

func addSchemaForkFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Forked schema name")
	addTextInputFlags(cmd, "description", "Forked schema description")
	addOutputFlag(cmd, outputJSON)
}

func addSchemaValidateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	addTextInputFlags(cmd, "content", "Schema Markdown content")
	addOutputFlag(cmd, outputJSON)
}

func runSchemaBuiltinList(cmd *cobra.Command, _ []string) error {
	_, client, err := authenticatedCLIClient(resolveProfile(cmd))
	if err != nil {
		return err
	}
	var schemas []protocol.Schema
	if err := client.get("/api/schemas/builtin", &schemas); err != nil {
		return fmt.Errorf("list builtin schemas: %w", err)
	}
	return writeSchemaListOutput(cmd, schemasToRows(schemas), schemas)
}

func runSchemaList(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	schemas, err := listSchemas(client, workspace)
	if err != nil {
		return err
	}
	return writeSchemaListOutput(cmd, schemaRows(schemas), schemas)
}

func runSchemaGet(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	schema, err := resolveSchema(client, workspace, args[0])
	if err != nil {
		return err
	}
	contentOnly, _ := cmd.Flags().GetBool("content")
	if contentOnly {
		fmt.Fprintln(cmd.OutOrStdout(), schema.Content)
		return nil
	}
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), schema)
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "PATH", "NAME", "VERSION", "SOURCE"}, [][]string{
		{schema.ID, schema.Path, schema.Name, schema.Version, schema.SourceType},
	})
	return nil
}

func runSchemaCreate(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	version, _ := cmd.Flags().GetString("version")
	description, hasDescription, err := resolveTextFlag(cmd, "description", cmd.InOrStdin())
	if err != nil {
		return err
	}
	content, hasContent, err := resolveTextFlag(cmd, "content", cmd.InOrStdin())
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("--name is required")
	}
	if !hasDescription {
		description = ""
	}
	if !hasContent {
		return fmt.Errorf("--content or --content-stdin is required")
	}
	req := protocol.CreateSchemaRequest{
		Name:        strings.TrimSpace(name),
		Description: description,
		Version:     strings.TrimSpace(version),
		Content:     content,
	}
	var schema protocol.Schema
	if _, err := client.post(schemaBasePath(workspace), req, &schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return writeSchemaOutput(cmd, schema)
}

func runSchemaUpdate(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	current, err := resolveSchema(client, workspace, args[0])
	if err != nil {
		return err
	}
	req := protocol.UpdateSchemaRequest{}
	if flagChanged(cmd, "name") {
		name, _ := cmd.Flags().GetString("name")
		req.Name = &name
	}
	if flagChanged(cmd, "version") {
		version, _ := cmd.Flags().GetString("version")
		req.Version = &version
	}
	if description, ok, err := resolveTextFlag(cmd, "description", cmd.InOrStdin()); err != nil {
		return err
	} else if ok {
		req.Description = &description
	}
	if content, ok, err := resolveTextFlag(cmd, "content", cmd.InOrStdin()); err != nil {
		return err
	} else if ok {
		req.Content = &content
	}
	if req.Name == nil && req.Description == nil && req.Version == nil && req.Content == nil {
		return fmt.Errorf("no schema fields to update")
	}
	var schema protocol.Schema
	if _, err := client.put(schemaItemPath(workspace, current.ID), req, &schema); err != nil {
		return fmt.Errorf("update schema: %w", err)
	}
	return writeSchemaOutput(cmd, schema)
}

func runSchemaDelete(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("refusing to delete schema without --yes")
	}
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	schema, err := resolveSchema(client, workspace, args[0])
	if err != nil {
		return err
	}
	if _, err := client.delete(schemaItemPath(workspace, schema.ID)); err != nil {
		return fmt.Errorf("delete schema: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted schema %s\n", schema.ID)
	return nil
}

func runSchemaFork(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	source, err := resolveSchema(client, workspace, args[0])
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	description, ok, err := resolveTextFlag(cmd, "description", cmd.InOrStdin())
	if err != nil {
		return err
	}
	req := protocol.ForkSchemaRequest{SchemaID: source.ID, Name: strings.TrimSpace(name)}
	if ok {
		req.Description = description
	}
	var schema protocol.Schema
	if _, err := client.post(schemaBasePath(workspace)+"/fork", req, &schema); err != nil {
		return fmt.Errorf("fork schema: %w", err)
	}
	return writeSchemaOutput(cmd, schema)
}

func runSchemaActivate(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	schema, err := resolveSchema(client, workspace, args[0])
	if err != nil {
		return err
	}
	var resp map[string]string
	if _, err := client.put("/api/workspaces/"+url.PathEscape(workspace)+"/activate-schema", protocol.ActivateSchemaRequest{SchemaID: schema.ID}, &resp); err != nil {
		return fmt.Errorf("activate schema: %w", err)
	}
	return printJSON(cmd.OutOrStdout(), resp)
}

func runSchemaValidate(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	content, ok, err := resolveTextFlag(cmd, "content", cmd.InOrStdin())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("--content or --content-stdin is required")
	}
	var result protocol.ValidateSchemaResponse
	if _, err := client.post(schemaBasePath(workspace)+"/validate", protocol.ValidateSchemaRequest{Content: content}, &result); err != nil {
		return fmt.Errorf("validate schema: %w", err)
	}
	if err := printJSON(cmd.OutOrStdout(), result); err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("schema validation failed")
	}
	return nil
}

func workspaceClient(cmd *cobra.Command) (CLIConfig, *apiClient, string, error) {
	cfg, client, err := authenticatedCLIClient(resolveProfile(cmd))
	if err != nil {
		return CLIConfig{}, nil, "", err
	}
	workspace := cfg.WorkspaceSlug
	if cmd.Flags().Lookup("workspace") != nil {
		if value, err := cmd.Flags().GetString("workspace"); err == nil && strings.TrimSpace(value) != "" {
			workspace = strings.TrimSpace(value)
		}
	}
	if workspace == "" {
		return CLIConfig{}, nil, "", fmt.Errorf("workspace is required: pass --workspace or run 'mulwiki workspace use <slug>'")
	}
	return cfg, client, workspace, nil
}

func listSchemas(client *apiClient, workspace string) ([]protocol.SchemaWithActive, error) {
	var schemas []protocol.SchemaWithActive
	if err := client.get(schemaBasePath(workspace), &schemas); err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	return schemas, nil
}

func resolveSchema(client *apiClient, workspace, ref string) (protocol.Schema, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return protocol.Schema{}, fmt.Errorf("schema id or path is required")
	}
	schemas, err := listSchemas(client, workspace)
	if err != nil {
		return protocol.Schema{}, err
	}
	matches := make([]protocol.SchemaWithActive, 0, 1)
	for _, schema := range schemas {
		if schema.ID == ref || schema.Path == ref || schema.Name == ref {
			matches = append(matches, schema)
		}
	}
	if len(matches) == 0 {
		return protocol.Schema{}, fmt.Errorf("schema %q not found", ref)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, schema := range matches {
			ids = append(ids, schema.ID)
		}
		return protocol.Schema{}, fmt.Errorf("schema %q is ambiguous; matching ids: %s", ref, strings.Join(ids, ", "))
	}
	var schema protocol.Schema
	if err := client.get(schemaItemPath(workspace, matches[0].ID), &schema); err != nil {
		return protocol.Schema{}, fmt.Errorf("get schema: %w", err)
	}
	return schema, nil
}

func schemaBasePath(workspace string) string {
	return "/api/workspaces/" + url.PathEscape(workspace) + "/schemas"
}

func schemaItemPath(workspace, id string) string {
	return schemaBasePath(workspace) + "/" + url.PathEscape(id)
}

func writeSchemaOutput(cmd *cobra.Command, schema protocol.Schema) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), schema)
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "PATH", "NAME", "VERSION", "SOURCE"}, [][]string{
		{schema.ID, schema.Path, schema.Name, schema.Version, schema.SourceType},
	})
	return nil
}

func writeSchemaListOutput(cmd *cobra.Command, rows [][]string, value any) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), value)
	}
	printTable(cmd.OutOrStdout(), []string{"ACTIVE", "ID", "PATH", "NAME", "VERSION", "SOURCE"}, rows)
	return nil
}

func schemaRows(schemas []protocol.SchemaWithActive) [][]string {
	rows := make([][]string, 0, len(schemas))
	for _, schema := range schemas {
		active := ""
		if schema.IsActive {
			active = "*"
		}
		rows = append(rows, []string{active, schema.ID, schema.Path, schema.Name, schema.Version, schema.SourceType})
	}
	return rows
}

func schemasToRows(schemas []protocol.Schema) [][]string {
	rows := make([][]string, 0, len(schemas))
	for _, schema := range schemas {
		rows = append(rows, []string{"", schema.ID, schema.Path, schema.Name, schema.Version, schema.SourceType})
	}
	return rows
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}
