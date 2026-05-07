package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
)

type contextKey string

const (
	WorkspaceIDKey contextKey = "workspace_id"
	WorkspaceSlugKey contextKey = "workspace_slug"
)

// Workspace resolves the workspace from the URL slug, validates it exists,
// and injects workspace_id into the request context.
func Workspace(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract slug from URL path: /api/workspaces/{slug}/...
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
			if len(parts) < 3 || parts[0] != "api" || parts[1] != "workspaces" {
				// Not a workspace-scoped route, pass through
				next.ServeHTTP(w, r)
				return
			}
			slug := parts[2]

			var workspaceID string
			if err := db.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"workspace not found"}`))
				return
			}

			ctx := context.WithValue(r.Context(), WorkspaceIDKey, workspaceID)
			ctx = context.WithValue(ctx, WorkspaceSlugKey, slug)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetWorkspaceID extracts the workspace ID from the request context.
func GetWorkspaceID(r *http.Request) string {
	if id, ok := r.Context().Value(WorkspaceIDKey).(string); ok {
		return id
	}
	return ""
}
