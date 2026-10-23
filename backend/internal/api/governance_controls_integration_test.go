// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// Verifies: GOV-024, SEC-103.
// Through the API a break-glass kill switch needs break_glass and a
// justification together: a justification alone, and break_glass without one,
// are refused with an invalid argument; with both the action is recorded and
// shows in the list of emergency actions, whose reviewer must be someone else.
func TestBreakGlassThroughTheAPI_GOV_024(t *testing.T) {
	f := newMatrixFixture(t)
	ctx := context.Background()
	owner, reviewer := f.callers[auth.RoleOwner], f.callers[auth.RoleReviewer]
	kill := func(breakGlass bool, justification string) error {
		_, err := f.w.control.SetKillSwitch(ctx, req(owner, &pluxv1.SetKillSwitchRequest{
			AppId: f.app, EnvironmentId: f.prod, ChannelKey: f.channel, BreakGlass: breakGlass, Justification: justification,
			KillSwitch: &pluxv1.KillSwitchEntry{Scope: pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_PLUGIN, Target: "loans"},
		}))
		return err
	}
	if code := codeOf(kill(false, "because")); code != connect.CodeInvalidArgument {
		t.Errorf("a justification without break_glass: %v", code)
	}
	if code := codeOf(kill(true, "")); code != connect.CodeInvalidArgument {
		t.Errorf("break_glass without a justification: %v", code)
	}
	if list := must(f.w.breakGlass.ListBreakGlass(ctx, req(owner, &pluxv1.ListBreakGlassRequest{})))(t); len(list.GetInvocations()) != 0 {
		t.Fatalf("refused actions were recorded: %+v", list.GetInvocations())
	}
	if err := kill(true, "loans crashes on launch"); err != nil {
		t.Fatalf("break-glass kill switch: %v", err)
	}

	list := must(f.w.breakGlass.ListBreakGlass(ctx, req(owner, &pluxv1.ListBreakGlassRequest{Status: "open"})))(t)
	if len(list.GetInvocations()) != 1 {
		t.Fatalf("invocations = %+v", list.GetInvocations())
	}
	inv := list.GetInvocations()[0]
	if inv.GetAction() != "killSwitch" || inv.GetJustification() != "loans crashes on launch" || inv.GetEnvironment() != "production" || inv.GetOverdue() ||
		inv.GetReviewDueAt().AsTime().Sub(inv.GetInvokedAt().AsTime()) != 24*time.Hour {
		t.Errorf("invocation = %+v", inv)
	}

	_, err := f.w.breakGlass.ReviewBreakGlass(ctx, req(owner, &pluxv1.ReviewBreakGlassRequest{Id: inv.GetId(), Comment: "mine"}))
	if code := codeOf(err); code != connect.CodePermissionDenied {
		t.Errorf("the invoker reviewing their own action: %v", code)
	}
	reviewed := must(f.w.breakGlass.ReviewBreakGlass(ctx, req(reviewer, &pluxv1.ReviewBreakGlassRequest{Id: inv.GetId(), Comment: "justified"})))(t)
	if reviewed.GetInvocation().GetReviewStatus() != "reviewed" || reviewed.GetInvocation().GetReviewComment() != "justified" {
		t.Errorf("reviewed = %+v", reviewed.GetInvocation())
	}
}

// Verifies: GOV-025, GOV-026, GOV-030.
// Through the API, the enterprise controls answer as the server they run in:
// the community calendar refuses a window with PLX-8600, and a compliance
// report is stored unsigned, because the API role holds no key, and listed
// and fetched without content until the worker signs it.
func TestFreezeAndComplianceThroughTheAPI_GOV_026(t *testing.T) {
	f := newMatrixFixture(t)
	ctx := context.Background()
	owner := f.callers[auth.RoleOwner]

	_, err := f.w.freeze.PutFreeze(ctx, req(owner, &pluxv1.PutFreezeRequest{Freeze: &pluxv1.ChangeFreeze{
		Name: "close", TimeZone: "Europe/Berlin", Start: "2026-12-30T18:00", DurationMinutes: 60,
	}}))
	if code := codeOf(err); code != connect.CodePermissionDenied {
		t.Errorf("a window on the community calendar: %v, want PLX-8600", code)
	}
	if list := must(f.w.freeze.ListFreezes(ctx, req(owner, &pluxv1.ListFreezesRequest{})))(t); len(list.GetFreezes()) != 0 {
		t.Errorf("freezes = %+v", list.GetFreezes())
	}

	from, to := time.Now().Add(-time.Hour).UTC(), time.Now().Add(time.Hour).UTC()
	created := must(f.w.compliance.CreateComplianceReport(ctx, req(owner, &pluxv1.CreateComplianceReportRequest{From: timestamppb.New(from), To: timestamppb.New(to)})))(t)
	info := created.GetReport()
	if info.GetId() == "" || info.GetSigned() || !info.GetPeriodFrom().AsTime().Equal(from.Truncate(time.Microsecond)) {
		t.Fatalf("created = %+v", info)
	}
	got := must(f.w.compliance.GetComplianceReport(ctx, req(owner, &pluxv1.GetComplianceReportRequest{Id: info.GetId(), Format: "json"})))(t)
	if got.GetReport().GetSigned() || len(got.GetContent()) != 0 {
		t.Errorf("an unsigned report served content: %+v", got)
	}
	listed := must(f.w.compliance.ListComplianceReports(ctx, req(owner, &pluxv1.ListComplianceReportsRequest{})))(t)
	if len(listed.GetReports()) != 1 || listed.GetReports()[0].GetId() != info.GetId() {
		t.Errorf("reports = %+v", listed.GetReports())
	}
	_, err = f.w.compliance.CreateComplianceReport(ctx, req(owner, &pluxv1.CreateComplianceReportRequest{From: timestamppb.New(to), To: timestamppb.New(from)}))
	if code := codeOf(err); code != connect.CodeInvalidArgument {
		t.Errorf("a backwards period: %v", code)
	}
	_, err = f.w.compliance.CreateComplianceReport(ctx, req(owner, &pluxv1.CreateComplianceReportRequest{}))
	if code := codeOf(err); code != connect.CodeInvalidArgument {
		t.Errorf("no period: %v", code)
	}
}
