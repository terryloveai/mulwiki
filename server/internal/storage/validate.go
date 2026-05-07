package storage

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

// Validation errors.
var (
	ErrFileTooLarge   = &ValidationError{Message: "file exceeds maximum allowed size"}
	ErrFileTypeDenied = &ValidationError{Message: "file type is not supported"}
	ErrEmptyFile      = &ValidationError{Message: "file is empty or zero-sized"}
	ErrInvalidName    = &ValidationError{Message: "file name contains invalid characters"}
)

// ValidationError is a user-facing file validation error.
type ValidationError struct {
	Message string
	Detail  string
}

func (e *ValidationError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Detail)
	}
	return e.Message
}

// ValidateConfig holds validation parameters.
type ValidateConfig struct {
	MaxSizeBytes int64
	AllowedTypes []string // e.g. ["application/pdf", "text/markdown", "text/plain"]
}

// DefaultValidateConfig returns a sensible default config.
func DefaultValidateConfig() ValidateConfig {
	return ValidateConfig{
		MaxSizeBytes: 100 * 1024 * 1024, // 100 MB
		AllowedTypes: []string{
			"application/pdf",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"text/markdown",
			"text/plain",
			"text/csv",
			"application/json",
			"image/png",
			"image/jpeg",
			"image/webp",
			"application/zip",
		},
	}
}

// extensionToMIME maps file extensions to MIME types for detection when
// the client doesn't provide a Content-Type or serves a generic one.
var extensionToMIME = map[string]string{
	".pdf":  "application/pdf",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".md":   "text/markdown",
	".txt":  "text/plain",
	".csv":  "text/csv",
	".json": "application/json",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".zip":  "application/zip",
}

// DetectMIME determines the MIME type from the file extension.
// Returns empty string if the extension is unknown.
func DetectMIME(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	return extensionToMIME[ext]
}

// ValidateMultipart validates a multipart file header against the config.
// It checks: name safety, non-empty, size limit, and MIME type.
// Returns the detected MIME type on success.
func (c ValidateConfig) ValidateMultipart(header *multipart.FileHeader) (string, error) {
	// 1. Validate file name.
	name := header.Filename
	if name == "" {
		return "", ErrEmptyFile
	}
	if err := ValidateFileName(name); err != nil {
		return "", err
	}

	// 2. Validate file is non-empty.
	if header.Size == 0 {
		return "", ErrEmptyFile
	}

	// 3. Validate file size.
	if header.Size > c.MaxSizeBytes {
		return "", &ValidationError{
			Message: "file exceeds maximum allowed size",
			Detail:  fmt.Sprintf("%d bytes exceeds limit of %d bytes", header.Size, c.MaxSizeBytes),
		}
	}

	// 4. Validate MIME type.
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		// Fall back to extension-based detection.
		mimeType = DetectMIME(name)
	}
	if mimeType == "" {
		return "", &ValidationError{
			Message: "unable to determine file type",
			Detail:  fmt.Sprintf("unknown extension: %s", filepath.Ext(name)),
		}
	}
	if !c.isAllowed(mimeType) {
		return "", &ValidationError{
			Message: "file type is not supported",
			Detail:  mimeType,
		}
	}

	return mimeType, nil
}

// ValidateSourceRead validates a source after it has been read into memory.
func (c ValidateConfig) ValidateSourceRead(filename string, data []byte) (string, error) {
	// 1. Validate file name.
	if err := ValidateFileName(filename); err != nil {
		return "", err
	}

	// 2. Validate non-empty.
	if len(data) == 0 {
		return "", ErrEmptyFile
	}

	// 3. Validate size.
	size := int64(len(data))
	if size > c.MaxSizeBytes {
		return "", &ValidationError{
			Message: "file exceeds maximum allowed size",
			Detail:  fmt.Sprintf("%d bytes exceeds limit of %d bytes", size, c.MaxSizeBytes),
		}
	}

	// 4. Detect type from magic bytes first, then fall back to extension.
	mimeType := http.DetectContentType(data)
	if mimeType == "application/octet-stream" {
		mimeType = DetectMIME(filename)
	}
	if mimeType == "" {
		return "", &ValidationError{
			Message: "unable to determine file type",
			Detail:  fmt.Sprintf("unknown extension: %s", filepath.Ext(filename)),
		}
	}
	if !c.isAllowed(mimeType) {
		return "", &ValidationError{
			Message: "file type is not supported",
			Detail:  mimeType,
		}
	}

	return mimeType, nil
}

// isAllowed checks whether the given MIME type is in the allowed list.
// It handles charset suffixes (e.g., "text/plain; charset=utf-8" matches "text/plain").
func (c ValidateConfig) isAllowed(mimeType string) bool {
	// Strip charset and other parameters.
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	for _, allowed := range c.AllowedTypes {
		if strings.EqualFold(mimeType, allowed) {
			return true
		}
	}
	return false
}

// ValidateFileName checks a file name for safety:
// - rejects empty names
// - rejects names with path separators
// - rejects names starting with "."
// - rejects names with only whitespace
func ValidateFileName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ErrEmptyFile
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return &ValidationError{
			Message: "file name contains invalid characters",
			Detail:  "path separators are not allowed",
		}
	}
	if strings.HasPrefix(trimmed, ".") {
		return &ValidationError{
			Message: "file name contains invalid characters",
			Detail:  "hidden files are not allowed",
		}
	}
	return nil
}
