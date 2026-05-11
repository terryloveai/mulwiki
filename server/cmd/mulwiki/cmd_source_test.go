package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSourceAddUploadsFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	filePath := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(filePath, []byte("# Note\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	var sawFile bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/demo/sources" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if header.Filename != "note.md" || string(data) != "# Note\n" {
			t.Fatalf("upload = %q %q", header.Filename, string(data))
		}
		sawFile = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"note.md","type":"markdown","path":"sources/note.md","size":7}`))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := sourceAddTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runSourceAdd(cmd, []string{filePath}); err != nil {
		t.Fatalf("source add: %v", err)
	}

	if !sawFile || !strings.Contains(out.String(), `"path": "sources/note.md"`) {
		t.Fatalf("sawFile=%v output=%q", sawFile, out.String())
	}
}

func TestSourceGetRawPrintsBody(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/demo/sources/sources/note.md/raw" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte("# Note\n"))
	}))
	defer server.Close()
	if err := saveCLIConfig(CLIConfig{ServerURL: server.URL, SessionID: "sess", WorkspaceSlug: "demo"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cmd := sourceGetTestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("raw", "true"); err != nil {
		t.Fatalf("set raw: %v", err)
	}

	if err := runSourceGet(cmd, []string{"sources/note.md"}); err != nil {
		t.Fatalf("source get: %v", err)
	}
	if out.String() != "# Note\n" {
		t.Fatalf("raw output = %q", out.String())
	}
}

func sourceAddTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addSourceAddFlags(cmd)
	return cmd
}

func sourceGetTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addSourceGetFlags(cmd)
	return cmd
}

var _ = multipart.ErrMessageTooLarge
