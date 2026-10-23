// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package breakglass_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/breakglass"
	"github.com/nightCode42/plux3/backend/internal/governancetest"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/prodchange"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// notices records the events the service publishes.
type notices struct{ events []notify.Event }

// Publish records an event.
func (n *notices) Publish(_ context.Context, e notify.Event) error {
	n.events = append(n.events, e)
	return nil
}

// fixture is a world with the break-glass service on its clock.
type fixture struct {
	*governancetest.World
	svc     *breakglass.Service
	notices *notices
}

// newFixture builds the world and the service; reviewHours overrides the
// review window when it is not zero.
func newFixture(t *testing.T, reviewHours int64) *fixture {
	t.Helper()
	w := governancetest.New(t)
	n := &notices{}
	lim := func(ctx context.Context, tx pgx.Tx, org string) (limits.Set, error) {
		set, err := w.Limits(ctx, tx, org)
		if err != nil || reviewHours == 0 {
			return set, err
		}
		return set.Tighten(limits.GovernanceBreakGlassReviewHours, limits.ScopeOrganization, reviewHours) //nolint:wrapcheck // a test
	}
	svc, err := breakglass.New(breakglass.Options{DB: w.DB, Audit: w.Log, IDs: w.Gen, Limits: lim, Notifier: n, Now: w.Clock})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{World: w, svc: svc, notices: n}
}

// invocation is a kill switch of a plugin in production.
func (f *fixture) invocation(justification string) breakglass.Invocation {
	return breakglass.Invocation{
		AppID: f.App, EnvironmentID: f.Envs["production"], EnvironmentKey: "production", Production: true,
		Action: breakglass.ActionKillSwitch, Subject: "kill switch plugin loans on production", ContentHash: strings.Repeat("a1", 32),
		Justification: justification,
	}
}

// invoke takes the action as p in a transaction of the organisation.
func (f *fixture) invoke(t *testing.T, p auth.Principal, in breakglass.Invocation) (breakglass.Record, error) {
	t.Helper()
	var rec breakglass.Record
	err := f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rec, err = f.svc.Invoke(ctx, tx, p, in)
		return err
	})
	return rec, err
}

// wantCode checks the code of a refusal.
func wantCode(t *testing.T, err error, want plxerr.Code, what string) {
	t.Helper()
	if code, ok := plxerr.CodeOf(err); !ok || code != want {
		t.Fatalf("%s: %v, want code %d", what, err, want)
	}
}

// Verifies: GOV-024.
// A written justification is required, and bounded; nothing is recorded for
// a refused action.
func TestInvoke_JustificationRequired_GOV_024(t *testing.T) {
	f := newFixture(t, 0)
	for _, text := range []string{"", "   \n\t"} {
		_, err := f.invoke(t, f.Manager, f.invocation(text))
		wantCode(t, err, plxerr.BreakGlassJustificationRequired, "an empty justification")
	}
	tooLong := strings.Repeat("x", int(limits.Defaults().Get(limits.GovernanceJustificationLength))+1)
	_, err := f.invoke(t, f.Manager, f.invocation(tooLong))
	wantCode(t, err, plxerr.LimitExceeded, "an over-long justification")
	list, err := f.svc.List(context.Background(), f.Owner, "", storage.Cursor{}, 10)
	if err != nil || len(list) != 0 {
		t.Fatalf("refused actions left %d invocations: %v", len(list), err)
	}
}

// Verifies: GOV-024, SEC-103, GOV-006.
// Only a principal who holds breakglass.invoke on the environment may invoke;
// the ordinary authorisation, with its restrictions, decides it.
func TestInvoke_PermissionRequired_GOV_024(t *testing.T) {
	f := newFixture(t, 0)
	for name, p := range map[string]auth.Principal{"viewer": f.Viewer, "reviewer": f.Reviewer} {
		_, err := f.invoke(t, p, f.invocation("the fleet is down"))
		wantCode(t, err, plxerr.PermissionDenied, name)
	}
	if _, err := f.invoke(t, f.Manager, f.invocation("the fleet is down")); err != nil {
		t.Fatalf("the release manager: %v", err)
	}
}

