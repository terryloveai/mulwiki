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

			workspaceID, daemonID, ok, err := auth.VerifyDaemonToken(r.Context(), db, raw)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to verify daemon token")
				return
			}
			if !ok {
				writeError(w, http.StatusUnauthorized, "invalid daemon token")
				return
			}

			ctx := WithDaemon(r.Context(), workspaceID, daemonID)
			ctx = context.WithValue(ctx, DaemonTokenKey, raw)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

const DaemonTokenKey contextKey = "daemon_token"

func WithDaemon(ctx context.Context, workspaceID, daemonID string) context.Context {
	ctx = context.WithValue(ctx, WorkspaceIDKey, workspaceID)
	ctx = context.WithValue(ctx, DaemonIDKey, daemonID)
	return ctx
}

func WorkspaceIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(WorkspaceIDKey).(string); ok {
		return id
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
