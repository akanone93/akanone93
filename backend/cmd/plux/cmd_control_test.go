// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeControl records what the CLI asked of the control service.
type fakeControl struct {
	pluxv1connect.UnimplementedControlServiceHandler
	pluxv1connect.UnimplementedAppServiceHandler
	calls *[]string
}

func (f fakeControl) ListEnvironments(context.Context, *connect.Request[pluxv1.ListEnvironmentsRequest]) (*connect.Response[pluxv1.ListEnvironmentsResponse], error) {
	return connect.NewResponse(&pluxv1.ListEnvironmentsResponse{Environments: []*pluxv1.Environment{{Id: "env-prod", Key: "production"}}}), nil
}

func (f fakeControl) document(call string) *pluxv1.ControlDocument {
	*f.calls = append(*f.calls, call)
	return &pluxv1.ControlDocument{
		AppId: "app-1", EnvironmentId: "env-prod", ChannelKey: "production", Version: 7, SignedVersion: 6,
		KillSwitches:    []*pluxv1.KillSwitchEntry{{Scope: pluxv1.KillSwitchScope_KILL_SWITCH_SCOPE_ACTION_GRAPH, Target: "graph-1"}},
		MandatoryUpdate: &pluxv1.ControlRelease{ReleaseSequence: 4, MessageKey: "update.now"},
		ForcedRollback:  &pluxv1.ControlRelease{ReleaseSequence: 3},
		ExpiresAt:       timestamppb.New(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)),
	}
}

func (f fakeControl) GetControlDocument(_ context.Context, r *connect.Request[pluxv1.GetControlDocumentRequest]) (*connect.Response[pluxv1.GetControlDocumentResponse], error) {
	res := f.document("get " + r.Msg.GetEnvironmentId() + " " + r.Msg.GetChannelKey())
	return connect.NewResponse(&pluxv1.GetControlDocumentResponse{Document: res}), nil
}

func (f fakeControl) SetKillSwitch(_ context.Context, r *connect.Request[pluxv1.SetKillSwitchRequest]) (*connect.Response[pluxv1.SetKillSwitchResponse], error) {
	return connect.NewResponse(&pluxv1.SetKillSwitchResponse{Document: f.document(fmt.Sprintf("kill %s %s", r.Msg.GetKillSwitch().GetScope(), r.Msg.GetKillSwitch().GetTarget()))}), nil
}

func (f fakeControl) ClearKillSwitch(_ context.Context, r *connect.Request[pluxv1.ClearKillSwitchRequest]) (*connect.Response[pluxv1.ClearKillSwitchResponse], error) {
	return connect.NewResponse(&pluxv1.ClearKillSwitchResponse{Document: f.document(fmt.Sprintf("unkill %s %s", r.Msg.GetKillSwitch().GetScope(), r.Msg.GetKillSwitch().GetTarget()))}), nil
}

func (f fakeControl) SetMandatoryUpdate(_ context.Context, r *connect.Request[pluxv1.SetMandatoryUpdateRequest]) (*connect.Response[pluxv1.SetMandatoryUpdateResponse], error) {
	return connect.NewResponse(&pluxv1.SetMandatoryUpdateResponse{Document: f.document(fmt.Sprintf("mandatory %d %s", r.Msg.GetReleaseSequence(), r.Msg.GetMessageKey()))}), nil
}

func (f fakeControl) ClearMandatoryUpdate(context.Context, *connect.Request[pluxv1.ClearMandatoryUpdateRequest]) (*connect.Response[pluxv1.ClearMandatoryUpdateResponse], error) {
	return connect.NewResponse(&pluxv1.ClearMandatoryUpdateResponse{Document: f.document("mandatory clear")}), nil
}

func (f fakeControl) SetForcedRollback(_ context.Context, r *connect.Request[pluxv1.SetForcedRollbackRequest]) (*connect.Response[pluxv1.SetForcedRollbackResponse], error) {
	return connect.NewResponse(&pluxv1.SetForcedRollbackResponse{Document: f.document(fmt.Sprintf("rollback %d", r.Msg.GetReleaseSequence()))}), nil
}

func (f fakeControl) ClearForcedRollback(context.Context, *connect.Request[pluxv1.ClearForcedRollbackRequest]) (*connect.Response[pluxv1.ClearForcedRollbackResponse], error) {
	return connect.NewResponse(&pluxv1.ClearForcedRollbackResponse{Document: f.document("rollback clear")}), nil
}

// Verifies: REL-060, REL-070, SYN-060.
// plux control reads a channel's document and sets and clears kill
// switches, the mandatory update and the forced rollback through the
// control service, with the caller's credential, and refuses an unknown
// scope or an ambiguous release flag before calling.
func TestControlCommands_REL_060(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_control")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	var calls []string
	fake := fakeControl{calls: &calls}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewControlServiceHandler(fake))
	mux.Handle(pluxv1connect.NewAppServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		if len(args) > 0 {
			args = append([]string{args[0], "--server", srv.URL, "--org", "org-1", "--app", "app-1", "--env", "production"}, args[1:]...)
		}
		code := e.control(args)
		return code, out.String(), errOut.String()
	}

	code, out, _ := run("get")
	if code != exitOK || !strings.Contains(out, "version 7 (version 6 is signed)") || !strings.Contains(out, "kill  actionGraph graph-1") ||
		!strings.Contains(out, "mandatory update to release 4") || !strings.Contains(out, "forced rollback to release 3") {
		t.Errorf("get: %d %q", code, out)
	}
	if code, out, _ := run("get", "--json"); code != exitOK || !strings.Contains(out, `"signedVersion": 6`) || !strings.Contains(out, `"scope": "actionGraph"`) {
		t.Errorf("get --json: %d %q", code, out)
	}
	for _, args := range [][]string{
		{"kill", "plugin", "loans"},
		{"unkill", "function", "orders.total"},
		{"mandatory", "--release", "4", "--message", "update.now"},
		{"mandatory", "--clear"},
		{"rollback", "--release", "3"},
		{"rollback", "--clear"},
	} {
		if code, _, errOut := run(args...); code != exitOK {
			t.Errorf("%v: %d %s", args, code, errOut)
		}
	}
	want := []string{
		"get env-prod ", "get env-prod ",
		"kill KILL_SWITCH_SCOPE_PLUGIN loans", "unkill KILL_SWITCH_SCOPE_FUNCTION orders.total",
		"mandatory 4 update.now", "mandatory clear", "rollback 3", "rollback clear",
	}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q, want %q", calls, want)
	}

	before := len(calls)
	for _, args := range [][]string{
		{"kill", "app", "loans"}, {"mandatory"}, {"rollback", "--release", "3", "--clear"}, {"bogus"},
	} {
		if code, _, _ := run(args...); code == exitOK {
			t.Errorf("%v succeeded", args)
		}
	}
	if len(calls) != before {
		t.Errorf("a refused command reached the server: %q", calls[before:])
	}
	if code, _, _ := run(); code != exitUsage {
		t.Errorf("no subcommand: %d", code)
	}
}
