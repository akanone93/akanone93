// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
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

// fakeAnalytics records what the CLI asked and answers from memory.
type fakeAnalytics struct {
	pluxv1connect.UnimplementedAnalyticsServiceHandler
	calls *[]string
}

func (f fakeAnalytics) record(s string) { *f.calls = append(*f.calls, s) }

func (f fakeAnalytics) QueryRollups(_ context.Context, r *connect.Request[pluxv1.QueryRollupsRequest]) (*connect.Response[pluxv1.QueryRollupsResponse], error) {
	if r.Header().Get("X-Plux-Organization") != "org-1" || r.Header().Get("Authorization") != "Bearer plux_pat_anl" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	m := r.Msg
	f.record("query " + m.GetAppId() + " " + m.GetMetric() + " " + m.GetGranularity() + " " + strings.Join(m.GetGroupBy(), "+") + " token=" + m.GetPageToken())
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	if m.GetPageToken() == "" {
		return connect.NewResponse(&pluxv1.QueryRollupsResponse{
			Points: []*pluxv1.RollupPoint{{
				Bucket: timestamppb.New(at), Events: 12, P50: 40, P90: 90, P99: 120,
				Group: &pluxv1.RollupGroup{ReleaseSequence: 7, Page: "/loans"},
			}},
			NextPageToken: "p2",
		}), nil
	}
	return connect.NewResponse(&pluxv1.QueryRollupsResponse{Points: []*pluxv1.RollupPoint{{Bucket: timestamppb.New(at.Add(time.Hour)), Events: 3}}}), nil
}

func (f fakeAnalytics) GetRetention(context.Context, *connect.Request[pluxv1.GetRetentionRequest]) (*connect.Response[pluxv1.GetRetentionResponse], error) {
	return connect.NewResponse(&pluxv1.GetRetentionResponse{
		Settings: []*pluxv1.RetentionSetting{
			{DataClass: "audit_production", Days: 3653, IsDefault: true},
			{DataClass: "raw_events", Days: 396, IsDefault: true, Enforced: true},
			{DataClass: "rollups", Days: 0, Enforced: true},
		},
		Scheduled: []*pluxv1.RetentionChange{{Id: "c1", DataClass: "raw_events", Days: 90, State: "scheduled", ApplyAt: timestamppb.New(time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC))}},
	}), nil
}

func (f fakeAnalytics) SetRetention(_ context.Context, r *connect.Request[pluxv1.SetRetentionRequest]) (*connect.Response[pluxv1.SetRetentionResponse], error) {
	m := r.Msg
	f.record("set " + m.GetDataClass() + " " + string(rune('0'+m.GetDays()/10)) + string(rune('0'+m.GetDays()%10)) + " immediate=" + map[bool]string{true: "yes", false: "no"}[m.GetImmediate()])
	res := &pluxv1.SetRetentionResponse{Change: &pluxv1.RetentionChange{Id: "c2", DataClass: m.GetDataClass(), Days: m.GetDays(), State: "scheduled"}}
	if m.GetImmediate() {
		res.Change.State = "applied"
		return connect.NewResponse(res), nil
	}
	res.Diagnostics = []*pluxv1.Diagnostic{{Code: "PLX-8301", Message: "the retention of raw_events becomes 90 days at 2026-10-19T02:00:00Z, in the maintenance window"}}
	return connect.NewResponse(res), nil
}

func (f fakeAnalytics) ListRetentionChanges(_ context.Context, r *connect.Request[pluxv1.ListRetentionChangesRequest]) (*connect.Response[pluxv1.ListRetentionChangesResponse], error) {
	f.record("list scheduled=" + map[bool]string{true: "yes", false: "no"}[r.Msg.GetOnlyScheduled()])
	return connect.NewResponse(&pluxv1.ListRetentionChangesResponse{Changes: []*pluxv1.RetentionChange{
		{Id: "c1", DataClass: "raw_events", Days: 90, State: "scheduled"},
	}}), nil
}

