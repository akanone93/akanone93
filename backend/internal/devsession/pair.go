// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

const (
	// ticketBytes is the size of a socket ticket.
	ticketBytes = 32
	// ticketLifetime is how long a socket ticket is accepted: long enough
	// for the device to open its socket right after pairing, and no longer
	// (ADR-0087).
	ticketLifetime = 60 * time.Second
)

// Caller is the authenticated device that pairs: its access token named the
// organisation, app and environment, and its DPoP proof the key thumbprint
// (SEC-021).
type Caller struct {
	DeviceID       string
	OrganizationID string
	AppID          string
	EnvironmentID  string
	// Thumbprint is the JWK thumbprint of the device's DPoP key.
	Thumbprint string
}

// PairRequest is what a device sends to pair: the pairing token of the
// link, or the short code a person typed instead.
type PairRequest struct {
	Token string
	// Code is the short code, used when Token is empty.
	Code           string
	Name           string
	Platform       string
	RuntimeVersion string
}

// Paired is the answer to a pairing. Ticket is a credential: it goes to the
// device in this answer and nowhere else.
type Paired struct {
	SessionID       string
	DeviceID        string
	Ticket          string
	TicketExpiresAt time.Time
}

// DeviceBinding is a device's place in a session, which a spent socket
// ticket proves.
type DeviceBinding struct {
	OrganizationID string
	AppID          string
	EnvironmentID  string
	SessionID      string
	// DeviceID identifies the device inside the session.
	DeviceID string
	// RegisteredDeviceID is the device's registered identifier.
	RegisteredDeviceID string
	// Thumbprint is the DPoP key the device paired with.
	Thumbprint string
}

// Pair adds the calling device to the session its pairing token or code
// names and returns a socket ticket (DEV-002, SEC-021). The token must carry
// a valid signature of a development token key, be unexpired (PLX-9201) and
// unused (PLX-9202), and name the device's own app and environment
// (PLX-9203); a code is checked by pairByCode. The session must be live
// (PLX-9200) and have room (PLX-9208). Pairing the same device again
// replaces its earlier pairing.
func (s *Service) Pair(ctx context.Context, d Caller, in PairRequest) (Paired, error) {
	if d.Thumbprint == "" || d.OrganizationID == "" {
		return Paired{}, plxerr.New(plxerr.AuthenticationRequired, "pairing needs a device token bound to a key")
	}
	switch {
	case in.Token != "" && in.Code != "":
		return Paired{}, plxerr.New(plxerr.InvalidFormat, "a pairing names a pairing token or a pairing code, not both")
	case in.Code != "":
		return s.pairByCode(ctx, d, in)
	}
	keys, err := s.o.Tokens.TokenKeys(ctx)
	if err != nil {
		return Paired{}, fmt.Errorf("devsession: list the token keys: %w", err)
	}
	c, err := VerifyPairing(in.Token, keys)
	if err != nil {
		return Paired{}, err
	}
	now := s.now()
	if !now.Before(time.Unix(c.Expiry, 0)) {
		return Paired{}, tokenExpired()
	}
	if c.App != d.AppID || c.Environment != d.EnvironmentID {
		return Paired{}, plxerr.New(plxerr.DevSessionEnvironment, "the pairing token is for another app or environment than the device's")
	}
	var out Paired
	err = s.inTenant(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		hash := digest(in.Token)
		sid, err := q.ConsumeDevPairingToken(ctx, dbgen.ConsumeDevPairingTokenParams{TokenHash: hash, At: storage.Timestamp(now)})
		if errors.Is(err, pgx.ErrNoRows) {
			return whyUnspendable(ctx, q, hash)
		}
		if err != nil {
			return fmt.Errorf("devsession: spend the pairing token: %w", err)
		}
		if storage.ID(sid) != c.Session {
			return invalidToken()
		}
		out, err = s.join(ctx, tx, d, sid, in, now)
		return err
	})
	return out, err
}

