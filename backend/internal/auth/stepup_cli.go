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
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file is the CLI's side of step-up (maintainer decision Q15,
// ADR-0075). The CLI has no authenticator, so it asks the server for a
// request bound to its token and to the RFC 7638 thumbprint of its DPoP key.
// A person performs the ceremony in a browser session of their own, and the
// CLI, polling with a proof of the same key, receives a grant. The grant
// counts as a step-up only when it is presented with that token and a proof of
// that key, for as long as the assertion is fresh (StepUpWindow).

// CLIStepUp is a request the CLI made. Secret is returned exactly once and
// is what the CLI polls with; only its hash is stored.
type CLIStepUp struct {
	RequestID string
	Secret    string
	ExpiresAt time.Time
}

// CLIGrant is the answer to a poll: not yet, or the grant. The grant's
// secret is returned exactly once.
type CLIGrant struct {
	// Approved is true once the person completed the ceremony.
	Approved bool
	// Grant is the secret to present with the token and a proof of the key.
	Grant string
	// ExpiresAt is when the grant stops counting as a step-up.
	ExpiresAt time.Time
}

// cliToken refuses an identity that is not a token of a person: a step-up
// belongs to someone who can perform the ceremony.
func cliToken(id Identity) error {
	if id.Kind != KindToken || id.UserID == "" || id.OrganizationID == "" {
		return plxerr.New(plxerr.PermissionDenied, "a step-up is requested with a personal access token or the token of plux login")
	}
	return nil
}

// CreateCLIStepUp records a step-up request of the CLI's, bound to its token
// and to the thumbprint jkt of the DPoP key it proved possession of
// (SEC-103).
func (s *Service) CreateCLIStepUp(ctx context.Context, id Identity, jkt string) (CLIStepUp, error) {
	if err := cliToken(id); err != nil {
		return CLIStepUp{}, err
	}
	if jkt == "" {
		return CLIStepUp{}, plxerr.New(plxerr.InvalidFormat, "a step-up request names the DPoP key it is bound to")
	}
	if err := s.throttled(ctx, id.UserID); err != nil {
		return CLIStepUp{}, err
	}
	secret, err := NewSecret(PrefixStepUp, 32)
	if err != nil {
		return CLIStepUp{}, err
	}
	rid, err := s.newID()
	if err != nil {
		return CLIStepUp{}, err
	}
	now := s.now()
	out := CLIStepUp{RequestID: rid, Secret: secret.Value, ExpiresAt: now.Add(stepUpRequestTTL)}
	err = s.inTx(ctx, storage.Tenant{OrganizationID: id.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).CreateStepUpRequest(ctx, dbgen.CreateStepUpRequestParams{
			ID: storage.MustUUID(rid), UserID: storage.MustUUID(id.UserID), OrganizationID: storage.MustUUID(id.OrganizationID),
			Kind: stepUpCLI, TokenID: storage.MustUUID(id.ID), Jkt: jkt, SecretHash: secret.Hash,
			CreatedAt: storage.Timestamp(now), ExpiresAt: storage.Timestamp(out.ExpiresAt),
		})
		if err != nil {
			return fmt.Errorf("auth: create a step-up request: %w", err)
		}
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: id.OrganizationID, Actor: id.Actor(), Action: audit.StepUpRequested,
			TargetKind: "step_up_request", TargetID: rid, Detail: "key " + jkt,
		})
	})
	return out, err
}

// PendingStepUp tells the page a person opened whether the request is theirs
// and still waiting, and when it expires. A request of another person is not
// found, which is what stops a link to someone else's request from asking
// this person to approve it (SEC-103).
func (s *Service) PendingStepUp(ctx context.Context, id Identity, requestID string) (time.Time, error) {
	rid, err := parseID(requestID, "step-up request")
	if err != nil {
		return time.Time{}, err
	}
	var expires time.Time
	err = s.inTx(ctx, storage.Tenant{Scope: storage.ScopeAuthentication}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetStepUpRequest(ctx, rid)
		if err != nil {
			return notFound(err, "step-up request")
		}
		if storage.ID(row.UserID) != id.UserID || row.Kind != stepUpCLI {
			return plxerr.New(plxerr.ResourceNotFound, "no such step-up request")
		}
		if row.State != stepUpPending || !s.now().Before(storage.Time(row.ExpiresAt)) {
			return plxerr.New(plxerr.PreconditionFailed, "the step-up request is no longer waiting; run the command again")
		}
		expires = storage.Time(row.ExpiresAt)
		return nil
	})
	return expires, err
}

// PollCLIStepUp is the CLI asking whether its request was approved. It must
// present the request's secret with the token and the key the request is
// bound to. Once approved, the first poll returns the grant and later ones
// are refused, so the grant is collected once (SEC-103).
func (s *Service) PollCLIStepUp(ctx context.Context, id Identity, requestID, secret, jkt string) (CLIGrant, error) {
	if err := cliToken(id); err != nil {
		return CLIGrant{}, err
	}
	rid, err := parseID(requestID, "step-up request")
	if err != nil {
		return CLIGrant{}, err
	}
	var out CLIGrant
	err = s.inTxRefusing(ctx, storage.Tenant{OrganizationID: id.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetStepUpRequestBySecretForUpdate(ctx, dbgen.GetStepUpRequestBySecretForUpdateParams{ID: rid, SecretHash: HashSecret(secret)})
		if err != nil {
			return notFound(err, "step-up request")
		}
		if storage.ID(row.TokenID) != id.ID || row.Jkt != jkt {
			return s.grantRefusal(ctx, tx, id, rid, "a poll with another token or key",
				plxerr.New(plxerr.PermissionDenied, "this step-up request is bound to another token or key"))
		}
		out, err = s.pollOutcome(ctx, tx, id, row)
		return err
	})
	return out, err
}

