// Package gitrepo manages bare git repositories as workspace storage backends.
//
// Each workspace gets a bare git repo under data/repos/{workspaceID}.git/.
// All file operations (create, read, update, delete) go through git commands,
// giving us free version history, diff, and rollback without any extra code.
package gitrepo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Repo wraps a bare git repository on the local filesystem.
type Repo struct {
	Path string // absolute path to the bare repo directory
}

// gitEnv returns the environment for git subprocesses. Disables TTY prompting
// so git never blocks waiting for a terminal.
func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
}

// InitBare creates a new bare git repository at the given path.
// If the directory already exists and is a bare repo, it's a no-op.
func InitBare(path string) (*Repo, error) {
	if isBareRepo(path) {
		return &Repo{Path: path}, nil
	}

	// Remove anything that's not a valid bare repo at that path.
	os.RemoveAll(path)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create parent dir: %w", err)
	}

	cmd := exec.Command("git", "init", "--bare", path)
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git init --bare: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return &Repo{Path: path}, nil
}

// Open opens an existing bare repo at the given path.
func Open(path string) (*Repo, error) {
	if !isBareRepo(path) {
		return nil, fmt.Errorf("not a bare git repo: %s", path)
	}
	return &Repo{Path: path}, nil
}

func isBareRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, "HEAD"))
	return err == nil
}

// SetRemote adds or updates the origin remote URL.
func (r *Repo) SetRemote(url string) error {
	// Check if remote already exists.
	cmd := exec.Command("git", "-C", r.Path, "remote", "get-url", "origin")
	cmd.Env = gitEnv()
	if err := cmd.Run(); err == nil {
		// Remote exists, update it.
		cmd = exec.Command("git", "-C", r.Path, "remote", "set-url", "origin", url)
	} else {
		// Remote doesn't exist, add it.
		cmd = exec.Command("git", "-C", r.Path, "remote", "add", "origin", url)
	}
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set remote: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Push pushes all refs to the origin remote. Non-fatal on failure.
func (r *Repo) Push() error {
	cmd := exec.Command("git", "-C", r.Path, "push", "--all", "origin")
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// WriteFile stages a file in the git index (without a worktree, you can't
// just write to disk — you hash the blob and update the index directly).
// Then commits with the given message. Returns the commit hash.
//
// In a bare repo, we use git hash-object + git update-index + git write-tree
// + git commit-tree + git update-ref to commit without a working directory.
func (r *Repo) WriteFile(relPath string, content []byte, message string) (string, error) {
	return r.writeCommit(relPath, content, "", message)
}

// RemoveFile deletes a file from the repo. Uses git rm --cached for bare repos.
// Returns the commit hash.
func (r *Repo) RemoveFile(relPath, message string) (string, error) {
	return r.writeCommit("", nil, relPath, message)
}

// writeCommit handles both add and remove commits in a bare repo.
// For add: pass newPath and content. For remove: pass removePath.
// Uses a temporary index file (GIT_INDEX_FILE) to handle nested paths correctly.
func (r *Repo) writeCommit(newPath string, content []byte, removePath, message string) (string, error) {
	// Create a temporary index file. Must be removed first — git's read-tree
	// initializes the index from scratch and an empty file is invalid.
	tmpIdx, err := os.CreateTemp("", "mulwiki-idx-")
	if err != nil {
		return "", fmt.Errorf("create temp index: %w", err)
	}
	tmpIdx.Close()
	os.Remove(tmpIdx.Name())
	defer os.Remove(tmpIdx.Name())

	idxEnv := append(os.Environ(), "GIT_INDEX_FILE="+tmpIdx.Name())

	// If there are existing commits, read the current tree into the temp index.
	// Otherwise, initialize an empty index.
	headHash, headErr := r.headCommitHash()
	if headErr == nil {
		// Read existing tree into temp index.
		cmd := exec.Command("git", "-C", r.Path, "read-tree", headHash)
		cmd.Env = idxEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("read-tree: %s: %w", strings.TrimSpace(string(out)), err)
		}
	} else {
		// Initialize an empty index.
		cmd := exec.Command("git", "-C", r.Path, "read-tree", "--empty")
		cmd.Env = idxEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("read-tree --empty: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	if newPath != "" && content != nil {
		// Hash the blob.
		hash, err := hashBlob(r.Path, content)
		if err != nil {
			return "", fmt.Errorf("hash blob: %w", err)
		}

		// Add to temp index. --cacheinfo <mode>,<sha1>,<path>
		cmd := exec.Command("git", "-C", r.Path, "update-index", "--add",
			"--cacheinfo", fmt.Sprintf("100644,%s,%s", hash, newPath))
		cmd.Env = idxEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("update-index: %s: %w", strings.TrimSpace(string(out)), err)
		}
	} else if removePath != "" {
		// For removal in a bare repo, rebuild the index without the removed path.
		// update-index --remove requires a worktree, so we re-init the index
		// and add back only the entries we want to keep.
		headHash, headErr := r.headCommitHash()
		if headErr != nil {
			return "", fmt.Errorf("nothing to remove — repo has no commits")
		}

		// List all files in HEAD.
		lsCmd := exec.Command("git", "-C", r.Path, "ls-tree", "--full-tree", "-r", headHash)
		lsCmd.Env = gitEnv()
		out, err := lsCmd.Output()
		if err != nil {
			return "", fmt.Errorf("ls-tree: %w", err)
		}

		// Re-init empty index.
		initCmd := exec.Command("git", "-C", r.Path, "read-tree", "--empty")
		initCmd.Env = idxEnv
		if out, err := initCmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("read-tree --empty: %s: %w", strings.TrimSpace(string(out)), err)
		}

		found := false
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			// Format: <mode> <type> <hash>\t<path>
			parts := strings.SplitN(line, " ", 3)
			if len(parts) < 3 {
				continue
			}
			tabParts := strings.SplitN(parts[2], "\t", 2)
			if len(tabParts) < 2 {
				continue
			}
			path := tabParts[1]
			if path == removePath {
				found = true
				continue
			}
			mode := parts[0]
			hash := tabParts[0]
			addCmd := exec.Command("git", "-C", r.Path, "update-index", "--add",
				"--cacheinfo", fmt.Sprintf("%s,%s,%s", mode, hash, path))
			addCmd.Env = idxEnv
			if out, err := addCmd.CombinedOutput(); err != nil {
				return "", fmt.Errorf("re-add %s: %s: %w", path, strings.TrimSpace(string(out)), err)
			}
		}
		if !found {
			return "", fmt.Errorf("file not found in repo: %s", removePath)
		}
	}

	// Write the tree from the temp index.
	cmd := exec.Command("git", "-C", r.Path, "write-tree")
	cmd.Env = idxEnv
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("write-tree: %s: %w", strings.TrimSpace(string(ee.Stderr)), err)
		}
		return "", fmt.Errorf("write-tree: %w", err)
	}
	treeHash := strings.TrimSpace(string(out))

	// Create commit.
	parentHash := ""
	if headErr == nil {
		parentHash = headHash
	}

	commitHash, err := createCommit(r.Path, treeHash, parentHash, message)
	if err != nil {
		return "", fmt.Errorf("create commit: %w", err)
	}

	// Update HEAD.
	if err := r.updateHead(commitHash); err != nil {
		return "", fmt.Errorf("update HEAD: %w", err)
	}

	return commitHash, nil
}

