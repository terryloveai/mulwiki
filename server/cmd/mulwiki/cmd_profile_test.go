package main

import "testing"

func TestActiveProfileRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if active, err := loadActiveProfile(); err != nil || active != "" {
		t.Fatalf("initial active profile = %q, err=%v", active, err)
	}
	if err := saveActiveProfile("dev"); err != nil {
		t.Fatalf("save active profile: %v", err)
	}
	if active, err := loadActiveProfile(); err != nil || active != "dev" {
		t.Fatalf("active profile = %q, err=%v", active, err)
	}
	if err := saveActiveProfile("default"); err != nil {
		t.Fatalf("clear active profile: %v", err)
	}
	if active, err := loadActiveProfile(); err != nil || active != "" {
		t.Fatalf("cleared active profile = %q, err=%v", active, err)
	}
}

func TestListProfilesIncludesDefaultAndNamedProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := saveCLIConfigForProfile("dev", CLIConfig{ServerURL: "http://localhost:8080"}); err != nil {
		t.Fatalf("save dev config: %v", err)
	}
	if err := saveCLIConfigForProfile("staging", CLIConfig{ServerURL: "https://example.test"}); err != nil {
		t.Fatalf("save staging config: %v", err)
	}
	profiles, err := listProfiles()
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	want := []string{"default", "dev", "staging"}
	if len(profiles) != len(want) {
		t.Fatalf("profiles = %#v, want %#v", profiles, want)
	}
	for i := range want {
		if profiles[i] != want[i] {
			t.Fatalf("profiles = %#v, want %#v", profiles, want)
		}
	}
}
