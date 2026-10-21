// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package breakglass

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/prodchange"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Action is an emergency action.
type Action string

// The emergency actions (GOV-024). A function is disabled with a kill switch
// of scope function.
const (
	// ActionKillSwitch switches a plugin, page, action graph, data source or
	// function off for a channel's devices.
	ActionKillSwitch Action = "killSwitch"
	// ActionForcedRollback directs devices to an earlier release.
	ActionForcedRollback Action = "forcedRollback"
	// ActionRollback re-promotes the content of an earlier release.
	ActionRollback Action = "rollback"
)

// Review statuses.
const (
	// StatusOpen is a review that is due.
	StatusOpen = "open"
	// StatusReviewed is a review that was completed.
	StatusReviewed = "reviewed"
)

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// LimitsFunc returns the limits in force for an organisation, read in the
// transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID string) (limits.Set, error)

// Options configures a Service.
type Options struct {
	DB     *storage.DB
	Audit  *audit.Log
	IDs    IDs
	Limits LimitsFunc
	// Notifier receives "break-glass used" and "break-glass review overdue";
	// nil publishes nothing, and a notification that fails never fails an
	// emergency action.
	Notifier notify.Publisher
	// Log receives the failures that must not fail an action; nil uses the
	// default logger.
	Log *slog.Logger
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service records and reviews break-glass actions.
type Service struct {
	o   Options
	now func() time.Time
}

// New returns the service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Limits == nil {
		return nil, errors.New("breakglass: a database, an audit log, identifiers and limits are required")
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, now: now}, nil
}

// Invocation is what a caller says about an emergency action it is about to
// take.
type Invocation struct {
	AppID          string
	EnvironmentID  string
	EnvironmentKey string
	// Production is whether the environment is a production one.
	Production bool
	Action     Action
	// Subject names what the action changes, as the approval engine would.
	Subject string
	// ContentHash is the SHA-256, in hex, of the content the action changes
	// to; it is what the compliance report lists.
	ContentHash   string
	Justification string
}

// Record is a stored invocation and its review.
type Record struct {
	ID             string
	OrganizationID string
	AppID          string
	EnvironmentID  string
	Environment    string
	Production     bool
	Action         Action
	Subject        string
	Justification  string
	InvokedBy      audit.Actor
	InvokedAt      time.Time
	ReviewDueAt    time.Time
	ReviewStatus   string
	ReviewedByID   string
	ReviewedBy     string
	ReviewedAt     time.Time
	ReviewComment  string
	// Overdue is whether the review is open past its due time (PLX-8006).
	Overdue bool
}

// Invoke records an emergency action in the caller's transaction, which is
// the transaction of the action itself. The principal needs
// breakglass.invoke on the environment; the justification is required
// (PLX-8005) and bounded by governance.justificationLength. The returned
// record is for Alert, which the caller calls after its transaction commits.
func (s *Service) Invoke(ctx context.Context, tx pgx.Tx, p auth.Principal, in Invocation) (Record, error) {
	if err := p.AuthorizeEnvironment(auth.BreakglassInvoke, in.AppID, in.EnvironmentKey); err != nil {
		return Record{}, fmt.Errorf("breakglass: %w", err)
	}
	set, err := s.o.Limits(ctx, tx, p.OrganizationID)
	if err != nil {
		return Record{}, fmt.Errorf("breakglass: read the limits: %w", err)
	}
	in.Justification = strings.TrimSpace(in.Justification)
	if err := checkJustification(in.Justification, set.Get(limits.GovernanceJustificationLength)); err != nil {
		return Record{}, err
	}
	now := s.now()
	due := now.Add(time.Duration(set.Get(limits.GovernanceBreakGlassReviewHours)) * time.Hour)
	row, err := s.insert(ctx, tx, p, in, now, due)
	if err != nil {
		return Record{}, err
	}
	entry, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.BreakGlassInvoked, TargetKind: "break_glass", TargetID: storage.ID(row.ID),
		AfterHash: in.ContentHash, Detail: fmt.Sprintf("%s: %s; justification: %s", in.Action, in.Subject, in.Justification),
	})
	if err != nil {
		return Record{}, fmt.Errorf("breakglass: %w", err)
	}
	if err := s.enterInLedger(ctx, tx, p, in, storage.ID(row.ID), now); err != nil {
		return Record{}, err
	}
	s.o.Log.InfoContext(ctx, "breakglass: an emergency action was taken", slog.String("invocation", storage.ID(row.ID)), slog.Int64("audit", entry.Sequence))
	return recordOf(row, now), nil
}

