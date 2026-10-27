// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package releasereq

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ActionRelease is the approval action of a release request; a policy may
// name it.
const ActionRelease = "release"

// approvalHash is the content an approval of a request is bound to: what the
// builds produced and where it goes (GOV-022).
func approvalHash(r Request) string {
	return approval.HashOf([]byte(r.ContentHash), []byte(r.EnvironmentID), []byte(r.Channel))
}

// RequestApproval completes the approval step. It asks the approval engine
// whether the policies need approval of this content: if not, or when the
// approvals on file satisfy them, the request moves to the rollout plan and
// the approval is bound to the content hash. If they do, the engine opens its
// request, which this request links, and this one is held as awaiting
// approval; calling again after the decision moves it on, ends it as
// rejected, or as expired. Approvers need a finished visual diff first
// (GOV-023). It needs release.publish.
func (s *Service) RequestApproval(ctx context.Context, p auth.Principal, appID, id string) (Request, error) {
	if err := authorize(p, auth.ReleasePublish, appID); err != nil {
		return Request{}, err
	}
	return s.update(ctx, p, appID, id, func(ctx context.Context, tx pgx.Tx, r *Request) error {
		switch {
		case !r.State.Open():
			return ended(*r)
		case r.State.Passed(StateApproval):
			return nil
		case r.State != StateApproval:
			return plxerr.New(plxerr.ReleaseRequestState, "approval is requested at the approval step; the request is at the %s step", r.State)
		}
		q := dbgen.New(tx)
		if err := finishedDiff(ctx, q, r); err != nil {
			return err
		}
		if done, err := decidedElsewhere(ctx, q, r); done || err != nil {
			return err
		}
		return s.askEngine(ctx, tx, p, r)
	})
}

// decidedElsewhere ends the request when the approval request it links was
// rejected or expired.
func decidedElsewhere(ctx context.Context, q *dbgen.Queries, r *Request) (bool, error) {
	if r.ApprovalRequestID == "" {
		return false, nil
	}
	id, err := storage.UUID(r.ApprovalRequestID)
	if err != nil {
		return false, fmt.Errorf("releasereq: %w", err)
	}
	row, err := q.GetApprovalRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		// The engine no longer holds it; it is asked again.
		return false, nil
	}
	if err != nil {
		return false, failure(err, "approval request")
	}
	switch row.Status {
	case approval.StatusRejected:
		r.end(StateRejected)
	case approval.StatusExpired:
		r.end(StateExpired)
	default:
		return false, nil
	}
	return true, nil
}

func (r *Request) end(state State) {
	r.State, r.Hold, r.HoldReason = state, HoldNone, ""
	r.EndedAt = time.Now().UTC()
}

// askEngine puts the release to the approval engine and applies the answer.
func (s *Service) askEngine(ctx context.Context, tx pgx.Tx, p auth.Principal, r *Request) error {
	env, err := storage.UUID(r.EnvironmentID)
	if err != nil {
		return fmt.Errorf("releasereq: %w", err)
	}
	e, err := dbgen.New(tx).GetEnvironment(ctx, env)
	if err != nil {
		return failure(err, "environment")
	}
	err = approval.Require(ctx, s.o.Approvals, tx, approval.Artifact{
		Resource: schema.PolicyResourceRelease, OrganizationID: p.OrganizationID, AppID: r.AppID,
		Environment: approval.Environment{ID: r.EnvironmentID, Key: e.Key, Production: e.Production},
		Action:      ActionRelease, Subject: "releaseRequest:" + r.ID, Hash: approvalHash(*r),
		Summary: fmt.Sprintf("Release %d plugin(s) to %s/%s", len(r.Plugins), e.Key, r.Channel),
		Author:  r.CreatedBy, Requester: p.Actor(), RequesterUser: p.UserID,
	})
	var refusal *plxerr.Error
	switch {
	case err == nil:
		r.ApprovedHash = r.ContentHash
		r.moveOn()
		return nil
	case !errors.As(err, &refusal):
		return err //nolint:wrapcheck // a domain error
	case refusal.Code == plxerr.ApprovalRequired || refusal.Code == plxerr.ApprovalInvalidated:
		r.Hold, r.HoldReason = HoldAwaiting, "waiting for approval"
		r.ApprovalRequestID = refusal.Details["request"]
		return nil
	case refusal.Code == plxerr.ApprovalExpired:
		r.ApprovalRequestID = refusal.Details["request"]
		r.end(StateExpired)
		return nil
	default:
		return err //nolint:wrapcheck // a domain error
	}
}

// Complete is the release step: it makes the app release of the built
// versions and promotes it to the channel, and the request is Released. The
// approval must be for the content the request has now, or it is void and the
// request returns to the build step (GOV-022). The release is recorded before
// it is promoted, so a promotion that is refused for approval or step-up
// is retried without making a second release. It needs release.promote;
// production still needs the WebAuthn step-up (SEC-103).
func (s *Service) Complete(ctx context.Context, p auth.Principal, appID, id string) (Request, error) {
	if err := authorize(p, auth.ReleasePromote, appID); err != nil {
		return Request{}, err
	}
	if s.o.Releaser == nil {
		return Request{}, plxerr.New(plxerr.PreconditionFailed, "this installation cannot make releases from requests")
	}
	r, err := s.update(ctx, p, appID, id, func(ctx context.Context, _ pgx.Tx, r *Request) error {
		switch {
		case r.State == StateReleased:
			return nil
		case !r.State.Open():
			return ended(*r)
		case r.State != StateRelease:
			return plxerr.New(plxerr.ReleaseRequestState, "a request is completed at the release step; it is at the %s step", r.State)
		case r.ApprovedHash == "" || r.ApprovedHash != r.ContentHash:
			r.backToBuild()
			return nil
		case r.ReleaseSequence != 0:
			return nil
		}
		versions := make(map[string]int64, len(r.Builds))
		for _, b := range r.Builds {
			versions[b.PluginKey] = b.Version
		}
		seq, err := s.o.Releaser.Create(ctx, p, r.AppID, r.EnvironmentID, versions, r.ChangelogNotes)
		if err != nil {
			return err //nolint:wrapcheck // a domain error
		}
		r.ReleaseSequence = seq
		return nil
	})
	if err != nil || r.State != StateRelease || r.ReleaseSequence == 0 {
		return r, err
	}
	return s.update(ctx, p, appID, id, func(ctx context.Context, _ pgx.Tx, r *Request) error {
		if r.State != StateRelease || r.ReleaseSequence == 0 {
			return nil
		}
		if err := s.o.Releaser.Promote(ctx, p, r.AppID, r.ReleaseSequence, r.EnvironmentID, r.Channel); err != nil {
			return err //nolint:wrapcheck // a domain error
		}
		r.end(StateReleased)
		return nil
	})
}

// backToBuild voids the builds and the approval: the content they were for
// is not the content the request has (GOV-022).
func (r *Request) backToBuild() {
	r.State, r.Hold, r.HoldReason = StateBuild, HoldNone, ""
	r.Builds, r.Acknowledged, r.ContentHash = nil, nil, ""
	r.voidApproval()
}
