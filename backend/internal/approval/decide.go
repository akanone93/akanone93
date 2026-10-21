// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// The two decisions.
const (
	DecisionApprove = "approve"
	DecisionReject  = "reject"
)

// Decide approves or rejects a request (GOV-021, GOV-023). The decider needs
// approvals.decide on the request's environment, which carries a step-up
// (SEC-103). Nobody who made, asked for or wrote the change is eligible, nor
// anyone the matching policies' roles and named approvers leave out, nor
// anyone who already decided (PLX-8003). The decision is a record bound to the
// request's content hash, written with the audit entry that appends it to
// the chain; one rejection closes the request.
func (s *Service) Decide(ctx context.Context, p auth.Principal, appID, id, decision, comment string) (Request, error) {
	if decision != DecisionApprove && decision != DecisionReject {
		return Request{}, plxerr.New(plxerr.InvalidEnumValue, "a decision is %q or %q", DecisionApprove, DecisionReject)
	}
	if err := authorizeSomeEnvironment(p, appID); err != nil {
		return Request{}, fmt.Errorf("approval: %w", err)
	}
	var out Request
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.decideIn(ctx, tx, p, appID, id, decision, comment)
		return err
	})
	if err != nil {
		return Request{}, err
	}
	if s.o.Signer != nil {
		if _, err := s.SignDecisions(ctx, p.OrganizationID); err != nil {
			s.o.Log.WarnContext(ctx, "approval: the decision was recorded but not yet signed", "error", err)
		}
	}
	return out, nil
}

// decideIn records a decision inside the transaction.
func (s *Service) decideIn(ctx context.Context, tx pgx.Tx, p auth.Principal, appID, id, decision, comment string) (Request, error) {
	q := dbgen.New(tx)
	rid, err := storage.UUID(id)
	if err != nil {
		return Request{}, plxerr.New(plxerr.InvalidFormat, "the approval identifier is not valid")
	}
	row, err := q.GetApprovalRequestForUpdate(ctx, rid)
	if err != nil || appOf(row.AppID) != appID {
		return Request{}, plxerr.New(plxerr.ResourceNotFound, "no such approval request")
	}
	req, err := s.withDecisions(ctx, q, row)
	if err != nil {
		return Request{}, err
	}
	if err := s.checkDecidable(p, req); err != nil {
		return Request{}, err
	}
	decider, matched, err := s.eligibleDecider(ctx, tx, p, req, decision == DecisionApprove)
	if err != nil {
		return Request{}, err
	}
	if err := s.record(ctx, tx, p, req, decider, decision, comment); err != nil {
		return Request{}, err
	}
	return s.settleStatus(ctx, tx, req, matched)
}

// checkDecidable refuses a decision the principal may not make on a request
// in its state.
func (s *Service) checkDecidable(p auth.Principal, req Request) error {
	var err error
	if req.Environment != "" && req.AppID != "" {
		err = p.AuthorizeEnvironment(auth.ApprovalsDecide, req.AppID, req.Environment)
	} else {
		err = authorizeScope(p, auth.ApprovalsDecide, req.AppID)
	}
	if err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	switch req.Status {
	case StatusOpen:
	case StatusExpired:
		return plxerr.New(plxerr.ApprovalExpired, "request %s expired on %s", req.ID, req.ExpiresAt.Format("2006-01-02 15:04 MST"))
	default:
		return plxerr.New(plxerr.PreconditionFailed, "request %s is %s and takes no more decisions", req.ID, req.Status)
	}
	switch {
	case p.UserID == "":
		return plxerr.New(plxerr.ApproverNotEligible, "only a person can decide an approval")
	case p.UserID == req.RequestedByID:
		return plxerr.New(plxerr.ApproverNotEligible, "you asked for this change and cannot approve or reject it")
	case p.UserID == req.AuthorID:
		return plxerr.New(plxerr.ApproverNotEligible, "you wrote this change and cannot approve or reject it")
	}
	return nil
}

