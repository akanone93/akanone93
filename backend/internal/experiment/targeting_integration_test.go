// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/experiment/stats"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Verifies: ABT-001.
// Several segments and attribute conditions become one rule: a device in any
// of the segments that also meets every condition. A single segment stays its
// own rule.
func TestTargetingSeveralSegmentsAndAttributes_ABT_001(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	ios := controldoc.Rule{Attribute: "os", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"ios"}}
	germans := controldoc.Rule{Attribute: "locale", Operator: controldoc.OpIn, ValueType: controldoc.ValueString, Values: []string{"de", "de-AT"}}
	phone := controldoc.Rule{Attribute: "deviceClass", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"phone"}}
	build := controldoc.Rule{Attribute: "build", Operator: controldoc.OpGte, ValueType: controldoc.ValueNumber, Values: []string{"42"}}
	cases := []struct {
		key  string
		edit func(map[string]any)
		want *controldoc.Rule
	}{
		{"one", func(d map[string]any) { d["segment"] = "germans" }, &germans},
		{"two", func(d map[string]any) { d["segment"], d["segments"] = "beta-testers", []any{"germans"} }, &controldoc.Rule{Any: []controldoc.Rule{ios, germans}}},
		{"attrs", func(d map[string]any) {
			d["segment"], d["segments"] = "beta-testers", []any{"germans"}
			d["attributes"] = []any{
				map[string]any{"attribute": "deviceClass", "operator": "eq", "value": "phone"},
				map[string]any{"attribute": "build", "operator": "gte", "value": 42},
			}
		}, &controldoc.Rule{All: []controldoc.Rule{{Any: []controldoc.Rule{ios, germans}}, phone, build}}},
		{"onlyattr", func(d map[string]any) {
			delete(d, "segment")
			d["attributes"] = []any{map[string]any{"attribute": "deviceClass", "operator": "eq", "value": "phone"}}
		}, &phone},
	}
	for _, c := range cases {
		e := w.create(w.a, w.a.dev, newDoc(t, c.key, "", c.edit))
		if _, err := w.start(e.ID, 0); err != nil {
			t.Fatalf("%s: %v", c.key, err)
		}
		got, ok := w.allocation(c.key)
		if !ok || got.Segment == nil {
			t.Fatalf("%s: allocation %+v", c.key, got)
		}
		if a, b := mustJSON(t, got.Segment), mustJSON(t, c.want); a != b {
			t.Errorf("%s: rule\n got %s\nwant %s", c.key, a, b)
		}
		if err := controldoc.ValidateExperimentation(w.control.last.Flags, w.control.last.Experiments, w.control.last.Holdout); err != nil {
			t.Errorf("%s: not a valid control document: %v", c.key, err)
		}
	}
}

// Verifies: ABT-001, LIM-001.
// Segments, attribute conditions and metrics are counted against the registry,
// and a segment named twice is refused.
func TestTargetingAndMetricLimits_ABT_001(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	var err error
	for _, k := range []limits.Key{limits.ExperimentsSegmentsPerTargeting, limits.ExperimentsAttributesPerTargeting, limits.ExperimentsPrimaryMetrics, limits.ExperimentsGuardrailMetrics} {
		if w.lim, err = w.lim.Tighten(k, limits.ScopeApp, 1); err != nil {
			t.Fatal(err)
		}
	}
	metric := func(event string) map[string]any { return map[string]any{"event": event, "kind": "proportion"} }
	attr := map[string]any{"attribute": "os", "operator": "eq", "value": "ios"}
	for name, edit := range map[string]func(map[string]any){
		"segments":   func(d map[string]any) { d["segment"], d["segments"] = "beta-testers", []any{"germans"} },
		"attributes": func(d map[string]any) { d["attributes"] = []any{attr, attr} },
		"primaries":  func(d map[string]any) { d["primaryMetrics"] = []any{metric("signup")} },
		"guardrails": func(d map[string]any) { d["guardrailMetrics"] = []any{metric("refund"), metric("crash")} },
	} {
		_, err := w.svc.CreateExperiment(context.Background(), w.a.admin, createRequest(w, newDoc(t, "limited", "", edit)))
		if code, ok := plxerr.CodeOf(err); !ok || code != plxerr.LimitExceeded {
			t.Errorf("%s: %v", name, err)
		}
	}
	w.lim = limits.Defaults()
	_, err = w.svc.CreateExperiment(context.Background(), w.a.admin, createRequest(w, newDoc(t, "twice", "", func(d map[string]any) {
		d["segment"], d["segments"] = "germans", []any{"germans"}
	})))
	wantCode(t, err, plxerr.DuplicateKey)
	_, err = w.svc.CreateExperiment(context.Background(), w.a.admin, createRequest(w, newDoc(t, "badattr", "", func(d map[string]any) {
		d["attributes"] = []any{map[string]any{"attribute": "nonsense", "operator": "eq", "value": "x"}}
	})))
	wantCode(t, err, plxerr.SegmentRuleInvalid)
}

