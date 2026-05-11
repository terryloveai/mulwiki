package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
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

var agentGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Show an agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentGet,
}

var agentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an agent",
	RunE:  runAgentCreate,
}

var agentUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Update an agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentUpdate,
}

var agentArchiveCmd = &cobra.Command{
	Use:   "archive <id-or-name>",
	Short: "Archive an agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentArchive,
}

var agentRestoreCmd = &cobra.Command{
	Use:   "restore <id-or-name>",
	Short: "Restore an archived agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentRestore,
}

var agentSkillCmd = &cobra.Command{Use: "skill", Short: "Manage agent skill associations"}
var agentSkillAddCmd = &cobra.Command{Use: "add <agent> <skill>", Short: "Add a skill to an agent", Args: cobra.ExactArgs(2), RunE: runAgentSkillAdd}
var agentSkillRemoveCmd = &cobra.Command{Use: "remove <agent> <skill>", Short: "Remove a skill from an agent", Args: cobra.ExactArgs(2), RunE: runAgentSkillRemove}
var agentTaskCmd = &cobra.Command{Use: "task", Short: "Inspect agent tasks"}
var agentTaskListCmd = &cobra.Command{Use: "list <agent>", Short: "List agent tasks", Args: cobra.ExactArgs(1), RunE: runAgentTaskList}
var agentTaskGetCmd = &cobra.Command{Use: "get <agent> <task-id>", Short: "Show an agent task", Args: cobra.ExactArgs(2), RunE: runAgentTaskGet}

func init() {
	addWorkspaceFlag(agentListCmd)
	addOutputFlag(agentListCmd, outputTable)
	addWorkspaceFlag(agentGetCmd)
	addOutputFlag(agentGetCmd, outputJSON)
	addAgentCreateFlags(agentCreateCmd)
	addAgentUpdateFlags(agentUpdateCmd)
	addWorkspaceFlag(agentArchiveCmd)
	addOutputFlag(agentArchiveCmd, outputJSON)
	addWorkspaceFlag(agentRestoreCmd)
	addOutputFlag(agentRestoreCmd, outputJSON)
	addWorkspaceFlag(agentSkillAddCmd)
	addOutputFlag(agentSkillAddCmd, outputJSON)
	addWorkspaceFlag(agentSkillRemoveCmd)
	addOutputFlag(agentSkillRemoveCmd, outputJSON)
	addWorkspaceFlag(agentTaskListCmd)
	addOutputFlag(agentTaskListCmd, outputTable)
	addWorkspaceFlag(agentTaskGetCmd)
	addOutputFlag(agentTaskGetCmd, outputJSON)
	agentTaskGetCmd.Flags().Bool("messages", false, "Include task messages")

	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentGetCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentUpdateCmd)
	agentCmd.AddCommand(agentArchiveCmd)
	agentCmd.AddCommand(agentRestoreCmd)
	agentSkillCmd.AddCommand(agentSkillAddCmd, agentSkillRemoveCmd)
	agentTaskCmd.AddCommand(agentTaskListCmd, agentTaskGetCmd)
	agentCmd.AddCommand(agentSkillCmd, agentTaskCmd)
	rootCmd.AddCommand(agentCmd)
}

func addAgentCreateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Agent name")
	addTextInputFlags(cmd, "description", "Agent description")
	addTextInputFlags(cmd, "instructions", "Agent instructions")
	cmd.Flags().String("runtime", "", "Runtime ID, name, or backend")
	cmd.Flags().String("model", "", "Model name")
	cmd.Flags().StringArray("env", nil, "Environment variable KEY=VALUE; repeatable")
	cmd.Flags().StringArray("arg", nil, "Custom runtime argument; repeatable")
	cmd.Flags().String("visibility", "private", "Agent visibility")
	cmd.Flags().Int("max-concurrent-tasks", 6, "Maximum concurrent tasks")
	addOutputFlag(cmd, outputJSON)
}

func addAgentUpdateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Agent name")
	addTextInputFlags(cmd, "description", "Agent description")
	addTextInputFlags(cmd, "instructions", "Agent instructions")
	cmd.Flags().String("runtime", "", "Runtime ID, name, or backend")
	cmd.Flags().String("model", "", "Model name")
	cmd.Flags().StringArray("env", nil, "Environment variable KEY=VALUE; repeatable")
	cmd.Flags().StringArray("arg", nil, "Custom runtime argument; repeatable")
	cmd.Flags().String("visibility", "", "Agent visibility")
	cmd.Flags().Int("max-concurrent-tasks", 0, "Maximum concurrent tasks")
	addOutputFlag(cmd, outputJSON)
}

func runAgentList(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agents, err := listAgents(client, workspace)
	if err != nil {
		return err
	}
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), agents)
	}
	rows := make([][]string, 0, len(agents))
	for _, a := range agents {
		model := a.Model
		if model == "" {
			model = "(default)"
		}
		rows = append(rows, []string{a.ID, a.Name, a.Status, a.Visibility, model})
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "NAME", "STATUS", "VISIBILITY", "MODEL"}, rows)
	return nil
}

