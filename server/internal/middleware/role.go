package middleware

import (
	"context"
	"net/http"
	"os"
	"strings"
)

const DaemonIDKey contextKey = "daemon_id"

// WorkspaceRole injects a workspace role into context. It is primarily useful
// for tests and narrow internal route adapters.
func WorkspaceRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), WorkspaceRoleKey, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Role checks that the authenticated user has one of the required roles for
// the current workspace.
func Role(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(roles) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			current := GetWorkspaceRole(r)
			for _, role := range roles {
				if current == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "insufficient workspace role")
		})
	}
}

// RequireMember is a convenience middleware that requires any workspace role.
func RequireMember() func(http.Handler) http.Handler {
	return Role("owner", "admin", "member")
}

// RequireOwner is a convenience middleware that requires the user to be
// the workspace owner.
func RequireOwner() func(http.Handler) http.Handler {
	return Role("owner")
}

// RequireAdmin is a convenience middleware that requires the user to be
// the workspace owner or admin.
func RequireAdmin() func(http.Handler) http.Handler {
	return Role("owner", "admin")
}

// DaemonIdentity validates daemon-facing requests. It accepts either
// X-Daemon-ID or a Bearer token matching DAEMON_TOKEN when that env var is set.
func DaemonIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		daemonID := r.Header.Get("X-Daemon-ID")
		if daemonID == "" && daemonToken(r) != "" {
			daemonID = "token-auth"
		}
		if daemonID != "" {
			ctx := context.WithValue(r.Context(), DaemonIDKey, daemonID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		writeError(w, http.StatusUnauthorized, "daemon identity required")
	})
}

func GetDaemonID(r *http.Request) string {
	if id, ok := r.Context().Value(DaemonIDKey).(string); ok {
		return id
	}
	return r.Header.Get("X-Daemon-ID")
}

func daemonToken(r *http.Request) string {
	expected := os.Getenv("DAEMON_TOKEN")
	if expected == "" {
		return ""
	}
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader || token != expected {
		return ""
	}
	return token
}
