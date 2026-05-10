package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
)

const sessionCookieName = "sw_session"

type CLIConfig struct {
	ServerURL     string            `json:"server_url,omitempty"`
	WorkspaceSlug string            `json:"workspace_slug,omitempty"`
	SessionID     string            `json:"session_id,omitempty"`
	DaemonToken   string            `json:"daemon_token,omitempty"`
	DaemonTokens  map[string]string `json:"daemon_tokens,omitempty"`
}

func cliConfigPath() (string, error) {
	return cliConfigPathForProfile("")
}

func cliConfigPathForProfile(profile string) (string, error) {
	dir, err := mulwikiProfileDir(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func mulwikiBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".mulwiki"), nil
}

func activeProfilePath() (string, error) {
	base, err := mulwikiBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "profile"), nil
}

func loadActiveProfile() (string, error) {
	path, err := activeProfilePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read active profile: %w", err)
	}
	profile := normalizeProfile(string(data))
	if profile == "default" {
		return "", nil
	}
	return profile, nil
}

func saveActiveProfile(profile string) error {
	profile = normalizeProfile(profile)
	path, err := activeProfilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	if profile == "" || profile == "default" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear active profile: %w", err)
		}
		return nil
	}
	return os.WriteFile(path, []byte(profile+"\n"), 0o600)
}

func mulwikiProfileDir(profile string) (string, error) {
	base, err := mulwikiBaseDir()
	if err != nil {
		return "", err
	}
	profile = normalizeProfile(profile)
	if profile == "" || profile == "default" {
		return base, nil
	}
	return filepath.Join(base, "profiles", profile), nil
}

func normalizeProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "..", "-")
	return replacer.Replace(profile)
}

func daemonHealthPortForProfile(profile string) int {
	profile = normalizeProfile(profile)
	if profile == "" || profile == "default" {
		return 19515
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(profile))
	return 19516 + int(h.Sum32()%1000)
}

func loadCLIConfig() (CLIConfig, error) {
	return loadCLIConfigForProfile("")
}

func loadCLIConfigForProfile(profile string) (CLIConfig, error) {
	path, err := cliConfigPathForProfile(profile)
	if err != nil {
		return CLIConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CLIConfig{}, nil
		}
		return CLIConfig{}, fmt.Errorf("read CLI config: %w", err)
	}
	var cfg CLIConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return CLIConfig{}, fmt.Errorf("parse CLI config: %w", err)
	}
	return cfg, nil
}

func saveCLIConfig(cfg CLIConfig) error {
	return saveCLIConfigForProfile("", cfg)
}

func saveCLIConfigForProfile(profile string, cfg CLIConfig) error {
	path, err := cliConfigPathForProfile(profile)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}
