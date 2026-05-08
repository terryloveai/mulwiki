package auth

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newDaemonTokenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE daemon_tokens (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			workspace_id TEXT NOT NULL,
			daemon_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			revoked_at TEXT
		);
	`); err != nil {
		t.Fatalf("create daemon_tokens: %v", err)
	}

	return db
}

func TestDaemonTokenCanBeVerifiedFromStoredHash(t *testing.T) {
	db := newDaemonTokenTestDB(t)

	raw, hash, err := NewDaemonToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if !strings.HasPrefix(raw, "mwd_") {
		t.Fatalf("expected mwd_ prefix, got %q", raw)
	}
	if len(strings.TrimPrefix(raw, "mwd_")) < 43 {
		t.Fatalf("token suffix is too short: %q", raw)
	}
	if hash == "" || hash == raw {
		t.Fatalf("expected non-empty hash distinct from raw token")
	}
	if hash != HashDaemonToken(raw) {
		t.Fatalf("returned hash does not match HashDaemonToken")
	}

	if _, err := db.Exec(
		`INSERT INTO daemon_tokens (workspace_id, daemon_id, token_hash) VALUES ('ws1', 'daemon-1', ?)`,
		hash,
	); err != nil {
		t.Fatalf("insert token: %v", err)
	}

	workspaceID, daemonID, ok, err := VerifyDaemonToken(context.Background(), db, raw)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if !ok {
		t.Fatal("expected token to verify")
	}
	if workspaceID != "ws1" || daemonID != "daemon-1" {
		t.Fatalf("expected ws1/daemon-1, got %s/%s", workspaceID, daemonID)
	}
}

func TestDaemonTokenRejectsRevokedExpiredAndMalformedTokens(t *testing.T) {
	db := newDaemonTokenTestDB(t)

	rawRevoked, hashRevoked, err := NewDaemonToken()
	if err != nil {
		t.Fatalf("new revoked token: %v", err)
	}
	rawExpired, hashExpired, err := NewDaemonToken()
	if err != nil {
		t.Fatalf("new expired token: %v", err)
	}
	expiredAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	revokedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO daemon_tokens (workspace_id, daemon_id, token_hash, expires_at, revoked_at)
		 VALUES ('ws1', 'daemon-revoked', ?, NULL, ?),
		        ('ws1', 'daemon-expired', ?, ?, NULL)`,
		hashRevoked, revokedAt, hashExpired, expiredAt,
	); err != nil {
		t.Fatalf("insert tokens: %v", err)
	}

	for _, raw := range []string{rawRevoked, rawExpired, "not-a-daemon-token", "mwd_missing"} {
		workspaceID, daemonID, ok, err := VerifyDaemonToken(context.Background(), db, raw)
		if err != nil {
			t.Fatalf("verify %q: %v", raw, err)
		}
		if ok || workspaceID != "" || daemonID != "" {
			t.Fatalf("expected %q to be rejected, got ok=%v workspace=%q daemon=%q", raw, ok, workspaceID, daemonID)
		}
	}
}
