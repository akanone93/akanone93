// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file holds what a session is once it exists: how a request proves it
// is still valid, how its timeouts are decided, and how administrators see
// and end sessions and tokens (GOV-005).
//
// Revocation takes effect at once on every node because nothing about a
// session is cached: every request reads its row from the database and
// checks revocation, the idle timeout and the absolute lifetime against it
// (see the package documentation). A cache would need invalidating through
// Valkey pub/sub to give the same guarantee, and would add a moment in which
// a revoked session still works; the read is one indexed lookup by the hash
// of the secret, which the request makes anyway.

// touchInterval is the least time between two refreshes of a session's last
// use. It bounds the writes a busy session causes, and it is how often the
// timeouts in force are read again from the organisations' limits.
const touchInterval = time.Minute

// userOrganizationsPage bounds the organisations whose limits shorten one
// person's session; a person belongs to far fewer.
const userOrganizationsPage = 1000

// timeouts are the idle and absolute lifetimes a session is held to.
type timeouts struct {
	idle     time.Duration
	absolute time.Duration
}

// installationTimeouts are the lifetimes with no organisation's limit
// applied: the registry's defaults as the installation set them, and the
// configured session lifetime as the absolute one.
func (s *Service) installationTimeouts() timeouts {
	return timeouts{
		idle:     time.Duration(s.o.Limits.Get(limits.AuthSessionIdleTimeout)) * time.Millisecond,
		absolute: min(s.o.SessionTTL, time.Duration(s.o.Limits.Get(limits.AuthSessionAbsoluteTimeout))*time.Millisecond),
	}
}

// sessionTimeouts returns the lifetimes in force for a person's sessions: the
// shortest an organisation they belong to allows, because a session is not
// bound to one organisation (GOV-005). An organisation can only shorten what
// the installation sets (LIM-002).
func (s *Service) sessionTimeouts(ctx context.Context, user pgtype.UUID) (timeouts, error) {
	var orgs []pgtype.UUID
	err := s.inTx(ctx, storage.Tenant{UserID: storage.ID(user)}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		orgs, err = dbgen.New(tx).ListUserOrganizations(ctx, dbgen.ListUserOrganizationsParams{UserID: user, PageSize: userOrganizationsPage})
		if err != nil {
			return fmt.Errorf("auth: list organisations: %w", err)
		}
		return nil
	})
	if err != nil {
		return timeouts{}, err
	}
	out := s.installationTimeouts()
	for _, org := range orgs {
		t, err := s.organizationTimeouts(ctx, org)
		if err != nil {
			return timeouts{}, err
		}
		out = timeouts{idle: min(out.idle, t.idle), absolute: min(out.absolute, t.absolute)}
	}
	return out, nil
}

// organizationTimeouts applies one organisation's overrides of the session
// limits to the installation's. An override that is not a tightening has no
// effect, as everywhere limits are overridden.
func (s *Service) organizationTimeouts(ctx context.Context, org pgtype.UUID) (timeouts, error) {
	set := s.o.Limits
	err := s.inTx(ctx, storage.Tenant{OrganizationID: storage.ID(org)}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListLimitOverrides(ctx, dbgen.ListLimitOverridesParams{Scope: "organization", ScopeID: org})
		if err != nil {
			return fmt.Errorf("auth: read limit overrides: %w", err)
		}
		for _, row := range rows {
			k := limits.Key(row.LimitKey)
			if k != limits.AuthSessionIdleTimeout && k != limits.AuthSessionAbsoluteTimeout {
				continue
			}
			if tightened, err := set.Tighten(k, limits.ScopeOrganization, row.Value); err == nil {
				set = tightened
			}
		}
		return nil
	})
	if err != nil {
		return timeouts{}, err
	}
	return timeouts{
		idle:     time.Duration(set.Get(limits.AuthSessionIdleTimeout)) * time.Millisecond,
		absolute: min(s.o.SessionTTL, time.Duration(set.Get(limits.AuthSessionAbsoluteTimeout))*time.Millisecond),
	}, nil
}

