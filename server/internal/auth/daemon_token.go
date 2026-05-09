package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const DaemonTokenPrefix = "mwd_"

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
	if !strings.HasPrefix(raw, DaemonTokenPrefix) {
		return "", "", false, nil
	}

	var expiresAt sql.NullString
	var revokedAt sql.NullString
	err = db.QueryRowContext(ctx,
		`SELECT workspace_id, daemon_id, expires_at, revoked_at
		 FROM daemon_tokens
		 WHERE token_hash = ?`,
		HashDaemonToken(raw),
	).Scan(&workspaceID, &daemonID, &expiresAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if revokedAt.Valid && strings.TrimSpace(revokedAt.String) != "" {
		return "", "", false, nil
	}
	if expiresAt.Valid && strings.TrimSpace(expiresAt.String) != "" {
		expires, err := parseDBTime(expiresAt.String)
		if err != nil {
			return "", "", false, err
		}
		if !time.Now().UTC().Before(expires) {
			return "", "", false, nil
		}
	}

	return workspaceID, daemonID, true, nil
}

func parseDBTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", value)
}
