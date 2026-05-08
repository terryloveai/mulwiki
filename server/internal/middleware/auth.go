package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tethy/mulwiki/server/pkg/protocol"
)

const SessionCookieName = "sw_session"

const (
	UserIDKey contextKey = "user_id"
	UserKey   contextKey = "user"
)

// Auth validates an HTTP-only session cookie or a Bearer JWT and injects the
// authenticated user into the request context.
func Auth(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := authenticateRequest(db, r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "not authenticated")
				return
			}

			r.Header.Set("X-User-ID", user.ID)
			ctx := context.WithValue(r.Context(), UserIDKey, user.ID)
			ctx = context.WithValue(ctx, UserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func authenticateRequest(db *sql.DB, r *http.Request) (*protocol.User, error) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		return authenticateSession(db, cookie.Value)
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("missing credentials")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return nil, errors.New("invalid authorization header format")
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-me"
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return nil, errors.New("invalid subject")
	}

	return lookupUser(db, sub)
}

func authenticateSession(db *sql.DB, sessionID string) (*protocol.User, error) {
	var user protocol.User
	var expiresAt string
	err := db.QueryRow(
		`SELECT u.id, u.email, u.created_at, s.expires_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.id = ?`,
		sessionID,
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &expiresAt)
	if err != nil {
		return nil, err
	}

	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		_, _ = db.Exec(`DELETE FROM sessions WHERE id = ?`, sessionID)
		return nil, errors.New("session expired")
	}
	return &user, nil
}

func lookupUser(db *sql.DB, id string) (*protocol.User, error) {
	var user protocol.User
	err := db.QueryRow(`SELECT id, email, created_at FROM users WHERE id = ?`, id).
		Scan(&user.ID, &user.Email, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserID extracts the authenticated user ID from request context.
func GetUserID(r *http.Request) string {
	if id, ok := r.Context().Value(UserIDKey).(string); ok {
		return id
	}
	return r.Header.Get("X-User-ID")
}

// GetUser extracts the authenticated user from request context.
func GetUser(r *http.Request) (*protocol.User, bool) {
	user, ok := r.Context().Value(UserKey).(*protocol.User)
	return user, ok
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}