// verdict decides whether a session is still valid at now, and says why not:
// PLX-6230 revoked, PLX-6232 past its absolute lifetime, PLX-6231 idle for
// longer than allowed (GOV-005). The absolute lifetime is the earlier of the
// stored expiry and the creation time plus the lifetime in force.
func verdict(row dbgen.GetSessionByHashRow, now time.Time, t timeouts) error {
	if row.RevokedAt.Valid {
		return plxerr.New(plxerr.SessionRevoked, "this session was ended; sign in again")
	}
	absolute := earlier(storage.Time(row.ExpiresAt), storage.Time(row.CreatedAt).Add(t.absolute))
	if !now.Before(absolute) {
		return plxerr.New(plxerr.SessionAbsoluteTimeout, "this session reached its maximum age; sign in again")
	}
	if now.Sub(storage.Time(row.LastSeenAt)) > t.idle {
		return plxerr.New(plxerr.SessionIdleTimeout, "this session was idle for too long; sign in again")
	}
	return nil
}

// earlier returns the earlier of two instants.
func earlier(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// storedTimeouts are the lifetimes recorded with a session the last time
// they were read.
func (s *Service) storedTimeouts(row dbgen.GetSessionByHashRow) timeouts {
	t := s.installationTimeouts()
	if row.IdleTimeoutMs != nil {
		t.idle = time.Duration(*row.IdleTimeoutMs) * time.Millisecond
	}
	t.absolute = max(storage.Time(row.ExpiresAt).Sub(storage.Time(row.CreatedAt)), 0)
	return t
}

// AuthenticateSession resolves a session cookie's value (SEC-101) and holds
// the session to its lifetimes: it fails with PLX-6230 when the session was
// revoked, PLX-6232 when it is older than the absolute lifetime and PLX-6231
// when it was idle for longer than the idle timeout (GOV-005). A session that
// does not exist, or whose person is disabled, is not told apart from a
// credential that is simply not valid.
//
// At most once a minute a valid session also records where it was last used
// and re-reads the limits of the person's organisations, so that an
// organisation that shortens a timeout reaches sessions that are already open
// within that minute.
func (s *Service) AuthenticateSession(ctx context.Context, secret string) (Identity, error) {
	if !strings.HasPrefix(secret, PrefixSession+"_") {
		return Identity{}, expired()
	}
	row, err := s.sessionByHash(ctx, secret)
	if err != nil {
		return Identity{}, err
	}
	now := s.now()
	t := s.storedTimeouts(row)
	if err := verdict(row, now, t); err != nil {
		return Identity{}, err
	}
	if now.Sub(storage.Time(row.LastSeenAt)) >= touchInterval {
		if err := s.touchSession(ctx, row, now); err != nil {
			return Identity{}, err
		}
	}
	id := Identity{
		Kind: KindUser, ID: storage.ID(row.UserID), UserID: storage.ID(row.UserID),
		Display: row.DisplayName, SessionID: storage.ID(row.ID), CSRFToken: row.CsrfToken,
		SecondFactor: row.MfaAt.Valid, InstallationAdmin: row.InstallationAdmin,
	}
	if row.StepUpAt.Valid {
		id = id.WithStepUp(now.Sub(storage.Time(row.StepUpAt)))
	}
	return id, nil
}

// sessionByHash reads a session by the hash of its secret.
func (s *Service) sessionByHash(ctx context.Context, secret string) (dbgen.GetSessionByHashRow, error) {
	var row dbgen.GetSessionByHashRow
	err := s.inTx(ctx, storage.Tenant{}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetSessionByHash(ctx, HashSecret(secret))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return expired()
			}
			return fmt.Errorf("auth: read a session: %w", err)
		}
		return nil
	})
	return row, err
}

// touchSession records a request in a session whose last use is a minute old
// or more, after holding the session to the lifetimes the organisations'
// limits give it now.
func (s *Service) touchSession(ctx context.Context, row dbgen.GetSessionByHashRow, now time.Time) error {
	t, err := s.sessionTimeouts(ctx, row.UserID)
	if err != nil {
		return err
	}
	if err := verdict(row, now, t); err != nil {
		return err
	}
	idle := t.idle.Milliseconds()
	req := audit.RequestFrom(ctx)
	return s.inTx(ctx, storage.Tenant{}, func(ctx context.Context, tx pgx.Tx) error {
		err := dbgen.New(tx).TouchSession(ctx, dbgen.TouchSessionParams{
			ID: row.ID, IpPrefix: ipPrefix(req.SourceIP), UserAgent: req.UserAgent, IdleTimeoutMs: &idle,
			At: storage.Timestamp(now), AbsoluteUntil: storage.Timestamp(storage.Time(row.CreatedAt).Add(t.absolute)),
		})
		if err != nil {
			return fmt.Errorf("auth: record a session's use: %w", err)
		}
		return nil
	})
}

