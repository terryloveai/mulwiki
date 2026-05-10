package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/tethy/mulwiki/server/internal/auth"
)

func DaemonAuth(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r)
			if raw == "" {
				writeError(w, http.StatusUnauthorized, "daemon token required")
				return
			}

			identity, ok, err := auth.VerifyDaemonTokenIdentity(r.Context(), db, raw)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to verify daemon token")
				return
			}
			if !ok {
				writeError(w, http.StatusUnauthorized, "invalid daemon token")
				return
			}

			ctx := WithDaemonIdentity(r.Context(), identity)
			ctx = context.WithValue(ctx, DaemonTokenKey, raw)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

const DaemonTokenKey contextKey = "daemon_token"
const DaemonWorkspaceIDKey contextKey = "daemon_workspace_id"
const DaemonUserIDKey contextKey = "daemon_user_id"
const DaemonScopeKey contextKey = "daemon_scope"

func WithDaemon(ctx context.Context, workspaceID, daemonID string) context.Context {
	ctx = context.WithValue(ctx, WorkspaceIDKey, workspaceID)
	ctx = context.WithValue(ctx, DaemonWorkspaceIDKey, workspaceID)
	ctx = context.WithValue(ctx, DaemonIDKey, daemonID)
	ctx = context.WithValue(ctx, DaemonScopeKey, "workspace")
	return ctx
}

func WithDaemonIdentity(ctx context.Context, identity auth.DaemonTokenIdentity) context.Context {
	if identity.Scope == "workspace" && identity.WorkspaceID != "" {
		ctx = context.WithValue(ctx, WorkspaceIDKey, identity.WorkspaceID)
		ctx = context.WithValue(ctx, DaemonWorkspaceIDKey, identity.WorkspaceID)
	}
	if identity.Scope == "user" && identity.UserID != "" {
		ctx = context.WithValue(ctx, UserIDKey, identity.UserID)
		ctx = context.WithValue(ctx, DaemonUserIDKey, identity.UserID)
	}
	ctx = context.WithValue(ctx, DaemonIDKey, identity.DaemonID)
	ctx = context.WithValue(ctx, DaemonScopeKey, identity.Scope)
	return ctx
}

func RequireDaemonWorkspaceAccess(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspaceID := GetWorkspaceID(r)
			if workspaceID == "" {
				writeError(w, http.StatusInternalServerError, "workspace not resolved")
				return
			}
			if tokenWorkspaceID := DaemonWorkspaceIDFromContext(r.Context()); tokenWorkspaceID != "" {
				if tokenWorkspaceID != workspaceID {
					writeError(w, http.StatusForbidden, "workspace access denied")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			userID := DaemonUserIDFromContext(r.Context())
			if userID == "" {
				writeError(w, http.StatusForbidden, "workspace access denied")
				return
			}
			var role string
			if err := db.QueryRow(`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role); err != nil {
				writeError(w, http.StatusForbidden, "workspace access denied")
				return
			}
			ctx := context.WithValue(r.Context(), WorkspaceRoleKey, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func DaemonWorkspaceIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(DaemonWorkspaceIDKey).(string); ok {
		return id
	}
	return ""
}

func WorkspaceIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(WorkspaceIDKey).(string); ok {
		return id
	}
	return ""
}

func DaemonUserIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(DaemonUserIDKey).(string); ok {
		return id
	}
	return ""
}

func DaemonScopeFromContext(ctx context.Context) string {
	if scope, ok := ctx.Value(DaemonScopeKey).(string); ok {
		return scope
	}
	return ""
}

func DaemonIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(DaemonIDKey).(string); ok {
		return id
	}
	return ""
}

func DaemonTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(DaemonTokenKey).(string); ok {
		return token
	}
	return ""
}

func bearerToken(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return ""
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		return ""
	}
	return strings.TrimSpace(token)
}
