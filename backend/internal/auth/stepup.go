// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file runs the WebAuthn step-up ceremony for a browser session
// (SEC-103, ADR-0075): the options with user verification required, and the
// assertion that sets the session's step_up_at. The same ceremony approves a
// request of the CLI's, whose grant stepup_cli.go issues.

// stepUpRequestTTL is how long a step-up request waits for its ceremony.
const stepUpRequestTTL = 5 * time.Minute

// Kinds and states of a step-up request, as stored.
const (
	stepUpSession = "session"
	stepUpCLI     = "cli"

	stepUpPending  = "pending"
	stepUpApproved = "approved"
	stepUpIssued   = "issued"
	stepUpClosed   = "closed"
)

// StepUpOptions is a ceremony the browser performs.
type StepUpOptions struct {
	// RequestID names the request for CompleteStepUp.
	RequestID string
	// Options is PublicKeyCredentialRequestOptions as JSON, with user
	// verification required.
	Options   []byte
	ExpiresAt time.Time
}

// refused is returned from a transaction body to commit what it wrote, such
// as the audit entry of a refusal, and then fail with err.
type refused struct{ err error }

func (r refused) Error() string { return r.err.Error() }

// refuse marks err as one to return after the transaction commits.
func refuse(err error) error { return refused{err: err} }

// inTxRefusing runs f in a transaction like inTx, except that an error f
// returns through refuse commits the transaction first.
func (s *Service) inTxRefusing(ctx context.Context, t storage.Tenant, f func(context.Context, pgx.Tx) error) error {
	var refusal error
	err := s.inTx(ctx, t, func(ctx context.Context, tx pgx.Tx) error {
		err := f(ctx, tx)
		var r refused
		if errors.As(err, &r) {
			refusal = r.err
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	return refusal
}

// stepUpTenant is the visibility the transaction on a request needs: the
// organisation whose chain records it, or the installation's.
func stepUpTenant(row dbgen.StepUpRequest) storage.Tenant {
	if row.OrganizationID.Valid {
		return storage.Tenant{OrganizationID: storage.ID(row.OrganizationID)}
	}
	return storage.Tenant{Scope: storage.ScopeInstallation}
}

// requestTenant reads which organisation a request belongs to, so that the
// transaction that changes it can append to that chain.
func (s *Service) requestTenant(ctx context.Context, rid pgtype.UUID) (storage.Tenant, error) {
	var t storage.Tenant
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeAuthentication}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetStepUpRequest(ctx, rid)
		if err != nil {
			return notFound(err, "step-up request")
		}
		t = stepUpTenant(row)
		return nil
	})
	return t, err
}

// stepUpSessionOnly refuses a caller that is not a browser session, and a
// installation without WebAuthn, and applies the failed-attempt throttle.
func (s *Service) stepUpSessionOnly(ctx context.Context, id Identity) error {
	if err := s.webAuthnRequired(); err != nil {
		return err
	}
	if id.Kind != KindUser || id.SessionID == "" {
		return plxerr.New(plxerr.PreconditionFailed, "a step-up is performed in a browser session")
	}
	return s.throttled(ctx, id.UserID)
}

// BeginStepUp starts a ceremony and returns the options for
// navigator.credentials.get(). With no cliRequestID it steps up the caller's
// own session; with one it performs a request the CLI made, which must be the
// caller's own (SEC-103, ADR-0075).
func (s *Service) BeginStepUp(ctx context.Context, id Identity, cliRequestID string) (StepUpOptions, error) {
	if err := s.stepUpSessionOnly(ctx, id); err != nil {
		return StepUpOptions{}, err
	}
	if cliRequestID == "" {
		return s.beginForSession(ctx, id)
	}
	rid, err := parseID(cliRequestID, "step-up request")
	if err != nil {
		return StepUpOptions{}, err
	}
	tenant, err := s.requestTenant(ctx, rid)
	if err != nil {
		return StepUpOptions{}, err
	}
	var out StepUpOptions
	err = s.inTxRefusing(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := s.openRequest(ctx, q, rid, id)
		if err != nil {
			return err
		}
		if row.Kind != stepUpCLI {
			return plxerr.New(plxerr.ResourceNotFound, "no such step-up request")
		}
		out, err = s.openCeremony(ctx, tx, id, row)
		return err
	})
	return out, err
}

