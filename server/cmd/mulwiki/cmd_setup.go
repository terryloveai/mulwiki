package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure the CLI, authenticate, and start the daemon",
	RunE:  runSetupSelfHost,
}

var setupSelfHostCmd = &cobra.Command{
	Use:   "self-host",
	Short: "Configure the CLI for a self-hosted Mulwiki server",
	RunE:  runSetupSelfHost,
}

func init() {
	addSetupFlags(setupCmd)
	addSetupFlags(setupSelfHostCmd)
	setupCmd.AddCommand(setupSelfHostCmd)
	rootCmd.AddCommand(setupCmd)
}

func addSetupFlags(cmd *cobra.Command) {
	cmd.Flags().String("server-url", "http://localhost:8080", "Mulwiki server URL")
	cmd.Flags().String("workspace", "", "Default workspace slug")
	cmd.Flags().String("email", "", "Login email (env: MULWIKI_EMAIL)")
	cmd.Flags().String("password", "", "Login password (env: MULWIKI_PASSWORD)")
	cmd.Flags().Bool("no-start", false, "Configure and authenticate without starting the daemon")
}

func runSetupSelfHost(cmd *cobra.Command, _ []string) error {
	serverURL, _ := cmd.Flags().GetString("server-url")
	workspace, _ := cmd.Flags().GetString("workspace")
	email := flagOrEnvConfig(cmd, "email", "MULWIKI_EMAIL", "", "")
	password := flagOrEnvConfig(cmd, "password", "MULWIKI_PASSWORD", "", "")
	noStart, _ := cmd.Flags().GetBool("no-start")

	var err error
	if email == "" {
		email, err = readLine(os.Stdin, "Email: ")
		if err != nil {
			return err
		}
	}
	if password == "" {
		password, err = readLine(os.Stdin, "Password: ")
		if err != nil {
			return err
		}
	}

	if err := setupSelfHost(serverURL, workspace, email, password, noStart); err != nil {
		return err
	}
	if noStart {
		fmt.Fprintln(os.Stderr, "Setup complete.")
		return nil
	}
	fmt.Fprintln(os.Stderr, "Setup complete. Daemon start requested.")
	return nil
}

func setupSelfHost(serverURL, workspace, email, password string, noStart bool) error {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	if _, err := loginWithCredentials(serverURL, email, password, strings.TrimSpace(workspace)); err != nil {
		return err
	}
	if noStart {
		return nil
	}

	cmd := daemonStartCmd
	cmd.Flags().Set("server-url", serverURL)
	if workspace != "" {
		cmd.Flags().Set("workspace", workspace)
	}
	return runDaemonBackground(cmd)
}
