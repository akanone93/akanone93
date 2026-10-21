// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package freeze_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/freeze"
	"github.com/nightCode42/plux3/backend/internal/governancetest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// calendar is a FreezeCalendar the test controls: a window named "closed" is in
// force for the whole of 2026-12-30, in every environment it names or none;
// windows it is told are invalid are refused.
type calendar struct {
	err   error
	asked []time.Time
}

// Validate implements governance.FreezeCalendar.
func (c *calendar) Validate(_ context.Context, w governance.Window) error {
	if w.TimeZone == "Mars/Olympus" {
		return fmt.Errorf("%w: unknown zone", governance.ErrInvalidWindow)
	}
	return nil
}

// InForce implements governance.FreezeCalendar.
func (c *calendar) InForce(_ context.Context, windows []governance.Window, env string, at time.Time) (governance.Occurrence, bool, error) {
	c.asked = append(c.asked, at)
	if c.err != nil {
		return governance.Occurrence{}, false, c.err
	}
	start := time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC)
	for _, w := range windows {
		if (w.EnvironmentKey == "" || w.EnvironmentKey == env) && !at.Before(start) && at.Before(start.Add(24*time.Hour)) {
			return governance.Occurrence{Window: w, Start: start, End: start.Add(24 * time.Hour)}, true, nil
		}
	}
	return governance.Occurrence{}, false, nil
}

// fixture is a world with the freeze service on its clock.
type fixture struct {
	*governancetest.World
	svc *freeze.Service
	cal *calendar
	ctx context.Context
}

// newFixture builds the world and a service whose licence lists the feature;
// windowsPerOrg and hours tighten the registry when they are not zero.
func newFixture(t *testing.T, windowsPerOrg, hours int64) *fixture {
	t.Helper()
	w := governancetest.New(t)
	cal := &calendar{}
	lim := func(ctx context.Context, tx pgx.Tx, org string) (limits.Set, error) {
		set, err := w.Limits(ctx, tx, org)
		if err == nil && windowsPerOrg > 0 {
			set, err = set.Tighten(limits.GovernanceFreezeWindowsPerOrg, limits.ScopeOrganization, windowsPerOrg)
		}
		if err == nil && hours > 0 {
			set, err = set.Tighten(limits.GovernanceFreezeWindowHours, limits.ScopeOrganization, hours)
		}
		return set, err //nolint:wrapcheck // a test
	}
	svc, err := freeze.New(freeze.Options{DB: w.DB, Audit: w.Log, IDs: w.Gen, Limits: lim, Calendar: cal, Now: w.Clock})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{World: w, svc: svc, cal: cal, ctx: governancetest.Licensed(context.Background(), edition.FeatureChangeFreezes)}
}

// window is a one-off window.
func window(name string) governance.Window {
	return governance.Window{Name: name, TimeZone: "Europe/Berlin", Start: "2026-12-30T00:00", Duration: 24 * time.Hour, Reason: "month-end close"}
}

// check asks the guard in a transaction of the organisation.
func (f *fixture) check(ctx context.Context, env string) error {
	return f.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error { return f.svc.Check(ctx, tx, env) })
}

// wantCode checks the code of a refusal.
func wantCode(t *testing.T, err error, want plxerr.Code, what string) {
	t.Helper()
	if code, ok := plxerr.CodeOf(err); !ok || code != want {
		t.Fatalf("%s: %v, want code %d", what, err, want)
	}
}

// Verifies: GOV-025.
// Inside a window a production change is refused with PLX-8010, naming the
// window and when it ends; outside it, and for another environment's window,
// it is not.
func TestCheck_RefusesInsideAWindow_GOV_025(t *testing.T) {
	f := newFixture(t, 0, 0)
	w := window("month-end")
	w.EnvironmentKey = "production"
	if _, err := f.svc.Put(f.ctx, f.Owner, w); err != nil {
		t.Fatal(err)
	}
	other := window("staging-close")
	other.EnvironmentKey = "staging"
	if _, err := f.svc.Put(f.ctx, f.Owner, other); err != nil {
		t.Fatal(err)
	}

	f.Now = time.Date(2026, 12, 29, 23, 59, 0, 0, time.UTC)
	if err := f.check(f.ctx, "production"); err != nil {
		t.Fatalf("before the window: %v", err)
	}
	f.Now = time.Date(2026, 12, 30, 9, 0, 0, 0, time.UTC)
	err := f.check(f.ctx, "production")
	wantCode(t, err, plxerr.ChangeFreezeActive, "inside the window")
	var e *plxerr.Error
	if !errors.As(err, &e) || e.Details["window"] != "month-end" || e.Details["until"] != "2026-12-31T00:00:00Z" || !strings.Contains(e.Message, "break-glass") || !strings.Contains(e.Message, "month-end close") {
		t.Errorf("the refusal = %+v", e)
	}
	if err := f.check(f.ctx, "qa"); err != nil {
		t.Fatalf("an environment no window names: %v", err)
	}
	f.Now = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	if err := f.check(f.ctx, "production"); err != nil {
		t.Fatalf("at the end of the window: %v", err)
	}
}

