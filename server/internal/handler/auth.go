package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tethy/mulwiki/server/pkg/protocol"
)

const sessionCookieName = "sw_session"

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func authCookie(sessionID string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func clearAuthCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func validateAuthRequest(req protocol.AuthRequest) (string, string, bool) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password
	if !strings.Contains(email, "@") || len(password) < 8 {
		return "", "", false
	}
	return email, password, true
}

func (h *Handler) createSession(w http.ResponseWriter, userID string) error {
	sessionID := uuid.New().String()
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	_, err := h.DB.Exec(
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		sessionID, userID, expires.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	http.SetCookie(w, authCookie(sessionID, expires))
	return nil
}

func (h *Handler) getSession(r *http.Request) (*protocol.User, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, errors.New("missing session")
	}

	var user protocol.User
	var expiresAt string
	err = h.DB.QueryRow(
		`SELECT u.id, u.email, u.created_at, s.expires_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.id = ?`,
		cookie.Value,
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &expiresAt)
	if err != nil {
		return nil, err
	}

	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		_, _ = h.DB.Exec(`DELETE FROM sessions WHERE id = ?`, cookie.Value)
		return nil, errors.New("session expired")
	}

	return &user, nil
}

// POST /api/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req protocol.AuthRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, password, ok := validateAuthRequest(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "valid email and password of at least 8 characters are required")
		return
	}

	var user protocol.User
	err := h.DB.QueryRow(
		`INSERT INTO users (email, password_hash) VALUES (?, ?)
		 RETURNING id, email, created_at`,
		email, hashPassword(password),
	).Scan(&user.ID, &user.Email, &user.CreatedAt)
	if err != nil {
		if isUniqueConstraint(err) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	if err := h.createSession(w, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

// POST /api/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req protocol.AuthRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, password, ok := validateAuthRequest(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "valid email and password of at least 8 characters are required")
		return
	}

	var user protocol.User
	var passwordHash string
	err := h.DB.QueryRow(
		`SELECT id, email, password_hash, created_at FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &user.Email, &passwordHash, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if passwordHash != hashPassword(password) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := h.createSession(w, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// POST /api/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		_, _ = h.DB.Exec(`DELETE FROM sessions WHERE id = ?`, cookie.Value)
	}
	http.SetCookie(w, clearAuthCookie())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/auth/me
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user, err := h.getSession(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, user)
}