func runAgentGet(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agent, err := resolveAgent(client, workspace, args[0])
	if err != nil {
		return err
	}
	return writeAgentOutput(cmd, agent)
}

func runAgentCreate(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("--name is required")
	}
	runtimeRef, _ := cmd.Flags().GetString("runtime")
	runtimeID := ""
	if strings.TrimSpace(runtimeRef) != "" {
		runtime, err := resolveRuntime(client, workspace, runtimeRef)
		if err != nil {
			return err
		}
		runtimeID = runtime.ID
	}
	description, hasDescription, err := resolveTextFlag(cmd, "description", cmd.InOrStdin())
	if err != nil {
		return err
	}
	instructions, hasInstructions, err := resolveTextFlag(cmd, "instructions", cmd.InOrStdin())
	if err != nil {
		return err
	}
	if !hasDescription {
		description = ""
	}
	if !hasInstructions {
		instructions = ""
	}
	model, _ := cmd.Flags().GetString("model")
	visibility, _ := cmd.Flags().GetString("visibility")
	maxConcurrent, _ := cmd.Flags().GetInt("max-concurrent-tasks")
	env, err := keyValueFlagMap(cmd, "env")
	if err != nil {
		return err
	}
	args, _ := cmd.Flags().GetStringArray("arg")
	req := protocol.AgentCreateRequest{
		Name:               strings.TrimSpace(name),
		Description:        description,
		Instructions:       instructions,
		RuntimeID:          runtimeID,
		RuntimeConfig:      json.RawMessage(`{}`),
		CustomEnv:          env,
		CustomArgs:         args,
		McpConfig:          json.RawMessage(`{}`),
		Visibility:         visibility,
		MaxConcurrentTasks: maxConcurrent,
		Model:              model,
	}
	var resp struct {
		Agent protocol.Agent `json:"agent"`
	}
	if _, err := client.post(agentBasePath(workspace), req, &resp); err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return writeAgentOutput(cmd, resp.Agent)
}

func runAgentUpdate(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agent, err := resolveAgent(client, workspace, args[0])
	if err != nil {
		return err
	}
	req := protocol.AgentUpdateRequest{}
	if flagChanged(cmd, "name") {
		name, _ := cmd.Flags().GetString("name")
		req.Name = &name
	}
	if description, ok, err := resolveTextFlag(cmd, "description", cmd.InOrStdin()); err != nil {
		return err
	} else if ok {
		req.Description = &description
	}
	if instructions, ok, err := resolveTextFlag(cmd, "instructions", cmd.InOrStdin()); err != nil {
		return err
	} else if ok {
		req.Instructions = &instructions
	}
	if flagChanged(cmd, "runtime") {
		runtimeRef, _ := cmd.Flags().GetString("runtime")
		runtimeID := ""
		if strings.TrimSpace(runtimeRef) != "" {
			runtime, err := resolveRuntime(client, workspace, runtimeRef)
			if err != nil {
				return err
			}
			runtimeID = runtime.ID
		}
		req.RuntimeID = &runtimeID
	}
	if flagChanged(cmd, "model") {
		model, _ := cmd.Flags().GetString("model")
		req.Model = &model
	}
	if flagChanged(cmd, "visibility") {
		visibility, _ := cmd.Flags().GetString("visibility")
		req.Visibility = &visibility
	}
	if flagChanged(cmd, "max-concurrent-tasks") {
		maxConcurrent, _ := cmd.Flags().GetInt("max-concurrent-tasks")
		req.MaxConcurrentTasks = &maxConcurrent
	}
	if flagChanged(cmd, "env") {
		env, err := keyValueFlagMap(cmd, "env")
		if err != nil {
			return err
		}
		req.CustomEnv = &env
	}
	if flagChanged(cmd, "arg") {
		customArgs, _ := cmd.Flags().GetStringArray("arg")
		req.CustomArgs = &customArgs
	}
	var resp struct {
		Agent protocol.Agent `json:"agent"`
	}
	if _, err := client.patch(agentItemPath(workspace, agent.ID), req, &resp); err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	return writeAgentOutput(cmd, resp.Agent)
}

func runAgentArchive(cmd *cobra.Command, args []string) error {
	return runAgentAction(cmd, args[0], "archive")
}

func runAgentRestore(cmd *cobra.Command, args []string) error {
	return runAgentAction(cmd, args[0], "restore")
}

func runAgentAction(cmd *cobra.Command, ref, action string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agent, err := resolveAgent(client, workspace, ref)
	if err != nil {
		return err
	}
	var resp map[string]string
	if _, err := client.post(agentItemPath(workspace, agent.ID)+"/"+action, nil, &resp); err != nil {
		return fmt.Errorf("%s agent: %w", action, err)
	}
	return printJSON(cmd.OutOrStdout(), resp)
}

func runAgentSkillAdd(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agent, err := resolveAgent(client, workspace, args[0])
	if err != nil {
		return err
	}
	skill, err := resolveSkill(client, workspace, args[1])
	if err != nil {
		return err
	}
	var resp map[string]string
	if _, err := client.post(agentItemPath(workspace, agent.ID)+"/skills", protocol.AddAgentSkillRequest{SkillID: skill.ID}, &resp); err != nil {
		return fmt.Errorf("add agent skill: %w", err)
	}
	return printJSON(cmd.OutOrStdout(), resp)
}

