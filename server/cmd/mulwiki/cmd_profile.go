package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage CLI profiles",
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List CLI profiles",
	RunE:  runProfileList,
}

var profileUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the default CLI profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileUse,
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a CLI profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileDelete,
}

func init() {
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileUseCmd)
	profileCmd.AddCommand(profileDeleteCmd)
	rootCmd.AddCommand(profileCmd)
}

func runProfileList(_ *cobra.Command, _ []string) error {
	profiles, err := listProfiles()
	if err != nil {
		return err
	}
	active, _ := loadActiveProfile()
	for _, profile := range profiles {
		marker := " "
		if normalizeProfile(profile) == normalizeProfile(active) || (profile == "default" && active == "") {
			marker = "*"
		}
		fmt.Fprintf(os.Stdout, "%s %s\n", marker, profile)
	}
	return nil
}

func runProfileUse(_ *cobra.Command, args []string) error {
	profile := normalizeProfile(args[0])
	if profile == "" {
		return fmt.Errorf("profile name is required")
	}
	if err := saveActiveProfile(profile); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Active profile: %s\n", profileLabel(profile))
	return nil
}

func runProfileDelete(_ *cobra.Command, args []string) error {
	profile := normalizeProfile(args[0])
	if profile == "" || profile == "default" {
		return fmt.Errorf("default profile cannot be deleted")
	}
	dir, err := mulwikiProfileDir(profile)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	active, _ := loadActiveProfile()
	if active == profile {
		if err := saveActiveProfile(""); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "Deleted profile: %s\n", profile)
	return nil
}

func listProfiles() ([]string, error) {
	base, err := mulwikiBaseDir()
	if err != nil {
		return nil, err
	}
	profiles := []string{"default"}
	profilesDir := filepath.Join(base, "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return profiles, nil
		}
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := normalizeProfile(entry.Name())
		if name != "" && name != "default" && !strings.HasPrefix(name, ".") {
			profiles = append(profiles, name)
		}
	}
	sort.Strings(profiles[1:])
	return profiles, nil
}
