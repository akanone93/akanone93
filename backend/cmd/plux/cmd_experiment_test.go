// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"fmt"
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

const heroID = "01f0c450-6c00-7000-8000-000000000706"

// fakeExperiments records what the CLI asked of the flag and experiment
// services.
type fakeExperiments struct {
	pluxv1connect.UnimplementedFlagServiceHandler
	pluxv1connect.UnimplementedExperimentServiceHandler
	pluxv1connect.UnimplementedAppServiceHandler
	calls *[]string
	// flag is the flag that exists, nil for none.
	flag *pluxv1.Flag
}

func (fakeExperiments) ListEnvironments(context.Context, *connect.Request[pluxv1.ListEnvironmentsRequest]) (*connect.Response[pluxv1.ListEnvironmentsResponse], error) {
	return connect.NewResponse(&pluxv1.ListEnvironmentsResponse{Environments: []*pluxv1.Environment{{Id: "env-prod", Key: "production"}}}), nil
}

func (f fakeExperiments) note(format string, args ...any) {
	*f.calls = append(*f.calls, fmt.Sprintf(format, args...))
}

func (f fakeExperiments) ListFlags(context.Context, *connect.Request[pluxv1.ListFlagsRequest]) (*connect.Response[pluxv1.ListFlagsResponse], error) {
	f.note("list flags")
	return connect.NewResponse(&pluxv1.ListFlagsResponse{Flags: []*pluxv1.Flag{f.flag}}), nil
}

func (f fakeExperiments) GetFlag(_ context.Context, r *connect.Request[pluxv1.GetFlagRequest]) (*connect.Response[pluxv1.FlagResponse], error) {
	f.note("get flag %s", r.Msg.GetKey())
	if f.flag == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no such flag"))
	}
	return connect.NewResponse(&pluxv1.FlagResponse{Flag: f.flag}), nil
}

func (f fakeExperiments) CreateFlag(_ context.Context, r *connect.Request[pluxv1.CreateFlagRequest]) (*connect.Response[pluxv1.FlagResponse], error) {
	s := r.Msg.GetSpec()
	f.note("create flag %s %s %s %d", r.Msg.GetKey(), s.GetType(), s.GetDefaultJson(), len(s.GetSegments()))
	return connect.NewResponse(&pluxv1.FlagResponse{Flag: &pluxv1.Flag{Key: r.Msg.GetKey(), Type: s.GetType(), DefaultJson: s.GetDefaultJson(), Segments: s.GetSegments()}}), nil
}

func (f fakeExperiments) UpdateFlag(_ context.Context, r *connect.Request[pluxv1.UpdateFlagRequest]) (*connect.Response[pluxv1.FlagResponse], error) {
	s := r.Msg.GetSpec()
	f.note("update flag %s %s %s %d %q", r.Msg.GetKey(), s.GetType(), s.GetDefaultJson(), len(s.GetSegments()), s.GetDescription())
	return connect.NewResponse(&pluxv1.FlagResponse{Flag: &pluxv1.Flag{Key: r.Msg.GetKey(), Type: f.flag.GetType(), DefaultJson: s.GetDefaultJson(), Segments: s.GetSegments()}}), nil
}

func (f fakeExperiments) DeleteFlag(_ context.Context, r *connect.Request[pluxv1.DeleteFlagRequest]) (*connect.Response[pluxv1.DeleteFlagResponse], error) {
	f.note("delete flag %s", r.Msg.GetKey())
	return connect.NewResponse(&pluxv1.DeleteFlagResponse{}), nil
}

func (fakeExperiments) experiment(state pluxv1.ExperimentState) *pluxv1.Experiment {
	return &pluxv1.Experiment{Id: heroID, Key: "heroLayout", State: state, LayerStart: 0, LayerEnd: 2500}
}

func (f fakeExperiments) CreateExperiment(_ context.Context, r *connect.Request[pluxv1.CreateExperimentRequest]) (*connect.Response[pluxv1.ExperimentResponse], error) {
	f.note("create experiment %s %s", r.Msg.GetEnvironmentId(), r.Msg.GetDocument())
	return connect.NewResponse(&pluxv1.ExperimentResponse{Experiment: f.experiment(pluxv1.ExperimentState_EXPERIMENT_STATE_DRAFT)}), nil
}