// codeWindow is the window in which a device may fail
// devSession.pairingCodeAttempts (5) times to type a pairing code.
//
// The margin: a code is 40 bits, lives at most two minutes
// (devSession.pairingTokenSeconds) and is spent by its first use, and only
// pairs a device of the session's organisation, app and environment. A
// registered device that guesses 5 codes per 10 minutes against the few
// codes open in its environment at one time succeeds with odds of about
// 5 * open / 2^40 per window, under one in 10^11 for a single open code.
const codeWindow = 10 * time.Minute

// pairByCode pairs with the short code of a pairing token. The code names
// an unconsumed, unexpired token of a session of the device's own
// organisation, app and environment; a code of any other is unknown to the
// device (PLX-9201), and a used one is PLX-9202. A device that failed
// devSession.pairingCodeAttempts times within codeWindow is refused
// (RATE_LIMITED) before its code is tried.
func (s *Service) pairByCode(ctx context.Context, d Caller, in PairRequest) (Paired, error) {
	code, ok := NormalizeCode(in.Code)
	if !ok {
		return Paired{}, plxerr.New(plxerr.InvalidFormat, "a pairing code is eight letters and digits, such as 7KQ2-M9XD")
	}
	app, err := parseID(d.AppID, "app")
	if err != nil {
		return Paired{}, err
	}
	env, err := parseID(d.EnvironmentID, "environment")
	if err != nil {
		return Paired{}, err
	}
	now := s.now()
	attempt, err := s.attempt(ctx, d, now)
	if err != nil {
		return Paired{}, err
	}
	var out Paired
	err = s.inTenant(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		sid, err := q.ConsumeDevPairingCode(ctx, dbgen.ConsumeDevPairingCodeParams{At: storage.Timestamp(now), Code: code, AppID: app, EnvironmentID: env})
		if errors.Is(err, pgx.ErrNoRows) {
			return whyCodeUnspendable(ctx, q, dbgen.GetDevPairingCodeParams{Code: code, AppID: app, EnvironmentID: env})
		}
		if err != nil {
			return fmt.Errorf("devsession: spend the pairing code: %w", err)
		}
		if err := q.DeleteDevPairingAttempt(ctx, attempt); err != nil {
			return fmt.Errorf("devsession: forget the attempt: %w", err)
		}
		out, err = s.join(ctx, tx, d, sid, in, now)
		return err
	})
	return out, err
}

// attempt records a pairing-code attempt of the device before its code is
// tried, in a transaction of its own so that the attempt stays counted when
// the code fails; pairByCode forgets it when the code pairs. It refuses the
// device once devSession.pairingCodeAttempts attempts lie within
// codeWindow. The attempts of one device are serialised, so concurrent
// guesses cannot pass the count together.
func (s *Service) attempt(ctx context.Context, d Caller, now time.Time) (pgtype.UUID, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("devsession: %w", err)
	}
	attempt := storage.MustUUID(id)
	err = s.inTenant(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		org := storage.MustUUID(d.OrganizationID)
		since := storage.Timestamp(now.Add(-codeWindow))
		if err := q.LockDevPairingAttempts(ctx, dbgen.LockDevPairingAttemptsParams{OrganizationID: org, DeviceID: d.DeviceID}); err != nil {
			return fmt.Errorf("devsession: lock the attempts: %w", err)
		}
		err := q.DropStaleDevPairingAttempts(ctx, dbgen.DropStaleDevPairingAttemptsParams{OrganizationID: org, DeviceID: d.DeviceID, Since: since})
		if err != nil {
			return fmt.Errorf("devsession: drop the old attempts: %w", err)
		}
		n, err := q.CountDevPairingAttempts(ctx, dbgen.CountDevPairingAttemptsParams{OrganizationID: org, DeviceID: d.DeviceID, Since: since})
		if err != nil {
			return fmt.Errorf("devsession: count the attempts: %w", err)
		}
		set, err := s.o.Limits(ctx, tx, d.OrganizationID, d.AppID)
		if err != nil {
			return fmt.Errorf("devsession: read the limits: %w", err)
		}
		if most := set.Get(limits.DevSessionPairingCodeAttempts); n >= most {
			return plxerr.New(plxerr.RateLimited, "too many pairing codes failed on this device (devSession.pairingCodeAttempts); wait %d minutes",
				int(codeWindow.Minutes()))
		}
		err = q.InsertDevPairingAttempt(ctx, dbgen.InsertDevPairingAttemptParams{ID: attempt, OrganizationID: org, DeviceID: d.DeviceID, At: storage.Timestamp(now)})
		if err != nil {
			return fmt.Errorf("devsession: record the attempt: %w", err)
		}
		return nil
	})
	return attempt, err
}