// Verifies: GOV-025, GOV-030.
// A community installation neither stores nor enforces windows; a lapsed
// licence keeps enforcing and reading them but not writing.
func TestCheck_Licence_GOV_030(t *testing.T) {
	f := newFixture(t, 0, 0)
	if _, err := f.svc.Put(f.ctx, f.Owner, window("close")); err != nil {
		t.Fatal(err)
	}
	f.Now = time.Date(2026, 12, 30, 9, 0, 0, 0, time.UTC)

	community := context.Background()
	if err := f.check(community, "production"); err != nil {
		t.Fatalf("a community installation enforced a window: %v", err)
	}
	_, err := f.svc.Put(community, f.Owner, window("other"))
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "writing without a licence")
	_, err = f.svc.List(community, f.Owner)
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "reading without a licence")
	wantCode(t, f.svc.Delete(community, f.Owner, f.NewID(t)), plxerr.EnterpriseFeatureNotLicensed, "deleting without a licence")

	expired := edition.WithProvider(context.Background(), expiredLicence{})
	wantCode(t, f.check(expired, "production"), plxerr.ChangeFreezeActive, "an expired licence still enforces")
	if list, err := f.svc.List(expired, f.Owner); err != nil || len(list) != 1 {
		t.Errorf("an expired licence reads: %v, %v", list, err)
	}
	_, err = f.svc.Put(expired, f.Owner, window("other"))
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "an expired licence writing")
}

// expiredLicence lists the feature but is past its grace period.
type expiredLicence struct{}

// Status returns an expired licence.
func (expiredLicence) Status() edition.Status {
	return edition.Status{State: edition.StateExpired, Features: []edition.Feature{edition.FeatureChangeFreezes}}
}

// Verifies: GOV-025.
// A calendar that cannot decide fails the change; a freeze is never silently
// lifted.
func TestCheck_CalendarFailureFailsClosed_GOV_025(t *testing.T) {
	f := newFixture(t, 0, 0)
	if _, err := f.svc.Put(f.ctx, f.Owner, window("close")); err != nil {
		t.Fatal(err)
	}
	f.cal.err = errors.New("the zone database is missing")
	err := f.check(f.ctx, "production")
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("a calendar failure let the change through: %v", err)
	}
	if code, ok := plxerr.CodeOf(err); ok && code == plxerr.ChangeFreezeActive {
		t.Fatalf("a calendar failure was reported as a freeze: %v", err)
	}
}

// Verifies: GOV-025.
// With no window stored the calendar is not asked.
func TestCheck_NoWindowsNoQuestion_GOV_025(t *testing.T) {
	f := newFixture(t, 0, 0)
	if err := f.check(f.ctx, "production"); err != nil || len(f.cal.asked) != 0 {
		t.Fatalf("err = %v, asked %d times", err, len(f.cal.asked))
	}
}