func (f fakeExperiments) ListExperiments(_ context.Context, r *connect.Request[pluxv1.ListExperimentsRequest]) (*connect.Response[pluxv1.ListExperimentsResponse], error) {
	f.note("list experiments %s %s", r.Msg.GetEnvironmentId(), r.Msg.GetState())
	return connect.NewResponse(&pluxv1.ListExperimentsResponse{Experiments: []*pluxv1.Experiment{f.experiment(pluxv1.ExperimentState_EXPERIMENT_STATE_RUNNING)}}), nil
}

func (f fakeExperiments) StartExperiment(_ context.Context, r *connect.Request[pluxv1.StartExperimentRequest]) (*connect.Response[pluxv1.ExperimentResponse], error) {
	f.note("start %s %g", r.Msg.GetId(), r.Msg.GetLayerSharePercent())
	return connect.NewResponse(&pluxv1.ExperimentResponse{Experiment: f.experiment(pluxv1.ExperimentState_EXPERIMENT_STATE_RUNNING)}), nil
}

func (f fakeExperiments) StopExperiment(_ context.Context, r *connect.Request[pluxv1.StopExperimentRequest]) (*connect.Response[pluxv1.ExperimentResponse], error) {
	f.note("stop %s", r.Msg.GetId())
	return connect.NewResponse(&pluxv1.ExperimentResponse{Experiment: f.experiment(pluxv1.ExperimentState_EXPERIMENT_STATE_STOPPED)}), nil
}

func (f fakeExperiments) SetHoldout(_ context.Context, r *connect.Request[pluxv1.SetHoldoutRequest]) (*connect.Response[pluxv1.HoldoutResponse], error) {
	f.note("set holdout %g", r.Msg.GetPercent())
	return connect.NewResponse(&pluxv1.HoldoutResponse{Percent: r.Msg.GetPercent()}), nil
}

func (f fakeExperiments) GetHoldout(context.Context, *connect.Request[pluxv1.GetHoldoutRequest]) (*connect.Response[pluxv1.HoldoutResponse], error) {
	f.note("get holdout")
	return connect.NewResponse(&pluxv1.HoldoutResponse{Percent: 5}), nil
}

func (f fakeExperiments) GetResults(_ context.Context, r *connect.Request[pluxv1.GetResultsRequest]) (*connect.Response[pluxv1.GetResultsResponse], error) {
	f.note("results %s", r.Msg.GetId())
	return connect.NewResponse(&pluxv1.GetResultsResponse{
		Key: "heroLayout", State: pluxv1.ExperimentState_EXPERIMENT_STATE_RUNNING,
		Variants:          []*pluxv1.VariantCount{{Name: "control", Control: true, Subjects: 5300, Share: 0.5}, {Name: "wide", Subjects: 4700, Share: 0.5}},
		SampleRatioPValue: 0.0001, SampleRatioMismatch: true, SampleRatioCode: 8202,
		Primary: &pluxv1.MetricResult{Event: "purchase", Kind: "proportion", Direction: "increase", Comparisons: []*pluxv1.Comparison{
			{Variant: "wide", Diff: 0.021, Lower: 0.004, Upper: 0.038, PValue: 0.01, Improved: true},
		}},
		Guardrails: []*pluxv1.MetricResult{{Event: "refund", Kind: "proportion", Direction: "decrease", Comparisons: []*pluxv1.Comparison{
			{Variant: "wide", Diff: 0.012, Lower: 0.003, Upper: 0.02, PValue: 0.02, Worsened: true, GuardrailTripped: true},
		}}},
	}), nil
}

