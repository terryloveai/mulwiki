package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var wikiCmd = &cobra.Command{Use: "wiki", Short: "Manage wiki pages"}
var wikiListCmd = &cobra.Command{Use: "list", Short: "List wiki pages", RunE: runWikiList}
var wikiGetCmd = &cobra.Command{Use: "get <path>", Short: "Show a wiki page", Args: cobra.ExactArgs(1), RunE: runWikiGet}
var wikiCreateCmd = &cobra.Command{Use: "create <path>", Short: "Create a wiki page", Args: cobra.ExactArgs(1), RunE: runWikiCreate}
var wikiDeleteCmd = &cobra.Command{Use: "delete <path>", Short: "Delete a wiki page", Args: cobra.ExactArgs(1), RunE: runWikiDelete}
var wikiSearchCmd = &cobra.Command{Use: "search <query>", Short: "Search wiki pages", Args: cobra.ExactArgs(1), RunE: runWikiSearch}
var wikiBacklinksCmd = &cobra.Command{Use: "backlinks <path>", Short: "List backlinks", Args: cobra.ExactArgs(1), RunE: runWikiBacklinks}
var wikiResolveCmd = &cobra.Command{Use: "resolve-links", Short: "Resolve wiki links", RunE: runWikiResolve}
var wikiExportCmd = &cobra.Command{Use: "export", Short: "Export wiki pages as a zip", RunE: runWikiExport}

func init() {
	addWorkspaceFlag(wikiListCmd)
	wikiListCmd.Flags().String("type", "", "Filter by page type")
	addOutputFlag(wikiListCmd, outputTable)
	addWorkspaceFlag(wikiGetCmd)
	wikiGetCmd.Flags().Bool("raw", false, "Print only Markdown content")
	addOutputFlag(wikiGetCmd, outputJSON)
	addWikiCreateFlags(wikiCreateCmd)
	addWorkspaceFlag(wikiDeleteCmd)
	wikiDeleteCmd.Flags().Bool("yes", false, "Confirm deletion")
	addWorkspaceFlag(wikiSearchCmd)
	addOutputFlag(wikiSearchCmd, outputTable)
	addWorkspaceFlag(wikiBacklinksCmd)
	addOutputFlag(wikiBacklinksCmd, outputTable)
	addWorkspaceFlag(wikiResolveCmd)
	wikiResolveCmd.Flags().StringArray("path", nil, "Wiki path to resolve; repeatable")
	addOutputFlag(wikiResolveCmd, outputJSON)
	addWorkspaceFlag(wikiExportCmd)
	wikiExportCmd.Flags().String("output-file", "wiki.zip", "Output zip file")
	wikiCmd.AddCommand(wikiListCmd, wikiGetCmd, wikiCreateCmd, wikiDeleteCmd, wikiSearchCmd, wikiBacklinksCmd, wikiResolveCmd, wikiExportCmd)
	rootCmd.AddCommand(wikiCmd)
}

func addWikiCreateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("title", "", "Page title")
	cmd.Flags().String("type", "page", "Page type")
	cmd.Flags().String("layer", "", "Page layer")
	addTextInputFlags(cmd, "content", "Page Markdown content")
	addOutputFlag(cmd, outputJSON)
}

func runWikiList(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	pages, err := listWikiPages(client, workspace, "")
	if err != nil {
		return err
	}
	pageType, _ := cmd.Flags().GetString("type")
	if pageType != "" {
		filtered := pages[:0]
		for _, page := range pages {
			if page.Type == pageType {
				filtered = append(filtered, page)
			}
		}
		pages = filtered
	}
	return writeWikiListOutput(cmd, pages)
}

func runWikiGet(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	page, err := getWikiPage(client, workspace, args[0])
	if err != nil {
		return err
	}
	raw, _ := cmd.Flags().GetBool("raw")
	if raw {
		fmt.Fprintln(cmd.OutOrStdout(), page.Content)
		return nil
	}
	return writeWikiOutput(cmd, page)
}

func runWikiCreate(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	title, _ := cmd.Flags().GetString("title")
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("--title is required")
	}
	content, ok, err := resolveTextFlag(cmd, "content", cmd.InOrStdin())
	if err != nil {
		return err
	}
	if !ok {
		content = ""
	}
	pageType, _ := cmd.Flags().GetString("type")
	layer, _ := cmd.Flags().GetString("layer")
	req := protocol.CreateWikiPageRequest{Path: normalizeWikiPath(args[0]), Title: title, Content: content, Type: pageType, Layer: layer}
	var page protocol.WikiPage
	if _, err := client.post(wikiBasePath(workspace), req, &page); err != nil {
		return fmt.Errorf("create wiki page: %w", err)
	}
	return writeWikiOutput(cmd, page)
}

