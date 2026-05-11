package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var sourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Manage workspace sources",
}

var sourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sources",
	RunE:  runSourceList,
}

var sourceAddCmd = &cobra.Command{
	Use:   "add <file>",
	Short: "Add a local source file",
	Args:  cobra.ExactArgs(1),
	RunE:  runSourceAdd,
}

var sourceGetCmd = &cobra.Command{
	Use:   "get <path>",
	Short: "Show source metadata or raw content",
	Args:  cobra.ExactArgs(1),
	RunE:  runSourceGet,
}

var sourceRemoveCmd = &cobra.Command{
	Use:     "remove <path>",
	Aliases: []string{"delete"},
	Short:   "Remove a source",
	Args:    cobra.ExactArgs(1),
	RunE:    runSourceRemove,
}

func init() {
	addWorkspaceFlag(sourceListCmd)
	addOutputFlag(sourceListCmd, outputTable)
	addSourceAddFlags(sourceAddCmd)
	addSourceGetFlags(sourceGetCmd)
	addWorkspaceFlag(sourceRemoveCmd)
	sourceRemoveCmd.Flags().Bool("yes", false, "Confirm deletion")

	sourceCmd.AddCommand(sourceListCmd)
	sourceCmd.AddCommand(sourceAddCmd)
	sourceCmd.AddCommand(sourceGetCmd)
	sourceCmd.AddCommand(sourceRemoveCmd)
	rootCmd.AddCommand(sourceCmd)
}

func addSourceAddFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("path", "", "Workspace path (reserved; server currently stores sources/<filename>)")
	addOutputFlag(cmd, outputJSON)
}

func addSourceGetFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().Bool("raw", false, "Print raw source content")
	addOutputFlag(cmd, outputJSON)
}

func runSourceList(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	sources, err := listSources(client, workspace)
	if err != nil {
		return err
	}
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), sources)
	}
	rows := make([][]string, 0, len(sources))
	for _, source := range sources {
		rows = append(rows, []string{source.Path, source.Type, fmt.Sprintf("%d", source.Size)})
	}
	printTable(cmd.OutOrStdout(), []string{"PATH", "TYPE", "SIZE"}, rows)
	return nil
}

func runSourceAdd(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	customPath, _ := cmd.Flags().GetString("path")
	if strings.TrimSpace(customPath) != "" {
		return fmt.Errorf("--path is not supported by the current server upload API")
	}
	source, err := uploadSourceFile(client, workspace, args[0])
	if err != nil {
		return err
	}
	return writeSourceOutput(cmd, source)
}

func runSourceGet(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	sourcePath := normalizeSourcePath(args[0])
	raw, _ := cmd.Flags().GetBool("raw")
	if raw {
		content, err := client.getRaw(sourceItemPath(workspace, sourcePath) + "/raw")
		if err != nil {
			return fmt.Errorf("get source raw: %w", err)
		}
		_, err = cmd.OutOrStdout().Write(content)
		return err
	}
	var source protocol.Source
	if err := client.get(sourceItemPath(workspace, sourcePath), &source); err != nil {
		return fmt.Errorf("get source: %w", err)
	}
	return writeSourceOutput(cmd, source)
}

func runSourceRemove(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("refusing to remove source without --yes")
	}
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	sourcePath := normalizeSourcePath(args[0])
	if _, err := client.delete(sourceItemPath(workspace, sourcePath)); err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed source %s\n", sourcePath)
	return nil
}

func uploadSourceFile(client *apiClient, workspace, filePath string) (protocol.Source, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return protocol.Source{}, fmt.Errorf("open source file: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return protocol.Source{}, fmt.Errorf("create multipart file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return protocol.Source{}, fmt.Errorf("read source file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return protocol.Source{}, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := client.newRequest(http.MethodPost, sourceBasePath(workspace), &body)
	if err != nil {
		return protocol.Source{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.http.Do(req)
	if err != nil {
		return protocol.Source{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return protocol.Source{}, fmt.Errorf("server returned %d", resp.StatusCode)
	}
	var source protocol.Source
	if err := json.NewDecoder(resp.Body).Decode(&source); err != nil {
		return protocol.Source{}, err
	}
	return source, nil
}

func listSources(client *apiClient, workspace string) ([]protocol.Source, error) {
	var sources []protocol.Source
	if err := client.get(sourceBasePath(workspace), &sources); err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	return sources, nil
}

func sourceBasePath(workspace string) string {
	return "/api/workspaces/" + url.PathEscape(workspace) + "/sources"
}

func sourceItemPath(workspace, sourcePath string) string {
	return sourceBasePath(workspace) + "/" + strings.TrimPrefix(sourcePath, "/")
}

func normalizeSourcePath(sourcePath string) string {
	sourcePath = strings.TrimPrefix(strings.TrimSpace(sourcePath), "/")
	if !strings.HasPrefix(sourcePath, "sources/") {
		sourcePath = "sources/" + sourcePath
	}
	return sourcePath
}

func writeSourceOutput(cmd *cobra.Command, source protocol.Source) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), source)
	}
	printTable(cmd.OutOrStdout(), []string{"PATH", "TYPE", "SIZE"}, [][]string{
		{source.Path, source.Type, fmt.Sprintf("%d", source.Size)},
	})
	return nil
}
