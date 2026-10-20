// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeRollouts answers the rollout and app services from memory and keeps
// the last request it was asked to create.
type fakeRollouts struct {
	pluxv1connect.UnimplementedRolloutServiceHandler
	pluxv1connect.UnimplementedAppServiceHandler
	created *pluxv1.CreateRolloutRequest
}

func (*fakeRollouts) ListEnvironments(context.Context, *connect.Request[pluxv1.ListEnvironmentsRequest]) (*connect.Response[pluxv1.ListEnvironmentsResponse], error) {
	return connect.NewResponse(&pluxv1.ListEnvironmentsResponse{Environments: []*pluxv1.Environment{{Id: "env-1", Key: "production"}}}), nil
}

func sampleRollout(id string, state pluxv1.RolloutState) *pluxv1.Rollout {
	return &pluxv1.Rollout{
		Id: id, AppId: "app-1", EnvironmentId: "env-1", EnvironmentKey: "production", Channel: "production",
		CandidateSequence: 7, PreviousSequence: 6, State: state, BasisPoints: 1000, Stages: []*pluxv1.Stage{{BasisPoints: 1000}},
		LastCheck: []byte(`{"decided":false}`),
	}
}

func (f *fakeRollouts) CreateRollout(_ context.Context, r *connect.Request[pluxv1.CreateRolloutRequest]) (*connect.Response[pluxv1.CreateRolloutResponse], error) {
	if r.Header().Get("Authorization") != "Bearer plux_pat_rollout" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	f.created = r.Msg
	return connect.NewResponse(&pluxv1.CreateRolloutResponse{Rollout: sampleRollout("ro-1", pluxv1.RolloutState_ROLLOUT_STATE_ACTIVE)}), nil
}

func (*fakeRollouts) GetRollout(_ context.Context, r *connect.Request[pluxv1.GetRolloutRequest]) (*connect.Response[pluxv1.GetRolloutResponse], error) {
	return connect.NewResponse(&pluxv1.GetRolloutResponse{Rollout: sampleRollout(r.Msg.GetId(), pluxv1.RolloutState_ROLLOUT_STATE_ACTIVE)}), nil
}

func (*fakeRollouts) PauseRollout(_ context.Context, r *connect.Request[pluxv1.PauseRolloutRequest]) (*connect.Response[pluxv1.PauseRolloutResponse], error) {
	ro := sampleRollout(r.Msg.GetId(), pluxv1.RolloutState_ROLLOUT_STATE_PAUSED)
	ro.StateReason = r.Msg.GetReason()
	return connect.NewResponse(&pluxv1.PauseRolloutResponse{Rollout: ro}), nil
}

func (*fakeRollouts) ResumeRollout(_ context.Context, r *connect.Request[pluxv1.ResumeRolloutRequest]) (*connect.Response[pluxv1.ResumeRolloutResponse], error) {
	return connect.NewResponse(&pluxv1.ResumeRolloutResponse{Rollout: sampleRollout(r.Msg.GetId(), pluxv1.RolloutState_ROLLOUT_STATE_ACTIVE)}), nil
}

func (*fakeRollouts) AbortRollout(_ context.Context, r *connect.Request[pluxv1.AbortRolloutRequest]) (*connect.Response[pluxv1.AbortRolloutResponse], error) {
	ro := sampleRollout(r.Msg.GetId(), pluxv1.RolloutState_ROLLOUT_STATE_ABORTED)
	ro.RevertedSequence = 8
	return connect.NewResponse(&pluxv1.AbortRolloutResponse{Rollout: ro}), nil
}

func (*fakeRollouts) ListRollouts(_ context.Context, r *connect.Request[pluxv1.ListRolloutsRequest]) (*connect.Response[pluxv1.ListRolloutsResponse], error) {
	if r.Msg.GetPage().GetPageToken() == "" {
		return connect.NewResponse(&pluxv1.ListRolloutsResponse{
			Rollouts: []*pluxv1.Rollout{sampleRollout("ro-2", pluxv1.RolloutState_ROLLOUT_STATE_ACTIVE)}, Page: &pluxv1.PageResult{NextPageToken: "p2"},
		}), nil
	}
	return connect.NewResponse(&pluxv1.ListRolloutsResponse{Rollouts: []*pluxv1.Rollout{sampleRollout("ro-1", pluxv1.RolloutState_ROLLOUT_STATE_COMPLETED)}}), nil
}

