package main

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Work with agent runtimes",
}

var runtimeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List runtimes",
	RunE:  runRuntimeList,
}

var runtimeGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Show a runtime",
	Args:  cobra.ExactArgs(1),
	RunE:  runRuntimeGet,
}

var runtimeCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a manual runtime",
	RunE:  runRuntimeCreate,
}

var runtimeUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Update a runtime",
	Args:  cobra.ExactArgs(1),
	RunE:  runRuntimeUpdate,
}

var runtimeDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a runtime",
	Args:  cobra.ExactArgs(1),
	RunE:  runRuntimeDelete,
}

func init() {
	addWorkspaceFlag(runtimeListCmd)
	addOutputFlag(runtimeListCmd, outputTable)
	addWorkspaceFlag(runtimeGetCmd)
	addOutputFlag(runtimeGetCmd, outputJSON)
	addRuntimeCreateFlags(runtimeCreateCmd)
	addRuntimeUpdateFlags(runtimeUpdateCmd)
	addWorkspaceFlag(runtimeDeleteCmd)
	runtimeDeleteCmd.Flags().Bool("yes", false, "Confirm deletion")

	runtimeCmd.AddCommand(runtimeListCmd)
	runtimeCmd.AddCommand(runtimeGetCmd)
	runtimeCmd.AddCommand(runtimeCreateCmd)
	runtimeCmd.AddCommand(runtimeUpdateCmd)
	runtimeCmd.AddCommand(runtimeDeleteCmd)
	rootCmd.AddCommand(runtimeCmd)
}

func addRuntimeCreateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Runtime name")
	cmd.Flags().String("backend", "", "Runtime backend")
	cmd.Flags().String("path", "", "CLI executable path")
	cmd.Flags().String("hostname", "", "Hostname")
	cmd.Flags().String("os", runtime.GOOS, "Operating system")
	cmd.Flags().String("version", "", "Runtime version")
	addOutputFlag(cmd, outputJSON)
}

func addRuntimeUpdateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Runtime name")
	cmd.Flags().String("backend", "", "Runtime backend")
	cmd.Flags().String("path", "", "CLI executable path")
	cmd.Flags().String("hostname", "", "Hostname")
	cmd.Flags().String("os", "", "Operating system")
	cmd.Flags().String("version", "", "Runtime version")
	addOutputFlag(cmd, outputJSON)
}

func runRuntimeList(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	runtimes, err := listRuntimes(client, workspace)
	if err != nil {
		return err
	}
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), runtimes)
	}
	rows := make([][]string, 0, len(runtimes))
	for _, rt := range runtimes {
		rows = append(rows, []string{rt.ID, rt.Name, rt.Backend, rt.Status, rt.Version, rt.DaemonID})
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "NAME", "BACKEND", "STATUS", "VERSION", "DAEMON"}, rows)
	return nil
}

func runRuntimeGet(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	rt, err := resolveRuntime(client, workspace, args[0])
	if err != nil {
		return err
	}
	return writeRuntimeOutput(cmd, rt)
}

func runRuntimeCreate(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	backend, _ := cmd.Flags().GetString("backend")
	path, _ := cmd.Flags().GetString("path")
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("--name is required")
	}
	req := protocol.CreateRuntimeRequest{Name: name, Backend: backend, Path: path}
	req.Hostname, _ = cmd.Flags().GetString("hostname")
	req.OS, _ = cmd.Flags().GetString("os")
	req.Version, _ = cmd.Flags().GetString("version")
	var resp struct {
		Runtime protocol.AgentRuntime `json:"runtime"`
	}
	if _, err := client.post(runtimeBasePath(workspace), req, &resp); err != nil {
		return fmt.Errorf("create runtime: %w", err)
	}
	return writeRuntimeOutput(cmd, resp.Runtime)
}

func runRuntimeUpdate(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	rt, err := resolveRuntime(client, workspace, args[0])
	if err != nil {
		return err
	}
	req := protocol.UpdateRuntimeRequest{}
	if flagChanged(cmd, "name") {
		value, _ := cmd.Flags().GetString("name")
		req.Name = &value
	}
	if flagChanged(cmd, "backend") {
		value, _ := cmd.Flags().GetString("backend")
		req.Backend = &value
	}
	if flagChanged(cmd, "path") {
		value, _ := cmd.Flags().GetString("path")
		req.Path = &value
	}
	if flagChanged(cmd, "hostname") {
		value, _ := cmd.Flags().GetString("hostname")
		req.Hostname = &value
	}
	if flagChanged(cmd, "os") {
		value, _ := cmd.Flags().GetString("os")
		req.OS = &value
	}
	if flagChanged(cmd, "version") {
		value, _ := cmd.Flags().GetString("version")
		req.Version = &value
	}
	var resp struct {
		Runtime protocol.AgentRuntime `json:"runtime"`
	}
	if _, err := client.patch(runtimeItemPath(workspace, rt.ID), req, &resp); err != nil {
		return fmt.Errorf("update runtime: %w", err)
	}
	return writeRuntimeOutput(cmd, resp.Runtime)
}

func runRuntimeDelete(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("refusing to delete runtime without --yes")
	}
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	rt, err := resolveRuntime(client, workspace, args[0])
	if err != nil {
		return err
	}
	if _, err := client.delete(runtimeItemPath(workspace, rt.ID)); err != nil {
		return fmt.Errorf("delete runtime: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted runtime %s\n", rt.ID)
	return nil
}

func listRuntimes(client *apiClient, workspace string) ([]protocol.AgentRuntime, error) {
	var resp struct {
		Runtimes []protocol.AgentRuntime `json:"runtimes"`
	}
	if err := client.get(runtimeBasePath(workspace), &resp); err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}
	return resp.Runtimes, nil
}

func runtimeItemPath(workspace, id string) string {
	return runtimeBasePath(workspace) + "/" + url.PathEscape(id)
}

func writeRuntimeOutput(cmd *cobra.Command, rt protocol.AgentRuntime) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), rt)
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "NAME", "BACKEND", "STATUS", "VERSION", "DAEMON"}, [][]string{
		{rt.ID, rt.Name, rt.Backend, rt.Status, rt.Version, rt.DaemonID},
	})
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "..."
}
