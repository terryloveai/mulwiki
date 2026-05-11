package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPrintJSONWritesIndentedObject(t *testing.T) {
	var out bytes.Buffer

	if err := printJSON(&out, map[string]string{"slug": "demo"}); err != nil {
		t.Fatalf("print json: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"slug": "demo"`) {
		t.Fatalf("json output = %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("json output should end with newline: %q", got)
	}
}

func TestPrintTableAlignsColumns(t *testing.T) {
	var out bytes.Buffer

	printTable(&out, []string{"SLUG", "NAME"}, [][]string{
		{"demo", "Demo"},
		{"long-workspace", "Long"},
	})

	got := out.String()
	if !strings.Contains(got, "SLUG            NAME") {
		t.Fatalf("table header not aligned: %q", got)
	}
	if !strings.Contains(got, "long-workspace  Long") {
		t.Fatalf("table row not aligned: %q", got)
	}
}

func TestResolveTextFlagDecodesEscapes(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("content", "", "")
	cmd.Flags().Bool("content-stdin", false, "")
	if err := cmd.Flags().Set("content", "line 1\\nline 2"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	got, ok, err := resolveTextFlag(cmd, "content", strings.NewReader(""))
	if err != nil {
		t.Fatalf("resolve text: %v", err)
	}
	if !ok || got != "line 1\nline 2" {
		t.Fatalf("resolved text = %q, %v", got, ok)
	}
}

func TestResolveTextFlagReadsStdinAndTrimsOneTrailingNewline(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("content", "", "")
	cmd.Flags().Bool("content-stdin", false, "")
	if err := cmd.Flags().Set("content-stdin", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	got, ok, err := resolveTextFlag(cmd, "content", strings.NewReader("line 1\nline 2\n"))
	if err != nil {
		t.Fatalf("resolve text: %v", err)
	}
	if !ok || got != "line 1\nline 2" {
		t.Fatalf("resolved stdin text = %q, %v", got, ok)
	}
}

func TestResolveTextFlagRejectsInlineAndStdinTogether(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("content", "", "")
	cmd.Flags().Bool("content-stdin", false, "")
	if err := cmd.Flags().Set("content", "inline"); err != nil {
		t.Fatalf("set content flag: %v", err)
	}
	if err := cmd.Flags().Set("content-stdin", "true"); err != nil {
		t.Fatalf("set stdin flag: %v", err)
	}

	if _, _, err := resolveTextFlag(cmd, "content", strings.NewReader("stdin")); err == nil {
		t.Fatal("expected mutual exclusion error")
	}
}
