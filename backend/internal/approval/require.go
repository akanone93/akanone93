// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Gate is what a guarded service holds: the engine, or nothing where no
// engine is configured.
type Gate interface {
	// Require answers nil when the changes may proceed, and otherwise the
	// refusal to return to the caller; see Require.
	Require(ctx context.Context, tx pgx.Tx, arts ...Artifact) error
}

var _ Gate = (*Service)(nil)

// Require is the one place a guarded change asks whether it may happen. It
// returns nil when no policy matches the change, or when the approvals on file
// satisfy every policy that does; in that case the approval is spent in the
// caller's transaction, so one approval authorises one change. A change that
// is several artifacts at once, such as a configuration that also turns
// device functions on, names them all in one call: each unsatisfied one gets
// its request, so that approvers see everything the change needs, and nothing
// is spent unless all are satisfied. Otherwise it
// returns PLX-8001 (an approval is needed; a request was opened and is named
// in the error's "request" detail), PLX-8002 (the content changed since it
// was approved) or PLX-8004 (the request expired and a new one was opened).
//
// A nil gate requires nothing: it is how a service built without the engine
// behaves, for tests and tools that never run a server.
//
// The caller must call Require once, before it appends to the audit log or
// takes a lock another transaction may wait for, because opening a request,
// and writing a profile's default policies, happen in transactions of their
// own that append to the same chain.
func Require(ctx context.Context, g Gate, tx pgx.Tx, arts ...Artifact) error {
	if g == nil {
		return nil
	}
	return g.Require(ctx, tx, arts...)
}

// spendable is an approval that satisfied its artifact and awaits being spent.
type spendable struct {
	art Artifact
	req Request
}

// Require implements Gate.
func (s *Service) Require(ctx context.Context, tx pgx.Tx, arts ...Artifact) error {
	var (
		refused error
		spend   []spendable
	)
	for _, a := range arts {
		sp, err := s.check(ctx, tx, a)
		switch {
		case err != nil && refused == nil:
			refused = err
		case err == nil && sp != nil:
			spend = append(spend, *sp)
		}
	}
	if refused != nil {
		return refused
	}
	for _, sp := range spend {
		if err := s.spend(ctx, tx, sp.art, sp.req); err != nil {
			return err
		}
	}
	return nil
}

