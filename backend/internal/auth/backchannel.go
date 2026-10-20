// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file ends sessions when an OpenID Connect provider says a person
// signed out there: OpenID Connect Back-Channel Logout 1.0 (GOV-004). The
// provider POSTs a logout token, a JWT it signs, to Plux. The token is
// accepted only if it verifies as the provider's, names the logout event,
// carries no nonce (it is not an ID token and must not be usable as one),
// names a subject or a session, and has an identifier no earlier token had.

// logoutEvent is the member of the events claim that makes a token a logout
// token (Back-Channel Logout §2.4).
const logoutEvent = "http://schemas.openid.net/event/backchannel-logout"

// Errors of a logout token. The HTTP edge answers both with 400.
var (
	// ErrLogoutToken reports a logout token that is not acceptable.
	ErrLogoutToken = errors.New("auth: the logout token is not valid")
	// ErrLogoutReplayed reports a logout token that was accepted before.
	ErrLogoutReplayed = errors.New("auth: the logout token was already used")
)

// SessionEnder ends the sessions of a person on behalf of federation. The
// sessions code implements it; Options.Sessions defaults to ending them by
// user in the sessions table.
type SessionEnder interface {
	// EndSessions ends the sessions of a user in the caller's transaction
	// and returns how many it ended. sessionIDs name the sessions; none
	// means every session of the user.
	EndSessions(ctx context.Context, tx pgx.Tx, userID string, sessionIDs []string) (int64, error)
}

// userSessionEnder ends sessions in the sessions table by revoking them.
type userSessionEnder struct{}

// EndSessions revokes the named sessions of a user, or all of them.
func (userSessionEnder) EndSessions(ctx context.Context, tx pgx.Tx, userID string, sessionIDs []string) (int64, error) {
	q := dbgen.New(tx)
	user := storage.MustUUID(userID)
	if len(sessionIDs) == 0 {
		n, err := q.RevokeAllUserSessions(ctx, user)
		if err != nil {
			return 0, fmt.Errorf("auth: end a user's sessions: %w", err)
		}
		return n, nil
	}
	uuids := make([]pgtype.UUID, len(sessionIDs))
	for i, id := range sessionIDs {
		uuid, err := storage.UUID(id)
		if err != nil {
			return 0, fmt.Errorf("auth: a session identifier is not valid: %w", err)
		}
		uuids[i] = uuid
	}
	n, err := q.RevokeSessionsByID(ctx, dbgen.RevokeSessionsByIDParams{UserID: user, Ids: uuids})
	if err != nil {
		return 0, fmt.Errorf("auth: end sessions: %w", err)
	}
	return n, nil
}

// checkLogoutToken applies the rules of Back-Channel Logout §2.6 that come
// after the signature, issuer, audience and expiry checks.
func checkLogoutToken(c Claims, now time.Time, skew time.Duration) error {
	if _, ok := c.Events[logoutEvent]; !ok {
		return fmt.Errorf("%w: it carries no logout event", ErrLogoutToken)
	}
	if _, has := c.raw["nonce"]; has {
		return fmt.Errorf("%w: a logout token has no nonce", ErrLogoutToken)
	}
	if c.Subject == "" && c.SessionID == "" {
		return fmt.Errorf("%w: it names neither a subject nor a session", ErrLogoutToken)
	}
	if c.ID == "" {
		return fmt.Errorf("%w: it has no identifier", ErrLogoutToken)
	}
	if c.IssuedAt == 0 || time.Unix(c.IssuedAt, 0).After(now.Add(skew)) {
		return fmt.Errorf("%w: it was not issued yet", ErrLogoutToken)
	}
	return nil
}

// BackChannelLogout handles a provider's logout token and ends the sessions
// it names. It returns how many it ended. An empty providerID is the provider
// in the configuration file.
func (s *Service) BackChannelLogout(ctx context.Context, providerID, token string) (int64, error) {
	target, err := s.oidcTargetFor(ctx, providerID)
	if err != nil {
		return 0, err
	}
	claims, err := target.provider.verifier.verify(ctx, token, target.provider.ClientID)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrLogoutToken, err)
	}
	now := s.now()
	if err := checkLogoutToken(claims, now, target.provider.verifier.maxSkew); err != nil {
		return 0, err
	}
	// A replay check that cannot run is not passed.
	ttl := max(time.Unix(claims.ExpiresAt, 0).Add(target.provider.verifier.maxSkew).Sub(now), time.Minute)
	fresh, err := s.o.Cache.SetNX(ctx, "oidc:logout:"+target.provider.Issuer+":"+claims.ID, []byte{1}, ttl)
	if err != nil {
		return 0, fmt.Errorf("auth: remember a logout token: %w", err)
	}
	if !fresh {
		return 0, ErrLogoutReplayed
	}
	return s.endFederatedSessions(ctx, target, claims)
}

// endFederatedSessions ends the sessions that signed in through the provider
// as the logout token's session or subject. A sign-in whose session is not
// known — it was waiting for a second factor — ends all of the person's
// sessions, which errs towards signing out.
func (s *Service) endFederatedSessions(ctx context.Context, target oidcTarget, c Claims) (int64, error) {
	var ended int64
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListFederatedSignInsFor(ctx, dbgen.ListFederatedSignInsForParams{
			Issuer: target.provider.Issuer, Sid: c.SessionID, Subject: c.Subject,
		})
		if err != nil {
			return fmt.Errorf("auth: find the signed-in sessions: %w", err)
		}
		type ending struct {
			ids []string
			all bool
		}
		byUser := map[string]*ending{}
		for _, r := range rows {
			e := byUser[storage.ID(r.UserID)]
			if e == nil {
				e = &ending{}
				byUser[storage.ID(r.UserID)] = e
			}
			if r.SessionID.Valid {
				e.ids = append(e.ids, storage.ID(r.SessionID))
			} else {
				e.all = true
			}
		}
		for user, e := range byUser {
			ids := e.ids
			if e.all {
				ids = nil
			}
			n, err := s.o.Sessions.EndSessions(ctx, tx, user, ids)
			if err != nil {
				return err //nolint:wrapcheck // the ender wraps its own failures
			}
			ended += n
			if err := s.record(ctx, tx, audit.Entry{
				Actor: audit.Actor{Kind: "identity_provider", Display: "back-channel logout"}, Action: audit.FederatedSignOut,
				TargetKind: "user", TargetID: user, Detail: fmt.Sprintf("issuer=%s sessions=%d", target.provider.Issuer, n),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return ended, err
}
