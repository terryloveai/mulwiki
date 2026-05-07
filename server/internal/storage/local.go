package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage provides simple local file storage for uploads.
type LocalStorage struct {
	DataDir string
}

// NewLocalStorage creates a new LocalStorage, ensuring the data directory exists.
func NewLocalStorage(dataDir string) (*LocalStorage, error) {
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &LocalStorage{DataDir: dataDir}, nil
}

// SaveFile writes data to a file at relativePath under DataDir.
// Returns the full path to the saved file.
func (s *LocalStorage) SaveFile(relativePath string, data []byte) (string, error) {
	fullPath := filepath.Join(s.DataDir, relativePath)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fullPath, nil
}

// SaveReader writes data from a reader to a file at relativePath under DataDir.
// Returns the full path to the saved file and the number of bytes written.
func (s *LocalStorage) SaveReader(relativePath string, reader io.Reader) (string, int64, error) {
	fullPath := filepath.Join(s.DataDir, relativePath)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", 0, fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, reader)
	if err != nil {
		return "", 0, fmt.Errorf("copy: %w", err)
	}
	return fullPath, written, nil
}

// ReadFile reads the contents of a file at relativePath under DataDir.
func (s *LocalStorage) ReadFile(relativePath string) ([]byte, error) {
	fullPath := filepath.Join(s.DataDir, relativePath)
	return os.ReadFile(fullPath)
}

// FilePath returns the absolute path for a relative path under DataDir.
func (s *LocalStorage) FilePath(relativePath string) string {
	return filepath.Join(s.DataDir, relativePath)
}
