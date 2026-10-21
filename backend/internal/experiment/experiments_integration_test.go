// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

func (w *world) start(id string, share float64) (experiment.Experiment, error) {
	return w.svc.StartExperiment(context.Background(), w.a.admin, id, experiment.StartOptions{LayerShare: share})
}

func (w *world) stop(id string) {
	w.t.Helper()
	if _, err := w.svc.StopExperiment(context.Background(), w.a.admin, id); err != nil {
		w.t.Fatalf("StopExperiment: %v", err)
	}
}

// allocation returns the allocation the control documents carry for a key.
func (w *world) allocation(key string) (controldoc.Allocation, bool) {
	for _, a := range w.control.last.Experiments {
		if a.Key == key {
			return a, true
		}
	}
	return controldoc.Allocation{}, false
}

// Verifies: ABT-001, ABT-004.
// Starting writes the experiment's allocation into the control documents with
// its salt, weights in basis points, layer range, holdout, segment rule and
// schedule; stopping takes it out again.
func TestStartWritesAllocation_ABT_001(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	e := w.create(w.a, w.a.prod, experimentDoc(t, "ctaLabel", nil))
	if e.State != experiment.StateDraft || w.control.writes != 0 {
		t.Fatalf("a draft reached the control documents: %+v, %d writes", e.State, w.control.writes)
	}
	started, err := w.start(e.ID, 40)
	if err != nil {
		t.Fatal(err)
	}
	if started.State != experiment.StateRunning || started.LayerStart != 0 || started.LayerEnd != 4000 || !started.StartedAt.Equal(w.now) {
		t.Fatalf("started = %+v", started)
	}
	got, ok := w.allocation("ctaLabel")
	if !ok {
		t.Fatalf("no allocation in %+v", w.control.last)
	}
	ios := controldoc.Rule{Attribute: "os", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"ios"}}
	want := controldoc.Allocation{
		Key: "ctaLabel", Scope: controldoc.ScopeExperimentProp, Salt: "cta-label-1",
		Variants: []controldoc.AllocationVariant{{Name: "control", Control: true, WeightBP: 5000}, {Name: "bold", WeightBP: 3000}, {Name: "short", WeightBP: 2000}},
		Layer:    "checkout", LayerStart: 0, LayerEnd: 4000, HoldoutBP: 500, Segment: &ios,
		StartsAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
	if a, b := mustJSON(t, got), mustJSON(t, want); a != b {
		t.Fatalf("allocation:\n got %s\nwant %s", a, b)
	}
	if err := controldoc.ValidateExperimentation(w.control.last.Flags, w.control.last.Experiments, w.control.last.Holdout); err != nil {
		t.Fatalf("what was written is not a valid control document: %v", err)
	}

	w.stop(e.ID)
	if _, ok := w.allocation("ctaLabel"); ok {
		t.Fatal("a stopped experiment is still allocated")
	}
	if _, err := w.start(e.ID, 0); err == nil {
		t.Fatal("a stopped experiment started again")
	} else {
		wantCode(t, err, plxerr.PreconditionFailed)
	}
	if _, err := w.svc.StopExperiment(context.Background(), w.a.admin, e.ID); err == nil {
		t.Fatal("a stopped experiment stopped again")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Verifies: ABT-004.
// Experiments in one layer are mutually exclusive: each holds its own range of
// the layer's buckets, a start with no free range is refused (PLX-8200), and a
// stop frees the range.
func TestLayerOverlapRefused_ABT_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	var ids []string
	for _, key := range []string{"first", "second", "third", "fourth"} {
		ids = append(ids, w.create(w.a, w.a.prod, newDoc(t, key, "checkout", nil)).ID)
	}
	// The default share is the whole layer, so a second experiment cannot start.
	if _, err := w.start(ids[0], 0); err != nil {
		t.Fatal(err)
	}
	_, err := w.start(ids[1], 0)
	wantCode(t, err, plxerr.ExperimentOverlapsLayer)
	_, err = w.start(ids[1], 10)
	wantCode(t, err, plxerr.ExperimentOverlapsLayer)
	if w.control.writes != 1 {
		t.Errorf("a refused start reached the control documents: %d writes", w.control.writes)
	}
	w.stop(ids[0])

	// Now they share: 40% and 60% take the layer between them.
	first, err := w.start(ids[1], 40)
	if err != nil {
		t.Fatal(err)
	}
	second, err := w.start(ids[2], 60)
	if err != nil {
		t.Fatal(err)
	}
	if first.LayerStart != 0 || first.LayerEnd != 4000 || second.LayerStart != 4000 || second.LayerEnd != 10000 {
		t.Fatalf("ranges: %d-%d and %d-%d", first.LayerStart, first.LayerEnd, second.LayerStart, second.LayerEnd)
	}
	_, err = w.start(ids[3], 1)
	wantCode(t, err, plxerr.ExperimentOverlapsLayer)

	// A freed range is reused and the other experiment keeps its own.
	w.stop(ids[1])
	fourth, err := w.start(ids[3], 25)
	if err != nil {
		t.Fatal(err)
	}
	if fourth.LayerStart != 0 || fourth.LayerEnd != 2500 {
		t.Fatalf("reused range = %d-%d", fourth.LayerStart, fourth.LayerEnd)
	}
	if a, ok := w.allocation("third"); !ok || a.LayerStart != 4000 || a.LayerEnd != 10000 {
		t.Errorf("a running experiment's range moved: %+v", a)
	}
	ranges := map[string][2]uint32{}
	for _, a := range w.control.last.Experiments {
		ranges[a.Key] = [2]uint32{a.LayerStart, a.LayerEnd}
	}
	if len(ranges) != 2 {
		t.Errorf("allocations = %v", ranges)
	}
	// Experiments outside a layer, or in another, do not compete.
	other := w.create(w.a, w.a.prod, newDoc(t, "other", "", nil))
	if _, err := w.start(other.ID, 0); err != nil {
		t.Fatalf("an experiment outside any layer: %v", err)
	}
	_, err = w.start("not-an-id", 0)
	wantCode(t, err, plxerr.InvalidFormat)
	for _, bad := range []float64{-1, 101, 12.345} {
		third := w.create(w.a, w.a.dev, newDoc(t, "bad", "checkout", nil))
		_, err = w.start(third.ID, bad)
		wantCode(t, err, plxerr.OutOfRange)
		if err := w.svc.DeleteExperiment(context.Background(), w.a.admin, third.ID); err != nil {
			t.Fatal(err)
		}
	}
}

// Verifies: ABT-004.
// A layer belongs to an environment: the same layer in another environment is
// not shared.
func TestLayersAreEnvironmentScoped_ABT_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	a := w.create(w.a, w.a.prod, newDoc(t, "same", "checkout", nil))
	b := w.create(w.a, w.a.dev, newDoc(t, "same", "checkout", nil))
	if _, err := w.start(a.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := w.start(b.ID, 0); err != nil {
		t.Fatalf("the same layer in another environment: %v", err)
	}
}

// Verifies: ABT-004.
// The global holdout is set per environment, written into the control
// documents with a salt that stays the same as the share changes, and removed
// by zero.
func TestGlobalHoldout_ABT_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	ctx := context.Background()
	set := func(percent float64) error {
		_, err := w.svc.SetHoldout(ctx, w.a.admin, experiment.Holdout{AppID: w.a.app, EnvironmentID: w.a.prod, Percent: percent})
		return err
	}
	if err := set(5); err != nil {
		t.Fatal(err)
	}
	h := w.control.last.Holdout
	if h == nil || h.BasisPoints != 500 || h.Salt == "" {
		t.Fatalf("holdout = %+v", h)
	}
	salt := h.Salt
	if err := set(12.5); err != nil {
		t.Fatal(err)
	}
	if h := w.control.last.Holdout; h.BasisPoints != 1250 || h.Salt != salt {
		t.Fatalf("after growing: %+v, want the salt %q kept", h, salt)
	}
	got, err := w.svc.GetHoldout(ctx, w.a.admin, w.a.app, w.a.prod)
	if err != nil || got.Percent != 12.5 {
		t.Fatalf("GetHoldout = %+v, %v", got, err)
	}
	if err := set(0); err != nil {
		t.Fatal(err)
	}
	if w.control.last.Holdout != nil {
		t.Fatalf("a zero holdout is still written: %+v", w.control.last.Holdout)
	}
	for _, bad := range []float64{-1, 100.5, 0.001} {
		wantCode(t, set(bad), plxerr.OutOfRange)
	}
	if other, _ := w.svc.GetHoldout(ctx, w.a.admin, w.a.app, w.a.dev); other.Percent != 0 {
		t.Errorf("another environment's holdout = %v", other.Percent)
	}
	if _, err := w.svc.SetHoldout(ctx, w.a.as(auth.AppRead, auth.FlagsManage), experiment.Holdout{AppID: w.a.app, EnvironmentID: w.a.prod, Percent: 1}); err == nil {
		t.Error("a holdout was set without experiments.manage")
	}
}