// Verifies: ABT-003.
// Every primary metric is analysed, the lead one first, beside the guardrails.
func TestResultsSeveralPrimaryMetrics_ABT_003(t *testing.T) {
	t.Parallel()
	w, obs := newAnalysisWorld(t)
	e := w.create(w.a, w.a.prod, newDoc(t, "heroLayout", "", func(d map[string]any) {
		d["minimumSampleSize"] = 1000
		d["primaryMetric"] = map[string]any{"event": "purchase", "kind": "proportion", "direction": "increase"}
		d["primaryMetrics"] = []any{map[string]any{"event": "signup", "kind": "proportion", "direction": "increase"}}
		d["guardrailMetrics"] = []any{
			map[string]any{"event": "refund", "kind": "proportion", "direction": "decrease", "maxRegression": 0.1},
			map[string]any{"event": "crash", "kind": "proportion", "direction": "decrease"},
		}
	}))
	if _, err := w.start(e.ID, 0); err != nil {
		t.Fatal(err)
	}
	obs.byEvent["purchase"] = variantMoments(stats.Proportion(5000, 500), stats.Proportion(5000, 640))
	obs.byEvent["signup"] = variantMoments(stats.Proportion(5000, 900), stats.Proportion(5000, 905))
	obs.byEvent["refund"] = variantMoments(stats.Proportion(5000, 100), stats.Proportion(5000, 100))
	obs.byEvent["crash"] = variantMoments(stats.Proportion(5000, 10), stats.Proportion(5000, 10))
	res, err := w.svc.Analyse(context.Background(), w.a.admin, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Primaries) != 2 || res.Primaries[0].Metric.Event != "purchase" || res.Primaries[1].Metric.Event != "signup" || res.Primary.Metric.Event != "purchase" {
		t.Fatalf("primaries = %+v", res.Primaries)
	}
	if len(res.Guardrails) != 2 || len(res.Primaries[1].Comparisons) != 1 || res.Primaries[1].Comparisons[0].Improved {
		t.Errorf("guardrails = %d, signup = %+v", len(res.Guardrails), res.Primaries[1].Comparisons)
	}
}

func createRequest(w *world, doc []byte) experiment.CreateExperimentRequest {
	return experiment.CreateExperimentRequest{AppID: w.a.app, EnvironmentID: w.a.prod, Document: doc}
}

// Verifies: ABT-006, LIM-001.
// A flag has one value per segment, up to flags.segmentValues of them.
func TestFlagSegmentValuesLimit_ABT_006(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	var err error
	if w.lim, err = limits.Defaults().Tighten(limits.FlagsSegmentValues, limits.ScopeApp, 1); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	one := experiment.SegmentValue{Segment: "beta-testers", Value: json.RawMessage("true")}
	two := experiment.SegmentValue{Segment: "germans", Value: json.RawMessage("true")}
	_, err = w.svc.CreateFlag(ctx, w.a.admin, w.flag(w.a, "limited"), boolSpec("false", one, two))
	wantCode(t, err, plxerr.LimitExceeded)
	if _, err := w.svc.CreateFlag(ctx, w.a.admin, w.flag(w.a, "limited"), boolSpec("false", one)); err != nil {
		t.Fatal(err)
	}
	_, err = w.svc.UpdateFlag(ctx, w.a.admin, w.flag(w.a, "limited"), boolSpec("false", one, two))
	wantCode(t, err, plxerr.LimitExceeded)
}
