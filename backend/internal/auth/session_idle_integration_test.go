// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// idleSession signs a person in and returns the session's secret and the
// identity it resolves to.
func idleSession(t *testing.T, f *fixture, email string) (string, auth.Identity) {
	t.Helper()
	ctx := context.Background()
	f.person(t, email)
	session, challenge, err := f.svc.StartPasswordLogin(ctx, email, password)
	if err != nil || challenge.Secret != "" {
		t.Fatalf("StartPasswordLogin = %+v, %+v, %v", session, challenge, err)
	}
	id, err := f.svc.AuthenticateSession(ctx, session.Secret)
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	return session.Secret, id
}

// lastSeen reads when a session was last used.
func lastSeen(t *testing.T, f *fixture, sessionID string) time.Time {
	t.Helper()
	var at time.Time
	if err := f.db.InTx(context.Background(), storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE id = $1`, sessionID).Scan(&at)
	}); err != nil {
		t.Fatalf("read last_seen_at: %v", err)
	}
	return at
}

// Verifies: GOV-005.
// Decision O-1: the idle timeout counts user activity only. A quiet
// authentication — a lock heartbeat or a stream — checks the session but
// does not move its last use, so heartbeats alone cannot keep an idle
// session alive (F19); a person's call does move it.
func TestQuietAuthenticationDoesNotRefreshLastUse_GOV_005(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	quiet, quietID := idleSession(t, f, "quiet@plux.example")
	loud, loudID := idleSession(t, f, "loud@plux.example")
	quietBefore, loudBefore := lastSeen(t, f, quietID.SessionID), lastSeen(t, f, loudID.SessionID)

	f.clock.advance(2 * time.Minute) // past touchInterval
	if _, err := f.svc.AuthenticateSessionQuietly(ctx, quiet); err != nil {
		t.Fatalf("AuthenticateSessionQuietly: %v", err)
	}
	if got := lastSeen(t, f, quietID.SessionID); !got.Equal(quietBefore) {
		t.Errorf("a quiet authentication moved last_seen_at from %v to %v", quietBefore, got)
	}
	if _, err := f.svc.AuthenticateSession(ctx, loud); err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	if got := lastSeen(t, f, loudID.SessionID); !got.After(loudBefore) {
		t.Errorf("AuthenticateSession left last_seen_at at %v", got)
	}

	// Heartbeats every five minutes until the 30-minute idle timeout has
	// passed since the person's last action: the session still ends.
	for elapsed := 2 * time.Minute; elapsed < 30*time.Minute; elapsed += 5 * time.Minute {
		if _, err := f.svc.AuthenticateSessionQuietly(ctx, quiet); err != nil {
			t.Fatalf("heartbeat at %v: %v", elapsed, err)
		}
		f.clock.advance(5 * time.Minute)
	}
	f.clock.advance(time.Minute)
	if _, err := f.svc.AuthenticateSessionQuietly(ctx, quiet); code(err) != plxerr.SessionIdleTimeout {
		t.Errorf("quiet authentication of an idle session: %v, want PLX-6231", err)
	}
	if _, err := f.svc.AuthenticateSession(ctx, quiet); code(err) != plxerr.SessionIdleTimeout {
		t.Errorf("authentication of an idle session: %v, want PLX-6231", err)
	}
}

// Verifies: GOV-005.
// Decision O-1: a background call is still refused once the session was
// revoked.
func TestQuietAuthenticationRefusesARevokedSession_GOV_005(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	secret, id := idleSession(t, f, "revoked@plux.example")
	if err := f.svc.Logout(ctx, id); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := f.svc.AuthenticateSessionQuietly(ctx, secret); code(err) != plxerr.SessionRevoked {
		t.Errorf("quiet authentication of a revoked session: %v, want PLX-6230", err)
	}
	if _, err := f.svc.AuthenticateSession(ctx, secret); code(err) != plxerr.SessionRevoked {
		t.Errorf("authentication of a revoked session: %v, want PLX-6230", err)
	}
}