type indexEntry struct {
	path string
	mode string
	hash string
}

// hashBlob hashes content as a git blob object and returns the hex hash.
func hashBlob(repoPath string, content []byte) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "hash-object", "-w", "--stdin")
	cmd.Stdin = strings.NewReader(string(content))
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// getHeadTree returns the tree entries from the current HEAD commit.
func (r *Repo) getHeadTree() []indexEntry {
	hash, err := r.headCommitHash()
	if err != nil {
		return nil // no commits yet
	}

	cmd := exec.Command("git", "-C", r.Path, "ls-tree", "--full-tree", "-r", hash)
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var entries []indexEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Format: <mode> <type> <hash>\t<path>
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 3 {
			continue
		}
		tabParts := strings.SplitN(parts[2], "\t", 2)
		if len(tabParts) < 2 {
			continue
		}
		entries = append(entries, indexEntry{
			mode: parts[0],
			hash: tabParts[0],
			path: tabParts[1],
		})
	}
	return entries
}

// headCommitHash returns the hex SHA of the current HEAD, or error if no commits.
func (r *Repo) headCommitHash() (string, error) {
	cmd := exec.Command("git", "-C", r.Path, "rev-parse", "--verify", "HEAD")
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// updateHead moves HEAD to point to the given commit hash.
func (r *Repo) updateHead(hash string) error {
	cmd := exec.Command("git", "-C", r.Path, "update-ref", "HEAD", hash)
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("update-ref: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// hashBlob hashes content as a git blob object and returns the hex hash.
func createCommit(repoPath, treeHash, parentHash, message string) (string, error) {
	args := []string{"-C", repoPath, "commit-tree", treeHash}
	if parentHash != "" {
		args = append(args, "-p", parentHash)
	}
	args = append(args, "-m", message)

	cmd := exec.Command("git", args...)
	cmd.Env = append(gitEnv(),
		"GIT_AUTHOR_NAME=Mulwiki",
		"GIT_AUTHOR_EMAIL=mulwiki@local",
		"GIT_COMMITTER_NAME=Mulwiki",
		"GIT_COMMITTER_EMAIL=mulwiki@local",
		"GIT_COMMITTER_DATE="+time.Now().UTC().Format(time.RFC3339),
	)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("commit-tree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ListFiles returns all tracked files in the repo, optionally filtered by
// a directory prefix (e.g. "sources/").
func (r *Repo) ListFiles(prefix string) ([]string, error) {
	cmd := exec.Command("git", "-C", r.Path, "ls-tree", "--full-tree", "-r", "--name-only", "HEAD")
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		// No commits yet — empty repo.
		return nil, nil
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if prefix == "" || strings.HasPrefix(line, prefix) {
			files = append(files, line)
		}
	}
	return files, nil
}

// ShowFile returns the content of a file at the given path from HEAD.
func (r *Repo) ShowFile(path string) ([]byte, error) {
	cmd := exec.Command("git", "-C", r.Path, "show", "HEAD:"+path)
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show HEAD:%s: %w", path, err)
	}
	return out, nil
}

// FileSize returns the size in bytes of a tracked file.
func (r *Repo) FileSize(path string) (int64, error) {
	cmd := exec.Command("git", "-C", r.Path, "cat-file", "-s", "HEAD:"+path)
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	var size int64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &size); err != nil {
		return 0, err
	}
	return size, nil
}

// HasFiles returns true if the repo has at least one commit.
func (r *Repo) HasFiles() bool {
	_, err := r.headCommitHash()
	return err == nil
}