// check decides one artifact without spending anything: nil with nothing to
// spend when no policy matches, nil with the approval to spend when it is
// satisfied, and otherwise the refusal, having opened the request.
func (s *Service) check(ctx context.Context, tx pgx.Tx, a Artifact) (*spendable, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	profile, err := s.profileOf(ctx, dbgen.New(tx), a)
	if err != nil {
		return nil, err
	}
	app, err := scopeUUID(a.AppID)
	if err != nil {
		return nil, err
	}
	policies, err := s.policiesIn(ctx, tx, a.OrganizationID, a.AppID, profile)
	if err != nil {
		return nil, err
	}
	matched := matching(policies, a)
	if len(matched) == 0 {
		return nil, nil
	}
	q := dbgen.New(tx)
	row, err := q.FindLiveApprovalRequest(ctx, dbgen.FindLiveApprovalRequestParams{
		OrganizationID: storage.MustUUID(a.OrganizationID), AppID: app, Resource: string(a.Resource), Action: a.Action, Subject: a.Subject, ContentHash: a.Hash,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, s.refuse(ctx, q, a, matched)
	case err != nil:
		return nil, fmt.Errorf("approval: find the request: %w", err)
	}
	return s.settle(ctx, tx, a, matched, row)
}

// refuse answers a change nothing was asked about: it opens the request and
// says whether an earlier approval of the same subject named other content.
func (s *Service) refuse(ctx context.Context, q *dbgen.Queries, a Artifact, matched []Policy) error {
	latest, err := q.LatestApprovalRequestForSubject(ctx, dbgen.LatestApprovalRequestForSubjectParams{
		OrganizationID: storage.MustUUID(a.OrganizationID), AppID: scopeOf(a.AppID), Resource: string(a.Resource), Action: a.Action, Subject: a.Subject,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("approval: find the earlier request: %w", err)
	}
	changed := err == nil && latest.Status == StatusApproved && latest.ContentHash != a.Hash
	id, err := s.open(ctx, a, matched, "")
	if err != nil {
		return err
	}
	if changed {
		return refusal(plxerr.ApprovalInvalidated, id,
			"%s was approved (request %s) but has changed since; the approval no longer counts and a new request %s was opened", a.Subject, storage.ID(latest.ID), id)
	}
	return refusal(plxerr.ApprovalRequired, id, "%s needs approval (%s); request %s was opened", a.Subject, policyList(matched), id)
}

// settle answers a change with a live request: the request is to be spent
// when the decisions satisfy every matching policy, and the change is refused
// otherwise.
func (s *Service) settle(ctx context.Context, tx pgx.Tx, a Artifact, matched []Policy, row dbgen.ApprovalRequest) (*spendable, error) {
	req, err := s.withDecisions(ctx, dbgen.New(tx), row)
	if err != nil {
		return nil, err
	}
	if req.Status == StatusExpired {
		id, err := s.open(ctx, a, matched, req.ID)
		if err != nil {
			return nil, err
		}
		return nil, refusal(plxerr.ApprovalExpired, id, "request %s for %s expired on %s; a new request %s was opened",
			req.ID, a.Subject, req.ExpiresAt.Format(time.RFC3339), id)
	}
	out := Evaluate(matched, facts(req.Decisions), excluded(a, req)...)
	if !out.Satisfied {
		return nil, refusal(plxerr.ApprovalRequired, req.ID, "%s still needs approval (request %s): %v", a.Subject, req.ID, out.Missing)
	}
	return &spendable{art: a, req: req}, nil
}

// excluded lists the people whose approval never counts for a change: who
// makes it, who asked for it and who wrote it.
func excluded(a Artifact, req Request) []string {
	return slices.DeleteFunc([]string{a.Requester.ID, a.Author, req.RequestedByID, req.AuthorID}, func(id string) bool { return id == "" })
}

// spend marks the request used by the change that is about to happen.
func (s *Service) spend(ctx context.Context, tx pgx.Tx, a Artifact, req Request) error {
	n, err := dbgen.New(tx).UseApprovalRequest(ctx, dbgen.UseApprovalRequestParams{
		ID: storage.MustUUID(req.ID), UsedAt: storage.Timestamp(s.now()), UsedBy: a.Requester.ID,
	})
	if err != nil {
		return fmt.Errorf("approval: spend a request: %w", err)
	}
	if n == 0 {
		return refusal(plxerr.ApprovalRequired, req.ID, "the approval of %s (request %s) was spent by another change", a.Subject, req.ID)
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: a.OrganizationID, Actor: a.Requester, Action: audit.ApprovalUsed, TargetKind: "approval_request", TargetID: req.ID,
		AfterHash: a.Hash, Detail: fmt.Sprintf("%s: %s", a.Action, a.Subject),
	})
	if err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	return nil
}

// refusal builds a refusal that names the request.
func refusal(code plxerr.Code, requestID, format string, args ...any) error {
	return (&plxerr.Error{Code: code, Message: fmt.Sprintf(format, args...)}).WithDetail("request", requestID)
}

// policyList names the policies a message refers to.
func policyList(matched []Policy) string {
	keys := make([]string, len(matched))
	for i, p := range matched {
		keys[i] = p.Doc.Key
	}
	return "policy " + fmt.Sprint(keys)
}

// expiryOf is when a request opened now stops counting: the shortest expiry
// among the policies it must satisfy.
func expiryOf(matched []Policy, now time.Time) time.Time {
	var hours int64
	for i, p := range matched {
		h := int64(DefaultExpiryHours)
		if p.Doc.Require.ExpiryHours != nil {
			h = *p.Doc.Require.ExpiryHours
		}
		if i == 0 || h < hours {
			hours = h
		}
	}
	return now.Add(time.Duration(hours) * time.Hour)
}

// open writes a request for the artifact in a transaction of its own, so
// that it outlives the refusal that rolls the caller back, and retires the
// request it replaces: one that expired, and any that named other content of
// the same subject. It returns the request's identifier.
func (s *Service) open(ctx context.Context, a Artifact, matched []Policy, retire string) (string, error) {
	var (
		id      string
		created bool
	)
	err := s.beside(ctx, a.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, created, err = s.insertRequest(ctx, tx, a, matched, retire)
		return err
	})
	if err != nil {
		return "", err
	}
	if created {
		s.announce(ctx, a, id)
	}
	return id, nil
}