// Verifies: GOV-024, GOV-026.
// An invocation is recorded with its justification, a review due in 24 hours
// by default, an audit entry and, in production, a ledger row that names it.
func TestInvoke_RecordsReviewAuditAndLedger_GOV_024(t *testing.T) {
	f := newFixture(t, 0)
	rec, err := f.invoke(t, f.Manager, f.invocation("  the fleet is down  "))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Justification != "the fleet is down" || rec.ReviewStatus != breakglass.StatusOpen || rec.Overdue {
		t.Errorf("record = %+v", rec)
	}
	if want := f.Now.Add(24 * time.Hour); !rec.ReviewDueAt.Equal(want) {
		t.Errorf("review due %s, want %s", rec.ReviewDueAt, want)
	}
	if rows := f.Audit(t, audit.BreakGlassInvoked); len(rows) != 1 || rows[0].TargetID != rec.ID || !strings.Contains(rows[0].Detail, "the fleet is down") {
		t.Errorf("audit entries = %+v", rows)
	}
	var changes int
	_ = f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		rows, err := prodchange.Between(ctx, tx, f.Now.Add(-time.Hour), f.Now.Add(time.Hour), 10)
		for _, r := range rows {
			if storage.ID(r.BreakGlassID) == rec.ID && r.Resource == "breakglass" {
				changes++
			}
		}
		return err
	})
	if changes != 1 {
		t.Errorf("ledger rows for the invocation = %d, want 1", changes)
	}
	dev := f.invocation("test the drill")
	dev.EnvironmentID, dev.EnvironmentKey, dev.Production = f.Envs["development"], "development", false
	if _, err := f.invoke(t, f.Manager, dev); err != nil {
		t.Fatal(err)
	}
	_ = f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		rows, _ := prodchange.Between(ctx, tx, f.Now.Add(-time.Hour), f.Now.Add(time.Hour), 10)
		if len(rows) != 1 {
			t.Errorf("an action in development made %d ledger rows in all, want 1", len(rows))
		}
		return nil
	})
}

// Verifies: GOV-024.
// The review window is configurable through the limits registry.
func TestInvoke_ReviewWindowIsConfigurable_GOV_024(t *testing.T) {
	f := newFixture(t, 4)
	rec, err := f.invoke(t, f.Manager, f.invocation("urgent"))
	if err != nil {
		t.Fatal(err)
	}
	if want := f.Now.Add(4 * time.Hour); !rec.ReviewDueAt.Equal(want) {
		t.Errorf("review due %s, want %s", rec.ReviewDueAt, want)
	}
}