// ipPrefix reduces a client address to the /24 (IPv4) or /48 (IPv6) network
// it belongs to, which is all a session list keeps of where a request came
// from. An address that does not parse yields "" rather than being stored.
func ipPrefix(addr string) string {
	a, err := netip.ParseAddr(strings.TrimSpace(addr))
	if err != nil {
		return ""
	}
	a = a.Unmap()
	bits := 48
	if a.Is4() {
		bits = 24
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return ""
	}
	return p.String()
}

// SessionInfo is a session or a token as an administrator sees it. The
// secret is never part of it (GOV-005).
type SessionInfo struct {
	ID     string
	UserID string
	// ClientKind is "studio" for a browser session, "cli" for a token from
	// `plux login` and "token" for any other token.
	ClientKind string
	// Name is a token's name; empty for a session.
	Name       string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	// IPPrefix is the /24 or /48 the last request came from.
	IPPrefix  string
	UserAgent string
	StepUpAt  time.Time
}

// ListSessions lists the active browser sessions of the organisation's
// members and the organisation's active tokens, oldest first. It needs
// sessions.manage, and the person's step-up (GOV-005, SEC-103).
func (s *Service) ListSessions(ctx context.Context, p Principal, after storage.Cursor, limit int32) ([]SessionInfo, error) {
	if err := p.Authorize(SessionsManage); err != nil {
		return nil, err
	}
	var out []SessionInfo
	err := s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListOrganizationSessions(ctx, dbgen.ListOrganizationSessionsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), At: storage.Timestamp(s.now()),
			AfterTime: after.AfterTime(), AfterID: after.AfterID(), PageSize: limit,
		})
		if err != nil {
			return fmt.Errorf("auth: list sessions: %w", err)
		}
		out = make([]SessionInfo, len(rows))
		for i, r := range rows {
			out[i] = SessionInfo{
				ID: storage.ID(r.ID), UserID: storage.ID(r.UserID), ClientKind: r.ClientKind, Name: r.Name,
				CreatedAt: storage.Time(r.CreatedAt), LastSeenAt: storage.Time(r.LastSeenAt), ExpiresAt: storage.Time(r.ExpiresAt),
				IPPrefix: r.IpPrefix, UserAgent: r.UserAgent, StepUpAt: storage.Time(r.StepUpAt),
			}
		}
		return nil
	})
	return out, err
}

// RevokeSession ends a browser session of one of the organisation's members,
// or revokes one of the organisation's tokens, at once on every node
// (GOV-005). Anything else, including the sessions and tokens of another
// organisation, is reported as not found.
func (s *Service) RevokeSession(ctx context.Context, p Principal, id string) error {
	if err := p.Authorize(SessionsManage); err != nil {
		return err
	}
	target, err := parseID(id, "session")
	if err != nil {
		return err
	}
	return s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		session, err := q.GetOrganizationSession(ctx, dbgen.GetOrganizationSessionParams{ID: target, OrganizationID: storage.MustUUID(p.OrganizationID)})
		switch {
		case err == nil:
			return s.endSession(ctx, tx, p, session)
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("auth: read a session: %w", err)
		}
		if _, err := q.GetAccessToken(ctx, target); err != nil {
			return notFound(err, "session")
		}
		if _, err := q.RevokeAccessToken(ctx, target); err != nil {
			return fmt.Errorf("auth: revoke a token: %w", err)
		}
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
			Action: audit.TokenRevoked, TargetKind: "token", TargetID: id,
		})
	})
}

// endSession revokes a session and records who ended it, in the chain of the
// organisation whose administrator did.
func (s *Service) endSession(ctx context.Context, tx pgx.Tx, p Principal, session dbgen.Session) error {
	if _, err := dbgen.New(tx).RevokeSession(ctx, dbgen.RevokeSessionParams{ID: session.ID, At: storage.Timestamp(s.now())}); err != nil {
		return fmt.Errorf("auth: revoke a session: %w", err)
	}
	return s.record(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(),
		Action: audit.SessionRevoked, TargetKind: "session", TargetID: storage.ID(session.ID),
		Detail: "user " + storage.ID(session.UserID),
	})
}