// insertRequest does the work of open inside its transaction.
func (s *Service) insertRequest(ctx context.Context, tx pgx.Tx, a Artifact, matched []Policy, retire string) (id string, created bool, err error) {
	q := dbgen.New(tx)
	org, app := storage.MustUUID(a.OrganizationID), scopeOf(a.AppID)
	if retire != "" {
		if err := q.SetApprovalRequestStatus(ctx, dbgen.SetApprovalRequestStatusParams{ID: storage.MustUUID(retire), Status: StatusSuperseded}); err != nil {
			return "", false, fmt.Errorf("approval: retire a request: %w", err)
		}
	}
	if _, err := q.SupersedeApprovalRequests(ctx, dbgen.SupersedeApprovalRequestsParams{
		OrganizationID: org, AppID: app, Resource: string(a.Resource), Action: a.Action, Subject: a.Subject, ContentHash: a.Hash,
	}); err != nil {
		return "", false, fmt.Errorf("approval: supersede requests: %w", err)
	}
	id, err = s.o.IDs.New()
	if err != nil {
		return "", false, fmt.Errorf("approval: %w", err)
	}
	params, err := s.requestParams(id, a, matched)
	if err != nil {
		return "", false, err
	}
	row, err := q.InsertApprovalRequest(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		live, err := q.FindLiveApprovalRequest(ctx, dbgen.FindLiveApprovalRequestParams{
			OrganizationID: org, AppID: app, Resource: string(a.Resource), Action: a.Action, Subject: a.Subject, ContentHash: a.Hash,
		})
		if err != nil {
			return "", false, fmt.Errorf("approval: find the request another change opened: %w", err)
		}
		return storage.ID(live.ID), false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("approval: open a request: %w", err)
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: a.OrganizationID, Actor: a.Requester, Action: audit.ApprovalRequested, TargetKind: "approval_request",
		TargetID: id, AfterHash: a.Hash, Detail: fmt.Sprintf("%s: %s", a.Action, a.Subject),
	})
	if err != nil {
		return "", false, fmt.Errorf("approval: %w", err)
	}
	return storage.ID(row.ID), true, nil
}

// requestParams builds the row of a new request.
func (s *Service) requestParams(id string, a Artifact, matched []Policy) (dbgen.InsertApprovalRequestParams, error) {
	env, err := storage.NullUUID(a.Environment.ID)
	if err != nil {
		return dbgen.InsertApprovalRequestParams{}, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	payload, err := CanonicalPayload(a.Payload)
	if err != nil {
		return dbgen.InsertApprovalRequestParams{}, err
	}
	keys, needed := make([]string, len(matched)), 1
	for i, p := range matched {
		keys[i] = p.Doc.Key
		needed = max(needed, int(p.Doc.Require.Approvers))
	}
	risk := make([]string, len(a.Risk))
	for i, r := range a.Risk {
		risk[i] = string(r)
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	return dbgen.InsertApprovalRequestParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(a.OrganizationID), AppID: scopeOf(a.AppID),
		EnvironmentID: env, EnvironmentKey: a.Environment.Key, Production: a.Environment.Production,
		Resource: string(a.Resource), Action: a.Action, Subject: a.Subject, ContentHash: a.Hash, Summary: a.Summary,
		Risk: risk, Payload: payload, PolicyKeys: keys, ApprovalsRequired: int32(needed), AuthorID: a.Author,
		RequestedByKind: a.Requester.Kind, RequestedByID: a.Requester.ID, RequestedBy: a.Requester.Display,
		CreatedAt: storage.Timestamp(now), ExpiresAt: storage.Timestamp(expiryOf(matched, now)),
	}, nil
}

// announce tells the approvers that a request waits (SRV-062). A
// notification that cannot be queued is logged and never fails the change.
func (s *Service) announce(ctx context.Context, a Artifact, id string) {
	if s.notifier == nil {
		return
	}
	err := s.notifier.Publish(ctx, notify.Event{
		Type: notify.ApprovalRequested, OrganizationID: a.OrganizationID, AppID: a.AppID,
		Title: "Approval requested: " + a.Summary,
		Body:  fmt.Sprintf("%s (%s) is waiting for an approver.", a.Subject, schema.PolicyResource(a.Resource)),
		Link:  studioLink(a.AppID, id),
		Attributes: map[string]string{
			"request": id, "resource": string(a.Resource), "subject": a.Subject, "environment": a.Environment.Key,
		},
	})
	if err != nil {
		s.o.Log.WarnContext(ctx, "approval: the notification of a request could not be queued", slog.String("request", id), slog.Any("error", err))
	}
}

// studioLink is where an approver decides a request in Studio.
func studioLink(appID, requestID string) string {
	if appID == "" {
		return "/approvals/" + requestID
	}
	return "/apps/" + appID + "/approvals/" + requestID
}