// checkJustification refuses an empty or over-long justification.
func checkJustification(text string, longest int64) error {
	if text == "" {
		return plxerr.New(plxerr.BreakGlassJustificationRequired, "break-glass needs a written justification")
	}
	if int64(utf8.RuneCountInString(text)) > longest {
		return plxerr.New(plxerr.LimitExceeded, "a break-glass justification is at most %d characters (governance.justificationLength)", longest)
	}
	return nil
}

// insert stores the invocation and its open review.
func (s *Service) insert(ctx context.Context, tx pgx.Tx, p auth.Principal, in Invocation, now, due time.Time) (dbgen.BreakGlassInvocation, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return dbgen.BreakGlassInvocation{}, fmt.Errorf("breakglass: %w", err)
	}
	actor := p.Actor()
	row, err := dbgen.New(tx).InsertBreakGlass(ctx, dbgen.InsertBreakGlassParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), AppID: storage.MustUUID(in.AppID),
		EnvironmentID: storage.MustUUID(in.EnvironmentID), EnvironmentKey: in.EnvironmentKey, Production: in.Production,
		Action: string(in.Action), Subject: in.Subject, Justification: in.Justification,
		InvokedByKind: actor.Kind, InvokedByID: actor.ID, InvokedBy: actor.Display, InvokedAt: storage.Timestamp(now), ReviewDueAt: storage.Timestamp(due),
	})
	if err != nil {
		return dbgen.BreakGlassInvocation{}, fmt.Errorf("breakglass: store the invocation: %w", err)
	}
	return row, nil
}

// enterInLedger records the action as a production change when it was one.
func (s *Service) enterInLedger(ctx context.Context, tx pgx.Tx, p auth.Principal, in Invocation, id string, at time.Time) error {
	if !in.Production {
		return nil
	}
	err := prodchange.Record(ctx, tx, s.o.IDs, prodchange.Change{
		OrganizationID: p.OrganizationID, AppID: in.AppID, EnvironmentID: in.EnvironmentID, EnvironmentKey: in.EnvironmentKey,
		Resource: "breakglass", Action: string(in.Action), Subject: in.Subject, ContentHash: in.ContentHash, Summary: "break-glass: " + in.Justification,
		By: p.Actor(), At: at, BreakGlassID: id,
	})
	if err != nil {
		return fmt.Errorf("breakglass: %w", err)
	}
	return nil
}

// Alert tells the people subscribed to "break-glass used", the approvers among
// them, that an emergency action was taken. It is called once the action's
// transaction has committed; a notification that cannot be queued is logged
// and never undoes the action (GOV-024).
func (s *Service) Alert(ctx context.Context, rec Record) {
	if s.o.Notifier == nil {
		return
	}
	err := s.o.Notifier.Publish(ctx, notify.Event{
		Type: notify.BreakGlassUsed, OrganizationID: rec.OrganizationID, AppID: rec.AppID,
		Title: fmt.Sprintf("Break-glass used: %s on %s", rec.Action, rec.Environment),
		Body:  fmt.Sprintf("%s by %s: %s Review due %s.", rec.Subject, rec.InvokedBy.Display, rec.Justification, rec.ReviewDueAt.UTC().Format(time.RFC3339)),
		Link:  "/breakglass/" + rec.ID,
		Attributes: map[string]string{
			"invocation": rec.ID, "action": string(rec.Action), "environment": rec.Environment, "reviewDueAt": rec.ReviewDueAt.UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		s.o.Log.WarnContext(ctx, "breakglass: the alert could not be queued", slog.String("invocation", rec.ID), slog.Any("error", err))
	}
}

// recordOf converts a stored invocation, working out whether its review is
// overdue at the instant now.
func recordOf(r dbgen.BreakGlassInvocation, now time.Time) Record {
	rec := Record{
		ID: storage.ID(r.ID), OrganizationID: storage.ID(r.OrganizationID), AppID: storage.ID(r.AppID), EnvironmentID: storage.ID(r.EnvironmentID),
		Environment: r.EnvironmentKey, Production: r.Production, Action: Action(r.Action), Subject: r.Subject, Justification: r.Justification,
		InvokedBy: audit.Actor{Kind: r.InvokedByKind, ID: r.InvokedByID, Display: r.InvokedBy}, InvokedAt: r.InvokedAt.Time, ReviewDueAt: r.ReviewDueAt.Time,
		ReviewStatus: r.ReviewStatus, ReviewedByID: r.ReviewedByID, ReviewedBy: r.ReviewedBy, ReviewedAt: r.ReviewedAt.Time, ReviewComment: r.ReviewComment,
	}
	rec.Overdue = rec.ReviewStatus == StatusOpen && !now.Before(rec.ReviewDueAt)
	return rec
}

// inOrg runs f in a transaction of the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}
