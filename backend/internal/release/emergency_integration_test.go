// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/breakglass"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// switchGate is the approval gate the test turns on after the first release
// has gone to production.
type switchGate struct {
	on     bool
	engine *approval.Service
}

// Require implements approval.Gate.
func (g *switchGate) Require(ctx context.Context, tx pgx.Tx, arts ...approval.Artifact) error {
	if !g.on {
		return nil
	}
	return g.engine.Require(ctx, tx, arts...) //nolint:wrapcheck // a refusal for the caller
}

// stubFreezes is a change-freeze guard the test switches on and off; it
// records the environments it was asked about.
type stubFreezes struct {
	held  bool
	asked []string
}

// Check implements freeze.Guard.
func (s *stubFreezes) Check(_ context.Context, _ pgx.Tx, environmentKey string) error {
	s.asked = append(s.asked, environmentKey)
	if s.held {
		return plxerr.New(plxerr.ChangeFreezeActive, "the change freeze %q is in force", "month-end")
	}
	return nil
}

// alerts records the notifications of break-glass.
type alerts struct{ events []notify.Event }

// Publish records an event.
func (a *alerts) Publish(_ context.Context, e notify.Event) error {
	a.events = append(a.events, e)
	return nil
}

// emergencyFixture is a production environment that has a release, with the
// approval engine, a change-freeze guard and the break-glass service wired in.
type emergencyFixture struct {
	*fixture
	bg      *breakglass.Service
	engine  *approval.Service
	gate    *switchGate
	freezes *stubFreezes
	alerts  *alerts
	ref     release.ChannelRef
	prod    string
	first   int64
}