// eligibleDecider describes the principal as a decider and checks that the
// policies that match the request accept them now (PLX-8003).
func (s *Service) eligibleDecider(ctx context.Context, tx pgx.Tx, p auth.Principal, req Request, approve bool) (Decision, []Policy, error) {
	profile, err := s.profileOf(ctx, dbgen.New(tx), req.Artifact())
	if err != nil {
		return Decision{}, nil, err
	}
	policies, err := s.policiesIn(ctx, tx, req.OrganizationID, req.AppID, profile)
	if err != nil {
		return Decision{}, nil, err
	}
	matched := matching(policies, req.Artifact())
	if len(matched) == 0 {
		return Decision{}, nil, plxerr.New(plxerr.PreconditionFailed, "no policy asks for approval of this change any more")
	}
	roles, teams, err := s.holder(ctx, dbgen.New(tx), p)
	if err != nil {
		return Decision{}, nil, err
	}
	d := Decision{DeciderID: p.UserID, Roles: roles, Teams: teams, Approve: approve, At: s.now()}
	var given []Decision
	for _, f := range facts(req.Decisions) {
		if f.Approve {
			given = append(given, f)
		}
	}
	if ok, reason := CanApprove(matched, given, d); !ok {
		return Decision{}, nil, plxerr.New(plxerr.ApproverNotEligible, "you cannot decide this request: %s", reason)
	}
	return d, matched, nil
}

// holder reads the roles and teams a person has in the organisation, which
// a policy's roles and distinct-team rule are matched against.
func (*Service) holder(ctx context.Context, q *dbgen.Queries, p auth.Principal) (roles, teams []string, err error) {
	org, user := storage.MustUUID(p.OrganizationID), storage.MustUUID(p.UserID)
	roles, err = q.ListApproverRoles(ctx, dbgen.ListApproverRolesParams{OrganizationID: org, UserID: user})
	if err != nil {
		return nil, nil, fmt.Errorf("approval: read the roles of a decider: %w", err)
	}
	ids, err := q.ListApproverTeams(ctx, dbgen.ListApproverTeamsParams{OrganizationID: org, UserID: user})
	if err != nil {
		return nil, nil, fmt.Errorf("approval: read the teams of a decider: %w", err)
	}
	roles = append([]string{}, roles...)
	slices.Sort(roles)
	teams = []string{}
	for _, id := range ids {
		teams = append(teams, storage.ID(id))
	}
	return roles, teams, nil
}

// record writes the decision and the audit entry that chains it.
func (s *Service) record(ctx context.Context, tx pgx.Tx, p auth.Principal, req Request, d Decision, decision, comment string) error {
	at := d.At.UTC().Truncate(time.Microsecond)
	payload, err := Record{
		RequestID: req.ID, OrganizationID: req.OrganizationID, AppID: req.AppID, Resource: string(req.Resource), Action: req.Action,
		Subject: req.Subject, ContentHash: req.ContentHash, Decision: decision, DeciderID: p.UserID, Comment: comment, DecidedAt: formatTime(at),
	}.Payload()
	if err != nil {
		return err
	}
	action := audit.ApprovalApproved
	if decision == DecisionReject {
		action = audit.ApprovalRejected
	}
	entry, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: req.OrganizationID, Actor: p.Actor(), Action: action, TargetKind: "approval_request", TargetID: req.ID,
		AfterHash: req.ContentHash, Detail: fmt.Sprintf("%s: %s; record %s", req.Action, req.Subject, HashOf(payload)),
	})
	if err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	did, err := s.o.IDs.New()
	if err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	err = dbgen.New(tx).InsertApprovalDecision(ctx, dbgen.InsertApprovalDecisionParams{
		ID: storage.MustUUID(did), RequestID: storage.MustUUID(req.ID), OrganizationID: storage.MustUUID(req.OrganizationID),
		DeciderID: p.UserID, Decider: p.Actor().Display, Roles: d.Roles, Teams: d.Teams, Decision: decision, Comment: comment,
		ContentHash: req.ContentHash, DecidedAt: storage.Timestamp(at), AuditSequence: entry.Sequence, Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("approval: store a decision: %w", err)
	}
	return nil
}

// settleStatus moves the request to rejected or approved when the decisions
// now say so, and returns it as stored. One rejection closes a request; the
// excluded people's decisions cannot exist, as they were refused.
func (s *Service) settleStatus(ctx context.Context, tx pgx.Tx, req Request, matched []Policy) (Request, error) {
	q := dbgen.New(tx)
	row, err := q.GetApprovalRequest(ctx, storage.MustUUID(req.ID))
	if err != nil {
		return Request{}, failure(err, "approval request")
	}
	fresh, err := s.withDecisions(ctx, q, row)
	if err != nil {
		return Request{}, err
	}
	status := StatusOpen
	if out := Evaluate(matched, facts(fresh.Decisions), req.RequestedByID, req.AuthorID); out.Rejected {
		status = StatusRejected
	} else if out.Satisfied {
		status = StatusApproved
	}
	if status != StatusOpen {
		if err := q.SetApprovalRequestStatus(ctx, dbgen.SetApprovalRequestStatusParams{ID: row.ID, Status: status}); err != nil {
			return Request{}, fmt.Errorf("approval: settle a request: %w", err)
		}
		fresh.Status = status
	}
	return fresh, nil
}