// join records the device in the session of a spent token, inside the
// transaction that spent it: the session must be live, of the device's app
// and environment, and have room.
func (s *Service) join(ctx context.Context, tx pgx.Tx, d Caller, sid pgtype.UUID, in PairRequest, now time.Time) (Paired, error) {
	q := dbgen.New(tx)
	row, err := q.GetDevSessionForUpdate(ctx, sid)
	if err != nil {
		return Paired{}, fmt.Errorf("devsession: find the session: %w", err)
	}
	session := sessionOf(row)
	set, err := s.o.Limits(ctx, tx, session.OrganizationID, session.AppID)
	if err != nil {
		return Paired{}, fmt.Errorf("devsession: read the limits: %w", err)
	}
	if !session.Live(now, minutes(set, limits.DevSessionIdleMinutes)) {
		return Paired{}, expired()
	}
	if session.AppID != d.AppID || session.EnvironmentID != d.EnvironmentID {
		return Paired{}, plxerr.New(plxerr.DevSessionEnvironment, "the dev session is for another app or environment than the device's")
	}
	at := storage.Timestamp(now)
	if err := q.RevokeDevSessionDevice(ctx, dbgen.RevokeDevSessionDeviceParams{SessionID: sid, DeviceID: d.DeviceID, RevokedAt: at}); err != nil {
		return Paired{}, fmt.Errorf("devsession: replace the earlier pairing: %w", err)
	}
	count, err := q.CountDevSessionDevices(ctx, sid)
	if err != nil {
		return Paired{}, fmt.Errorf("devsession: count the devices: %w", err)
	}
	if most := set.Get(limits.DevSessionDevicesPerSession); count >= most {
		return Paired{}, plxerr.New(plxerr.DevSessionDeviceLimit, "the dev session already has %d devices (devSession.devicesPerSession)", most)
	}
	ticket, err := s.ticket()
	if err != nil {
		return Paired{}, err
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Paired{}, fmt.Errorf("devsession: %w", err)
	}
	expires := now.Add(ticketLifetime)
	_, err = q.InsertDevSessionDevice(ctx, dbgen.InsertDevSessionDeviceParams{
		ID: storage.MustUUID(id), OrganizationID: row.OrganizationID, SessionID: sid, DeviceID: d.DeviceID, DpopThumbprint: d.Thumbprint,
		Name: in.Name, Platform: in.Platform, RuntimeVersion: in.RuntimeVersion, PairedAt: at,
		TicketHash: digest(ticket), TicketExpiresAt: storage.Timestamp(expires),
	})
	if err != nil {
		return Paired{}, fmt.Errorf("devsession: store the device: %w", err)
	}
	if err := q.TouchDevSession(ctx, dbgen.TouchDevSessionParams{ID: sid, At: at}); err != nil {
		return Paired{}, fmt.Errorf("devsession: record the activity: %w", err)
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: session.OrganizationID, Actor: audit.Actor{Kind: "device", ID: d.DeviceID, Display: d.DeviceID},
		Action: audit.DevSessionPaired, TargetKind: "dev_session", TargetID: session.ID, Detail: "device " + id,
	})
	if err != nil {
		return Paired{}, fmt.Errorf("devsession: %w", err)
	}
	return Paired{SessionID: session.ID, DeviceID: id, Ticket: ticket, TicketExpiresAt: expires}, nil
}

// Ticket is a socket ticket. Ticket is a credential: it goes to the device
// in the answer that issued it and nowhere else.
type Ticket struct {
	Ticket    string
	ExpiresAt time.Time
}

