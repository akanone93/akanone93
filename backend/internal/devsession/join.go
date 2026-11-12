// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// A signed-in phone joins its person's Studio session of an app (DEV-002,
// ADR-0104). The phone asks; Studio shows that person a two-digit number and
// the phone offers it among three; the right pick gives the phone a pairing
// token, the one GetPairing issues, so that its runtime pairs as after a
// scan. A phone that joined a session joins it again without a number while
// the session is live. A wrong pick keeps the phone out of the session until
// the person lets it try again, so that a phone in other hands gets one
// guess; a refusal in Studio ends the request alone.

// Join request states, as stored.
const (
	joinWaiting = "waiting"
	joinJoined  = "joined"
	joinFailed  = "failed"
	joinDenied  = "denied"
	// joinRetry is a failed request whose phone the person let try again.
	joinRetry = "retry"
)

// joinAttemptWindow is the window of devSession.phoneAttempts.
const joinAttemptWindow = 10 * time.Minute

// JoinRequest is a phone's request to join a session: the numbers to offer,
// or, for a phone that joined the session before, its pairing at once.
type JoinRequest struct {
	ID          string
	SessionID   string
	Environment string
	Options     []int
	ExpiresAt   time.Time
	// Pairing is set only when no number is needed.
	Pairing *Pairing
}

// Joined is the pairing a phone receives for the right number.
type Joined struct {
	Pairing     Pairing
	SessionID   string
	Environment string
}

// JoinStatus is a join request as its session's person sees it; Number is
// set only while it waits.
type JoinStatus struct {
	// RequestID is set in a listing.
	RequestID string
	SessionID string
	State     pluxv1.DevJoinState
	Number    int
	PhoneName string
	ExpiresAt time.Time
}

// phoneOnly refuses a principal that is not a signed-in phone.
func phoneOnly(p auth.Principal) error {
	if p.Kind != auth.KindPhone || p.UserID == "" {
		return plxerr.New(plxerr.PermissionDenied, "only a phone signed in to the Plux Dev app joins this way")
	}
	return nil
}

// liveStudioSession returns the newest live Studio session the person of p
// created for an app, in an environment or, with a null one, in any.
func (s *Service) liveStudioSession(ctx context.Context, tx pgx.Tx, p auth.Principal, app pgtype.UUID, env pgtype.UUID, idle time.Duration) (Session, bool, error) {
	now := s.now()
	row, err := dbgen.New(tx).FindStudioSession(ctx, dbgen.FindStudioSessionParams{
		AppID: app, Creator: creatorOf(p), EnvironmentID: env, Now: storage.Timestamp(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("devsession: find the Studio session: %w", err)
	}
	session := sessionOf(row)
	return session, session.Live(now, idle), nil
}

// LiveApps reports which of the apps the person of p has open in Studio, in
// a session a phone of theirs may join.
func (s *Service) LiveApps(ctx context.Context, p auth.Principal, appIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		for _, appID := range appIDs {
			app, err := parseID(appID, "app")
			if err != nil {
				return err
			}
			set, err := s.o.Limits(ctx, tx, p.OrganizationID, appID)
			if err != nil {
				return fmt.Errorf("devsession: read the limits: %w", err)
			}
			_, live, err := s.liveStudioSession(ctx, tx, p, app, pgtype.UUID{}, minutes(set, limits.DevSessionIdleMinutes))
			if err != nil {
				return err
			}
			out[appID] = live
		}
		return nil
	})
	return out, err
}

