package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var profileFlag string

var rootCmd = &cobra.Command{
	Use:           "mulwiki",
	Short:         "Mulwiki CLI — local agent runtime and management tool",
	Long:          "Work with Mulwiki from the command line.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)\ngo: %s, os/arch: %s/%s",
		version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	rootCmd.SetVersionTemplate("mulwiki {{.Version}}\n")
	rootCmd.PersistentFlags().StringVar(&profileFlag, "profile", "", "CLI profile name")

	// Subcommands
	rootCmd.AddCommand(daemonCmd)
}

func resolveProfile(cmd *cobra.Command) string {
	if cmd != nil {
		if v, err := cmd.Root().PersistentFlags().GetString("profile"); err == nil && strings.TrimSpace(v) != "" {
			return normalizeProfile(v)
		}
	}
	return normalizeProfile(profileFlag)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
