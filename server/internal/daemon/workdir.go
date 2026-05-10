package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// buildWorkdir creates the isolated workdir for an agent job using git.
//
// Instead of copying individual files over HTTP, the daemon:
//  1. Clones the workspace's bare git repo (or fetches if already cached)
//  2. Creates a git worktree from the bare clone into the job's workdir
//  3. Writes schema.md (wiki type/layer taxonomy)
//  4. Writes AGENTS.md (platform protocol + agent identity)
//
// The agent sees a complete file tree:
//
//	/tmp/mulwiki-job-{jobID}/
//	├── AGENTS.md
//	├── schema.md
//	├── sources/
//	├── wiki/
//	├── schemas/
//	└── output/
func (d *Daemon) buildWorkdir(job protocol.Job, agent *protocol.Agent) (string, error) {
	workdir := filepath.Join(os.TempDir(), fmt.Sprintf("mulwiki-job-%s", job.ID))

	if _, err := os.Stat(workdir); err == nil {
		slog.Warn("removing stale workdir", "workdir", workdir)
		if err := os.RemoveAll(workdir); err != nil {
			return "", fmt.Errorf("cleanup stale workdir: %w", err)
		}
	}

	wsInfo, err := d.fetchWorkspace(job.WorkspaceSlug)
	if err != nil {
		return "", fmt.Errorf("fetch workspace: %w", err)
	}

	barePath, err := d.ensureRepo(wsInfo.ID)
	if err != nil {
		return "", fmt.Errorf("ensure repo: %w", err)
	}

	if err := d.createWorktree(barePath, workdir, job.ID); err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(workdir, "output"), 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	if job.SchemaID != "" {
		schema, err := d.fetchSchema(job.WorkspaceSlug, job.SchemaID)
		if err != nil {
			slog.Warn("failed to fetch schema, continuing without schema content", "schema_id", job.SchemaID, "error", err)
		} else if err := os.WriteFile(filepath.Join(workdir, "schema.md"), []byte(schema.Content), 0o644); err != nil {
			return "", fmt.Errorf("write schema.md: %w", err)
		}
	}

	agentsMD := d.buildAgentsMD(agent, job)
	if err := os.WriteFile(filepath.Join(workdir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		return "", fmt.Errorf("write AGENTS.md: %w", err)
	}

	return workdir, nil
}

// ensureRepo clones or fetches the workspace git repo into a local bare cache.
func (d *Daemon) ensureRepo(workspaceID string) (string, error) {
	cacheDir := filepath.Join(d.WorkDir, ".repos")
	barePath := filepath.Join(cacheDir, workspaceID+".git")
	repoURL := d.repoURL(workspaceID)

	if isBareRepo(barePath) {
		slog.Debug("repo cached, fetching", "path", barePath, "url", repoURL)
		cmd := exec.Command("git", "-C", barePath, "fetch", "origin")
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("git fetch failed (continuing with cached data)", "error", err, "output", string(out))
		}
	} else {
		slog.Info("cloning workspace repo", "url", repoURL, "path", barePath)
		if err := os.MkdirAll(filepath.Dir(barePath), 0o755); err != nil {
			return "", fmt.Errorf("create cache dir: %w", err)
		}
		os.RemoveAll(barePath)

		cmd := exec.Command("git", "clone", "--bare", repoURL, barePath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git clone --bare: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	return barePath, nil
}

// repoURL returns the git URL for a workspace's bare repo.
func (d *Daemon) repoURL(workspaceID string) string {
	if strings.HasPrefix(d.ReposURL, "http") {
		return strings.TrimRight(d.ReposURL, "/") + "/" + workspaceID + ".git"
	}
	return filepath.Join(d.ReposURL, workspaceID+".git")
}

func (d *Daemon) createWorktree(barePath, workdir, jobID string) error {
	branchName := fmt.Sprintf("job/%s", shortJobID(jobID))

	_ = exec.Command("git", "-C", barePath, "worktree", "prune").Run()
	_ = exec.Command("git", "-C", barePath, "branch", "-D", branchName).Run()

	cmd := exec.Command("git", "-C", barePath, "worktree", "add", "-b", branchName, workdir, "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	slog.Debug("worktree created", "bare", barePath, "workdir", workdir, "branch", branchName)
	return nil
}

func isBareRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, "HEAD"))
	return err == nil
}

func shortJobID(jobID string) string {
	normalized := strings.ReplaceAll(jobID, "-", "")
	if len(normalized) > 8 {
		return normalized[:8]
	}
	return normalized
}