// RequestJoin is a signed-in phone's request to join its person's live
// Studio session of an app. phoneID is the phone's token; phoneName is the
// name it signed in with.
func (s *Service) RequestJoin(ctx context.Context, p auth.Principal, phoneID, phoneName, appID string) (JoinRequest, error) {
	if err := phoneOnly(p); err != nil {
		return JoinRequest{}, err
	}
	if err := p.AuthorizeSomeEnvironment(auth.DevSessionCreate, appID); err != nil {
		return JoinRequest{}, fmt.Errorf("devsession: %w", err)
	}
	app, err := parseID(appID, "app")
	if err != nil {
		return JoinRequest{}, err
	}
	number, options, err := auth.DrawNumbers(s.random)
	if err != nil {
		return JoinRequest{}, fmt.Errorf("devsession: %w", err)
	}
	var out JoinRequest
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		t, err := s.joinTarget(ctx, tx, p, app, storage.MustUUID(phoneID))
		if err != nil {
			return err
		}
		out, err = s.answerJoin(ctx, tx, p, t, phoneName, number, options)
		return err
	})
	if err != nil {
		return JoinRequest{}, err
	}
	if out.ID != "" {
		s.announceJoin(out.SessionID, out.ID, pluxv1.DevJoinState_DEV_JOIN_STATE_WAITING, phoneName, out.ExpiresAt)
	}
	return out, nil
}

// joinTarget is the session a phone asks to join, with what answering
// needs: its limits, the phone's token and the instant of the request.
type joinTarget struct {
	session Session
	app     pgtype.UUID
	set     limits.Set
	token   pgtype.UUID
	now     time.Time
}

// joinTarget finds the session a phone may join and checks that the phone
// may ask: within devSession.phoneAttempts, the person's own live Studio
// session, and no wrong pick left standing there.
func (s *Service) joinTarget(ctx context.Context, tx pgx.Tx, p auth.Principal, app, token pgtype.UUID) (joinTarget, error) {
	q := dbgen.New(tx)
	set, err := s.o.Limits(ctx, tx, p.OrganizationID, storage.ID(app))
	if err != nil {
		return joinTarget{}, fmt.Errorf("devsession: read the limits: %w", err)
	}
	now := s.now()
	n, err := q.CountJoinRequests(ctx, dbgen.CountJoinRequestsParams{TokenID: token, Since: storage.Timestamp(now.Add(-joinAttemptWindow))})
	if err != nil {
		return joinTarget{}, fmt.Errorf("devsession: count the phone's requests: %w", err)
	}
	if n >= set.Get(limits.DevSessionPhoneAttempts) {
		return joinTarget{}, plxerr.New(plxerr.RateLimited, "this phone asked to join too often; wait ten minutes")
	}
	session, live, err := s.liveStudioSession(ctx, tx, p, app, pgtype.UUID{}, minutes(set, limits.DevSessionIdleMinutes))
	if err != nil {
		return joinTarget{}, err
	}
	if !live {
		return joinTarget{}, plxerr.New(plxerr.NoLiveStudioSession, "open the app in Studio, then choose it on the phone again")
	}
	if err := p.AuthorizeEnvironment(auth.DevSessionCreate, session.AppID, session.EnvironmentKey); err != nil {
		return joinTarget{}, fmt.Errorf("devsession: %w", err)
	}
	blocked, err := q.JoinBlocked(ctx, dbgen.JoinBlockedParams{SessionID: storage.MustUUID(session.ID), TokenID: token})
	if err != nil {
		return joinTarget{}, fmt.Errorf("devsession: read the phone's requests: %w", err)
	}
	if blocked {
		return joinTarget{}, plxerr.New(plxerr.NumberMismatch, "a wrong number was picked on this phone; let it try again in Studio")
	}
	return joinTarget{session: session, app: app, set: set, token: token, now: now}, nil
}

// answerJoin answers a phone that may ask: with a pairing at once when it
// joined the session before, else with a new request and its numbers.
func (s *Service) answerJoin(ctx context.Context, tx pgx.Tx, p auth.Principal, t joinTarget, phoneName string, number int, options []int) (JoinRequest, error) {
	q := dbgen.New(tx)
	sessionID := storage.MustUUID(t.session.ID)
	out := JoinRequest{SessionID: t.session.ID, Environment: t.session.EnvironmentKey}
	joined, err := q.HasJoinedDevSession(ctx, dbgen.HasJoinedDevSessionParams{SessionID: sessionID, TokenID: t.token})
	if err != nil {
		return JoinRequest{}, fmt.Errorf("devsession: read the phone's requests: %w", err)
	}
	if joined {
		pairing, err := s.issue(ctx, tx, t.session, t.set, t.now)
		if err != nil {
			return JoinRequest{}, err
		}
		out.Pairing = &pairing
		return out, nil
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return JoinRequest{}, fmt.Errorf("devsession: %w", err)
	}
	expires := t.now.Add(time.Duration(t.set.Get(limits.DevSessionJoinRequestSeconds)) * time.Second)
	if _, err := q.InsertJoinRequest(ctx, dbgen.InsertJoinRequestParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), AppID: t.app, SessionID: sessionID,
		TokenID: t.token, Number: int16(number), PhoneName: phoneName, //nolint:gosec // two digits
		ExpiresAt: storage.Timestamp(expires), CreatedAt: storage.Timestamp(t.now),
	}); err != nil {
		return JoinRequest{}, fmt.Errorf("devsession: store the join request: %w", err)
	}
	out.ID, out.Options, out.ExpiresAt = id, options, expires
	return out, nil
}

