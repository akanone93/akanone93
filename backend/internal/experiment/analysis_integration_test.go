// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/experiment/stats"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// memObservations serves the statistics a test sets, per metric event.
type memObservations struct {
	byEvent map[string][]experiment.VariantMoments
	asked   []experiment.MetricQuery
}

func (m *memObservations) Moments(_ context.Context, q experiment.MetricQuery) ([]experiment.VariantMoments, error) {
	m.asked = append(m.asked, q)
	return m.byEvent[q.Event], nil
}

func variantMoments(control, treatment stats.Moments) []experiment.VariantMoments {
	return []experiment.VariantMoments{{Variant: "control", Moments: control}, {Variant: "wide", Moments: treatment}}
}

// runningExperiment is the two-variant hero experiment with guardrail and
// primary metrics, started.
func (w *world) runningExperiment() experiment.Experiment {
	w.t.Helper()
	e := w.create(w.a, w.a.prod, newDoc(w.t, "heroLayout", "", func(d map[string]any) {
		d["minimumSampleSize"] = 1000
		d["primaryMetric"] = map[string]any{"event": "purchase", "kind": "proportion", "direction": "increase"}
		d["guardrailMetrics"] = []any{map[string]any{"event": "refund", "kind": "proportion", "direction": "decrease", "maxRegression": 0.1}}
	}))
	started, err := w.start(e.ID, 0)
	if err != nil {
		w.t.Fatal(err)
	}
	return started
}

func newAnalysisWorld(t *testing.T) (*world, *memObservations) {
	t.Helper()
	obs := &memObservations{byEvent: map[string][]experiment.VariantMoments{}}
	return newWorld(t, func(o *experiment.Options) { o.Observations = obs }), obs
}

// Verifies: ABT-003.
// A result is withheld until every variant has the minimum sample size
// (PLX-8201), and a draft has none.
func TestResultsNeedTheMinimumSample_ABT_003(t *testing.T) {
	t.Parallel()
	w, obs := newAnalysisWorld(t)
	e := w.runningExperiment()
	ctx := context.Background()
	obs.byEvent["purchase"] = variantMoments(stats.Proportion(1000, 100), stats.Proportion(999, 130))
	_, err := w.svc.Analyse(ctx, w.a.admin, e.ID)
	wantCode(t, err, plxerr.SampleSizeNotReached)
	obs.byEvent["purchase"] = nil
	_, err = w.svc.Analyse(ctx, w.a.admin, e.ID)
	wantCode(t, err, plxerr.SampleSizeNotReached)

	draft := w.create(w.a, w.a.dev, newDoc(t, "untouched", "", nil))
	_, err = w.svc.Analyse(ctx, w.a.admin, draft.ID)
	wantCode(t, err, plxerr.SampleSizeNotReached)
}