// Verifies: GOV-024.
// Approvers are alerted at once: Alert publishes "break-glass used" with the
// justification, the action and when the review is due, and a notification
// that fails never fails the action.
func TestAlert_ApproversAreNotified_GOV_024(t *testing.T) {
	f := newFixture(t, 0)
	rec, err := f.invoke(t, f.Manager, f.invocation("the fleet is down"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.notices.events) != 0 {
		t.Fatal("the service announced before the action committed")
	}
	f.svc.Alert(context.Background(), rec)
	if len(f.notices.events) != 1 {
		t.Fatalf("events = %+v", f.notices.events)
	}
	e := f.notices.events[0]
	if e.Type != notify.BreakGlassUsed || e.OrganizationID != f.Org || !strings.Contains(e.Body, "the fleet is down") ||
		e.Attributes["invocation"] != rec.ID || e.Attributes["action"] != "killSwitch" || e.Attributes["reviewDueAt"] == "" {
		t.Errorf("event = %+v", e)
	}
	silent, err := breakglass.New(breakglass.Options{DB: f.DB, Audit: f.Log, IDs: f.Gen, Limits: f.Limits})
	if err != nil {
		t.Fatal(err)
	}
	silent.Alert(context.Background(), rec)
}

// Verifies: GOV-024.
// A review is completed by someone else, once; the invoker, a person without
// approvals.decide and an unknown identifier are refused.
func TestReview_FourEyes_GOV_024(t *testing.T) {
	f := newFixture(t, 0)
	rec, err := f.invoke(t, f.Manager, f.invocation("the fleet is down"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = f.svc.Review(ctx, f.Manager, rec.ID, "fine")
	wantCode(t, err, plxerr.ApproverNotEligible, "the invoker")
	_, err = f.svc.Review(ctx, f.Viewer, rec.ID, "fine")
	wantCode(t, err, plxerr.PermissionDenied, "a viewer")
	_, err = f.svc.Review(ctx, f.Reviewer, "not-an-id", "fine")
	wantCode(t, err, plxerr.InvalidFormat, "a malformed identifier")
	_, err = f.svc.Review(ctx, f.Reviewer, f.NewID(t), "fine")
	wantCode(t, err, plxerr.ResourceNotFound, "an unknown identifier")

	done, err := f.svc.Review(ctx, f.Reviewer, rec.ID, "justified: the plugin crashed on launch")
	if err != nil || done.ReviewStatus != breakglass.StatusReviewed || done.ReviewedByID != f.Reviewer.UserID || done.Overdue {
		t.Fatalf("review = %+v, %v", done, err)
	}
	_, err = f.svc.Review(ctx, f.Owner, rec.ID, "again")
	wantCode(t, err, plxerr.PreconditionFailed, "a second review")
}

// Verifies: GOV-024.
// An action taken through an access token is the token owner's: the owner,
// signed in, cannot review it, although the token's ID is the actor recorded.
func TestReview_TokenInvokerRefused_GOV_024(t *testing.T) {
	f := newFixture(t, 0)
	viaToken := f.Manager
	viaToken.Kind, viaToken.ID = auth.KindToken, f.NewID(t)
	rec, err := f.invoke(t, viaToken, f.invocation("the fleet is down"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.InvokedBy.ID == f.Manager.UserID {
		t.Fatalf("the actor recorded is the person, not the token: %+v", rec.InvokedBy)
	}
	_, err = f.svc.Review(context.Background(), f.Manager, rec.ID, "fine")
	wantCode(t, err, plxerr.ApproverNotEligible, "the person behind the invoking token")
}

// Verifies: GOV-024.
// The list shows reviews newest first, narrowed by status, and marks an open
// review past its due time as overdue.
func TestList_StatusAndOverdue_GOV_024(t *testing.T) {
	f := newFixture(t, 0)
	first, err := f.invoke(t, f.Manager, f.invocation("one"))
	if err != nil {
		t.Fatal(err)
	}
	f.Now = f.Now.Add(time.Minute)
	second, err := f.invoke(t, f.Manager, f.invocation("two"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := f.svc.Review(ctx, f.Reviewer, first.ID, "ok"); err != nil {
		t.Fatal(err)
	}
	all, err := f.svc.List(ctx, f.Owner, "", storage.Cursor{}, 10)
	if err != nil || len(all) != 2 || all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatalf("list = %+v, %v", all, err)
	}
	open, _ := f.svc.List(ctx, f.Owner, breakglass.StatusOpen, storage.Cursor{}, 10)
	if len(open) != 1 || open[0].ID != second.ID || open[0].Overdue {
		t.Fatalf("open = %+v", open)
	}
	f.Now = f.Now.Add(25 * time.Hour)
	open, _ = f.svc.List(ctx, f.Owner, breakglass.StatusOpen, storage.Cursor{}, 10)
	if len(open) != 1 || !open[0].Overdue {
		t.Fatalf("a review 25 hours on is not overdue: %+v", open)
	}
	_, err = f.svc.List(ctx, f.Owner, "bogus", storage.Cursor{}, 10)
	wantCode(t, err, plxerr.InvalidEnumValue, "an unknown status")
	_, err = f.svc.List(ctx, f.Viewer, "", storage.Cursor{}, 10)
	wantCode(t, err, plxerr.PermissionDenied, "a viewer")
}

// Verifies: GOV-024.
// A review still open at its due time is escalated once, with an injected
// clock: an audit entry that names PLX-8006 and a critical notification;
// reviewed actions and actions not yet due are left alone.
func TestEscalate_OverdueReview_GOV_024(t *testing.T) {
	f := newFixture(t, 0)
	overdue, err := f.invoke(t, f.Manager, f.invocation("overdue"))
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := f.invoke(t, f.Manager, f.invocation("reviewed"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := f.svc.Review(ctx, f.Reviewer, reviewed.ID, "ok"); err != nil {
		t.Fatal(err)
	}
	f.Now = f.Now.Add(23*time.Hour + 59*time.Minute)
	if n, err := f.svc.Escalate(ctx, f.Org); err != nil || n != 0 {
		t.Fatalf("a minute before the due time: raised %d, %v", n, err)
	}
	f.Now = f.Now.Add(time.Minute)
	f.invokeLate(t)
	if n, err := f.svc.Escalate(ctx, f.Org); err != nil || n != 1 {
		t.Fatalf("at the due time: raised %d, %v", n, err)
	}
	if n, err := f.svc.Escalate(ctx, f.Org); err != nil || n != 0 {
		t.Fatalf("a second run raised %d again, %v", n, err)
	}
	if len(f.notices.events) != 1 {
		t.Fatalf("events = %+v", f.notices.events)
	}
	e := f.notices.events[0]
	if e.Type != notify.BreakGlassOverdue || e.Attributes["code"] != "PLX-8006" || e.Attributes["invocation"] != overdue.ID {
		t.Errorf("event = %+v", e)
	}
	if rows := f.Audit(t, audit.BreakGlassOverdue); len(rows) != 1 || !strings.Contains(rows[0].Detail, "PLX-8006") || rows[0].TargetID != overdue.ID {
		t.Errorf("audit entries = %+v", rows)
	}
}

// invokeLate takes an action at the injected time, which is not yet due when
// the escalation runs.
func (f *fixture) invokeLate(t *testing.T) {
	t.Helper()
	if _, err := f.invoke(t, f.Manager, f.invocation("recent")); err != nil {
		t.Fatal(err)
	}
}
