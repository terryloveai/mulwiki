package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate the Mulwiki CLI",
	RunE:  runLogin,
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage CLI authentication",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show CLI authentication status",
	RunE:  runAuthStatus,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored CLI authentication",
	RunE:  runAuthLogout,
}

func init() {
	loginCmd.Flags().String("server-url", "", "Mulwiki server URL (env: MULWIKI_SERVER_URL)")
	loginCmd.Flags().String("workspace", "", "Default workspace slug (env: MULWIKI_WORKSPACE)")
	loginCmd.Flags().String("email", "", "Login email (env: MULWIKI_EMAIL)")
	loginCmd.Flags().String("password", "", "Login password (env: MULWIKI_PASSWORD)")

	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(authCmd)
}

func runLogin(cmd *cobra.Command, _ []string) error {
	cfg, _ := loadCLIConfig()
	serverURL := flagOrEnvConfig(cmd, "server-url", "MULWIKI_SERVER_URL", cfg.ServerURL, "http://localhost:8080")
	workspace := flagOrEnvConfig(cmd, "workspace", "MULWIKI_WORKSPACE", cfg.WorkspaceSlug, "")
	email := flagOrEnvConfig(cmd, "email", "MULWIKI_EMAIL", "", "")
	password := flagOrEnvConfig(cmd, "password", "MULWIKI_PASSWORD", "", "")

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

	user, err := loginWithCredentials(serverURL, email, password, workspace)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Authenticated as %s\n", user.Email)

	cfg, _ = loadCLIConfig()
	if cfg.WorkspaceSlug != "" {
		fmt.Fprintf(os.Stderr, "Default workspace: %s\n", cfg.WorkspaceSlug)
	}
	return nil
}

func loginWithCredentials(serverURL, email, password, workspace string) (protocol.User, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	client := newAPIClient(serverURL)
	var user protocol.User
	resp, err := client.post("/api/auth/login", protocol.AuthRequest{
		Email:    email,
		Password: password,
	}, &user)
	if err != nil {
		return protocol.User{}, fmt.Errorf("login: %w", err)
	}

	sessionID := sessionIDFromResponse(resp)
	if sessionID == "" {
		return protocol.User{}, errors.New("login response did not include a session cookie")
	}

	cfg, err := loadCLIConfig()
	if err != nil {
		return protocol.User{}, err
	}
	cfg.ServerURL = serverURL
	cfg.SessionID = sessionID
	cfg.DaemonTokens = nil
	if workspace != "" {
		cfg.WorkspaceSlug = workspace
	}

	client.setSessionID(sessionID)
	if cfg.WorkspaceSlug == "" {
		if slug, err := firstWorkspaceSlug(client); err == nil && slug != "" {
			cfg.WorkspaceSlug = slug
		}
	}

	if err := saveCLIConfig(cfg); err != nil {
		return protocol.User{}, err
	}
	return user, nil
}

func sessionIDFromResponse(resp *http.Response) string {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie.Value
		}
	}
	return ""
}

func firstWorkspaceSlug(client *apiClient) (string, error) {
	var workspaces []protocol.Workspace
	if err := client.get("/api/workspaces", &workspaces); err != nil {
		return "", err
	}
	if len(workspaces) == 0 {
		return "", nil
	}
	return workspaces[0].Slug, nil
}

func runAuthStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := loadCLIConfig()
	if err != nil {
		return err
	}
	if cfg.SessionID == "" {
		fmt.Fprintln(os.Stdout, "Not authenticated. Run 'mulwiki login' first.")
		return nil
	}

	serverURL := cfg.ServerURL
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	client := newAPIClient(serverURL)
	client.setSessionID(cfg.SessionID)

	var user protocol.User
	if err := client.get("/api/auth/me", &user); err != nil {
		return fmt.Errorf("stored session is invalid or expired: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Server:    %s\n", serverURL)
	fmt.Fprintf(os.Stdout, "User:      %s\n", user.Email)
	if cfg.WorkspaceSlug != "" {
		fmt.Fprintf(os.Stdout, "Workspace: %s\n", cfg.WorkspaceSlug)
	}
	return nil
}

func runAuthLogout(_ *cobra.Command, _ []string) error {
	cfg, _ := loadCLIConfig()
	if cfg.SessionID != "" && cfg.ServerURL != "" {
		client := newAPIClient(cfg.ServerURL)
		client.setSessionID(cfg.SessionID)
		_, _ = client.post("/api/auth/logout", map[string]any{}, nil)
	}
	if err := clearCLIAuth(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Logged out.")
	return nil
}

func clearCLIAuth() error {
	cfg, err := loadCLIConfig()
	if err != nil {
		return err
	}
	cfg.SessionID = ""
	cfg.DaemonTokens = nil
	return saveCLIConfig(cfg)
}

func readLine(r io.Reader, prompt string) (string, error) {
	if prompt != "" {
		fmt.Fprint(os.Stderr, prompt)
	}
	reader := bufio.NewReader(r)
	line, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func flagOrEnvConfig(cmd *cobra.Command, flagName, envKey, configValue, defaultValue string) string {
	if v, _ := cmd.Flags().GetString(flagName); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	if strings.TrimSpace(configValue) != "" {
		return strings.TrimSpace(configValue)
	}
	return defaultValue
}
