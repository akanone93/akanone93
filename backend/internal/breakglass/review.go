// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package breakglass

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Review completes the post-hoc review of an emergency action. The reviewer
// needs approvals.decide, which carries a step-up (SEC-103), and cannot be the
// person who took the action (PLX-8003): a review is a second pair of eyes.
func (s *Service) Review(ctx context.Context, p auth.Principal, id, comment string) (Record, error) {
	if err := p.Authorize(auth.ApprovalsDecide); err != nil {
		return Record{}, fmt.Errorf("breakglass: %w", err)
	}
	uid, err := storage.UUID(id)
	if err != nil {
		return Record{}, plxerr.New(plxerr.InvalidFormat, "the break-glass identifier is not valid")
	}
	var out Record
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		out, err = s.review(ctx, tx, p, uid, comment)
		return err
	})
	return out, err
}

// review completes a review inside the transaction.
func (s *Service) review(ctx context.Context, tx pgx.Tx, p auth.Principal, id pgtype.UUID, comment string) (Record, error) {
	q := dbgen.New(tx)
	row, err := q.GetBreakGlassForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, plxerr.New(plxerr.ResourceNotFound, "no such break-glass action")
	}
	if err != nil {
		return Record{}, fmt.Errorf("breakglass: find the invocation: %w", err)
	}
	switch {
	case row.ReviewStatus != StatusOpen:
		return Record{}, plxerr.New(plxerr.PreconditionFailed, "the review of this break-glass action is already complete")
	case p.UserID == "" || p.UserID == row.InvokedByUser || p.UserID == row.InvokedByID:
		return Record{}, plxerr.New(plxerr.ApproverNotEligible, "the person who took a break-glass action cannot review it")
	}
	now := s.now()
	done, err := q.ReviewBreakGlass(ctx, dbgen.ReviewBreakGlassParams{
		ID: id, ReviewedByID: p.UserID, ReviewedBy: p.Actor().Display, ReviewedAt: storage.Timestamp(now), ReviewComment: comment,
	})
	if err != nil {
		return Record{}, fmt.Errorf("breakglass: complete the review: %w", err)
	}
	_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.BreakGlassReviewed, TargetKind: "break_glass", TargetID: storage.ID(id),
		Detail: fmt.Sprintf("%s: %s", done.Action, done.Subject),
	})
	if err != nil {
		return Record{}, fmt.Errorf("breakglass: %w", err)
	}
	return recordOf(done, now), nil
}

// List returns the organisation's emergency actions, newest first, after the
// cursor. status narrows it to "open" or "reviewed" when set. It needs
// audit.read.
func (s *Service) List(ctx context.Context, p auth.Principal, status string, after storage.Cursor, limit int32) ([]Record, error) {
	if err := p.Authorize(auth.AuditRead); err != nil {
		return nil, fmt.Errorf("breakglass: %w", err)
	}
	if status != "" && status != StatusOpen && status != StatusReviewed {
		return nil, plxerr.New(plxerr.InvalidEnumValue, "a review status is %q or %q", StatusOpen, StatusReviewed)
	}
	var out []Record
	now := s.now()
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListBreakGlass(ctx, dbgen.ListBreakGlassParams{
			Status: status, BeforeTime: after.BeforeTime(), BeforeID: after.BeforeID(), PageSize: limit,
		})
		if err != nil {
			return fmt.Errorf("breakglass: list the invocations: %w", err)
		}
		for _, r := range rows {
			out = append(out, recordOf(r, now))
		}
		return nil
	})
	return out, err
}

// escalationBatch bounds how many overdue reviews one run escalates.
const escalationBatch = 100

// Escalate raises every review of an organisation that is open past its due
// time and has not been raised before (PLX-8006), and returns how many it
// raised. Each gets an audit entry, in the chain, and a critical
// notification; it is raised once, so a run that finds the same review again
// does nothing. It is the worker's job, and the organisation's reviews it
// reads and writes are its own.
func (s *Service) Escalate(ctx context.Context, organizationID string) (int, error) {
	var raised []Record
	now := s.now()
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListOverdueBreakGlass(ctx, dbgen.ListOverdueBreakGlassParams{Now: storage.Timestamp(now), PageSize: escalationBatch})
		if err != nil {
			return fmt.Errorf("breakglass: list the overdue reviews: %w", err)
		}
		for _, r := range rows {
			if err := s.escalate(ctx, tx, r, now); err != nil {
				return err
			}
			raised = append(raised, recordOf(r, now))
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("breakglass: escalate the overdue reviews: %w", err)
	}
	for _, rec := range raised {
		s.alertOverdue(ctx, rec)
	}
	return len(raised), nil
}

// escalate marks one review as raised and audits it.
func (s *Service) escalate(ctx context.Context, tx pgx.Tx, r dbgen.BreakGlassInvocation, now time.Time) error {
	if err := dbgen.New(tx).MarkBreakGlassEscalated(ctx, dbgen.MarkBreakGlassEscalatedParams{ID: r.ID, EscalatedAt: storage.Timestamp(now)}); err != nil {
		return fmt.Errorf("breakglass: mark the escalation: %w", err)
	}
	org := storage.ID(r.OrganizationID)
	_, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: org, Actor: auth.System(org).Actor(), Action: audit.BreakGlassOverdue, TargetKind: "break_glass", TargetID: storage.ID(r.ID),
		Detail: fmt.Sprintf("PLX-%d: the review of %s by %s was due %s", plxerr.PostHocReviewOverdue, r.Subject, r.InvokedBy, r.ReviewDueAt.Time.UTC().Format(time.RFC3339)),
	})
	if err != nil {
		return fmt.Errorf("breakglass: %w", err)
	}
	return nil
}

// alertOverdue notifies that a review is overdue; a notification that cannot
// be queued is logged, and the escalation stands.
func (s *Service) alertOverdue(ctx context.Context, rec Record) {
	if s.o.Notifier == nil {
		return
	}
	err := s.o.Notifier.Publish(ctx, notify.Event{
		Type: notify.BreakGlassOverdue, OrganizationID: rec.OrganizationID, AppID: rec.AppID,
		Title: fmt.Sprintf("Break-glass review overdue: %s on %s", rec.Action, rec.Environment),
		Body:  fmt.Sprintf("The review of %s by %s was due %s and is still open.", rec.Subject, rec.InvokedBy.Display, rec.ReviewDueAt.UTC().Format(time.RFC3339)),
		Link:  "/breakglass/" + rec.ID,
		Attributes: map[string]string{
			"invocation": rec.ID, "code": fmt.Sprintf("PLX-%d", plxerr.PostHocReviewOverdue), "reviewDueAt": rec.ReviewDueAt.UTC().Format(time.RFC3339),
		},
		Params: map[string]string{
			"action": string(rec.Action), "environment": rec.Environment, "subject": rec.Subject, "invokedBy": rec.InvokedBy.Display,
			"reviewDueAt": rec.ReviewDueAt.UTC().Format(time.RFC3339),
		},
		Subject: rec.ID,
	})
	if err != nil {
		s.o.Log.WarnContext(ctx, "breakglass: the overdue alert could not be queued", slog.String("invocation", rec.ID), slog.Any("error", err))
	}
}