func (f fakeAnalytics) CancelRetentionChange(_ context.Context, r *connect.Request[pluxv1.CancelRetentionChangeRequest]) (*connect.Response[pluxv1.CancelRetentionChangeResponse], error) {
	f.record("cancel " + r.Msg.GetId())
	if r.Msg.GetId() == "gone" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&pluxv1.CancelRetentionChangeResponse{Change: &pluxv1.RetentionChange{Id: r.Msg.GetId(), DataClass: "raw_events", State: "cancelled"}}), nil
}

func (f fakeAnalytics) GetExport(context.Context, *connect.Request[pluxv1.GetExportRequest]) (*connect.Response[pluxv1.GetExportResponse], error) {
	return connect.NewResponse(&pluxv1.GetExportResponse{Export: &pluxv1.Export{
		Endpoint: "https://otel.example/v1/metrics", Enabled: true, LastError: "PLX-8302 the endpoint did not answer",
	}}), nil
}

func (f fakeAnalytics) SetExport(_ context.Context, r *connect.Request[pluxv1.SetExportRequest]) (*connect.Response[pluxv1.SetExportResponse], error) {
	f.record("export " + r.Msg.GetEndpoint() + " enabled=" + map[bool]string{true: "yes", false: "no"}[r.Msg.GetEnabled()])
	return connect.NewResponse(&pluxv1.SetExportResponse{Export: &pluxv1.Export{Endpoint: r.Msg.GetEndpoint() + "/v1/metrics", Enabled: r.Msg.GetEnabled()}}), nil
}

func (f fakeAnalytics) DeleteExport(context.Context, *connect.Request[pluxv1.DeleteExportRequest]) (*connect.Response[pluxv1.DeleteExportResponse], error) {
	f.record("unexport")
	return connect.NewResponse(&pluxv1.DeleteExportResponse{}), nil
}