// pollOutcome decides what a poll of a request bound to the caller gets.
func (s *Service) pollOutcome(ctx context.Context, tx pgx.Tx, id Identity, row dbgen.StepUpRequest) (CLIGrant, error) {
	switch row.State {
	case stepUpPending:
		if !s.now().Before(storage.Time(row.ExpiresAt)) {
			return CLIGrant{}, plxerr.New(plxerr.PreconditionFailed, "the step-up request expired before it was approved; run the command again")
		}
		return CLIGrant{}, nil
	case stepUpApproved:
		return s.issueGrant(ctx, tx, id, row)
	default:
		return CLIGrant{}, plxerr.New(plxerr.PreconditionFailed, "the grant of this step-up request was already collected, or the request is closed")
	}
}

// issueGrant hands the CLI the grant of an approved request. It lasts as long
// as the assertion is fresh, so a CLI that collects it late has less of the
// window left.
func (s *Service) issueGrant(ctx context.Context, tx pgx.Tx, id Identity, row dbgen.StepUpRequest) (CLIGrant, error) {
	q := dbgen.New(tx)
	until := storage.Time(row.ApprovedAt).Add(StepUpWindow)
	if !s.now().Before(until) {
		if err := q.CloseStepUpRequest(ctx, row.ID); err != nil {
			return CLIGrant{}, fmt.Errorf("auth: close a step-up request: %w", err)
		}
		return CLIGrant{}, refuse(plxerr.New(plxerr.StepUpExpired, "the step-up was approved too long ago to be collected; run the command again"))
	}
	grant, err := NewSecret(PrefixStepUpGrant, 32)
	if err != nil {
		return CLIGrant{}, err
	}
	if err := q.IssueStepUpGrant(ctx, dbgen.IssueStepUpGrantParams{ID: row.ID, GrantHash: grant.Hash, GrantExpiresAt: storage.Timestamp(until)}); err != nil {
		return CLIGrant{}, fmt.Errorf("auth: issue a grant: %w", err)
	}
	err = s.record(ctx, tx, audit.Entry{
		OrganizationID: id.OrganizationID, Actor: id.Actor(), Action: audit.StepUpGrantIssued,
		TargetKind: "step_up_request", TargetID: storage.ID(row.ID),
	})
	return CLIGrant{Approved: true, Grant: grant.Value, ExpiresAt: until}, err
}

// grantRefusal records a grant or a poll used with a token or a key it is not
// bound to, and refuses it.
func (s *Service) grantRefusal(ctx context.Context, tx pgx.Tx, id Identity, request pgtype.UUID, why string, err error) error {
	if err := s.record(ctx, tx, audit.Entry{
		OrganizationID: id.OrganizationID, Actor: id.Actor(), Action: audit.StepUpGrantRefused,
		TargetKind: "step_up_request", TargetID: storage.ID(request), Detail: why,
	}); err != nil {
		return err
	}
	return refuse(err)
}

// errGrantMismatch is the refusal of a grant used with another token or key;
// CheckStepUpGrant swallows it, because the grant then adds nothing and the
// call fails, if it needs a step-up, with PLX-6200.
var errGrantMismatch = errors.New("auth: the step-up grant is bound to another token or key")

// CheckStepUpGrant decides whether a grant presented with a token and a proof
// of the DPoP key with thumbprint jkt makes the identity count as stepped up.
// The grant counts only for the token and the key it was issued for, and
// only while its assertion is fresh; the age it gives the identity is the
// assertion's, so an old grant is PLX-6201 where the step-up is needed. A
// grant that is unknown, or bound to another token or key, adds nothing: the
// identity is returned as it was, and a binding mismatch is audited (SEC-103).
func (s *Service) CheckStepUpGrant(ctx context.Context, id Identity, grant, jkt string) (Identity, error) {
	if id.Kind != KindToken || id.OrganizationID == "" {
		return id, nil
	}
	out := id
	err := s.inTxRefusing(ctx, storage.Tenant{OrganizationID: id.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetStepUpGrant(ctx, HashSecret(grant))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("auth: read a grant: %w", err)
		}
		if storage.ID(row.TokenID) != id.ID || storage.ID(row.UserID) != id.UserID || row.Jkt != jkt {
			return s.grantRefusal(ctx, tx, id, row.ID, "a grant presented with another token or key", errGrantMismatch)
		}
		out = id.WithStepUp(s.now().Sub(storage.Time(row.ApprovedAt)))
		return nil
	})
	if errors.Is(err, errGrantMismatch) {
		return id, nil
	}
	return out, err
}