// Verifies: REL-050, REL-051, REL-052, REL-053.
// plux rollout create builds the schedule, targeting files, tester lists and
// gate from its flags; show, pause, resume, abort and list call the service
// with the caller's credential; bad flags are refused before any call.
func TestRolloutCommands_REL_052(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_rollout")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	fake := &fakeRollouts{}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewRolloutServiceHandler(fake))
	mux.Handle(pluxv1connect.NewAppServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		args = append([]string{args[0], "--server", srv.URL, "--org", "org-1", "--app", "app-1"}, args[1:]...)
		code := e.rollout(args)
		return code, out.String(), errOut.String()
	}
	dir := t.TempDir()
	rule := filepath.Join(dir, "rule.json")
	segment := filepath.Join(dir, "beta.segment.json")
	for file, doc := range map[string]string{rule: `{"attribute":"os","operator":"eq","value":"ios"}`, segment: `{"kind":"segment","key":"beta"}`} {
		if err := os.WriteFile(file, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	code, out, stderr := run("create", "--env", "production", "--release", "7", "--stages", "1%:1h,10%:2d,100%",
		"--end", "2026-12-01T00:00:00Z", "--rule", rule, "--segment", segment, "--allow", "d1,d2", "--deny", "d3",
		"--gate-crash-free", "50", "--gate-custom", "checkout_failed=100", "--on-regression", "rollback", "--gate-min-sample", "300", "--gate-window", "2h")
	if code != exitOK || !strings.Contains(out, "ro-1") || !strings.Contains(out, "release 7") {
		t.Fatalf("create = %d, %q, %q", code, out, stderr)
	}
	req := fake.created
	if req.GetAppId() != "app-1" || req.GetEnvironmentId() != "env-1" || req.GetCandidateSequence() != 7 || len(req.GetStages()) != 3 {
		t.Fatalf("request = %v", req)
	}
	if s := req.GetStages(); s[0].GetBasisPoints() != 100 || s[0].GetDurationSeconds() != 3600 || s[1].GetBasisPoints() != 1000 ||
		s[1].GetDurationSeconds() != 172800 || s[2].GetBasisPoints() != 10000 || s[2].GetDurationSeconds() != 0 {
		t.Errorf("stages = %v", s)
	}
	h := req.GetHealth()
	if h.GetMaxCrashFreeDropBasisPoints() != 50 || h.GetAction() != pluxv1.HealthAction_HEALTH_ACTION_ROLLBACK || h.GetMinSample() != 300 ||
		h.GetWindowSeconds() != 7200 || len(h.GetCustom()) != 1 || h.GetCustom()[0].GetEvent() != "checkout_failed" {
		t.Errorf("health = %v", h)
	}
	if req.GetEndAt().AsTime().Year() != 2026 || req.GetStartAt() != nil || len(req.GetSegments()) != 1 || !strings.Contains(string(req.GetRule()), `"os"`) ||
		len(req.GetAllowDevices()) != 2 || len(req.GetDenyDevices()) != 1 {
		t.Errorf("request = %v", req)
	}

	if code, out, _ := run("show", "ro-1"); code != exitOK || !strings.Contains(out, "devices outside the rollout keep release 6") || !strings.Contains(out, `"decided":false`) {
		t.Errorf("show = %d, %q", code, out)
	}
	if code, out, _ := run("pause", "--reason", "look", "ro-1"); code != exitOK || !strings.Contains(out, "paused") {
		t.Errorf("pause = %d, %q", code, out)
	}
	if code, out, _ := run("resume", "ro-1"); code != exitOK || !strings.Contains(out, "active") {
		t.Errorf("resume = %d, %q", code, out)
	}
	if code, out, _ := run("abort", "ro-1"); code != exitOK || !strings.Contains(out, "reverted by release 8") {
		t.Errorf("abort = %d, %q", code, out)
	}
	if code, out, _ := run("list", "--json"); code != exitOK || !strings.Contains(out, "ro-2") || !strings.Contains(out, "ro-1") {
		t.Errorf("list = %d, %q", code, out)
	}

	for _, bad := range [][]string{
		{"create", "--release", "7", "--stages", "10%"},
		{"create", "--env", "production", "--release", "7", "--stages", "ten"},
		{"create", "--env", "production", "--release", "7", "--stages", "10%:soon,100%"},
		{"create", "--env", "production", "--release", "7", "--stages", "10%", "--end", "tomorrow"},
		{"create", "--env", "production", "--release", "7", "--stages", "10%", "--gate-custom", "oops"},
		{"create", "--env", "production", "--release", "7", "--stages", "10%", "--gate-crash-free", "5", "--on-regression", "explode"},
		{"create", "--env", "staging", "--release", "7", "--stages", "10%"},
		{"show"},
		{"nope"},
	} {
		fake.created = nil
		if code, _, _ := run(bad...); code == exitOK || fake.created != nil {
			t.Errorf("plux rollout %v = %d, want a refusal before any call", bad, code)
		}
	}
	if code := newEnv(&bytes.Buffer{}, &bytes.Buffer{}).rollout(nil); code != exitUsage {
		t.Errorf("no subcommand = %d", code)
	}
}