// Verifies: GOV-025.
// Writing a window needs freezes.manage; the calendar checks it, a duration
// beyond governance.freezeWindowHours and a window beyond
// governance.freezeWindowsPerOrg are refused, and a window of the same name is
// replaced.
func TestPut_PermissionCalendarLimitsAndUpsert_GOV_025(t *testing.T) {
	f := newFixture(t, 2, 48)
	_, err := f.svc.Put(f.ctx, f.Viewer, window("close"))
	wantCode(t, err, plxerr.PermissionDenied, "a viewer")
	_, err = f.svc.Put(f.ctx, f.Reviewer, window("close"))
	wantCode(t, err, plxerr.PermissionDenied, "a reviewer")

	bad := window("close")
	bad.TimeZone = "Mars/Olympus"
	_, err = f.svc.Put(f.ctx, f.Owner, bad)
	wantCode(t, err, plxerr.InvalidFormat, "a window the calendar refuses")
	for name, mutate := range map[string]func(*governance.Window){
		"no name":       func(w *governance.Window) { w.Name = "" },
		"a long name":   func(w *governance.Window) { w.Name = strings.Repeat("n", 101) },
		"a long reason": func(w *governance.Window) { w.Reason = strings.Repeat("r", 501) },
		"odd seconds":   func(w *governance.Window) { w.Duration = 90 * time.Second },
		"too long":      func(w *governance.Window) { w.Duration = 49 * time.Hour },
	} {
		w := window("limits")
		mutate(&w)
		if _, err := f.svc.Put(f.ctx, f.Owner, w); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	longest := window("limits")
	longest.Duration = 48 * time.Hour
	if _, err := f.svc.Put(f.ctx, f.Owner, longest); err != nil {
		t.Errorf("a window of exactly governance.freezeWindowHours: %v", err)
	}

	first, err := f.svc.Put(f.ctx, f.Owner, window("a"))
	if err != nil {
		t.Fatal(err)
	}
	replaced := window("a")
	replaced.Reason = "changed"
	again, err := f.svc.Put(f.ctx, f.Owner, replaced)
	if err != nil || again.ID != first.ID || again.Reason != "changed" {
		t.Fatalf("replacing a window: %+v, %v", again, err)
	}
	_, err = f.svc.Put(f.ctx, f.Owner, window("b"))
	wantCode(t, err, plxerr.LimitExceeded, "a window beyond governance.freezeWindowsPerOrg")
}

// Verifies: GOV-025.
// Listing needs organization.read; deleting a window needs freezes.manage;
// every write is audited.
func TestListDeleteAndAudit_GOV_025(t *testing.T) {
	f := newFixture(t, 0, 0)
	w, err := f.svc.Put(f.ctx, f.Owner, window("close"))
	if err != nil {
		t.Fatal(err)
	}
	if list, err := f.svc.List(f.ctx, f.Viewer); err != nil || len(list) != 1 || list[0].ID != w.ID || list[0].Duration != 24*time.Hour {
		t.Fatalf("list = %+v, %v", list, err)
	}
	wantCode(t, f.svc.Delete(f.ctx, f.Viewer, w.ID), plxerr.PermissionDenied, "a viewer deleting")
	wantCode(t, f.svc.Delete(f.ctx, f.Owner, "nope"), plxerr.InvalidFormat, "a malformed identifier")
	wantCode(t, f.svc.Delete(f.ctx, f.Owner, f.NewID(t)), plxerr.ResourceNotFound, "an unknown window")
	if err := f.svc.Delete(f.ctx, f.Owner, w.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.svc.List(f.ctx, f.Owner); len(list) != 0 {
		t.Errorf("the window is still listed: %+v", list)
	}
	if rows := f.Audit(t, audit.FreezeSet); len(rows) != 1 || rows[0].TargetID != w.ID {
		t.Errorf("set entries = %+v", rows)
	}
	if rows := f.Audit(t, audit.FreezeDeleted); len(rows) != 1 || rows[0].TargetID != w.ID || rows[0].Detail != "close" {
		t.Errorf("deleted entries = %+v", rows)
	}
}

// Verifies: GOV-025, GOV-030.
// The community calendar accepts no window: a server built without the
// enterprise edition refuses with PLX-8600.
func TestCommunityCalendarAcceptsNothing_GOV_025(t *testing.T) {
	w := governancetest.New(t)
	svc, err := freeze.New(freeze.Options{DB: w.DB, Audit: w.Log, IDs: w.Gen, Limits: w.Limits, Now: w.Clock})
	if err != nil {
		t.Fatal(err)
	}
	ctx := governancetest.Licensed(context.Background(), edition.FeatureChangeFreezes)
	_, err = svc.Put(ctx, w.Owner, window("close"))
	wantCode(t, err, plxerr.EnterpriseFeatureNotLicensed, "the community calendar")
	if err := w.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error { return svc.Check(ctx, tx, "production") }); err != nil {
		t.Fatalf("the community calendar held a change: %v", err)
	}
}

// Verifies: GOV-025.
// A nil guard holds nothing back.
func TestNilGuardHoldsNothing_GOV_025(t *testing.T) {
	t.Parallel()
	if err := freeze.Check(context.Background(), nil, nil, "production"); err != nil {
		t.Fatal(err)
	}
}