// Verifies: ABT-001.
// A document that breaks the rules of an experiment is refused with the code
// of the rule: no control (PLX-1702), weights off 100 (PLX-1703).
func TestCreateExperimentValidation_ABT_001(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	ctx := context.Background()
	create := func(doc []byte) error {
		_, err := w.svc.CreateExperiment(ctx, w.a.admin, experiment.CreateExperimentRequest{AppID: w.a.app, EnvironmentID: w.a.prod, Document: doc})
		return err
	}
	variants := func(d map[string]any) []any { return d["variants"].([]any) }
	wantCode(t, create(newDoc(t, "noControl", "", func(d map[string]any) { variants(d)[0].(map[string]any)["control"] = false })), plxerr.VariantWithoutControl)
	wantCode(t, create(newDoc(t, "twoControls", "", func(d map[string]any) { variants(d)[1].(map[string]any)["control"] = true })), plxerr.VariantWithoutControl)
	wantCode(t, create(newDoc(t, "weights", "", func(d map[string]any) { variants(d)[1].(map[string]any)["weight"] = 40 })), plxerr.VariantWeightsInvalid)
	wantCode(t, create(newDoc(t, "decimals", "", func(d map[string]any) {
		variants(d)[0].(map[string]any)["weight"] = 50.001
		variants(d)[1].(map[string]any)["weight"] = 49.999
	})), plxerr.VariantWeightsInvalid)
	wantCode(t, create(newDoc(t, "sameName", "", func(d map[string]any) { variants(d)[1].(map[string]any)["name"] = "control" })), plxerr.DuplicateKey)
	wantCode(t, create(newDoc(t, "noSample", "", func(d map[string]any) { delete(d, "minimumSampleSize") })), plxerr.MissingProperty)
	wantCode(t, create(newDoc(t, "mean", "", func(d map[string]any) {
		d["primaryMetric"] = map[string]any{"event": "purchase", "kind": "mean"}
	})), plxerr.MissingProperty)
	wantCode(t, create(newDoc(t, "backwards", "", func(d map[string]any) {
		d["schedule"] = map[string]any{"start": "2026-11-02T00:00:00Z", "end": "2026-11-01T00:00:00Z"}
	})), plxerr.OutOfRange)
	wantCode(t, create([]byte(`{"kind":"experiment"}`)), plxerr.MissingProperty)
	wantCode(t, create([]byte(`not json`)), plxerr.InvalidJSON)

	ok := newDoc(t, "fine", "", nil)
	e := w.create(w.a, w.a.prod, ok)
	if e.Key != "fine" || e.State != experiment.StateDraft || e.Salt != "fine-salt" {
		t.Fatalf("experiment = %+v", e)
	}
	wantCode(t, create(ok), plxerr.DuplicateKey)
	// A salt left out is drawn, not guessed.
	drawn := w.create(w.a, w.a.prod, newDoc(t, "drawn", "", func(d map[string]any) { delete(d, "salt") }))
	if drawn.Salt == "" || drawn.Salt == e.Salt {
		t.Errorf("drawn salt = %q", drawn.Salt)
	}
	// Only a draft is discarded.
	if err := w.svc.DeleteExperiment(ctx, w.a.admin, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.GetExperiment(ctx, w.a.admin, e.ID); err == nil {
		t.Error("a deleted draft was read")
	}
	if _, err := w.start(drawn.ID, 0); err != nil {
		t.Fatal(err)
	}
	wantCode(t, w.svc.DeleteExperiment(ctx, w.a.admin, drawn.ID), plxerr.PreconditionFailed)
	list, err := w.svc.ListExperiments(ctx, w.a.admin, w.a.app, "", experiment.StateRunning)
	if err != nil || len(list) != 1 || list[0].Key != "drawn" {
		t.Errorf("ListExperiments(running) = %+v, %v", list, err)
	}
}

// Verifies: ABT-001, LIM-001.
// An app runs at most experiments.activePerApp experiments at once.
func TestActiveExperimentsLimit_ABT_001(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	var err error
	if w.lim, err = limits.Defaults().Tighten(limits.ExperimentsActivePerApp, limits.ScopeApp, 1); err != nil {
		t.Fatal(err)
	}
	a := w.create(w.a, w.a.prod, newDoc(t, "a", "", nil))
	b := w.create(w.a, w.a.dev, newDoc(t, "b", "", nil))
	if _, err := w.start(a.ID, 0); err != nil {
		t.Fatal(err)
	}
	_, err = w.start(b.ID, 0)
	wantCode(t, err, plxerr.LimitExceeded)
	w.stop(a.ID)
	if _, err := w.start(b.ID, 0); err != nil {
		t.Fatalf("after a stop: %v", err)
	}
}

// Verifies: ABT-007.
// The approval gate is asked before anything is written, and a refusal keeps
// the experiment a draft.
func TestApprovalGateRefusesStart_ABT_007(t *testing.T) {
	t.Parallel()
	gate := &refusingGate{}
	w := newWorld(t, func(o *experiment.Options) { o.Approval = gate })
	e := w.create(w.a, w.a.prod, newDoc(t, "secure", "", nil))
	_, err := w.start(e.ID, 0)
	wantCode(t, err, plxerr.ApprovalRequired)
	if gate.asked != 1 || w.control.writes != 0 {
		t.Errorf("gate asked %d times, %d control writes", gate.asked, w.control.writes)
	}
	got, err := w.svc.GetExperiment(context.Background(), w.a.admin, e.ID)
	if err != nil || got.State != experiment.StateDraft {
		t.Fatalf("after the refusal: %+v, %v", got, err)
	}
}

// Verifies: ABT-001, SEC-102.
// Experiments are changed with experiments.manage, results read with
// analytics.read, and one organisation never sees another's experiment.
func TestExperimentAuthorisation_ABT_001(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	e := w.create(w.a, w.a.prod, newDoc(t, "guarded", "", nil))
	ctx := context.Background()
	reader := w.a.as(auth.AppRead)
	_, err := w.svc.StartExperiment(ctx, reader, e.ID, experiment.StartOptions{})
	wantCode(t, err, plxerr.PermissionDenied)
	_, err = w.svc.StopExperiment(ctx, reader, e.ID)
	wantCode(t, err, plxerr.PermissionDenied)
	wantCode(t, w.svc.DeleteExperiment(ctx, reader, e.ID), plxerr.PermissionDenied)
	_, err = w.svc.CreateExperiment(ctx, reader, experiment.CreateExperimentRequest{AppID: w.a.app, EnvironmentID: w.a.prod, Document: newDoc(t, "x", "", nil)})
	wantCode(t, err, plxerr.PermissionDenied)
	_, err = w.svc.Analyse(ctx, reader, e.ID)
	wantCode(t, err, plxerr.PermissionDenied)
	if _, err := w.svc.GetExperiment(ctx, reader, e.ID); err != nil {
		t.Errorf("a reader reads an experiment: %v", err)
	}
	if _, err := w.svc.GetExperiment(ctx, w.other.admin, e.ID); err == nil {
		t.Error("another organisation read the experiment")
	}
	if _, err := w.svc.StartExperiment(ctx, w.other.admin, e.ID, experiment.StartOptions{}); err == nil {
		t.Error("another organisation started the experiment")
	}
	// A grant on another app does not help.
	elsewhere := w.a.as(auth.AppRead)
	elsewhere.AppPermissions = map[string][]auth.Permission{w.other.app: {auth.ExperimentsManage}}
	_, err = w.svc.StartExperiment(ctx, elsewhere, e.ID, experiment.StartOptions{})
	wantCode(t, err, plxerr.PermissionDenied)
	if len(w.control.last.Experiments) != 0 || w.control.writes != 0 {
		t.Error("a refused start was written")
	}
}