// RenewSocketTicket gives a paired device a new single-use socket ticket to
// reconnect with (DEV-021). The caller must be the device paired as
// deviceID in the session, with the key it paired with, and the session
// must be live (PLX-9200); anything else is PLX-9204.
func (s *Service) RenewSocketTicket(ctx context.Context, d Caller, sessionID, deviceID string) (Ticket, error) {
	if d.Thumbprint == "" || d.OrganizationID == "" {
		return Ticket{}, plxerr.New(plxerr.AuthenticationRequired, "renewing a ticket needs a device token bound to a key")
	}
	sid, err := parseID(sessionID, "session")
	if err != nil {
		return Ticket{}, err
	}
	did, err := parseID(deviceID, "device")
	if err != nil {
		return Ticket{}, err
	}
	var out Ticket
	err = s.inTenant(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		out, err = s.renew(ctx, tx, d, sid, did)
		return err
	})
	return out, err
}

// renew checks the session and the device inside the transaction and
// stores the new ticket.
func (s *Service) renew(ctx context.Context, tx pgx.Tx, d Caller, sid, did pgtype.UUID) (Ticket, error) {
	q := dbgen.New(tx)
	row, err := q.GetDevSessionForUpdate(ctx, sid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, notPaired()
	}
	if err != nil {
		return Ticket{}, fmt.Errorf("devsession: find the session: %w", err)
	}
	session := sessionOf(row)
	if session.AppID != d.AppID || session.EnvironmentID != d.EnvironmentID {
		return Ticket{}, notPaired()
	}
	set, err := s.o.Limits(ctx, tx, session.OrganizationID, session.AppID)
	if err != nil {
		return Ticket{}, fmt.Errorf("devsession: read the limits: %w", err)
	}
	now := s.now()
	if !session.Live(now, minutes(set, limits.DevSessionIdleMinutes)) {
		return Ticket{}, expired()
	}
	ticket, err := s.ticket()
	if err != nil {
		return Ticket{}, err
	}
	expires := now.Add(ticketLifetime)
	_, err = q.RenewDevSocketTicket(ctx, dbgen.RenewDevSocketTicketParams{
		TicketHash: digest(ticket), TicketExpiresAt: storage.Timestamp(expires),
		ID: did, SessionID: sid, DeviceID: d.DeviceID, DpopThumbprint: d.Thumbprint,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, notPaired()
	}
	if err != nil {
		return Ticket{}, fmt.Errorf("devsession: store the ticket: %w", err)
	}
	if err := q.TouchDevSession(ctx, dbgen.TouchDevSessionParams{ID: sid, At: storage.Timestamp(now)}); err != nil {
		return Ticket{}, fmt.Errorf("devsession: record the activity: %w", err)
	}
	return Ticket{Ticket: ticket, ExpiresAt: expires}, nil
}

// notPaired is the refusal of a device that is not paired with a session.
func notPaired() error {
	return plxerr.New(plxerr.DeviceNotPaired, "the device is not paired with this dev session")
}

// whyUnspendable tells a used token from an expired or unknown one.
func whyUnspendable(ctx context.Context, q *dbgen.Queries, hash []byte) error {
	row, err := q.GetDevPairingToken(ctx, hash)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return tokenExpired()
	case err != nil:
		return fmt.Errorf("devsession: find the pairing token: %w", err)
	case row.ConsumedAt.Valid:
		return plxerr.New(plxerr.PairingTokenUsed, "the pairing token was already used; ask for a new one")
	default:
		return tokenExpired()
	}
}

// whyCodeUnspendable tells a used code from an expired or unknown one, among
// the codes of the device's app and environment.
func whyCodeUnspendable(ctx context.Context, q *dbgen.Queries, arg dbgen.GetDevPairingCodeParams) error {
	consumed, err := q.GetDevPairingCode(ctx, arg)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return codeExpired()
	case err != nil:
		return fmt.Errorf("devsession: find the pairing code: %w", err)
	case consumed.Valid:
		return plxerr.New(plxerr.PairingTokenUsed, "the pairing code was already used; ask for a new one")
	default:
		return codeExpired()
	}
}

// codeExpired is the refusal of a pairing code that is past its expiry or
// unknown to the device.
func codeExpired() error {
	return plxerr.New(plxerr.PairingTokenExpired, "the pairing code has expired or is not known; ask for a new one")
}

// tokenExpired is the refusal of a pairing token past its expiry.
func tokenExpired() error {
	return plxerr.New(plxerr.PairingTokenExpired, "the pairing token has expired; ask for a new one")
}

