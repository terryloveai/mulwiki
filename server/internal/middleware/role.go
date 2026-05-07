package middleware

import (
	"net/http"
)

// Role checks that the authenticated user has one of the required roles
// for the current workspace. Currently placeholder — all users pass.
// In production, this would query workspace_members table.
func Role(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Placeholder: all authenticated users have full access.
			// Production: query workspace_members for the user's role
			// and check if it's in the allowed roles set.
			next.ServeHTTP(w, r)
		})
	}
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
