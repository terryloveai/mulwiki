package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage agent skills",
}

var skillListCmd = &cobra.Command{Use: "list", Short: "List skills", RunE: runSkillList}
var skillGetCmd = &cobra.Command{Use: "get <id-or-name>", Short: "Show a skill", Args: cobra.ExactArgs(1), RunE: runSkillGet}
var skillCreateCmd = &cobra.Command{Use: "create", Short: "Create a skill", RunE: runSkillCreate}
var skillUpdateCmd = &cobra.Command{Use: "update <id-or-name>", Short: "Update a skill", Args: cobra.ExactArgs(1), RunE: runSkillUpdate}
var skillDeleteCmd = &cobra.Command{Use: "delete <id-or-name>", Short: "Delete a skill", Args: cobra.ExactArgs(1), RunE: runSkillDelete}

func init() {
	addWorkspaceFlag(skillListCmd)
	addOutputFlag(skillListCmd, outputTable)
	addWorkspaceFlag(skillGetCmd)
	addOutputFlag(skillGetCmd, outputJSON)
	addSkillCreateFlags(skillCreateCmd)
	addSkillUpdateFlags(skillUpdateCmd)
	addWorkspaceFlag(skillDeleteCmd)
	skillDeleteCmd.Flags().Bool("yes", false, "Confirm deletion")
	skillCmd.AddCommand(skillListCmd, skillGetCmd, skillCreateCmd, skillUpdateCmd, skillDeleteCmd)
	rootCmd.AddCommand(skillCmd)
}

func addSkillCreateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Skill name")
	addTextInputFlags(cmd, "description", "Skill description")
	addOutputFlag(cmd, outputJSON)
}

func addSkillUpdateFlags(cmd *cobra.Command) {
	addWorkspaceFlag(cmd)
	cmd.Flags().String("name", "", "Skill name")
	addTextInputFlags(cmd, "description", "Skill description")
	addOutputFlag(cmd, outputJSON)
}

func runSkillList(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	skills, err := listSkills(client, workspace)
	if err != nil {
		return err
	}
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), skills)
	}
	rows := make([][]string, 0, len(skills))
	for _, skill := range skills {
		rows = append(rows, []string{skill.ID, skill.Name, skill.Description})
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "NAME", "DESCRIPTION"}, rows)
	return nil
}

func runSkillGet(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	skill, err := resolveSkill(client, workspace, args[0])
	if err != nil {
		return err
	}
	return writeSkillOutput(cmd, skill)
}

func runSkillCreate(cmd *cobra.Command, _ []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	description, ok, err := resolveTextFlag(cmd, "description", cmd.InOrStdin())
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("--name is required")
	}
	if !ok {
		description = ""
	}
	req := protocol.CreateSkillRequest{Name: strings.TrimSpace(name), Description: description}
	var resp struct {
		Skill protocol.AgentSkill `json:"skill"`
	}
	if _, err := client.post(skillBasePath(workspace), req, &resp); err != nil {
		return fmt.Errorf("create skill: %w", err)
	}
	return writeSkillOutput(cmd, resp.Skill)
}

func runSkillUpdate(cmd *cobra.Command, args []string) error {
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	skill, err := resolveSkill(client, workspace, args[0])
	if err != nil {
		return err
	}
	req := protocol.UpdateSkillRequest{}
	if flagChanged(cmd, "name") {
		name, _ := cmd.Flags().GetString("name")
		req.Name = &name
	}
	if description, ok, err := resolveTextFlag(cmd, "description", cmd.InOrStdin()); err != nil {
		return err
	} else if ok {
		req.Description = &description
	}
	var resp struct {
		Skill protocol.AgentSkill `json:"skill"`
	}
	if _, err := client.patch(skillItemPath(workspace, skill.ID), req, &resp); err != nil {
		return fmt.Errorf("update skill: %w", err)
	}
	return writeSkillOutput(cmd, resp.Skill)
}

func runSkillDelete(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("refusing to delete skill without --yes")
	}
	_, client, workspace, err := workspaceClient(cmd)
	if err != nil {
		return err
	}
	skill, err := resolveSkill(client, workspace, args[0])
	if err != nil {
		return err
	}
	if _, err := client.delete(skillItemPath(workspace, skill.ID)); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted skill %s\n", skill.ID)
	return nil
}

func listSkills(client *apiClient, workspace string) ([]protocol.AgentSkill, error) {
	var resp struct {
		Skills []protocol.AgentSkill `json:"skills"`
	}
	if err := client.get(skillBasePath(workspace), &resp); err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	return resp.Skills, nil
}

func resolveSkill(client *apiClient, workspace, ref string) (protocol.AgentSkill, error) {
	skills, err := listSkills(client, workspace)
	if err != nil {
		return protocol.AgentSkill{}, err
	}
	matches := make([]protocol.AgentSkill, 0, 1)
	for _, skill := range skills {
		if skill.ID == ref || skill.Name == ref {
			matches = append(matches, skill)
		}
	}
	if len(matches) == 0 {
		return protocol.AgentSkill{}, fmt.Errorf("skill %q not found", ref)
	}
	if len(matches) > 1 {
		return protocol.AgentSkill{}, fmt.Errorf("skill %q is ambiguous", ref)
	}
	return matches[0], nil
}

func skillBasePath(workspace string) string {
	return agentBasePath(workspace) + "/skills"
}

func skillItemPath(workspace, id string) string {
	return skillBasePath(workspace) + "/" + url.PathEscape(id)
}

func writeSkillOutput(cmd *cobra.Command, skill protocol.AgentSkill) error {
	format, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return printJSON(cmd.OutOrStdout(), skill)
	}
	printTable(cmd.OutOrStdout(), []string{"ID", "NAME", "DESCRIPTION"}, [][]string{
		{skill.ID, skill.Name, skill.Description},
	})
	return nil
}
