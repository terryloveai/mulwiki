package daemon

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

// collectOutput reads output/manifest.json, bundles referenced files, and delivers to server.
func (d *Daemon) collectOutput(workdir string, job protocol.Job, _ *protocol.AgentTask) (string, error) {
	manifestPath := filepath.Join(workdir, "output", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("read manifest.json: %w (agent may not have written output)", err)
	}

	var manifest struct {
		Pages []struct {
			Path  string `json:"path"`
			Title string `json:"title"`
			Type  string `json:"type"`
			Layer string `json:"layer"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("parse manifest.json: %w", err)
	}

	if len(manifest.Pages) == 0 {
		return "no pages in manifest", nil
	}

	type pageOut struct {
		Path    string `json:"path"`
		Title   string `json:"title"`
		Type    string `json:"type"`
		Layer   string `json:"layer"`
		Content string `json:"content"`
	}
	pages := make([]pageOut, 0, len(manifest.Pages))
	for _, page := range manifest.Pages {
		content, err := os.ReadFile(filepath.Join(workdir, "output", page.Path))
		if err != nil {
			slog.Warn("failed to read output file, skipping", "path", page.Path, "error", err)
			continue
		}
		pages = append(pages, pageOut{page.Path, page.Title, page.Type, page.Layer, string(content)})
	}

	payload := struct {
		JobID string    `json:"job_id"`
		Pages []pageOut `json:"pages"`
	}{job.ID, pages}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/output", d.ServerURL, d.workspaceSlugForJob(job), job.ID)
	resp, err := d.postJSON(url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("deliver output: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server rejected output: %d %s", resp.StatusCode, string(body))
	}

	return fmt.Sprintf("pages=%d", len(pages)), nil
}

func (d *Daemon) mergeEnv(parent []string, custom map[string]string) []string {
	merged := make([]string, len(parent))
	copy(merged, parent)

	for key, value := range custom {
		idx := -1
		prefix := key + "="
		for i, envVar := range merged {
			if strings.HasPrefix(envVar, prefix) {
				idx = i
				break
			}
		}

		newVal := prefix + value
		if idx >= 0 {
			merged[idx] = newVal
		} else {
			merged = append(merged, newVal)
		}

		if isSecretKey(key) {
			slog.Debug("set env", "key", key, "value", "***")
		} else {
			slog.Debug("set env", "key", key, "value", value)
		}
	}

	return merged
}

func isSecretKey(key string) bool {
	upper := strings.ToUpper(key)
	return strings.Contains(upper, "SECRET") ||
		strings.Contains(upper, "KEY") ||
		strings.Contains(upper, "TOKEN") ||
		strings.Contains(upper, "PASSWORD") ||
		strings.Contains(upper, "API_KEY")
}

func (d *Daemon) streamLogs(reader io.Reader, jobID, stream string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		slog.Debug("agent log", "job_id", jobID, "stream", stream, "line", line)
		d.postLogLine(jobID, stream, line)
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("agent log stream error", "job_id", jobID, "stream", stream, "error", err)
	}
}

func (d *Daemon) postLogLine(jobID, stream, line string, workspaceSlug ...string) {
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/log-line", d.ServerURL, d.workspaceSlugForCall(workspaceSlug...), jobID)
	body := map[string]string{
		"stream": stream,
		"line":   line,
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		slog.Debug("post log line failed", "error", err)
		return
	}
	resp.Body.Close()
}

func (d *Daemon) updateProgress(jobOrID any, progress int) {
	jobID, workspaceSlug := d.jobIDAndWorkspace(jobOrID)
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/progress", d.ServerURL, workspaceSlug, jobID)
	body := map[string]int{"progress": progress}
	jsonBody, _ := json.Marshal(body)
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		slog.Error("update progress failed", "job_id", jobID, "error", err)
		return
	}
	resp.Body.Close()
}

func (d *Daemon) completeJob(jobOrID any) {
	jobID, workspaceSlug := d.jobIDAndWorkspace(jobOrID)
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/complete", d.ServerURL, workspaceSlug, jobID)
	resp, err := d.postJSON(url, nil)
	if err != nil {
		slog.Error("complete job failed", "job_id", jobID, "error", err)
		return
	}
	resp.Body.Close()
	slog.Info("job completed", "job_id", jobID)
}

func (d *Daemon) failJob(jobOrID any, errMsg string) {
	jobID, workspaceSlug := d.jobIDAndWorkspace(jobOrID)
	url := fmt.Sprintf("%s/api/daemon/workspaces/%s/jobs/%s/fail", d.ServerURL, workspaceSlug, jobID)
	body := map[string]string{"error": errMsg}
	jsonBody, _ := json.Marshal(body)
	resp, err := d.postJSON(url, bytes.NewReader(jsonBody))
	if err != nil {
		slog.Error("fail job failed", "job_id", jobID, "error", err)
		return
	}
	resp.Body.Close()
	slog.Error("job failed", "job_id", jobID, "error", errMsg)
}

func (d *Daemon) jobIDAndWorkspace(jobOrID any) (string, string) {
	switch v := jobOrID.(type) {
	case protocol.Job:
		return v.ID, d.workspaceSlugForJob(v)
	case *protocol.Job:
		if v != nil {
			return v.ID, d.workspaceSlugForJob(*v)
		}
	}
	return fmt.Sprint(jobOrID), d.workspaceSlugForCall()
}

func (d *Daemon) workspaceSlugForJob(job protocol.Job) string {
	if strings.TrimSpace(job.WorkspaceSlug) != "" {
		return strings.TrimSpace(job.WorkspaceSlug)
	}
	return d.workspaceSlugForCall()
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	srcInfo, _ := srcFile.Stat()
	if srcInfo != nil {
		os.Chmod(dst, srcInfo.Mode())
	}

	return nil
}
