package main

import (
	"bufio"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage ingest jobs",
}

var jobListCmd = &cobra.Command{
	Use:   "list",
	Short: "List jobs",
	RunE:  runJobList,
}

var jobCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an ingest job",
	RunE:  runJobCreate,
}

var jobGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Show a job",
	Args:  cobra.ExactArgs(1),
	RunE:  runJobGet,
}

var jobLogsCmd = &cobra.Command{
	Use:   "logs <id>",
	Short: "Stream job logs",
	Args:  cobra.ExactArgs(1),
	RunE:  runJobLogs,
}
var jobCancelCmd = &cobra.Command{Use: "cancel <id>", Short: "Cancel a job", Args: cobra.ExactArgs(1), RunE: runJobCancel}
var jobRetryCmd = &cobra.Command{Use: "retry <id>", Short: "Retry a job", Args: cobra.ExactArgs(1), RunE: runJobRetry}

func init() {
	addWorkspaceFlag(jobListCmd)
	jobListCmd.Flags().String("status", "", "Filter by status")
	addOutputFlag(jobListCmd, outputTable)
	addJobCreateFlags(jobCreateCmd)
	addWorkspaceFlag(jobGetCmd)
	addOutputFlag(jobGetCmd, outputJSON)
	addWorkspaceFlag(jobLogsCmd)
	addWorkspaceFlag(jobCancelCmd)
	addOutputFlag(jobCancelCmd, outputJSON)
	addWorkspaceFlag(jobRetryCmd)
	addOutputFlag(jobRetryCmd, outputJSON)

	jobCmd.AddCommand(jobListCmd)
	jobCmd.AddCommand(jobCreateCmd)
	jobCmd.AddCommand(jobGetCmd)
	jobCmd.AddCommand(jobLogsCmd)
	jobCmd.AddCommand(jobCancelCmd)
	jobCmd.AddCommand(jobRetryCmd)
	rootCmd.AddCommand(jobCmd)
}

func addJobCreateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("agent", "", "Agent ID")
	cmd.Flags().String("schema", "", "Schema ID")
	cmd.Flags().StringArray("source", nil, "Source path; repeatable")
	cmd.Flags().Bool("wait", false, "Wait until the job reaches a terminal status")
	addOutputFlag(cmd, outputJSON)
}

func runJobList(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	jobs, err := listJobs(client, workspace)
	if err != nil {
		return err
	}
	status, _ := cmd.Flags().GetString("status")
	if strings.TrimSpace(status) != "" {
		filtered := jobs[:0]
		for _, job := range jobs {
			if job.Status == status {
				filtered = append(filtered, job)
			}
		}
		jobs = filtered
	}
	return writeJobListOutput(cmd, jobs)
}

func runJobCreate(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agentID, _ := cmd.Flags().GetString("agent")
	schemaID, _ := cmd.Flags().GetString("schema")
	sources, _ := cmd.Flags().GetStringArray("source")
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("--agent is required")
	}
	if strings.TrimSpace(schemaID) == "" {
		return fmt.Errorf("--schema is required")
	}
	if len(sources) == 0 {
		return fmt.Errorf("--source is required")
	}
	for i := range sources {
		sources[i] = normalizeSourcePath(sources[i])
	}
	req := protocol.CreateJobRequest{
		AgentID:     strings.TrimSpace(agentID),
		SchemaID:    strings.TrimSpace(schemaID),
		SourcePath:  sources[0],
		SourcePaths: sources,
	}
	var job protocol.Job
	if _, err := client.post(jobBasePath(workspace), req, &job); err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	wait, _ := cmd.Flags().GetBool("wait")
	if wait {
		job, err = waitForJob(client, workspace, job.ID)
		if err != nil {
			return err
		}
	}
	return writeJobOutput(cmd, job)
}

func runJobGet(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	job, err := getJob(client, workspace, args[0])
	if err != nil {
		return err
	}
	return writeJobOutput(cmd, job)
}

func runJobLogs(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	content, err := client.getRaw(jobItemPath(workspace, args[0]) + "/logs")
	if err != nil {
		return fmt.Errorf("job logs: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			fmt.Fprintln(cmd.OutOrStdout(), strings.TrimPrefix(line, "data: "))
		}
	}
	return scanner.Err()
}

func runJobCancel(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	var resp map[string]string
	if _, err := client.post(jobItemPath(workspace, args[0])+"/cancel", nil, &resp); err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	return printJSON(cmd.OutOrStdout(), resp)
}

func runJobRetry(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	var job protocol.Job
	if _, err := client.post(jobItemPath(workspace, args[0])+"/retry", nil, &job); err != nil {
		return fmt.Errorf("retry job: %w", err)
	}
	return writeJobOutput(cmd, job)
}

func listJobs(client *apiClient, workspace string) ([]protocol.Job, error) {
	var jobs []protocol.Job
	if err := client.get(jobBasePath(workspace), &jobs); err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return jobs, nil
}

func getJob(client *apiClient, workspace, id string) (protocol.Job, error) {
	var job protocol.Job
	if err := client.get(jobItemPath(workspace, id), &job); err != nil {
		return protocol.Job{}, fmt.Errorf("get job: %w", err)
	}
	return job, nil
}

func waitForJob(client *apiClient, workspace, id string) (protocol.Job, error) {
	for {
		job, err := getJob(client, workspace, id)
		if err != nil {
			return protocol.Job{}, err
		}
		if job.Status == "completed" {
			return job, nil
		}
		if job.Status == "failed" || job.Status == "cancelled" {
			return job, fmt.Errorf("job %s ended with status %s: %s", job.ID, job.Status, job.Error)
		}
		time.Sleep(time.Second)
	}
}

func jobBasePath(workspace string) string {
	return "/api/workspaces/" + url.PathEscape(workspace) + "/jobs"
}

func jobItemPath(workspace, id string) string {
	return jobBasePath(workspace) + "/" + url.PathEscape(id)
}

func writeJobOutput(cmd *cobra.Command, job protocol.Job) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), job)
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "STATUS", "AGENT", "SCHEMA", "PROGRESS"}, [][]string{
		{job.ID, job.Status, job.AgentID, job.SchemaID, fmt.Sprintf("%d", job.Progress)},
	})
	return nil
}

func writeJobListOutput(cmd *cobra.Command, jobs []protocol.Job) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), jobs)
	}
	rows := make([][]string, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, []string{job.ID, job.Status, job.AgentID, job.SchemaID, fmt.Sprintf("%d", job.Progress)})
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "STATUS", "AGENT", "SCHEMA", "PROGRESS"}, rows)
	return nil
}
