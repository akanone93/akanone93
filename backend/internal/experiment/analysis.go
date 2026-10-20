// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/experiment/stats"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// VariantCount is the subjects of a variant against the share promised.
type VariantCount struct {
	Name    string
	Control bool
	// Subjects exposed to the variant; Share is the fraction of the exposed
	// subjects the allocation promises it.
	Subjects int64
	Share    float64
}

// VariantMean is the mean outcome of a variant, adjusted by the covariate
// where the comparison used one.
type VariantMean struct {
	Variant  string
	Subjects int64
	Mean     float64
}

// Comparison is a variant against the control on one metric.
type Comparison struct {
	Variant  string
	Estimate stats.Estimate
	// RelativeDiff is the difference over the control's mean; zero when that
	// is zero. A point estimate: its sequence is the absolute one.
	RelativeDiff float64
	// Improved and Worsened say the confidence sequence excludes zero on the
	// good or the bad side of the metric's direction.
	Improved, Worsened bool
	// GuardrailTripped is set for a guardrail whose sequence lies wholly
	// beyond the regression it tolerates.
	GuardrailTripped bool
}

// MetricResult is the analysis of one metric.
type MetricResult struct {
	Metric      schema.ExperimentMetric
	Means       []VariantMean
	Comparisons []Comparison
}

// Result is the analysis of an experiment (ABT-003).
type Result struct {
	ExperimentID, Key string
	State             State
	// At is when the analysis was made; the events counted end there, or
	// where the experiment stopped.
	At       time.Time
	Variants []VariantCount
	// SampleRatio is the chi-squared check of Variants; when it flags a
	// mismatch, SampleRatioCode is PLX-8202 and the comparisons are not to be
	// trusted.
	SampleRatio     stats.SampleRatioResult
	SampleRatioCode plxerr.Code
	Primary         MetricResult
	Guardrails      []MetricResult
}

// Analyse compares each variant of an experiment with its control on the
// primary and the guardrail metrics (ABT-003). It reports PLX-8201 until every
// variant has the minimum sample size, and a sample ratio mismatch (PLX-8202)
// in the result. Several variants are compared with the control at an error
// rate divided by their number, so that the family of comparisons keeps the
// error rate.
func (s *Service) Analyse(ctx context.Context, p auth.Principal, id string) (Result, error) {
	if s.o.Observations == nil {
		return Result{}, errors.New("experiment: no event source to analyse with")
	}
	exp, err := s.readForAnalysis(ctx, p, id)
	if err != nil {
		return Result{}, err
	}
	until := s.now().UTC()
	if !exp.StoppedAt.IsZero() {
		until = exp.StoppedAt
	}
	primary, err := s.observe(ctx, p, exp, exp.Definition.PrimaryMetric, until)
	if err != nil {
		return Result{}, err
	}
	counts, err := countsOf(exp, primary)
	if err != nil {
		return Result{}, err
	}
	res := Result{ExperimentID: exp.ID, Key: exp.Key, State: exp.State, At: until, Variants: counts}
	if res.SampleRatio, err = sampleRatio(counts); err != nil {
		return Result{}, err
	}
	if res.SampleRatio.Mismatch {
		res.SampleRatioCode = plxerr.SampleRatioMismatch
	}
	if res.Primary, err = compareMetric(exp, exp.Definition.PrimaryMetric, primary, false); err != nil {
		return Result{}, err
	}
	for _, m := range exp.Definition.GuardrailMetrics {
		moments, err := s.observe(ctx, p, exp, m, until)
		if err != nil {
			return Result{}, err
		}
		g, err := compareMetric(exp, m, moments, true)
		if err != nil {
			return Result{}, err
		}
		res.Guardrails = append(res.Guardrails, g)
	}
	return res, nil
}

// readForAnalysis reads the experiment and checks that the principal may
// read results and that there can be some.
func (s *Service) readForAnalysis(ctx context.Context, p auth.Principal, id string) (Experiment, error) {
	var out Experiment
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		uid, err := parseID(id, "experiment")
		if err != nil {
			return err
		}
		row, err := dbgen.New(tx).GetExperiment(ctx, uid)
		if err != nil {
			return failure(err, "experiment")
		}
		if err := wrapAuth(p.AuthorizeApp(auth.AnalyticsRead, storage.ID(row.AppID))); err != nil {
			return err
		}
		out, err = experimentOf(row)
		return err
	})
	if err != nil {
		return Experiment{}, err
	}
	if out.State == StateDraft {
		return Experiment{}, plxerr.New(plxerr.SampleSizeNotReached, "experiment %q has not started, so it has no results", out.Key)
	}
	return out, nil
}