// CompleteJoin is the phone's pick. The right number gives the phone a
// pairing token for the session; a wrong one ends the request, keeps the
// phone out of the session until its person lets it try again, and is
// recorded before the refusal returns.
func (s *Service) CompleteJoin(ctx context.Context, p auth.Principal, phoneID, requestID string, number int) (Joined, error) {
	if err := phoneOnly(p); err != nil {
		return Joined{}, err
	}
	id, err := parseID(requestID, "join request")
	if err != nil {
		return Joined{}, err
	}
	var (
		out     Joined
		row     dbgen.DevJoinRequest
		refusal error
	)
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		now := s.now()
		var err error
		row, err = openJoinRequest(ctx, dbgen.New(tx), id, phoneID, now)
		if err != nil {
			return err
		}
		if number != int(row.Number) {
			if _, err := dbgen.New(tx).SetJoinRequestState(ctx, dbgen.SetJoinRequestStateParams{ID: row.ID, State: joinFailed}); err != nil {
				return fmt.Errorf("devsession: end the join request: %w", err)
			}
			refusal = plxerr.New(plxerr.NumberMismatch, "that is not the number Studio showed; let this phone try again in Studio")
			return nil
		}
		out, err = s.grantJoin(ctx, tx, p, row, now)
		return err
	})
	if err != nil {
		return Joined{}, err
	}
	state := pluxv1.DevJoinState_DEV_JOIN_STATE_JOINED
	if refusal != nil {
		state = pluxv1.DevJoinState_DEV_JOIN_STATE_FAILED
	}
	s.announceJoin(storage.ID(row.SessionID), requestID, state, row.PhoneName, row.ExpiresAt.Time)
	if refusal != nil {
		return Joined{}, refusal
	}
	return out, nil
}

// openJoinRequest reads, for update, a waiting request of the phone that
// has not expired.
func openJoinRequest(ctx context.Context, q *dbgen.Queries, id pgtype.UUID, phoneID string, now time.Time) (dbgen.DevJoinRequest, error) {
	row, err := q.GetJoinRequestForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.DevJoinRequest{}, requestClosed()
	}
	if err != nil {
		return dbgen.DevJoinRequest{}, fmt.Errorf("devsession: read the join request: %w", err)
	}
	if storage.ID(row.TokenID) != phoneID || row.State != joinWaiting || !now.Before(row.ExpiresAt.Time) {
		return dbgen.DevJoinRequest{}, requestClosed()
	}
	return row, nil
}

// grantJoin ends a request whose number was right and issues the phone's
// pairing token, while the session is still its person's and live.
func (s *Service) grantJoin(ctx context.Context, tx pgx.Tx, p auth.Principal, row dbgen.DevJoinRequest, now time.Time) (Joined, error) {
	session, err := s.authorized(ctx, tx, p, row.SessionID, auth.DevSessionCreate, false)
	if err != nil {
		return Joined{}, err
	}
	set, err := s.o.Limits(ctx, tx, p.OrganizationID, session.AppID)
	if err != nil {
		return Joined{}, fmt.Errorf("devsession: read the limits: %w", err)
	}
	if session.Creator != creatorOf(p) || !session.Live(now, minutes(set, limits.DevSessionIdleMinutes)) {
		return Joined{}, requestClosed()
	}
	if _, err := dbgen.New(tx).SetJoinRequestState(ctx, dbgen.SetJoinRequestStateParams{ID: row.ID, State: joinJoined}); err != nil {
		return Joined{}, fmt.Errorf("devsession: end the join request: %w", err)
	}
	pairing, err := s.issue(ctx, tx, session, set, now)
	if err != nil {
		return Joined{}, err
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.DevSessionJoined, TargetKind: "dev_session", TargetID: session.ID,
		Detail: "phone " + row.PhoneName,
	})
	if err != nil {
		return Joined{}, fmt.Errorf("devsession: %w", err)
	}
	return Joined{Pairing: pairing, SessionID: session.ID, Environment: session.EnvironmentKey}, nil
}

