package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileConfigPathAndDaemonPortAreIsolated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	defaultPath, err := cliConfigPathForProfile("")
	if err != nil {
		t.Fatalf("default config path: %v", err)
	}
	profilePath, err := cliConfigPathForProfile("dev")
	if err != nil {
		t.Fatalf("profile config path: %v", err)
	}

	if filepath.Base(defaultPath) != "config.json" {
		t.Fatalf("unexpected default config path: %s", defaultPath)
	}
	if profilePath == defaultPath {
		t.Fatalf("profile config path should differ from default")
	}
	if !strings.Contains(filepath.ToSlash(profilePath), "/.mulwiki/profiles/dev/config.json") {
		t.Fatalf("unexpected profile config path: %s", profilePath)
	}
	if daemonHealthPortForProfile("") != 19515 {
		t.Fatalf("default profile should use legacy health port")
	}
	if daemonHealthPortForProfile("dev") == 19515 {
		t.Fatalf("named profile should use an isolated health port")
	}
}
