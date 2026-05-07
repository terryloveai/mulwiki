package gitrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempDir creates a temporary directory and returns a cleanup function.
func tempDir(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mulwiki-git-test-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	return dir, func() { os.RemoveAll(dir) }
}

func TestInitBare(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if r.Path != repoPath {
		t.Errorf("expected path %s, got %s", repoPath, r.Path)
	}

	// Verify it's a valid bare repo.
	if !isBareRepo(repoPath) {
		t.Error("expected bare repo to have HEAD file")
	}

	// InitBare on existing bare repo should be a no-op.
	r2, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare on existing repo: %v", err)
	}
	if r2.Path != repoPath {
		t.Errorf("expected same path, got %s", r2.Path)
	}
}

func TestOpen(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	_, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	r, err := Open(repoPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if r.Path != repoPath {
		t.Errorf("expected path %s, got %s", repoPath, r.Path)
	}

	// Open on non-repo path should fail.
	_, err = Open("/tmp/no-such-repo-99999.git")
	if err == nil {
		t.Error("expected error opening non-existent repo")
	}
}

func TestWriteFileAndShowFile(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	content := []byte("# Hello World\n\nThis is a test file.\n")
	commitHash, err := r.WriteFile("wiki/test.md", content, "add test wiki page")
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if commitHash == "" {
		t.Error("expected non-empty commit hash")
	}
	if !strings.HasPrefix(commitHash, "HEAD -> ") {
		// commit-tree returns a hash, update-ref changes HEAD
		_ = commitHash // hash may vary
	}

	// Read it back.
	read, err := r.ShowFile("wiki/test.md")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if string(read) != string(content) {
		t.Errorf("content mismatch:\nexpected: %q\ngot:      %q", content, read)
	}

	// Write a second file with nested path.
	content2 := []byte(`{"key": "value"}`)
	_, err = r.WriteFile("sources/config.json", content2, "add config")
	if err != nil {
		t.Fatalf("WriteFile second file: %v", err)
	}

	read2, err := r.ShowFile("sources/config.json")
	if err != nil {
		t.Fatalf("ShowFile second file: %v", err)
	}
	if string(read2) != string(content2) {
		t.Errorf("content mismatch")
	}
}

func TestListFiles(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	// Empty repo — no files.
	files, err := r.ListFiles("")
	if err != nil {
		t.Fatalf("ListFiles on empty repo: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}

	r.WriteFile("sources/a.txt", []byte("a"), "add a")
	r.WriteFile("sources/b.txt", []byte("b"), "add b")
	r.WriteFile("wiki/readme.md", []byte("# Wiki"), "add wiki")

	// List all files.
	all, err := r.ListFiles("")
	if err != nil {
		t.Fatalf("ListFiles all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(all), all)
	}

	// Filter by prefix.
	srcs, err := r.ListFiles("sources/")
	if err != nil {
		t.Fatalf("ListFiles sources: %v", err)
	}
	if len(srcs) != 2 {
		t.Errorf("expected 2 source files, got %d: %v", len(srcs), srcs)
	}

	// Filter by non-matching prefix.
	empty, err := r.ListFiles("nonexistent/")
	if err != nil {
		t.Fatalf("ListFiles nonexistent: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 files, got %d", len(empty))
	}
}

func TestRemoveFile(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	r.WriteFile("sources/to-keep.txt", []byte("keep"), "add keep")
	r.WriteFile("sources/to-delete.txt", []byte("delete"), "add delete")

	// Verify both exist.
	files, _ := r.ListFiles("sources/")
	if len(files) != 2 {
		t.Fatalf("expected 2 files before delete, got %d", len(files))
	}

	// Delete one.
	_, err = r.RemoveFile("sources/to-delete.txt", "remove test file")
	if err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}

	// Verify only one remains.
	files, _ = r.ListFiles("sources/")
	if len(files) != 1 {
		t.Errorf("expected 1 file after delete, got %d", len(files))
	}
	if files[0] != "sources/to-keep.txt" {
		t.Errorf("expected 'sources/to-keep.txt', got '%s'", files[0])
	}

	// Verify deleted file is gone.
	_, err = r.ShowFile("sources/to-delete.txt")
	if err == nil {
		t.Error("expected error reading deleted file")
	}
}

func TestRemoveFile_NotFound(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	r.WriteFile("sources/exists.txt", []byte("data"), "add file")

	_, err = r.RemoveFile("sources/does-not-exist.txt", "remove missing")
	if err == nil {
		t.Error("expected error removing non-existent file")
	}
}

func TestHasFiles(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	if r.HasFiles() {
		t.Error("expected HasFiles=false for empty repo")
	}

	r.WriteFile("file.txt", []byte("content"), "initial commit")

	if !r.HasFiles() {
		t.Error("expected HasFiles=true after commit")
	}
}

func TestFileSize(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	content := []byte("hello world, 21 chars!")
	r.WriteFile("sources/hello.txt", content, "add hello")

	size, err := r.FileSize("sources/hello.txt")
	if err != nil {
		t.Fatalf("FileSize: %v", err)
	}
	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}

	// Non-existent file.
	_, err = r.FileSize("sources/missing.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSetRemote(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	// Add a remote.
	err = r.SetRemote("https://github.com/test/repo.git")
	if err != nil {
		t.Fatalf("SetRemote add: %v", err)
	}

	// Update the remote (no error expected).
	err = r.SetRemote("https://github.com/test/repo-updated.git")
	if err != nil {
		t.Fatalf("SetRemote update: %v", err)
	}
}

func TestWriteFile_EmptyRepo(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	// First commit on an empty repo should work (uses read-tree --empty).
	hash, err := r.WriteFile("sources/first.txt", []byte("first!"), "initial commit")
	if err != nil {
		t.Fatalf("WriteFile on empty repo: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty commit hash")
	}

	// Second commit should also work (uses read-tree HEAD).
	hash2, err := r.WriteFile("sources/second.txt", []byte("second!"), "second commit")
	if err != nil {
		t.Fatalf("WriteFile second commit: %v", err)
	}
	if hash2 == hash {
		t.Error("expected different commit hashes")
	}

	// Both files should be listed.
	files, _ := r.ListFiles("sources/")
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestWriteFile_UpdateExisting(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	r.WriteFile("wiki/page.md", []byte("version 1"), "create page")
	r.WriteFile("wiki/page.md", []byte("version 2"), "update page")

	content, err := r.ShowFile("wiki/page.md")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if string(content) != "version 2" {
		t.Errorf("expected 'version 2', got '%s'", content)
	}
}

func TestShowFile_NotFound(t *testing.T) {
	dir, clean := tempDir(t)
	defer clean()

	repoPath := filepath.Join(dir, "repo.git")
	r, err := InitBare(repoPath)
	if err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	r.WriteFile("exists.txt", []byte("data"), "commit")

	_, err = r.ShowFile("nope.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}