// Verifies: ABT-003.
// With enough data every variant is compared with the control through a
// confidence sequence, guardrails are judged in their direction, and the query
// starts where the experiment started.
func TestResultsCompareWithControl_ABT_003(t *testing.T) {
	t.Parallel()
	w, obs := newAnalysisWorld(t)
	e := w.runningExperiment()
	obs.byEvent["purchase"] = variantMoments(stats.Proportion(5000, 500), stats.Proportion(5000, 640))
	obs.byEvent["refund"] = variantMoments(stats.Proportion(5000, 100), stats.Proportion(5000, 160))
	w.now = w.now.Add(48 * time.Hour)
	res, err := w.svc.Analyse(context.Background(), w.a.admin, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.SampleRatio.Mismatch || res.SampleRatioCode != 0 {
		t.Errorf("an even split was flagged: %+v", res.SampleRatio)
	}
	if len(res.Variants) != 2 || res.Variants[0].Name != "control" || res.Variants[0].Subjects != 5000 || res.Variants[0].Share != 0.5 {
		t.Errorf("variants = %+v", res.Variants)
	}
	c := res.Primary.Comparisons[0]
	if c.Variant != "wide" || !c.Improved || c.Worsened || c.Estimate.Lower <= 0 || c.Estimate.Diff < 0.027 || c.Estimate.Diff > 0.029 {
		t.Errorf("primary comparison = %+v", c)
	}
	if got := res.Primary.Means[0]; got.Variant != "control" || got.Mean < 0.099 || got.Mean > 0.101 {
		t.Errorf("control mean = %+v", got)
	}
	g := res.Guardrails[0].Comparisons[0]
	if !g.Worsened || !g.GuardrailTripped {
		t.Errorf("a refund rate that rose from 2%% to 3.2%% with a 10%% tolerance did not trip: %+v", g)
	}
	q := obs.asked[0]
	if !q.Since.Equal(e.StartedAt) || !q.Until.Equal(w.now) || q.PreExposureDays != 14 || q.Experiment != "heroLayout" || q.AppID != w.a.app {
		t.Errorf("query = %+v", q)
	}
	if q.Kind != schema.MetricKindProportion {
		t.Errorf("kind = %q", q.Kind)
	}
}

// Verifies: ABT-003.
// A split that chance does not explain flags the experiment (PLX-8202); a
// stopped experiment is analysed up to where it stopped.
func TestResultsFlagSampleRatioMismatch_ABT_003(t *testing.T) {
	t.Parallel()
	w, obs := newAnalysisWorld(t)
	e := w.runningExperiment()
	obs.byEvent["purchase"] = variantMoments(stats.Proportion(5300, 500), stats.Proportion(4700, 480))
	obs.byEvent["refund"] = obs.byEvent["purchase"]
	w.stop(e.ID)
	stopped := w.now
	w.now = w.now.Add(time.Hour)
	res, err := w.svc.Analyse(context.Background(), w.a.admin, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.SampleRatio.Mismatch || res.SampleRatioCode != plxerr.SampleRatioMismatch || res.SampleRatio.PValue >= 0.001 {
		t.Fatalf("a 53/47 split of 10000 was not flagged: %+v", res.SampleRatio)
	}
	if !obs.asked[0].Until.Equal(stopped) {
		t.Errorf("a stopped experiment was analysed up to %s, not %s", obs.asked[0].Until, stopped)
	}
}

// Verifies: ABT-003.
// The holdout is part of the promised split: its subjects see the control, so
// a control that is larger by exactly the holdout is not a mismatch.
func TestSampleRatioAccountsForHoldout_ABT_003(t *testing.T) {
	t.Parallel()
	w, obs := newAnalysisWorld(t)
	e := w.create(w.a, w.a.prod, newDoc(t, "held", "", func(d map[string]any) {
		d["holdout"] = 20
		d["minimumSampleSize"] = 100
	}))
	if _, err := w.start(e.ID, 0); err != nil {
		t.Fatal(err)
	}
	// 20% held out to the control, the rest split evenly: 60% control, 40% variant.
	obs.byEvent["purchase"] = variantMoments(stats.Proportion(6000, 600), stats.Proportion(4000, 440))
	res, err := w.svc.Analyse(context.Background(), w.a.admin, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.SampleRatio.Mismatch || math.Abs(res.Variants[0].Share-0.6) > 1e-9 {
		t.Fatalf("holdout split flagged: %+v, shares %+v", res.SampleRatio, res.Variants)
	}
}

// Verifies: ABT-003.
// Analysis is for those who may read analytics.
func TestResultsAuthorisation_ABT_003(t *testing.T) {
	t.Parallel()
	w, _ := newAnalysisWorld(t)
	e := w.runningExperiment()
	_, err := w.svc.Analyse(context.Background(), w.a.as(auth.AppRead, auth.ExperimentsManage), e.ID)
	wantCode(t, err, plxerr.PermissionDenied)
}

// Verifies: ABT-003.
// The PostgreSQL source forms the statistics from the raw events: a device
// counts in the variant of its first exposure, its outcome is its events from
// then on, and the covariate is the same metric in the 14 days before.
func TestPostgresObservations_ABT_003(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	obs := experiment.PostgresObservations{DB: w.db}
	x := w.a
	base := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	for _, month := range []string{"2026-09-01", "2026-10-01", "2026-11-01"} {
		if _, err := w.db.Pool().Exec(context.Background(), `SELECT plux_events_partition($1::date)`, month); err != nil {
			t.Fatal(err)
		}
	}
	device := func() string {
		d := w.id()
		w.exec(x.org, `INSERT INTO devices (id, organization_id, app_id, environment_id, platform, secret_hash) VALUES ($1, $2, $3, $4, 'android', '\x00')`,
			storage.MustUUID(d), storage.MustUUID(x.org), storage.MustUUID(x.app), storage.MustUUID(x.prod))
		return d
	}
	event := func(dev, name, fields string, at time.Time) {
		w.exec(x.org, `INSERT INTO analytics_events (id, organization_id, app_id, environment_id, device_id, name, time, fields) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)`,
			storage.MustUUID(w.id()), storage.MustUUID(x.org), storage.MustUUID(x.app), storage.MustUUID(x.prod), storage.MustUUID(dev), name, at, fields)
	}
	expose := func(dev, variant string, at time.Time) {
		event(dev, "experiment_exposure", fmt.Sprintf(`{"experiment":"hero","variant":%q}`, variant), at)
	}
	// Control: one device that bought 20 and 30 after exposure and 10 before,
	// one that never bought. Treatment: one that bought 50 after, 100 a month
	// before (outside the window), and one exposed to both variants, counted
	// in its first. Another experiment's exposure and an exposure after the
	// window are ignored.
	c1, c2, t1, t2 := device(), device(), device(), device()
	expose(c1, "control", base)
	event(c1, "purchase", `{"amount":20}`, base.Add(time.Hour))
	event(c1, "purchase", `{"amount":30}`, base.Add(2*time.Hour))
	event(c1, "purchase", `{"amount":10}`, base.Add(-48*time.Hour))
	expose(c1, "control", base.Add(time.Hour))
	expose(c2, "control", base)
	expose(t1, "wide", base)
	event(t1, "purchase", `{"amount":50}`, base.Add(time.Hour))
	event(t1, "purchase", `{"amount":100}`, base.Add(-30*24*time.Hour))
	expose(t2, "wide", base)
	expose(t2, "other", base.Add(time.Minute))
	event(t2, "purchase", `{"amount":"many"}`, base.Add(time.Hour))
	event(c2, "purchase", `{"amount":5}`, base.Add(40*24*time.Hour))
	event(c2, "experiment_exposure", `{"experiment":"another","variant":"control"}`, base)

	query := func(kind schema.MetricKind) map[string]stats.Moments {
		t.Helper()
		got, err := obs.Moments(context.Background(), experiment.MetricQuery{
			OrganizationID: x.org, AppID: x.app, EnvironmentID: x.prod, Experiment: "hero", Event: "purchase", Property: "amount", Kind: kind,
			Since: base.Add(-time.Hour), Until: base.Add(24 * time.Hour), PreExposureDays: 14,
		})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]stats.Moments{}
		for _, v := range got {
			out[v.Variant] = v.Moments
		}
		return out
	}
	mean := query(schema.MetricKindMean)
	// Control: y = 50 and 0, x = 10 and 0. Treatment: y = 50 and 0 (the
	// non-numeric amount adds nothing), x = 0 and 0.
	wantC := stats.Moments{N: 2, SumY: 50, SumYY: 2500, SumX: 10, SumXX: 100, SumXY: 500}
	wantT := stats.Moments{N: 2, SumY: 50, SumYY: 2500}
	if mean["control"] != wantC || mean["wide"] != wantT || len(mean) != 2 {
		t.Fatalf("mean metric:\n got %+v\nwant control %+v and wide %+v", mean, wantC, wantT)
	}
	prop := query(schema.MetricKindProportion)
	wantPC := stats.Moments{N: 2, SumY: 1, SumYY: 1, SumX: 1, SumXX: 1, SumXY: 1}
	wantPT := stats.Moments{N: 2, SumY: 2, SumYY: 2}
	if prop["control"] != wantPC || prop["wide"] != wantPT {
		t.Fatalf("proportion metric:\n got %+v\nwant control %+v and wide %+v", prop, wantPC, wantPT)
	}
	// Events of another organisation are out of reach.
	other, err := obs.Moments(context.Background(), experiment.MetricQuery{
		OrganizationID: w.other.org, AppID: x.app, EnvironmentID: x.prod, Experiment: "hero", Event: "purchase",
		Since: base.Add(-time.Hour), Until: base.Add(24 * time.Hour), PreExposureDays: 14,
	})
	if err != nil || len(other) != 0 {
		t.Fatalf("another organisation read %+v, %v", other, err)
	}
}