// beginForSession creates a request for the caller's session and opens its
// ceremony.
func (s *Service) beginForSession(ctx context.Context, id Identity) (StepUpOptions, error) {
	rid, err := s.newID()
	if err != nil {
		return StepUpOptions{}, err
	}
	now := s.now()
	var out StepUpOptions
	err = s.inTxRefusing(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).CreateStepUpRequest(ctx, dbgen.CreateStepUpRequestParams{
			ID: storage.MustUUID(rid), UserID: storage.MustUUID(id.UserID), Kind: stepUpSession,
			SessionID: storage.MustUUID(id.SessionID),
			CreatedAt: storage.Timestamp(now), ExpiresAt: storage.Timestamp(now.Add(stepUpRequestTTL)),
		})
		if err != nil {
			return fmt.Errorf("auth: create a step-up request: %w", err)
		}
		out, err = s.openCeremony(ctx, tx, id, row)
		return err
	})
	return out, err
}

// openRequest locks a request and checks that it is the caller's, still
// waiting and not expired. A request of another person, or of another
// session, is reported as not found.
func (s *Service) openRequest(ctx context.Context, q *dbgen.Queries, rid pgtype.UUID, id Identity) (dbgen.StepUpRequest, error) {
	row, err := q.GetStepUpRequestForUpdate(ctx, rid)
	if err != nil {
		return dbgen.StepUpRequest{}, notFound(err, "step-up request")
	}
	if storage.ID(row.UserID) != id.UserID || (row.Kind == stepUpSession && storage.ID(row.SessionID) != id.SessionID) {
		return dbgen.StepUpRequest{}, plxerr.New(plxerr.ResourceNotFound, "no such step-up request")
	}
	if row.State != stepUpPending {
		return dbgen.StepUpRequest{}, plxerr.New(plxerr.PreconditionFailed, "the step-up request is already %s", row.State)
	}
	if !s.now().Before(storage.Time(row.ExpiresAt)) {
		return dbgen.StepUpRequest{}, plxerr.New(plxerr.PreconditionFailed, "the step-up request has expired; start it again")
	}
	return row, nil
}

// openCeremony counts a ceremony against the request, which is closed once
// auth.stepUpAttempts of them were begun, and returns the options for it.
func (s *Service) openCeremony(ctx context.Context, tx pgx.Tx, id Identity, row dbgen.StepUpRequest) (StepUpOptions, error) {
	q := dbgen.New(tx)
	if int64(row.Attempts) >= s.o.Limits.Get(limits.AuthStepUpAttempts) {
		if err := q.CloseStepUpRequest(ctx, row.ID); err != nil {
			return StepUpOptions{}, fmt.Errorf("auth: close a step-up request: %w", err)
		}
		return StepUpOptions{}, s.stepUpRefusal(ctx, tx, id, row, "too many ceremonies", plxerr.New(plxerr.RateLimited,
			"this step-up request accepted its last ceremony; start it again"))
	}
	user, err := q.GetUser(ctx, row.UserID)
	if err != nil {
		return StepUpOptions{}, fmt.Errorf("auth: read a user: %w", err)
	}
	allow, err := credentials(ctx, q, user)
	if err != nil {
		return StepUpOptions{}, err
	}
	if len(allow) == 0 {
		return StepUpOptions{}, plxerr.New(plxerr.PreconditionFailed, "a step-up needs a security key; register one first")
	}
	challenge := newChallenge()
	if err := q.BeginStepUpCeremony(ctx, dbgen.BeginStepUpCeremonyParams{ID: row.ID, Challenge: challenge}); err != nil {
		return StepUpOptions{}, fmt.Errorf("auth: record the challenge: %w", err)
	}
	options, err := json.Marshal(map[string]any{
		"challenge": b64(challenge), "rpId": s.o.WebAuthn.RPID, "timeout": webAuthnTimeout,
		"allowCredentials": allow, "userVerification": "required",
	})
	if err != nil {
		return StepUpOptions{}, fmt.Errorf("auth: %w", err)
	}
	return StepUpOptions{RequestID: storage.ID(row.ID), Options: options, ExpiresAt: storage.Time(row.ExpiresAt)}, nil
}