// requestClosed is the refusal of a join request that is unknown, another
// phone's, expired, already answered, or whose session ended; they are not
// told apart.
func requestClosed() error {
	return plxerr.New(plxerr.PhoneRequestClosed, "this request to join is no longer open; choose the app on the phone again")
}

// ownJoinRequest reads a join request of a session the person of p created,
// for update when lock is true.
func (s *Service) ownJoinRequest(ctx context.Context, tx pgx.Tx, p auth.Principal, requestID string, lock bool) (dbgen.DevJoinRequest, Session, error) {
	id, err := parseID(requestID, "join request")
	if err != nil {
		return dbgen.DevJoinRequest{}, Session{}, err
	}
	q := dbgen.New(tx)
	read := q.GetJoinRequest
	if lock {
		read = q.GetJoinRequestForUpdate
	}
	row, err := read(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.DevJoinRequest{}, Session{}, plxerr.New(plxerr.ResourceNotFound, "no such join request")
	}
	if err != nil {
		return dbgen.DevJoinRequest{}, Session{}, fmt.Errorf("devsession: read the join request: %w", err)
	}
	session, err := s.authorized(ctx, tx, p, row.SessionID, auth.DevSessionCreate, false)
	if err != nil {
		return dbgen.DevJoinRequest{}, Session{}, err
	}
	if session.Creator != creatorOf(p) {
		return dbgen.DevJoinRequest{}, Session{}, plxerr.New(plxerr.ResourceNotFound, "no such join request")
	}
	return row, session, nil
}

// GetJoinRequest reports a phone's request to join a session the caller
// created, with the number to show while it waits.
func (s *Service) GetJoinRequest(ctx context.Context, p auth.Principal, requestID string) (JoinStatus, error) {
	if err := authorizeAnywhere(p, auth.DevSessionCreate); err != nil {
		return JoinStatus{}, err
	}
	var out JoinStatus
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, session, err := s.ownJoinRequest(ctx, tx, p, requestID, false)
		if err != nil {
			return err
		}
		out = JoinStatus{SessionID: session.ID, PhoneName: row.PhoneName, ExpiresAt: row.ExpiresAt.Time, State: joinState(row.State)}
		if row.State == joinWaiting {
			if s.now().Before(row.ExpiresAt.Time) {
				out.Number = int(row.Number)
			} else {
				out.State = pluxv1.DevJoinState_DEV_JOIN_STATE_EXPIRED
			}
		}
		return nil
	})
	return out, err
}

// ListJoinRequests lists the open requests to join the caller's live
// Studio sessions of the app of sessionID, which the caller has open in
// Studio. Asking it records traffic on that session, so it stays live while
// Studio shows it and is the newest the caller's phones join.
func (s *Service) ListJoinRequests(ctx context.Context, p auth.Principal, sessionID string) ([]JoinStatus, error) {
	if err := authorizeAnywhere(p, auth.DevSessionCreate); err != nil {
		return nil, err
	}
	id, err := parseID(sessionID, "session")
	if err != nil {
		return nil, err
	}
	var out []JoinStatus
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		session, err := s.authorized(ctx, tx, p, id, auth.DevSessionCreate, false)
		if err != nil {
			return err
		}
		if session.Kind != KindStudio || session.Creator != creatorOf(p) {
			return plxerr.New(plxerr.ResourceNotFound, "no such session")
		}
		now := s.now()
		q := dbgen.New(tx)
		if err := q.TouchDevSession(ctx, dbgen.TouchDevSessionParams{ID: id, At: storage.Timestamp(now)}); err != nil {
			return fmt.Errorf("devsession: touch the session: %w", err)
		}
		rows, err := q.ListOpenJoinRequests(ctx, dbgen.ListOpenJoinRequestsParams{
			AppID: storage.MustUUID(session.AppID), Creator: creatorOf(p), Now: storage.Timestamp(now),
		})
		if err != nil {
			return fmt.Errorf("devsession: list the join requests: %w", err)
		}
		for _, r := range rows {
			st := JoinStatus{
				RequestID: storage.ID(r.ID), SessionID: storage.ID(r.SessionID), State: joinState(r.State),
				PhoneName: r.PhoneName, ExpiresAt: r.ExpiresAt.Time,
			}
			if r.State == joinWaiting {
				st.Number = int(r.Number)
			}
			out = append(out, st)
		}
		return nil
	})
	return out, err
}