func newEmergencyFixture(t *testing.T) *emergencyFixture {
	t.Helper()
	e := &emergencyFixture{freezes: &stubFreezes{}, alerts: &alerts{}}
	e.fixture = newFixtureWith(t, "loan-calculator", limits.Set{}, func(o *release.Options) {
		var err error
		if e.engine, err = approval.New(approval.Options{DB: o.DB, Audit: o.Audit, IDs: o.IDs, Now: o.Now}); err != nil {
			t.Fatal(err)
		}
		e.engine.UseFreezes(e.freezes)
		e.engine.Use(func(context.Context, string, string) (settings.Profile, error) { return settings.Standard, nil }, nil)
		lim := func(ctx context.Context, tx pgx.Tx, org string) (limits.Set, error) {
			return o.Tenancy.Effective(ctx, tx, org, "", "") //nolint:wrapcheck // a test
		}
		if e.bg, err = breakglass.New(breakglass.Options{DB: o.DB, Audit: o.Audit, IDs: o.IDs, Limits: lim, Notifier: e.alerts, Now: o.Now}); err != nil {
			t.Fatal(err)
		}
		e.gate = &switchGate{engine: e.engine}
		o.Approvals, o.BreakGlass, o.Freezes = e.gate, e.bg, e.freezes
	})
	ctx := context.Background()
	// The first production release goes out before the approval policy of
	// the standard profile, which asks for one approver, is switched on.
	e.publish(t, "", false)
	e.publish(t, e.loans, false)
	first, _, err := e.rel.CreateRelease(ctx, e.owner, e.app, e.envs["development"].ID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	e.prod = e.envs["production"].ID
	if _, err := e.rel.PromoteRelease(ctx, e.owner, e.app, first.Sequence, e.prod, ""); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	e.first = first.Sequence
	e.gate.on = true
	e.ref = release.ChannelRef{AppID: e.app, EnvironmentID: e.prod, ChannelKey: "production"}
	return e
}

// invocations lists the emergency actions of the organisation.
func (e *emergencyFixture) invocations(t *testing.T) []breakglass.Record {
	t.Helper()
	recs, err := e.bg.List(context.Background(), e.owner, "", storage.Cursor{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

// version is the newest version of the production channel's control document.
func (e *emergencyFixture) version(t *testing.T) int64 {
	t.Helper()
	state, err := e.rel.ControlDocument(context.Background(), e.owner, e.ref)
	if err != nil {
		t.Fatal(err)
	}
	return state.Version
}

// Verifies: GOV-024, GOV-025.
// During a change freeze an ordinary kill switch in production is refused
// with PLX-8010; through break-glass it is made, without asking the freeze
// guard, recorded with its justification and announced once to the
// approvers; without a justification it is refused with PLX-8005 and
// changes nothing.
func TestKillSwitch_FrozenThenBreakGlass_GOV_025(t *testing.T) {
	e := newEmergencyFixture(t)
	ctx := context.Background()
	k := controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: "loans"}
	e.freezes.held = true
	before := e.version(t)

	if _, err := e.rel.SetKillSwitch(ctx, e.owner, e.ref, k); code(err) != plxerr.ChangeFreezeActive {
		t.Fatalf("ordinary kill switch in a freeze: %v, want PLX-8010", err)
	}
	asked := len(e.freezes.asked)
	if _, err := e.rel.SetKillSwitch(ctx, e.owner, e.ref, k, release.WithBreakGlass(" ")); code(err) != plxerr.BreakGlassJustificationRequired {
		t.Fatalf("break-glass without a justification: %v, want PLX-8005", err)
	}
	if e.version(t) != before || len(e.alerts.events) != 0 || len(e.invocations(t)) != 0 {
		t.Fatal("a refused action changed the document, alerted or was recorded")
	}

	state, err := e.rel.SetKillSwitch(ctx, e.owner, e.ref, k, release.WithBreakGlass("loans crashes on launch"))
	if err != nil {
		t.Fatalf("break-glass in a freeze: %v", err)
	}
	if len(state.KillSwitches) != 1 || state.KillSwitches[0] != k || state.Version != before+1 {
		t.Errorf("state = %+v", state)
	}
	if len(e.freezes.asked) != asked {
		t.Error("break-glass asked the freeze guard")
	}
	recs := e.invocations(t)
	if len(recs) != 1 || recs[0].Action != breakglass.ActionKillSwitch || recs[0].Justification != "loans crashes on launch" || recs[0].Environment != "production" {
		t.Fatalf("invocations = %+v", recs)
	}
	if len(e.alerts.events) != 1 || e.alerts.events[0].Type != notify.BreakGlassUsed || e.alerts.events[0].Attributes["invocation"] != recs[0].ID {
		t.Errorf("alerts = %+v", e.alerts.events)
	}
}

// Verifies: GOV-024.
// A function is disabled in an emergency with a kill switch of scope
// function, and clearing a switch is an emergency action too.
func TestFunctionDisableAndClear_BreakGlass_GOV_024(t *testing.T) {
	e := newEmergencyFixture(t)
	ctx := context.Background()
	e.freezes.held = true
	k := controldoc.KillSwitch{Scope: controldoc.ScopeFunction, Target: "orders.total"}
	if _, err := e.rel.SetKillSwitch(ctx, e.owner, e.ref, k, release.WithBreakGlass("orders.total double-charges")); err != nil {
		t.Fatalf("disable a function: %v", err)
	}
	if _, err := e.rel.ClearKillSwitch(ctx, e.owner, e.ref, k); code(err) != plxerr.ChangeFreezeActive {
		t.Fatalf("an ordinary clear in a freeze: %v", err)
	}
	state, err := e.rel.ClearKillSwitch(ctx, e.owner, e.ref, k, release.WithBreakGlass("the fix is deployed"))
	if err != nil || len(state.KillSwitches) != 0 {
		t.Fatalf("clear: %+v, %v", state, err)
	}
	if recs := e.invocations(t); len(recs) != 2 || len(e.alerts.events) != 2 {
		t.Errorf("invocations = %d, alerts = %d, want 2 each", len(recs), len(e.alerts.events))
	}
}

// Verifies: GOV-024, GOV-025.
// An ordinary rollback in production is held by a freeze and then by the
// approval policy; through break-glass it needs neither. The server's own
// revert of a failing rollout is never held.
func TestRollback_ApprovalFreezeAndBreakGlass_GOV_024(t *testing.T) {
	e := newEmergencyFixture(t)
	ctx := context.Background()

	e.freezes.held = true
	if _, err := e.rel.RollbackRelease(ctx, e.owner, e.app, e.prod, "", e.first, ""); code(err) != plxerr.ChangeFreezeActive {
		t.Fatalf("a rollback in a freeze: %v, want PLX-8010", err)
	}
	e.freezes.held = false
	if _, err := e.rel.RollbackRelease(ctx, e.owner, e.app, e.prod, "", e.first, ""); code(err) != plxerr.ApprovalRequired {
		t.Fatalf("a rollback the policy guards: %v, want PLX-8001", err)
	}

	e.freezes.held = true
	if _, err := e.rel.RollbackReleaseWith(ctx, e.owner, e.app, e.prod, "", e.first, "", release.WithBreakGlass("")); code(err) != plxerr.BreakGlassJustificationRequired {
		t.Fatalf("break-glass without a justification: %v", err)
	}
	back, err := e.rel.RollbackReleaseWith(ctx, e.owner, e.app, e.prod, "", e.first, "", release.WithBreakGlass("release 1 is the last known good"))
	if err != nil || back.Sequence <= e.first {
		t.Fatalf("break-glass rollback: %+v, %v", back, err)
	}
	if recs := e.invocations(t); len(recs) != 1 || recs[0].Action != breakglass.ActionRollback || len(e.alerts.events) != 1 {
		t.Fatalf("invocations = %+v, alerts = %d", recs, len(e.alerts.events))
	}

	revert, err := e.rel.RollbackRelease(ctx, auth.System(e.org), e.app, e.prod, "", e.first, "revert of a failing rollout")
	if err != nil || revert.Sequence <= back.Sequence {
		t.Fatalf("the server's own revert: %+v, %v", revert, err)
	}
}

// Verifies: GOV-024, GOV-025.
// A forced rollback follows the same rule: held in a freeze, allowed through
// break-glass.
func TestForcedRollback_FrozenThenBreakGlass_GOV_025(t *testing.T) {
	e := newEmergencyFixture(t)
	ctx := context.Background()
	e.freezes.held = true
	back, err := e.rel.RollbackReleaseWith(ctx, e.owner, e.app, e.prod, "", e.first, "", release.WithBreakGlass("get off release 1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.rel.SetForcedRollback(ctx, e.owner, e.ref, e.first); code(err) != plxerr.ChangeFreezeActive {
		t.Fatalf("an ordinary forced rollback in a freeze: %v", err)
	}
	state, err := e.rel.SetForcedRollback(ctx, e.owner, e.ref, e.first, release.WithBreakGlass("devices must leave release "+fmt.Sprint(back.Sequence)))
	if err != nil || state.ForcedRollback == nil || state.ForcedRollback.ReleaseSequence != uint64(e.first) { //nolint:gosec // positive
		t.Fatalf("forced rollback: %+v, %v", state, err)
	}
	state, err = e.rel.ClearForcedRollback(ctx, e.owner, e.ref, release.WithBreakGlass("devices are safe again"))
	if err != nil || state.ForcedRollback != nil {
		t.Fatalf("clear: %+v, %v", state, err)
	}
}

// Verifies: GOV-025.
// The freeze guard is asked for production only; a change to development is
// never held, and the legacy control call cannot go round a freeze.
func TestFreeze_ProductionOnlyAndLegacyControl_GOV_025(t *testing.T) {
	e := newEmergencyFixture(t)
	ctx := context.Background()
	e.freezes.held = true
	dev := release.ChannelRef{AppID: e.app, EnvironmentID: e.envs["development"].ID, ChannelKey: "production"}
	if _, err := e.rel.SetKillSwitch(ctx, e.owner, dev, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: "loans"}); err != nil {
		t.Fatalf("a change to development in a freeze: %v", err)
	}
	for _, env := range e.freezes.asked {
		if env != "production" {
			t.Errorf("the guard was asked about %q", env)
		}
	}
	_, err := e.rel.SetControl(ctx, e.owner, release.Control{AppID: e.app, EnvironmentID: e.prod, ChannelKey: "production", KillSwitchPlugins: []string{"loans"}})
	if code(err) != plxerr.ChangeFreezeActive {
		t.Fatalf("the legacy control call in a freeze: %v, want PLX-8010", err)
	}
}

// Verifies: GOV-024, SEC-103.
// Break-glass needs breakglass.invoke on top of the call's own permission:
// a principal without it is refused before anything is recorded, even with a
// justification.
func TestBreakGlass_NeedsPermission_GOV_024(t *testing.T) {
	e := newEmergencyFixture(t)
	ctx := context.Background()
	stripped := e.owner
	stripped.Permissions = nil
	for _, p := range auth.Permissions() {
		if p != auth.BreakglassInvoke {
			stripped.Permissions = append(stripped.Permissions, p)
		}
	}
	_, err := e.rel.SetKillSwitch(ctx, stripped, e.ref, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: "loans"}, release.WithBreakGlass("because"))
	if code(err) != plxerr.PermissionDenied {
		t.Fatalf("without breakglass.invoke: %v, want PermissionDenied", err)
	}
	if len(e.invocations(t)) != 0 || len(e.alerts.events) != 0 {
		t.Error("a refused invocation was recorded or announced")
	}
}
