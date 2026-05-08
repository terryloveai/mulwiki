package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDaemonTokenPrecedence(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("mwd_file-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	t.Setenv("MULWIKI_DAEMON_TOKEN", "mwd_env-token")
	token, err := resolveDaemonToken("mwd_flag-token", tokenPath)
	if err != nil {
		t.Fatalf("resolve flag token: %v", err)
	}
	if token != "mwd_flag-token" {
		t.Fatalf("expected flag token, got %q", token)
	}

	token, err = resolveDaemonToken("", tokenPath)
	if err != nil {
		t.Fatalf("resolve env token: %v", err)
	}
	if token != "mwd_env-token" {
		t.Fatalf("expected env token, got %q", token)
	}

	t.Setenv("MULWIKI_DAEMON_TOKEN", "")
	token, err = resolveDaemonToken("", tokenPath)
	if err != nil {
		t.Fatalf("resolve file token: %v", err)
	}
	if token != "mwd_file-token" {
		t.Fatalf("expected file token, got %q", token)
	}
}

func TestResolveDaemonTokenMissingIsEmpty(t *testing.T) {
	t.Setenv("MULWIKI_DAEMON_TOKEN", "")
	token, err := resolveDaemonToken("", filepath.Join(t.TempDir(), "missing-token"))
	if err != nil {
		t.Fatalf("resolve missing token: %v", err)
	}
	if token != "" {
		t.Fatalf("expected empty token, got %q", token)
	}
}
