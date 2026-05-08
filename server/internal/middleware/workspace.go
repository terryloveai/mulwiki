package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
)

type contextKey string

const (
	WorkspaceIDKey   contextKey = "workspace_id"
	WorkspaceSlugKey contextKey = "workspace_slug"
	WorkspaceRoleKey contextKey = "workspace_role"
)

// Workspace resolves the workspace from the URL slug and injects workspace_id
// into the request context. Membership is enforced by RequireWorkspaceMember.
func Workspace(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract slug from URL path:
			//   /api/workspaces/{slug}/...
			//   /api/daemon/workspaces/{slug}/...
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
			slug := ""
			if len(parts) >= 3 && parts[0] == "api" && parts[1] == "workspaces" {
				slug = parts[2]
			}
			if len(parts) >= 4 && parts[0] == "api" && parts[1] == "daemon" && parts[2] == "workspaces" {
				slug = parts[3]
			}
			if slug == "" {
				// Not a workspace-scoped route, pass through
				next.ServeHTTP(w, r)
				return
			}

			var workspaceID string
			if err := db.QueryRow(`SELECT id FROM workspaces WHERE slug = ?`, slug).Scan(&workspaceID); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"workspace not found"}`))
				return
			}
			if existingID := WorkspaceIDFromContext(r.Context()); existingID != "" && existingID != workspaceID {
				writeError(w, http.StatusForbidden, "workspace access denied")
				return
			}

			ctx := context.WithValue(r.Context(), WorkspaceIDKey, workspaceID)
			ctx = context.WithValue(ctx, WorkspaceSlugKey, slug)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireWorkspaceMember validates that the authenticated user belongs to the
// resolved workspace and injects the membership role into request context.
func RequireWorkspaceMember(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspaceID := GetWorkspaceID(r)
			if workspaceID == "" {
				writeError(w, http.StatusInternalServerError, "workspace not resolved")
				return
			}
			userID := GetUserID(r)
			if userID == "" {
				writeError(w, http.StatusUnauthorized, "not authenticated")
				return
			}

			var role string
			err := db.QueryRow(
				`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
				workspaceID, userID,
			).Scan(&role)
			if err != nil {
				writeError(w, http.StatusForbidden, "workspace access denied")
				return
			}

			ctx := context.WithValue(r.Context(), WorkspaceRoleKey, role)
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

// GetWorkspaceSlug extracts the workspace slug from the request context.
func GetWorkspaceSlug(r *http.Request) string {
	if slug, ok := r.Context().Value(WorkspaceSlugKey).(string); ok {
		return slug
	}
	return ""
}

// GetWorkspaceRole extracts the current user's workspace role.
func GetWorkspaceRole(r *http.Request) string {
	if role, ok := r.Context().Value(WorkspaceRoleKey).(string); ok {
		return role
	}
	return ""
}