// DenyJoinRequest refuses a phone's waiting request to join a session the
// caller created.
func (s *Service) DenyJoinRequest(ctx context.Context, p auth.Principal, requestID string) error {
	if err := authorizeAnywhere(p, auth.DevSessionCreate); err != nil {
		return err
	}
	var row dbgen.DevJoinRequest
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, _, err = s.ownJoinRequest(ctx, tx, p, requestID, true)
		if err != nil {
			return err
		}
		if _, err := dbgen.New(tx).SetJoinRequestState(ctx, dbgen.SetJoinRequestStateParams{ID: row.ID, State: joinDenied}); err != nil {
			return fmt.Errorf("devsession: deny the join request: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.announceJoin(storage.ID(row.SessionID), requestID, pluxv1.DevJoinState_DEV_JOIN_STATE_DENIED, row.PhoneName, row.ExpiresAt.Time)
	return nil
}

// AllowJoinRetry lets the phone of a request whose number was wrong ask to
// join the session again.
func (s *Service) AllowJoinRetry(ctx context.Context, p auth.Principal, requestID string) error {
	if err := authorizeAnywhere(p, auth.DevSessionCreate); err != nil {
		return err
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, _, err := s.ownJoinRequest(ctx, tx, p, requestID, false)
		if err != nil {
			return err
		}
		if _, err := dbgen.New(tx).AllowJoinRetry(ctx, dbgen.AllowJoinRetryParams{SessionID: row.SessionID, TokenID: row.TokenID}); err != nil {
			return fmt.Errorf("devsession: let the phone try again: %w", err)
		}
		return nil
	})
}

// joinState converts a stored state.
func joinState(state string) pluxv1.DevJoinState {
	switch state {
	case joinWaiting:
		return pluxv1.DevJoinState_DEV_JOIN_STATE_WAITING
	case joinJoined:
		return pluxv1.DevJoinState_DEV_JOIN_STATE_JOINED
	case joinFailed:
		return pluxv1.DevJoinState_DEV_JOIN_STATE_FAILED
	case joinDenied:
		return pluxv1.DevJoinState_DEV_JOIN_STATE_DENIED
	case joinRetry:
		return pluxv1.DevJoinState_DEV_JOIN_STATE_RETRY_ALLOWED
	default:
		return pluxv1.DevJoinState_DEV_JOIN_STATE_UNSPECIFIED
	}
}

// announceJoin tells the session's watchers where a join request stands.
// The event never carries the number. It is best effort, as a device's
// connection events are: an event the shared layer fails to take leaves the
// request to expire, and the phone asks again.
func (s *Service) announceJoin(sessionID, requestID string, state pluxv1.DevJoinState, phoneName string, expires time.Time) {
	ev := &pluxv1.DevEvent{
		TimeUnixMicros: s.now().UnixMicro(),
		Payload: &pluxv1.DevEvent_JoinRequest{JoinRequest: &pluxv1.JoinRequestEvent{
			RequestId: requestID, State: state, PhoneName: phoneName, ExpiresAt: timestamppb.New(expires),
		}},
	}
	_ = s.hub.room(sessionID, nil).appendEvent("", 0, ev) //nolint:contextcheck // the hub's lifetime
}