// ticket draws a socket ticket.
func (s *Service) ticket() (string, error) {
	b := make([]byte, ticketBytes)
	if _, err := io.ReadFull(s.random, b); err != nil {
		return "", fmt.Errorf("devsession: draw a ticket: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ticketInvalid is the refusal of a socket ticket.
func ticketInvalid() error {
	return plxerr.New(plxerr.SocketTicketInvalid, "the socket ticket is not valid")
}

// ConsumeSocketTicket spends a socket ticket presented at the WebSocket
// upgrade with the DPoP key of thumbprint dpopThumbprint, and returns the
// device's place in its session. A ticket is accepted once, before its
// expiry, with the key the device paired with, while the device is paired;
// anything else is PLX-9205. A session that is no longer live is PLX-9200.
func (s *Service) ConsumeSocketTicket(ctx context.Context, ticket, dpopThumbprint string) (DeviceBinding, error) {
	if ticket == "" || dpopThumbprint == "" {
		return DeviceBinding{}, ticketInvalid()
	}
	hash := digest(ticket)
	now := s.now()
	var out DeviceBinding
	err := s.inTenant(ctx, storage.Tenant{Scope: storage.ScopeDevSocket}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		org, err := q.FindDevSocketTicket(ctx, hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return ticketInvalid()
		}
		if err != nil {
			return fmt.Errorf("devsession: find the ticket: %w", err)
		}
		if err := storage.Rescope(ctx, tx, storage.Tenant{OrganizationID: storage.ID(org)}); err != nil {
			return fmt.Errorf("devsession: %w", err)
		}
		dev, err := q.ConsumeDevSocketTicket(ctx, dbgen.ConsumeDevSocketTicketParams{TicketHash: hash, DpopThumbprint: dpopThumbprint, At: storage.Timestamp(now)})
		if errors.Is(err, pgx.ErrNoRows) {
			return ticketInvalid()
		}
		if err != nil {
			return fmt.Errorf("devsession: spend the ticket: %w", err)
		}
		session, err := s.liveSession(ctx, tx, storage.ID(dev.SessionID), now)
		if err != nil {
			return err
		}
		out = DeviceBinding{
			OrganizationID: session.OrganizationID, AppID: session.AppID, EnvironmentID: session.EnvironmentID, SessionID: session.ID,
			DeviceID: storage.ID(dev.ID), RegisteredDeviceID: dev.DeviceID, Thumbprint: dev.DpopThumbprint,
		}
		return nil
	})
	return out, err
}

// Touch records traffic on a live session, which keeps it from going idle;
// a session that is no longer live is PLX-9200 and stays ended.
func (s *Service) Touch(ctx context.Context, organizationID, sessionID string) error {
	if _, err := parseID(sessionID, "session"); err != nil {
		return err
	}
	if _, err := storage.UUID(organizationID); err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the organisation identifier is not valid")
	}
	now := s.now()
	return s.inTenant(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := s.liveSession(ctx, tx, sessionID, now)
		return err
	})
}

// liveSession locks a session, refuses it when it is no longer live and
// records the traffic that asked.
func (s *Service) liveSession(ctx context.Context, tx pgx.Tx, sessionID string, now time.Time) (Session, error) {
	q := dbgen.New(tx)
	id := storage.MustUUID(sessionID)
	row, err := q.GetDevSessionForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, expired()
	}
	if err != nil {
		return Session{}, fmt.Errorf("devsession: find the session: %w", err)
	}
	session := sessionOf(row)
	set, err := s.o.Limits(ctx, tx, session.OrganizationID, session.AppID)
	if err != nil {
		return Session{}, fmt.Errorf("devsession: read the limits: %w", err)
	}
	if !session.Live(now, minutes(set, limits.DevSessionIdleMinutes)) {
		return Session{}, expired()
	}
	if err := q.TouchDevSession(ctx, dbgen.TouchDevSessionParams{ID: id, At: storage.Timestamp(now)}); err != nil {
		return Session{}, fmt.Errorf("devsession: record the activity: %w", err)
	}
	return session, nil
}