func runWikiDelete(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("refusing to delete wiki page without --yes")
	}
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	pagePath := normalizeWikiPath(args[0])
	if _, err := client.delete(wikiItemPath(workspace, pagePath)); err != nil {
		return fmt.Errorf("delete wiki page: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted wiki page %s\n", pagePath)
	return nil
}

func runWikiSearch(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	pages, err := listWikiPages(client, workspace, args[0])
	if err != nil {
		return err
	}
	return writeWikiListOutput(cmd, pages)
}

func runWikiBacklinks(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	var backlinks []protocol.WikiBacklink
	path := "/api/workspaces/" + url.PathEscape(workspace) + "/wiki/backlinks?path=" + url.QueryEscape(normalizeWikiPath(args[0]))
	if err := client.get(path, &backlinks); err != nil {
		return fmt.Errorf("wiki backlinks: %w", err)
	}
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), backlinks)
	}
	rows := make([][]string, 0, len(backlinks))
	for _, backlink := range backlinks {
		rows = append(rows, []string{backlink.Path, backlink.Title, backlink.Snippet})
	}
	printTable(cmd.OutOrStdout(), []string{"PATH", "TITLE", "SNIPPET"}, rows)
	return nil
}

func runWikiResolve(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	paths, _ := cmd.Flags().GetStringArray("path")
	if len(paths) == 0 {
		return fmt.Errorf("--path is required")
	}
	var resp any
	if _, err := client.post("/api/workspaces/"+url.PathEscape(workspace)+"/wiki/resolve-links", map[string]any{"paths": paths}, &resp); err != nil {
		return fmt.Errorf("resolve wiki links: %w", err)
	}
	return printJSON(cmd.OutOrStdout(), resp)
}

func runWikiExport(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	outputFile, _ := cmd.Flags().GetString("output-file")
	if strings.TrimSpace(outputFile) == "" {
		return fmt.Errorf("--output-file is required")
	}
	content, err := client.getRaw(wikiBasePath(workspace) + "/export")
	if err != nil {
		return fmt.Errorf("export wiki: %w", err)
	}
	if err := os.WriteFile(outputFile, content, 0o644); err != nil {
		return fmt.Errorf("write export: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Exported wiki to %s\n", outputFile)
	return nil
}

func listWikiPages(client *apiClient, workspace, query string) ([]protocol.WikiPage, error) {
	path := wikiBasePath(workspace)
	if query != "" {
		path += "/search?q=" + url.QueryEscape(query)
	}
	var pages []protocol.WikiPage
	if err := client.get(path, &pages); err != nil {
		return nil, fmt.Errorf("list wiki pages: %w", err)
	}
	return pages, nil
}

func getWikiPage(client *apiClient, workspace, pagePath string) (protocol.WikiPage, error) {
	var page protocol.WikiPage
	if err := client.get(wikiItemPath(workspace, pagePath), &page); err != nil {
		return protocol.WikiPage{}, fmt.Errorf("get wiki page: %w", err)
	}
	return page, nil
}

func wikiBasePath(workspace string) string {
	return "/api/workspaces/" + url.PathEscape(workspace) + "/wiki"
}

func wikiItemPath(workspace, pagePath string) string {
	return wikiBasePath(workspace) + normalizeWikiPath(pagePath)
}

func normalizeWikiPath(pagePath string) string {
	pagePath = strings.TrimSpace(pagePath)
	if !strings.HasPrefix(pagePath, "/") {
		pagePath = "/" + pagePath
	}
	return strings.TrimSuffix(pagePath, "/")
}

func writeWikiOutput(cmd *cobra.Command, page protocol.WikiPage) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), page)
	}
	printTable(cmd.OutOrStdout(), []string{"PATH", "TITLE", "TYPE", "LAYER"}, [][]string{{page.Path, page.Title, page.Type, page.Layer}})
	return nil
}

func writeWikiListOutput(cmd *cobra.Command, pages []protocol.WikiPage) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), pages)
	}
	rows := make([][]string, 0, len(pages))
	for _, page := range pages {
		rows = append(rows, []string{page.Path, page.Title, page.Type, page.Layer})
	}
	printTable(cmd.OutOrStdout(), []string{"PATH", "TITLE", "TYPE", "LAYER"}, rows)
	return nil
}
