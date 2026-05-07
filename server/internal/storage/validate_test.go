package storage

import (
	"strings"
	"testing"
)

func TestValidateFileName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"document.pdf", false},
		{"research-paper.docx", false},
		{"我的文件.md", false},
		{"file (1).txt", false},
		{"", true},
		{"   ", true},
		{"../escape.pdf", true},
		{".hidden", true},
		{"path/traversal.txt", true},
		{"back\\slash.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileName(%q) error = %v, wantErr = %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestDetectMIME(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"doc.pdf", "application/pdf"},
		{"doc.DOCX", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"slides.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"data.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"readme.md", "text/markdown"},
		{"notes.txt", "text/plain"},
		{"data.csv", "text/csv"},
		{"config.json", "application/json"},
		{"image.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"icon.webp", "image/webp"},
		{"archive.zip", "application/zip"},
		{"unknown.xyz", ""},
		{"noextension", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := DetectMIME(tt.filename)
			if got != tt.expected {
				t.Errorf("DetectMIME(%q) = %q, want %q", tt.filename, got, tt.expected)
			}
		})
	}
}

func TestValidateConfig_AllowedTypes(t *testing.T) {
	cfg := DefaultValidateConfig()

	allowed := []string{
		"application/pdf",
		"text/markdown",
		"image/png",
		"application/json",
	}
	for _, mime := range allowed {
		if !cfg.isAllowed(mime) {
			t.Errorf("expected %q to be allowed", mime)
		}
	}

	denied := []string{
		"application/x-msdownload",
		"application/x-sh",
		"text/html",
		"application/javascript",
	}
	for _, mime := range denied {
		if cfg.isAllowed(mime) {
			t.Errorf("expected %q to be denied", mime)
		}
	}
}

func TestValidateConfig_ValidateSourceRead(t *testing.T) {
	cfg := DefaultValidateConfig()

	// Valid markdown content.
	mime, err := cfg.ValidateSourceRead("test.md", []byte("# Hello\n\nWorld"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "text/plain; charset=utf-8" && !strings.Contains(mime, "text/plain") {
		t.Errorf("expected text/plain MIME for markdown content, got %q", mime)
	}

	// Valid PDF (magic bytes).
	pdfHeader := []byte("%PDF-1.4\n%test")
	mime, err = cfg.ValidateSourceRead("doc.pdf", pdfHeader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "application/pdf" {
		t.Errorf("expected application/pdf, got %q", mime)
	}

	// Empty file.
	_, err = cfg.ValidateSourceRead("empty.md", []byte{})
	if err == nil {
		t.Fatal("expected error for empty file")
	}

	// Bad file name.
	_, err = cfg.ValidateSourceRead("../escape.pdf", pdfHeader)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestLocalStorage_Integration(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	// Save and read back.
	path, err := s.SaveFile("test/hello.txt", []byte("hello world"))
	if err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}

	data, err := s.ReadFile("test/hello.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(data))
	}

	// FilePath.
	absPath := s.FilePath("test/hello.txt")
	if absPath != path {
		t.Errorf("FilePath mismatch: %q vs %q", absPath, path)
	}
}
