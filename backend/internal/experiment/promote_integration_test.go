// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
)

const heroPage = "plugins/shop/pages/home.page.json"

// promotionWorld is a world whose draft is the conformance project with the
// hero experiment registered and running.
func promotionWorld(t *testing.T) (*world, *fakeDocuments, *fakePublisher, experiment.Experiment) {
	t.Helper()
	docs := &fakeDocuments{files: projectFiles(t)}
	pub := &fakePublisher{job: uuid()}
	w := newWorld(t, func(o *experiment.Options) { o.Documents, o.Publisher = docs, pub })
	hero := experimentDoc(t, "heroLayout", func(d map[string]any) { d["minimumSampleSize"] = 100 })
	e, err := w.start(w.create(w.a, w.a.prod, hero).ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return w, docs, pub, e
}

// handWritten is the conformance project written with the "wide" hero layout
// directly: the Variant node is its winner's subtree and the experiment's
// document is gone.
func handWritten(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	for name, data := range projectFiles(t) {
		m[name] = &fstest.MapFile{Data: data}
	}
	delete(m, "experiments/heroLayout.experiment.json")
	var page map[string]any
	if err := json.Unmarshal(m[heroPage].Data, &page); err != nil {
		t.Fatal(err)
	}
	children := page["root"].(map[string]any)["slots"].(map[string]any)["body"].(map[string]any)["children"].([]any)
	children[0] = children[0].(map[string]any)["variants"].(map[string]any)["wide"]
	out, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	m[heroPage] = &fstest.MapFile{Data: out}
	return m
}

// Verifies: ABT-005, CMP-002.
// Promoting a winner replaces the draft with the compiler's promoted project,
// which compiles to the same bytes as a project written with the winner; the
// release is made by the publish path and the experiment leaves the control
// documents.
func TestPromoteWritesWinnerAndPublishes_ABT_005(t *testing.T) {
	t.Parallel()
	w, docs, pub, e := promotionWorld(t)
	if _, ok := w.allocation("heroLayout"); !ok {
		t.Fatal("the experiment is not allocated before the promotion")
	}
	got, err := w.svc.Promote(context.Background(), w.a.admin, experiment.PromoteRequest{
		ExperimentID: e.ID, Variant: "wide", EnvironmentID: w.a.prod, Label: "hero wins", Notes: "A/B result",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Experiment.State != experiment.StateConcluded || got.Experiment.Winner != "wide" || got.Experiment.PromoteJobID != pub.job || got.Job.ID != pub.job {
		t.Fatalf("promotion = %+v", got)
	}
	if len(pub.asked) != 1 || pub.asked[0] != (release.PublishRequest{AppID: w.a.app, EnvironmentID: w.a.prod, Label: "hero wins", Notes: "A/B result"}) {
		t.Fatalf("publish requests = %+v", pub.asked)
	}
	if _, ok := w.allocation("heroLayout"); ok {
		t.Error("a concluded experiment is still allocated")
	}

	promoted := fstest.MapFS{}
	for _, f := range docs.imported {
		promoted[f.Path] = &fstest.MapFile{Data: f.Content}
	}
	if _, ok := promoted["experiments/heroLayout.experiment.json"]; ok {
		t.Error("the promoted project still holds the experiment's document")
	}
	a, b := compiler.Compile(promoted, compiler.DefaultOptions()), compiler.Compile(handWritten(t), compiler.DefaultOptions())
	if len(a.Diagnostics) > 0 || len(b.Diagnostics) > 0 {
		t.Fatalf("diagnostics: promoted %v, written %v", a.Diagnostics, b.Diagnostics)
	}
	if !bytes.Equal(a.App.Data, b.App.Data) || len(a.Plugins) != len(b.Plugins) {
		t.Fatal("the app bundle differs from the one written with the winner")
	}
	for i := range a.Plugins {
		if !bytes.Equal(a.Plugins[i].Data, b.Plugins[i].Data) {
			t.Errorf("plugin %s differs from the one written with the winner", a.Plugins[i].Key)
		}
	}
	// It cannot be promoted twice.
	_, err = w.svc.Promote(context.Background(), w.a.admin, experiment.PromoteRequest{ExperimentID: e.ID, Variant: "wide", EnvironmentID: w.a.prod})
	wantCode(t, err, plxerr.PreconditionFailed)
}

// Verifies: ABT-005.
// A winner that is not a variant, or an experiment that never ran, changes
// nothing: no draft is replaced and no release is started.
func TestPromoteRefusals_ABT_005(t *testing.T) {
	t.Parallel()
	w, docs, pub, e := promotionWorld(t)
	ctx := context.Background()
	_, err := w.svc.Promote(ctx, w.a.admin, experiment.PromoteRequest{ExperimentID: e.ID, Variant: "narrow", EnvironmentID: w.a.prod})
	wantCode(t, err, plxerr.WinnerNotAVariant)
	if docs.imported != nil || len(pub.asked) != 0 {
		t.Fatal("a refused promotion changed the draft or published")
	}
	got, err := w.svc.GetExperiment(ctx, w.a.admin, e.ID)
	if err != nil || got.State != experiment.StateRunning {
		t.Fatalf("after the refusal: %+v, %v", got, err)
	}
	draft := w.create(w.a, w.a.dev, newDoc(t, "idle", "", nil))
	_, err = w.svc.Promote(ctx, w.a.admin, experiment.PromoteRequest{ExperimentID: draft.ID, Variant: "control", EnvironmentID: w.a.prod})
	wantCode(t, err, plxerr.PreconditionFailed)
	_, err = w.svc.Promote(ctx, w.a.as(), experiment.PromoteRequest{ExperimentID: e.ID, Variant: "wide"})
	wantCode(t, err, plxerr.PermissionDenied)

	bare := newWorld(t, nil)
	_, err = bare.svc.Promote(ctx, bare.a.admin, experiment.PromoteRequest{ExperimentID: e.ID, Variant: "wide"})
	if err == nil || errors.Is(err, context.Canceled) {
		t.Errorf("a service without a document or release service promoted: %v", err)
	}
}