// Verifies: ANL-010, ANL-011.
// plux analytics calls the analytics service with the caller's credential
// and organisation, follows page tokens, and prints text or JSON.
func TestAnalyticsCommands_ANL_010(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_anl")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	var calls []string
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewAnalyticsServiceHandler(fakeAnalytics{calls: &calls}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		if len(args) > 1 {
			args = append([]string{args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		}
		code := e.analytics(args)
		return code, out.String(), errOut.String()
	}
	runSub := func(sub string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		full := append([]string{sub, args[0], "--server", srv.URL, "--org", "org-1"}, args[1:]...)
		code := e.analytics(full)
		return code, out.String(), errOut.String()
	}

	code, out, stderr := run("query", "--app", "a1", "--metric", "render_perf", "--granularity", "5m", "--group-by", "release, page",
		"--from", "2026-10-18T00:00:00Z", "--to", "2026-10-19T00:00:00Z")
	if code != exitOK || !strings.Contains(out, "12 events\tp50 40.0\tp90 90.0\tp99 120.0\trelease=7 page=/loans") || !strings.Contains(out, "3 events") {
		t.Errorf("query: %d %q %s", code, out, stderr)
	}
	code, out, _ = run("query", "--app", "a1", "--metric", "render_perf", "--json", "--since", "2h")
	var queried struct {
		Metric string
		Points []map[string]any
	}
	if err := json.Unmarshal([]byte(out), &queried); code != exitOK || err != nil || len(queried.Points) != 2 || queried.Metric != "render_perf" {
		t.Errorf("query --json: %d %v %s", code, err, out)
	}

	if code, out, _ = runSub("retention", "show"); code != exitOK ||
		!strings.Contains(out, "audit_production\t3653 days\t(recorded; nothing deletes it yet)") ||
		!strings.Contains(out, "raw_events\t396 days\t(default)") || !strings.Contains(out, "rollups\tindefinitely") ||
		!strings.Contains(out, "scheduled\tc1\traw_events\t90 days\tscheduled") {
		t.Errorf("retention show: %d %q", code, out)
	}
	if code, out, _ = runSub("retention", "set", "--class", "raw_events", "--days", "90"); code != exitOK ||
		!strings.Contains(out, "Set raw_events to 90 days (scheduled)") || !strings.Contains(out, "in the maintenance window") {
		t.Errorf("retention set: %d %q", code, out)
	}
	if code, out, _ = runSub("retention", "set", "--class", "rollups", "--days", "0", "--immediate"); code != exitOK || !strings.Contains(out, "Set rollups to indefinitely (applied)") {
		t.Errorf("retention set --immediate: %d %q", code, out)
	}
	if code, out, _ = runSub("retention", "list", "--scheduled"); code != exitOK || !strings.Contains(out, "c1\traw_events\t90 days\tscheduled") {
		t.Errorf("retention list: %d %q", code, out)
	}
	if code, out, _ = runSub("retention", "cancel", "c1"); code != exitOK || !strings.Contains(out, "Cancelled the change of raw_events") {
		t.Errorf("retention cancel: %d %q", code, out)
	}
	if code, _, _ = runSub("retention", "cancel", "gone"); code != exitFailed {
		t.Errorf("cancelling an unknown change: %d", code)
	}

	if code, out, _ = runSub("export", "show"); code != exitOK || !strings.Contains(out, "https://otel.example/v1/metrics\tenabled") || !strings.Contains(out, "last error: PLX-8302") {
		t.Errorf("export show: %d %q", code, out)
	}
	if code, out, _ = runSub("export", "set", "--endpoint", "https://otel.example", "--disabled"); code != exitOK || !strings.Contains(out, "disabled") {
		t.Errorf("export set: %d %q", code, out)
	}
	if code, out, _ = runSub("export", "delete"); code != exitOK || !strings.Contains(out, "Removed the OTLP destination") {
		t.Errorf("export delete: %d %q", code, out)
	}

	want := []string{
		"query a1 render_perf 5m release+page token=", "query a1 render_perf 5m release+page token=p2",
		"query a1 render_perf 1h  token=", "query a1 render_perf 1h  token=p2",
		"set raw_events 90 immediate=no", "set rollups 00 immediate=yes", "list scheduled=yes", "cancel c1", "cancel gone",
		"export https://otel.example enabled=no", "unexport",
	}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %q\nwant    %q", calls, want)
	}
}

// Verifies: ANL-010, CLI-007.
// plux analytics refuses a missing subcommand, flag or credential before
// or without calling the server.
func TestAnalyticsCommandsUsage_ANL_010(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_anl")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	for name, args := range map[string][]string{
		"no subcommand":          nil,
		"an unknown subcommand":  {"purge"},
		"retention without sub":  {"retention"},
		"export without sub":     {"export"},
		"a missing metric":       {"query", "--server", "https://x.example", "--app", "a1"},
		"a missing app":          {"query", "--server", "https://x.example", "--metric", "render_perf"},
		"a bad time":             {"query", "--server", "https://x.example", "--app", "a1", "--metric", "m", "--from", "yesterday"},
		"a reversed range":       {"query", "--server", "https://x.example", "--app", "a1", "--metric", "m", "--from", "2026-10-19T00:00:00Z", "--to", "2026-10-18T00:00:00Z"},
		"retention without days": {"retention", "set", "--server", "https://x.example", "--class", "rollups"},
		"export without url":     {"export", "set", "--server", "https://x.example"},
		"a missing id":           {"retention", "cancel", "--server", "https://x.example"},
	} {
		var errOut bytes.Buffer
		e := newEnv(&bytes.Buffer{}, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		if code := e.analytics(args); code != exitUsage {
			t.Errorf("%s: %d %s", name, code, errOut.String())
		}
	}
	t.Setenv(tokenEnv, "")
	var errOut bytes.Buffer
	e := newEnv(&bytes.Buffer{}, &errOut)
	e.creds = credentials{dir: t.TempDir()}
	if code := e.analytics([]string{"export", "show", "--server", "https://x.example"}); code != exitAuth {
		t.Errorf("signed out: %d %s", code, errOut.String())
	}
}