// observe reads the statistics of one metric per variant.
func (s *Service) observe(ctx context.Context, p auth.Principal, e Experiment, m schema.ExperimentMetric, until time.Time) (map[string]stats.Moments, error) {
	got, err := s.o.Observations.Moments(ctx, MetricQuery{
		OrganizationID: p.OrganizationID, AppID: e.AppID, EnvironmentID: e.EnvironmentID, Experiment: e.Key,
		Event: m.Event, Property: m.Property, Kind: m.Kind, Since: e.StartedAt, Until: until, PreExposureDays: preExposureDays,
	})
	if err != nil {
		return nil, fmt.Errorf("experiment: %w", err)
	}
	out := make(map[string]stats.Moments, len(got))
	for _, v := range got {
		out[v.Variant] = v.Moments
	}
	return out, nil
}

// countsOf lists the subjects per variant in the order of the definition
// and refuses a result before every variant has the minimum sample size.
func countsOf(e Experiment, moments map[string]stats.Moments) ([]VariantCount, error) {
	variants, err := variantsOf(e.Definition)
	if err != nil {
		return nil, err
	}
	hold := float64(holdoutBP(e.Definition)) / controldoc.BucketCount
	min := *e.Definition.MinimumSampleSize
	out := make([]VariantCount, len(variants))
	for i, v := range variants {
		share := (1 - hold) * float64(v.WeightBP) / controldoc.BucketCount
		if v.Control {
			share += hold
		}
		n := int64(moments[v.Name].N)
		out[i] = VariantCount{Name: v.Name, Control: v.Control, Subjects: n, Share: share}
		if n < min {
			return nil, plxerr.New(plxerr.SampleSizeNotReached,
				"variant %q of experiment %q has %d of the %d subjects needed before a result is read", v.Name, e.Key, n, min)
		}
	}
	return out, nil
}

// sampleRatio checks the subjects per variant against the promised shares.
func sampleRatio(counts []VariantCount) (stats.SampleRatioResult, error) {
	observed := make([]int64, len(counts))
	shares := make([]float64, len(counts))
	for i, c := range counts {
		observed[i], shares[i] = c.Subjects, c.Share
	}
	res, err := stats.SampleRatio(observed, shares)
	if err != nil {
		return stats.SampleRatioResult{}, fmt.Errorf("experiment: %w", err)
	}
	return res, nil
}

// compareMetric compares every variant with the control on one metric.
func compareMetric(e Experiment, m schema.ExperimentMetric, moments map[string]stats.Moments, guardrail bool) (MetricResult, error) {
	variants, err := variantsOf(e.Definition)
	if err != nil {
		return MetricResult{}, err
	}
	var control controldoc.AllocationVariant
	for _, v := range variants {
		if v.Control {
			control = v
		}
	}
	out := MetricResult{Metric: m}
	alpha := stats.DefaultAlpha / float64(len(variants)-1)
	opts := stats.Options{Alpha: alpha, PlannedPerArm: float64(*e.Definition.MinimumSampleSize), CUPED: true}
	for _, v := range variants {
		if v.Control {
			continue
		}
		est, err := stats.Compare(moments[control.Name], moments[v.Name], opts)
		if err != nil {
			return MetricResult{}, plxerr.Wrap(plxerr.SampleSizeNotReached, err, "variant %q of experiment %q has too few subjects to compare", v.Name, e.Key)
		}
		if len(out.Means) == 0 {
			out.Means = append(out.Means, VariantMean{Variant: control.Name, Subjects: int64(moments[control.Name].N), Mean: est.ControlMean})
		}
		out.Means = append(out.Means, VariantMean{Variant: v.Name, Subjects: int64(moments[v.Name].N), Mean: est.TreatmentMean})
		out.Comparisons = append(out.Comparisons, judge(v.Name, est, m, guardrail))
	}
	return out, nil
}

// judge reads a comparison in the direction of its metric.
func judge(variant string, est stats.Estimate, m schema.ExperimentMetric, guardrail bool) Comparison {
	c := Comparison{Variant: variant, Estimate: est}
	if est.ControlMean != 0 {
		c.RelativeDiff = est.Diff / math.Abs(est.ControlMean)
	}
	up := m.Direction != schema.MetricDirectionDecrease
	good, bad := est.Lower > 0, est.Upper < 0
	if !up {
		good, bad = bad, good
	}
	c.Improved, c.Worsened = good, bad
	if guardrail {
		c.GuardrailTripped = beyondTolerance(est, up, m.MaxRegression)
	}
	return c
}

// beyondTolerance reports whether the whole confidence sequence lies past
// the relative regression a guardrail tolerates.
func beyondTolerance(est stats.Estimate, up bool, tolerance *float64) bool {
	allowed := 0.0
	if tolerance != nil {
		allowed = *tolerance * math.Abs(est.ControlMean)
	}
	if up {
		return est.Upper < -allowed
	}
	return est.Lower > allowed
}
