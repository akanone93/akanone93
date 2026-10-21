// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// weightEpsilon is how far a weight in percent may be from a whole number
// of basis points: weights have at most two decimals.
const weightEpsilon = 1e-6

// scopes maps the scopes of the experiment document to the control
// document's.
var scopes = map[schema.ExperimentScope]controldoc.ExperimentScope{
	schema.ExperimentScopePage:          controldoc.ScopeExperimentPage,
	schema.ExperimentScopeSubtree:       controldoc.ScopeExperimentSubtree,
	schema.ExperimentScopeProp:          controldoc.ScopeExperimentProp,
	schema.ExperimentScopeFlow:          controldoc.ScopeExperimentFlow,
	schema.ExperimentScopePluginVersion: controldoc.ScopeExperimentPluginVersion,
}

// checkDefinition checks what the JSON Schema of the experiment document
// cannot: one control, weights that sum to 100, distinct variant names, a
// minimum sample size, metrics that can be analysed and a schedule that ends
// after it starts (ABT-001; PLX-1702, PLX-1703).
func checkDefinition(d schema.ExperimentDocument) error {
	if err := checkKey(d.Key, "experiment"); err != nil {
		return err
	}
	if _, err := variantsOf(d); err != nil {
		return err
	}
	if d.MinimumSampleSize == nil {
		return plxerr.New(plxerr.MissingProperty, "experiment %q needs a minimumSampleSize: no result is read before every variant has it", d.Key)
	}
	if d.Layer != "" && len(d.Layer) > maxName {
		return plxerr.New(plxerr.InvalidFormat, "the layer name is at most %d characters", maxName)
	}
	for _, m := range append([]schema.ExperimentMetric{d.PrimaryMetric}, d.GuardrailMetrics...) {
		if m.Kind == schema.MetricKindMean && m.Property == "" {
			return plxerr.New(plxerr.MissingProperty, "metric %q is a mean and needs the event property to average", m.Event)
		}
	}
	if _, _, err := window(d); err != nil {
		return err
	}
	return nil
}

// variantsOf converts the variants of a definition to their allocation:
// weights in basis points that sum to 10000, exactly one control.
func variantsOf(d schema.ExperimentDocument) ([]controldoc.AllocationVariant, error) {
	out := make([]controldoc.AllocationVariant, 0, len(d.Variants))
	names := map[string]bool{}
	controls, total := 0, 0
	for _, v := range d.Variants {
		if names[v.Name] {
			return nil, plxerr.New(plxerr.DuplicateKey, "experiment %q has two variants named %q", d.Key, v.Name)
		}
		names[v.Name] = true
		bp := math.Round(v.Weight * 100)
		if math.Abs(v.Weight*100-bp) > weightEpsilon {
			return nil, plxerr.New(plxerr.VariantWeightsInvalid, "the weight of %q has more than two decimals", v.Name)
		}
		control := v.Control != nil && *v.Control
		if control {
			controls++
		}
		total += int(bp)
		out = append(out, controldoc.AllocationVariant{Name: v.Name, Control: control, WeightBP: uint32(bp)}) //nolint:gosec // a weight is between 0 and 100
	}
	if controls != 1 {
		return nil, plxerr.New(plxerr.VariantWithoutControl, "experiment %q needs exactly one control variant, has %d", d.Key, controls)
	}
	if total != controldoc.BucketCount {
		return nil, plxerr.New(plxerr.VariantWeightsInvalid, "the weights of experiment %q sum to %.2f, not 100", d.Key, float64(total)/100)
	}
	return out, nil
}

// holdoutBP is the experiment's own holdout in basis points.
func holdoutBP(d schema.ExperimentDocument) uint32 {
	if d.Holdout == nil {
		return 0
	}
	return uint32(math.Round(*d.Holdout * 100)) //nolint:gosec // the schema bounds it to 0 to 100
}

// window reads the experiment's schedule; a zero time is unbounded.
func window(d schema.ExperimentDocument) (start, end time.Time, err error) {
	if d.Schedule == nil {
		return start, end, nil
	}
	if start, err = optionalTime(d.Schedule.Start, "start"); err != nil {
		return start, end, err
	}
	if end, err = optionalTime(d.Schedule.End, "end"); err != nil {
		return start, end, err
	}
	if !start.IsZero() && !end.IsZero() && !end.After(start) {
		return start, end, plxerr.New(plxerr.OutOfRange, "the schedule ends at %s, not after it starts at %s", end.Format(time.RFC3339), start.Format(time.RFC3339))
	}
	return start, end, nil
}

func optionalTime(s, what string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, plxerr.New(plxerr.InvalidFormat, "the schedule's %s %q is not an RFC 3339 time", what, s)
	}
	return t.UTC(), nil
}

// parseDefinition reads a stored definition.
func parseDefinition(raw []byte) (schema.ExperimentDocument, error) {
	var d schema.ExperimentDocument
	if err := json.Unmarshal(raw, &d); err != nil {
		return schema.ExperimentDocument{}, fmt.Errorf("experiment: the stored definition: %w", err)
	}
	return d, nil
}

// scopeOf is the control document's scope of the definition.
func scopeOf(d schema.ExperimentDocument) controldoc.ExperimentScope { return scopes[d.Scope] }