func runAgentSkillRemove(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agent, err := resolveAgent(client, workspace, args[0])
	if err != nil {
		return err
	}
	skill, err := resolveSkill(client, workspace, args[1])
	if err != nil {
		return err
	}
	if _, err := client.delete(agentItemPath(workspace, agent.ID) + "/skills/" + url.PathEscape(skill.ID)); err != nil {
		return fmt.Errorf("remove agent skill: %w", err)
	}
	return printJSON(cmd.OutOrStdout(), map[string]string{"status": "removed"})
}

func runAgentTaskList(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agent, err := resolveAgent(client, workspace, args[0])
	if err != nil {
		return err
	}
	var resp struct {
		Tasks []protocol.AgentTask `json:"tasks"`
	}
	if err := client.get(agentItemPath(workspace, agent.ID)+"/tasks", &resp); err != nil {
		return fmt.Errorf("list agent tasks: %w", err)
	}
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), resp.Tasks)
	}
	rows := make([][]string, 0, len(resp.Tasks))
	for _, task := range resp.Tasks {
		rows = append(rows, []string{task.ID, task.JobID, task.Status, task.SchemaID, task.SourcePath})
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "JOB", "STATUS", "SCHEMA", "SOURCE"}, rows)
	return nil
}

func runAgentTaskGet(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	agent, err := resolveAgent(client, workspace, args[0])
	if err != nil {
		return err
	}
	var resp struct {
		Task protocol.AgentTask `json:"task"`
	}
	if err := client.get(agentItemPath(workspace, agent.ID)+"/tasks/"+url.PathEscape(args[1]), &resp); err != nil {
		return fmt.Errorf("get agent task: %w", err)
	}
	return printJSON(cmd.OutOrStdout(), resp.Task)
}

func listAgents(client *apiClient, workspace string) ([]protocol.Agent, error) {
	var resp struct {
		Agents []protocol.Agent `json:"agents"`
	}
	if err := client.get(agentBasePath(workspace), &resp); err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	return resp.Agents, nil
}

func resolveAgent(client *apiClient, workspace, ref string) (protocol.Agent, error) {
	agents, err := listAgents(client, workspace)
	if err != nil {
		return protocol.Agent{}, err
	}
	matches := make([]protocol.Agent, 0, 1)
	for _, agent := range agents {
		if agent.ID == ref || agent.Name == ref {
			matches = append(matches, agent)
		}
	}
	if len(matches) == 0 {
		return protocol.Agent{}, fmt.Errorf("agent %q not found", ref)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, agent := range matches {
			ids = append(ids, agent.ID)
		}
		return protocol.Agent{}, fmt.Errorf("agent %q is ambiguous; matching ids: %s", ref, strings.Join(ids, ", "))
	}
	var resp struct {
		Agent protocol.Agent `json:"agent"`
	}
	if err := client.get(agentItemPath(workspace, matches[0].ID), &resp); err != nil {
		return protocol.Agent{}, fmt.Errorf("get agent: %w", err)
	}
	return resp.Agent, nil
}

func resolveRuntime(client *apiClient, workspace, ref string) (protocol.AgentRuntime, error) {
	var resp struct {
		Runtimes []protocol.AgentRuntime `json:"runtimes"`
	}
	if err := client.get(runtimeBasePath(workspace), &resp); err != nil {
		return protocol.AgentRuntime{}, fmt.Errorf("list runtimes: %w", err)
	}
	matches := make([]protocol.AgentRuntime, 0, 1)
	for _, runtime := range resp.Runtimes {
		if runtime.ID == ref || runtime.Name == ref || runtime.Backend == ref {
			matches = append(matches, runtime)
		}
	}
	if len(matches) == 0 {
		return protocol.AgentRuntime{}, fmt.Errorf("runtime %q not found", ref)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, runtime := range matches {
			ids = append(ids, runtime.ID)
		}
		return protocol.AgentRuntime{}, fmt.Errorf("runtime %q is ambiguous; matching ids: %s", ref, strings.Join(ids, ", "))
	}
	return matches[0], nil
}

func agentBasePath(workspace string) string {
	return "/api/workspaces/" + url.PathEscape(workspace) + "/agents"
}

func agentItemPath(workspace, id string) string {
	return agentBasePath(workspace) + "/" + url.PathEscape(id)
}

func runtimeBasePath(workspace string) string {
	return agentBasePath(workspace) + "/runtimes"
}

func writeAgentOutput(cmd *cobra.Command, agent protocol.Agent) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), agent)
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "NAME", "STATUS", "VISIBILITY", "MODEL"}, [][]string{
		{agent.ID, agent.Name, agent.Status, agent.Visibility, agent.Model},
	})
	return nil
}

func keyValueFlagMap(cmd *cobra.Command, name string) (map[string]string, error) {
	values, err := cmd.Flags().GetStringArray(name)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("--%s values must use KEY=VALUE", name)
		}
		result[key] = val
	}
	return result, nil
}
