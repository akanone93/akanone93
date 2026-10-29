// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// IDs generates identifiers for the rows this package writes.
type IDs interface{ New() (string, error) }

// Options are what the service needs.
type Options struct {
	DB      *storage.DB
	Audit   *audit.Log
	Crypter signing.Crypter
	IDs     IDs
	// HTTP is the SSRF-safe client every outbound call uses (SEC-105); nil
	// builds one with Policy.
	HTTP *http.Client
	// Policy decides which addresses the HTTP client and the SMTP dialer
	// may reach.
	Policy httpx.Policy
	// SMTPDial replaces the default SSRF-checked dialer; for tests.
	SMTPDial func(ctx context.Context, network, address string) (net.Conn, error)
	// SMTPTLS adds trust roots or similar to the STARTTLS handshake; nil
	// uses the system's.
	SMTPTLS *tls.Config
	// TelegramBase is the Bot API's base URL; empty is Telegram's own.
	TelegramBase string
	// StudioURL turns a path in an event's link into a URL for chat and
	// mail.
	StudioURL string
	// AllowInsecure admits http:// destinations and SMTP without TLS. It is
	// for development only and never a default (security-practices.md §4).
	AllowInsecure bool
	// Limits returns the limits of an organisation (LIM-001); nil uses the
	// defaults.
	Limits func(ctx context.Context, organizationID string) (limits.Set, error)
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// Jitter returns a number in [0, 1) for the backoff; nil uses a random
	// source.
	Jitter func() float64
	// Log receives warnings; it never receives a secret.
	Log *slog.Logger
	// Events announces a person's unread count to the Studio stream
	// (STU-075); nil announces nothing.
	Events InboxEvents
}

// InboxEvents receives the unread counts the Studio events stream carries.
// It never fails its caller.
type InboxEvents interface {
	InboxChanged(ctx context.Context, org, user string, unread int64)
}

// Service stores channels, subscriptions, the inbox and webhooks, and sends
// what the events queue.
type Service struct{ o Options }

// New returns a service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.Crypter == nil || o.IDs == nil {
		return nil, errors.New("notify: a service needs a database, an audit log, envelope encryption and an ID generator")
	}
	if o.HTTP == nil {
		client, err := httpx.NewClient(httpx.Options{Destination: httpx.KindNotify, Policy: o.Policy, Timeout: 15 * time.Second})
		if err != nil {
			return nil, fmt.Errorf("notify: %w", err)
		}
		o.HTTP = client
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Jitter == nil {
		o.Jitter = randomJitter
	}
	if o.Limits == nil {
		o.Limits = func(context.Context, string) (limits.Set, error) { return limits.Defaults(), nil }
	}
	return &Service{o: o}, nil
}

func (s *Service) now() time.Time { return s.o.Now().UTC() }

func (s *Service) warn(ctx context.Context, msg string, args ...any) {
	if s.o.Log != nil {
		s.o.Log.WarnContext(ctx, "notify: "+msg, args...)
	}
}

func (s *Service) newID() (string, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return "", fmt.Errorf("notify: %w", err)
	}
	return id, nil
}

// inOrg runs f in a transaction bound to the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.inTenant(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f)
}

func (s *Service) inTenant(ctx context.Context, t storage.Tenant, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, t, f) //nolint:wrapcheck // InTx wraps its own failures
}

// authorize is Principal.Authorize with the refusal marked as this
// package's.
func authorize(p auth.Principal, want auth.Permission) error {
	if err := p.Authorize(want); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// record appends an audit entry in the transaction of the change.
func (s *Service) record(ctx context.Context, tx pgx.Tx, org string, actor audit.Actor, action audit.Action, kind, target, detail string) error {
	_, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: org, Actor: actor, Action: action, TargetKind: kind, TargetID: target, Detail: detail,
	})
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// failure turns a database error into the refusal a caller understands;
// nil stays nil.
func failure(err error, what string) error {
	var pg *pgconn.PgError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	case errors.As(err, &pg) && pg.Code == "23505":
		return plxerr.New(plxerr.ResourceExists, "a %s with this name already exists", what)
	}
	return fmt.Errorf("notify: %s: %w", what, err)
}

func parseUUID(s, what string) (pgtype.UUID, error) {
	u, err := storage.UUID(s)
	if err != nil {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the %s identifier is not valid", what)
	}
	return u, nil
}
