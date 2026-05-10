package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const DaemonTokenPrefix = "mwd_"

type DaemonTokenIdentity struct {
	WorkspaceID string
	UserID      string
	DaemonID    string
	Scope       string
}

func NewDaemonToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = DaemonTokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashDaemonToken(raw), nil
}

func HashDaemonToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func VerifyDaemonToken(ctx context.Context, db *sql.DB, raw string) (workspaceID, daemonID string, ok bool, err error) {
	identity, ok, err := VerifyDaemonTokenIdentity(ctx, db, raw)
	if err != nil || !ok {
		return "", "", ok, err
	}
	return identity.WorkspaceID, identity.DaemonID, true, nil
}

func VerifyDaemonTokenIdentity(ctx context.Context, db *sql.DB, raw string) (DaemonTokenIdentity, bool, error) {
	if !strings.HasPrefix(raw, DaemonTokenPrefix) {
		return DaemonTokenIdentity{}, false, nil
	}

	identity := DaemonTokenIdentity{}
	var expiresAt sql.NullString
	var revokedAt sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT workspace_id, user_id, daemon_id, scope, expires_at, revoked_at
		 FROM daemon_tokens
		 WHERE token_hash = ?`,
		HashDaemonToken(raw),
	).Scan(&identity.WorkspaceID, &identity.UserID, &identity.DaemonID, &identity.Scope, &expiresAt, &revokedAt)
	if err != nil && strings.Contains(err.Error(), "no such column") {
		err = db.QueryRowContext(ctx,
			`SELECT workspace_id, daemon_id, expires_at, revoked_at
			 FROM daemon_tokens
			 WHERE token_hash = ?`,
			HashDaemonToken(raw),
		).Scan(&identity.WorkspaceID, &identity.DaemonID, &expiresAt, &revokedAt)
		identity.Scope = "workspace"
	}
	if errors.Is(err, sql.ErrNoRows) {
		return DaemonTokenIdentity{}, false, nil
	}
	if err != nil {
		return DaemonTokenIdentity{}, false, err
	}
	if revokedAt.Valid && strings.TrimSpace(revokedAt.String) != "" {
		return DaemonTokenIdentity{}, false, nil
	}
	if expiresAt.Valid && strings.TrimSpace(expiresAt.String) != "" {
		expires, err := parseDBTime(expiresAt.String)
		if err != nil {
			return DaemonTokenIdentity{}, false, err
		}
		if !time.Now().UTC().Before(expires) {
			return DaemonTokenIdentity{}, false, nil
		}
	}

	identity.Scope = strings.TrimSpace(identity.Scope)
	if identity.Scope == "" {
		identity.Scope = "workspace"
	}
	if identity.Scope == "user" && identity.UserID == "" {
		return DaemonTokenIdentity{}, false, fmt.Errorf("user daemon token missing user_id")
	}
	return identity, true, nil
}

func parseDBTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", value)
}