func (f fakeExperiments) PromoteExperiment(_ context.Context, r *connect.Request[pluxv1.PromoteExperimentRequest]) (*connect.Response[pluxv1.PromoteExperimentResponse], error) {
	f.note("promote %s %s %s %s", r.Msg.GetId(), r.Msg.GetVariant(), r.Msg.GetEnvironmentId(), r.Msg.GetLabel())
	e := f.experiment(pluxv1.ExperimentState_EXPERIMENT_STATE_CONCLUDED)
	e.Winner = r.Msg.GetVariant()
	return connect.NewResponse(&pluxv1.PromoteExperimentResponse{Experiment: e, PublishJobId: "job-9"}), nil
}

func experimentRunner(t *testing.T, fake fakeExperiments) func(sub string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv(tokenEnv, "plux_pat_experiment")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewFlagServiceHandler(fake))
	mux.Handle(pluxv1connect.NewExperimentServiceHandler(fake))
	mux.Handle(pluxv1connect.NewAppServiceHandler(fake))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return func(sub string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		all := append([]string{args[0], "--server", srv.URL, "--org", "org-1", "--app", "app-1"}, args[1:]...)
		var code int
		if sub == "flags" {
			code = e.flags(all)
		} else {
			code = e.experiment(all)
		}
		return code, out.String(), errOut.String()
	}
}

// Verifies: ABT-006.
// plux flags lists, shows and deletes flags, creates one that does not exist
// and changes one that does without sending what it left out, and refuses a
// bad type, a bad segment value or a new flag without a default before
// calling.
func TestFlagsCommands_ABT_006(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	var calls []string
	existing := &pluxv1.Flag{
		Key: "newCheckout", Type: pluxv1.FlagType_FLAG_TYPE_BOOL, DefaultJson: "false", Description: "the new checkout",
		Segments: []*pluxv1.FlagSegmentValue{{Segment: "beta-testers", ValueJson: "true"}},
	}
	run := experimentRunner(t, fakeExperiments{calls: &calls, flag: existing})

	code, out, errOut := run("flags", "list", "--env", "production")
	if code != exitOK || !strings.Contains(out, "newCheckout  bool  default false") || !strings.Contains(out, "beta-testers: true") {
		t.Errorf("list: %d %q %q", code, out, errOut)
	}
	if code, out, _ := run("flags", "get", "--env", "production", "--json", "newCheckout"); code != exitOK || !strings.Contains(out, `"type": "bool"`) {
		t.Errorf("get --json: %d %q", code, out)
	}
	if code, _, errOut := run("flags", "set", "--env", "production", "--default", "true", "newCheckout"); code != exitOK {
		t.Errorf("update: %d %s", code, errOut)
	}
	if code, _, errOut := run("flags", "set", "--env", "production", "--segment", "germans=false", "--segment", "beta-testers=true", "newCheckout"); code != exitOK {
		t.Errorf("update segments: %d %s", code, errOut)
	}
	if code, _, errOut := run("flags", "delete", "--env", "production", "newCheckout"); code != exitOK {
		t.Errorf("delete: %d %s", code, errOut)
	}
	want := []string{
		"list flags", "get flag newCheckout",
		"get flag newCheckout", `update flag newCheckout FLAG_TYPE_UNSPECIFIED true 1 "the new checkout"`,
		"get flag newCheckout", `update flag newCheckout FLAG_TYPE_UNSPECIFIED false 2 "the new checkout"`,
		"delete flag newCheckout",
	}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls:\n got %q\nwant %q", calls, want)
	}

	// A flag that does not exist is created, with the type and the default.
	calls = nil
	missing := experimentRunner(t, fakeExperiments{calls: &calls})
	if code, _, errOut := missing("flags", "set", "--env", "production", "--type", "number", "--default", "10", "maxItems"); code != exitOK {
		t.Errorf("create: %d %s", code, errOut)
	}
	if got := strings.Join(calls, "|"); got != "get flag maxItems|create flag maxItems FLAG_TYPE_NUMBER 10 0" {
		t.Errorf("create calls = %q", got)
	}

	calls = nil
	for _, args := range [][]string{
		{"set", "--env", "production", "--type", "float", "--default", "1", "x"},
		{"set", "--env", "production", "--type", "bool", "--default", "true", "--segment", "oops", "x"},
		{"set", "--env", "production", "x"},
		{"set", "--env", "production", "--type", "bool", "x"},
		{"get", "x"},
		{"bogus"},
	} {
		if code, _, _ := missing("flags", args...); code == exitOK {
			t.Errorf("%v succeeded", args)
		}
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "create") || strings.HasPrefix(c, "update") {
			t.Errorf("a refused command reached the server: %q", c)
		}
	}
}