// stepUpRefusal records why a ceremony was refused and returns err to be
// returned after the record commits.
func (s *Service) stepUpRefusal(ctx context.Context, tx pgx.Tx, id Identity, row dbgen.StepUpRequest, why string, err error) error {
	if aerr := s.record(ctx, tx, audit.Entry{
		OrganizationID: storage.ID(row.OrganizationID), Actor: id.Actor(), Action: audit.StepUpRefused,
		TargetKind: "step_up_request", TargetID: storage.ID(row.ID), Detail: why,
	}); aerr != nil {
		return aerr
	}
	return refuse(err)
}

// CompleteStepUp verifies the assertion that answers a ceremony. The
// authenticator must have verified the user (the UV flag): a key that only
// proved presence is refused. A verified assertion sets the session's
// step_up_at, or approves the CLI's request, and returns the time. Whatever
// the answer, the ceremony's challenge is spent; an assertion that does not
// verify is PLX-6202 (SEC-103).
func (s *Service) CompleteStepUp(ctx context.Context, id Identity, requestID string, a WebAuthnAssertion) (time.Time, error) {
	if err := s.stepUpSessionOnly(ctx, id); err != nil {
		return time.Time{}, err
	}
	rid, err := parseID(requestID, "step-up request")
	if err != nil {
		return time.Time{}, err
	}
	tenant, err := s.requestTenant(ctx, rid)
	if err != nil {
		return time.Time{}, err
	}
	var at time.Time
	err = s.inTxRefusing(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := s.openRequest(ctx, q, rid, id)
		if err != nil {
			return err
		}
		if row.Challenge == nil {
			return plxerr.New(plxerr.PreconditionFailed, "ask for the security key's options first")
		}
		// The challenge answers once, right or wrong.
		if err := q.ClearStepUpChallenge(ctx, rid); err != nil {
			return fmt.Errorf("auth: spend the challenge: %w", err)
		}
		if !s.verifyUserAssertion(ctx, q, row.UserID, row.Challenge, a, true) {
			return s.assertionRefused(ctx, tx, id, row)
		}
		at = s.now()
		return s.approve(ctx, tx, id, row, at)
	})
	var plx *plxerr.Error
	switch {
	case errors.As(err, &plx) && plx.Code == plxerr.WebAuthnAssertionInvalid:
		s.failed(ctx, id.UserID)
	case err == nil:
		s.succeeded(ctx, id.UserID)
	}
	return at, err
}

// assertionRefused records a wrong assertion and closes the request when it
// was the last ceremony it accepts.
func (s *Service) assertionRefused(ctx context.Context, tx pgx.Tx, id Identity, row dbgen.StepUpRequest) error {
	if int64(row.Attempts) >= s.o.Limits.Get(limits.AuthStepUpAttempts) {
		if err := dbgen.New(tx).CloseStepUpRequest(ctx, row.ID); err != nil {
			return fmt.Errorf("auth: close a step-up request: %w", err)
		}
	}
	return s.stepUpRefusal(ctx, tx, id, row, "assertion does not verify", plxerr.New(plxerr.WebAuthnAssertionInvalid,
		"the security key's answer does not verify, or did not verify you"))
}

// approve records a verified assertion: on the session for a browser
// step-up, on the request for the CLI's.
func (s *Service) approve(ctx context.Context, tx pgx.Tx, id Identity, row dbgen.StepUpRequest, at time.Time) error {
	q := dbgen.New(tx)
	target, targetID := "step_up_request", storage.ID(row.ID)
	if row.Kind == stepUpSession {
		n, err := q.SetSessionStepUp(ctx, dbgen.SetSessionStepUpParams{ID: row.SessionID, At: storage.Timestamp(at)})
		if err != nil {
			return fmt.Errorf("auth: record a step-up: %w", err)
		}
		if n == 0 {
			return plxerr.New(plxerr.SessionRevoked, "this session was ended; sign in again")
		}
		target, targetID = "session", storage.ID(row.SessionID)
	}
	if err := q.ApproveStepUpRequest(ctx, dbgen.ApproveStepUpRequestParams{ID: row.ID, At: storage.Timestamp(at)}); err != nil {
		return fmt.Errorf("auth: approve a step-up request: %w", err)
	}
	return s.record(ctx, tx, audit.Entry{
		OrganizationID: storage.ID(row.OrganizationID), Actor: id.Actor(), Action: audit.StepUpApproved,
		TargetKind: target, TargetID: targetID, Detail: row.Kind,
	})
}
