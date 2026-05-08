package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/tethy/mulwiki/server/internal/events"
	"github.com/tethy/mulwiki/server/internal/logbuf"
	"github.com/tethy/mulwiki/server/internal/middleware"
	"github.com/tethy/mulwiki/server/internal/realtime"
	"github.com/tethy/mulwiki/server/internal/store"
)

// Handler provides HTTP handler methods with shared dependencies.
// Pattern matches Multica's Handler struct — one Handler for all domains,
// with method receivers on individual handler files.
type Handler struct {
	DB                *sql.DB
	EventBus          *events.Bus
	Realtime          *realtime.Hub
	ReposDir          string // directory for bare git repos (e.g. "./data/repos")
	BuiltinSchemasDir string // directory for builtin schema .md files to seed (e.g. "./data/builtin-schemas")
	LogBuf            *logbuf.Store
}

// New creates a new Handler with the given database connection.
func New(db *sql.DB) *Handler {
	return &Handler{DB: db}
}

// NewWithDeps creates a new Handler with full dependencies.
func NewWithDeps(db *sql.DB, bus *events.Bus, hub *realtime.Hub) *Handler {
	return &Handler{DB: db, EventBus: bus, Realtime: hub}
}

// reposDir returns the configured repos directory, with fallback to "./data/repos".
func (h *Handler) reposDir() string {
	if h.ReposDir != "" {
		return h.ReposDir
	}
	return "./data/repos"
}

// writeJSON serializes v as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode failed", "error", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// workspaceSlug extracts the {slug} chi URL parameter.
func workspaceSlug(r *http.Request) string {
	return chi.URLParam(r, "slug")
}

// idParam extracts a named chi URL parameter (e.g. "id").
func idParam(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}

// decodeJSON decodes the request body JSON into v.
func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// isUniqueConstraint checks if the error is a SQLite UNIQUE constraint violation.
func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// userID extracts the X-User-ID header set by auth middleware.
func userID(r *http.Request) string {
	if id := middleware.GetUserID(r); id != "" {
		return id
	}
	return r.Header.Get("X-User-ID")
}

func isDaemonRequest(r *http.Request) bool {
	return middleware.GetDaemonID(r) != ""
}

func daemonID(r *http.Request) string {
	return middleware.GetDaemonID(r)
}

// nullStr returns a pointer to s, or nil if s is empty.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// strOrEmpty returns the string value or empty string if nil.
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (h *Handler) workspaceIDForRequest(r *http.Request) (string, error) {
	if id := middleware.GetWorkspaceID(r); id != "" {
		return id, nil
	}

	return store.NewWorkspaceStore(h.DB).GetIDBySlug(r.Context(), workspaceSlug(r))
}