// Verifies: ABT-001, ABT-003, ABT-004, ABT-005.
// plux experiment registers, starts, stops and analyses experiments and
// promotes a winner, naming an experiment by its key where an environment is
// given, and refuses what is incomplete before calling.
func TestExperimentCommands_ABT_005(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	var calls []string
	run := experimentRunner(t, fakeExperiments{calls: &calls})
	doc := filepath.Join(t.TempDir(), "heroLayout.experiment.json")
	if err := os.WriteFile(doc, []byte(`{"key":"heroLayout"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, out, errOut := run("experiment", "create", "--env", "production", doc); code != exitOK || !strings.Contains(out, "heroLayout  draft") {
		t.Errorf("create: %d %q %q", code, out, errOut)
	}
	if code, out, _ := run("experiment", "list", "--state", "running"); code != exitOK || !strings.Contains(out, "layer buckets 0-2500") {
		t.Errorf("list: %d %q", code, out)
	}
	if code, _, errOut := run("experiment", "start", "--layer-share", "25", heroID); code != exitOK {
		t.Errorf("start: %d %s", code, errOut)
	}
	if code, _, errOut := run("experiment", "stop", "--env", "production", "heroLayout"); code != exitOK {
		t.Errorf("stop by key: %d %s", code, errOut)
	}
	if code, out, errOut := run("experiment", "holdout", "--env", "production"); code != exitOK || !strings.Contains(out, "holdout 5%") {
		t.Errorf("holdout: %d %q %q", code, out, errOut)
	}
	if code, out, errOut := run("experiment", "holdout", "--env", "production", "--percent", "2.5"); code != exitOK || !strings.Contains(out, "holdout 2.5%") {
		t.Errorf("set holdout: %d %q %q", code, out, errOut)
	}
	code, out, _ := run("experiment", "results", heroID)
	for _, want := range []string{"WARNING PLX-8202", "control", "5300 subjects", "primary purchase", "better", "GUARDRAIL TRIPPED"} {
		if code != exitOK || !strings.Contains(out, want) {
			t.Errorf("results: %d, missing %q in %q", code, want, out)
		}
	}
	if code, out, _ := run("experiment", "results", "--json", heroID); code != exitOK || !strings.Contains(out, `"mismatch": true`) {
		t.Errorf("results --json: %d %q", code, out)
	}
	if code, out, errOut := run("experiment", "promote", "--env", "production", "--label", "hero wins", heroID, "wide"); code != exitOK ||
		!strings.Contains(out, "winner wide") || !strings.Contains(out, "publish job job-9") {
		t.Errorf("promote: %d %q %q", code, out, errOut)
	}
	want := []string{
		"create experiment env-prod {\"key\":\"heroLayout\"}", "list experiments  EXPERIMENT_STATE_RUNNING",
		"start " + heroID + " 25", "list experiments env-prod EXPERIMENT_STATE_UNSPECIFIED", "stop " + heroID,
		"get holdout", "set holdout 2.5", "results " + heroID, "results " + heroID,
		"promote " + heroID + " wide env-prod hero wins",
	}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls:\n got %q\nwant %q", calls, want)
	}

	calls = nil
	for _, args := range [][]string{
		{"create", doc},
		{"start", "heroLayout"},
		{"list", "--state", "paused"},
		{"promote", heroID, "wide"},
		{"promote", "--env", "production", heroID},
		{"holdout"},
		{"bogus"},
	} {
		if code, _, _ := run("experiment", args...); code == exitOK {
			t.Errorf("%v succeeded", args)
		}
	}
	if len(calls) != 0 {
		t.Errorf("a refused command reached the server: %q", calls)
	}
	if code, _, _ := run("experiment", "get", "--env", "production", "unknownKey"); code == exitOK {
		t.Error("an unknown key was found")
	}
}
