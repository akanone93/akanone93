// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeAlerts answers the alert and error group calls from memory.
type fakeAlerts struct {
	pluxv1connect.UnimplementedAnalyticsServiceHandler
	calls *[]string
}

func (f fakeAlerts) record(s string) { *f.calls = append(*f.calls, s) }

func (f fakeAlerts) CreateAlertRule(_ context.Context, r *connect.Request[pluxv1.CreateAlertRuleRequest]) (*connect.Response[pluxv1.CreateAlertRuleResponse], error) {
	m := r.Msg.GetRule()
	f.record(fmt.Sprintf("create %s %s %s %s %g %d/%d %s filters=%v", m.GetAppId(), m.GetMetric(), m.GetAggregation(), m.GetComparison(),
		m.GetThreshold(), m.GetWindowSeconds(), m.GetForSeconds(), m.GetSeverity(), m.GetFilters()))
	m.Id = "r1"
	return connect.NewResponse(&pluxv1.CreateAlertRuleResponse{Rule: m}), nil
}

func (f fakeAlerts) UpdateAlertRule(_ context.Context, r *connect.Request[pluxv1.UpdateAlertRuleRequest]) (*connect.Response[pluxv1.UpdateAlertRuleResponse], error) {
	f.record("update " + r.Msg.GetRule().GetId())
	return connect.NewResponse(&pluxv1.UpdateAlertRuleResponse{Rule: r.Msg.GetRule()}), nil
}

func (f fakeAlerts) DeleteAlertRule(_ context.Context, r *connect.Request[pluxv1.DeleteAlertRuleRequest]) (*connect.Response[pluxv1.DeleteAlertRuleResponse], error) {
	f.record("delete " + r.Msg.GetId())
	if r.Msg.GetId() == "gone" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&pluxv1.DeleteAlertRuleResponse{}), nil
}

func (f fakeAlerts) ListAlertRules(context.Context, *connect.Request[pluxv1.ListAlertRulesRequest]) (*connect.Response[pluxv1.ListAlertRulesResponse], error) {
	return connect.NewResponse(&pluxv1.ListAlertRulesResponse{Rules: []*pluxv1.AlertRule{
		{Id: "r1", Name: "crashes", Metric: "fatal_error", Aggregation: "count", WindowSeconds: 300, Comparison: ">", Threshold: 5, Severity: "critical"},
	}}), nil
}

func (f fakeAlerts) ListAlertStates(_ context.Context, r *connect.Request[pluxv1.ListAlertStatesRequest]) (*connect.Response[pluxv1.ListAlertStatesResponse], error) {
	f.record("states " + r.Msg.GetState())
	v := 7.0
	return connect.NewResponse(&pluxv1.ListAlertStatesResponse{States: []*pluxv1.AlertState{
		{RuleId: "r1", RuleName: "crashes", State: "firing", Since: timestamppb.New(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)), Value: &v},
	}}), nil
}

func (f fakeAlerts) ListErrorGroups(_ context.Context, r *connect.Request[pluxv1.ListErrorGroupsRequest]) (*connect.Response[pluxv1.ListErrorGroupsResponse], error) {
	f.record(fmt.Sprintf("groups %s %d", r.Msg.GetAppId(), r.Msg.GetReleaseSequence()))
	return connect.NewResponse(&pluxv1.ListErrorGroupsResponse{Groups: []*pluxv1.ErrorGroup{errorGroupFixture()}}), nil
}

func (f fakeAlerts) GetErrorGroup(_ context.Context, r *connect.Request[pluxv1.GetErrorGroupRequest]) (*connect.Response[pluxv1.GetErrorGroupResponse], error) {
	f.record("group " + r.Msg.GetId())
	return connect.NewResponse(&pluxv1.GetErrorGroupResponse{
		Group:   errorGroupFixture(),
		Samples: []*pluxv1.ErrorSample{{EventId: "e1", OccurredAt: timestamppb.New(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)), FieldsJson: `{"code":"PLX-4101"}`}},
	}), nil
}

func errorGroupFixture() *pluxv1.ErrorGroup {
	at := timestamppb.New(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC))
	return &pluxv1.ErrorGroup{
		Id: "g1", AppId: "a1", ReleaseSequence: 7, Code: "PLX-4101", NodePath: "root/0", JsonPath: "/children/0",
		FirstSeen: at, LastSeen: at, Events: 4, Devices: 2, Trend: []*pluxv1.ErrorDay{{Day: "2026-10-18", Events: 4}},
	}
}

// Verifies: ANL-030, ANL-040.
// plux analytics alerts and errors send the flags as the rule and the query
// and print text or JSON.
func TestAnalyticsAlertsAndErrorsCommands_ANL_030_ANL_040(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_anl")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	var calls []string
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewAnalyticsServiceHandler(fakeAlerts{calls: &calls}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(group string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		full := append([]string{group, args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		code := e.analytics(full)
		return code, out.String(), errOut.String()
	}

	code, out, stderr := run("alerts", "create", "--app", "a1", "--name", "crashes", "--metric", "fatal_error", "--threshold", "5",
		"--window", "5m", "--for", "10m", "--severity", "critical", "--filter", "release=7")
	if code != exitOK || !strings.Contains(out, "r1\tcrashes\tcount(fatal_error) over 300s > 5 for 600s\tcritical") {
		t.Errorf("create: %d %q %s", code, out, stderr)
	}
	if code, out, _ = run("alerts", "update", "--app", "a1", "--name", "crashes", "--metric", "fatal_error", "--disabled", "r1"); code != exitOK || !strings.Contains(out, "(disabled)") {
		t.Errorf("update: %d %q", code, out)
	}
	if code, out, _ = run("alerts", "list"); code != exitOK || !strings.Contains(out, "r1\tcrashes") {
		t.Errorf("list: %d %q", code, out)
	}
	code, out, _ = run("alerts", "states", "--state", "firing", "--json")
	var states struct{ States []map[string]any }
	if err := json.Unmarshal([]byte(out), &states); code != exitOK || err != nil || len(states.States) != 1 || states.States[0]["value"] != 7.0 {
		t.Errorf("states --json: %d %v %q", code, err, out)
	}
	if code, out, _ = run("alerts", "delete", "r1"); code != exitOK || !strings.Contains(out, "Removed the rule r1") {
		t.Errorf("delete: %d %q", code, out)
	}
	if code, _, _ = run("alerts", "delete", "gone"); code != exitFailed {
		t.Errorf("deleting an unknown rule: %d", code)
	}
	if code, _, stderr = run("alerts", "create", "--app", "a1", "--filter", "release"); code != exitUsage && code != exitFailed || !strings.Contains(stderr, "dimension=value") {
		t.Errorf("a malformed filter: %d %q", code, stderr)
	}

	if code, out, _ = run("errors", "list", "--app", "a1", "--release", "7"); code != exitOK || !strings.Contains(out, "g1\tPLX-4101\trelease 7\t4 events\t2 devices") || !strings.Contains(out, "/children/0") {
		t.Errorf("errors list: %d %q", code, out)
	}
	if code, out, _ = run("errors", "show", "g1"); code != exitOK || !strings.Contains(out, "2026-10-18\t4 events") || !strings.Contains(out, "sample e1") {
		t.Errorf("errors show: %d %q", code, out)
	}
	if code, _, _ = run("errors", "list"); code == exitOK {
		t.Error("errors list without --app succeeded")
	}

	want := []string{
		"create a1 fatal_error count > 5 300/600 critical filters=map[release:7]", "update r1", "states firing", "delete r1", "delete gone",
		"groups a1 7", "group g1",
	}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %q\nwant    %q", calls, want)
	}
}
