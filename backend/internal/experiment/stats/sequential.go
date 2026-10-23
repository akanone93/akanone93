// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package stats

import (
	"errors"
	"fmt"
	"math"
)

// DefaultAlpha is the error rate of the confidence sequences: they cover the
// true difference with probability at least 1 - DefaultAlpha. A statistical
// convention, not a tenant limit.
const DefaultAlpha = 0.05

// ErrTooFewSubjects means a variant has fewer than two subjects, so its
// variance is undefined.
var ErrTooFewSubjects = errors.New("stats: a variant needs at least two subjects")

// Options configure [Compare].
type Options struct {
	// Alpha is the error rate, between 0 and 1 exclusive; zero means
	// [DefaultAlpha].
	Alpha float64
	// PlannedPerArm is the sample size per variant at which the confidence
	// sequence is tightest, normally the experiment's minimum sample size.
	// It must be positive.
	PlannedPerArm float64
	// CUPED asks for variance reduction with the pre-exposure covariate. It
	// is applied only where the covariate varies; Estimate.CUPED says
	// whether it was.
	CUPED bool
}

// Estimate is the effect of a treatment over the control.
type Estimate struct {
	// ControlMean and TreatmentMean are the mean outcomes, adjusted by the
	// covariate when CUPED is set.
	ControlMean, TreatmentMean float64
	// Diff is TreatmentMean - ControlMean; Variance is its estimated
	// variance.
	Diff, Variance float64
	// Lower and Upper bound the true difference at the error rate, at every
	// look at once.
	Lower, Upper float64
	// PValue is the always-valid p-value of "no difference".
	PValue float64
	// CUPED says the covariate adjustment was applied; VarianceReduction is
	// then the share of the variance of Diff it removed, 0 to 1.
	CUPED             bool
	VarianceReduction float64
}

// Significant reports whether the confidence sequence excludes zero.
func (e Estimate) Significant() bool { return e.Lower > 0 || e.Upper < 0 }

// Compare estimates the difference between a treatment and the control with
// an always-valid confidence sequence; see the package documentation.
func Compare(control, treatment Moments, o Options) (Estimate, error) {
	if control.N < 2 || treatment.N < 2 {
		return Estimate{}, ErrTooFewSubjects
	}
	if o.PlannedPerArm <= 0 || math.IsNaN(o.PlannedPerArm) || math.IsInf(o.PlannedPerArm, 0) {
		return Estimate{}, fmt.Errorf("stats: the planned sample size per variant must be positive, got %v", o.PlannedPerArm)
	}
	alpha := o.Alpha
	if alpha == 0 {
		alpha = DefaultAlpha
	}
	if alpha <= 0 || alpha >= 1 {
		return Estimate{}, fmt.Errorf("stats: alpha must be between 0 and 1, got %v", alpha)
	}
	adj := adjust(control, treatment, o.CUPED)
	est := Estimate{
		ControlMean: adj.controlMean, TreatmentMean: adj.treatmentMean, Diff: adj.treatmentMean - adj.controlMean,
		Variance: adj.varianceC/control.N + adj.varianceT/treatment.N, CUPED: adj.applied, VarianceReduction: adj.reduction,
	}
	tau2 := (adj.varianceC + adj.varianceT) / o.PlannedPerArm
	est.Lower, est.Upper = ConfidenceSequence(est.Diff, est.Variance, tau2, alpha)
	est.PValue = AlwaysValidP(est.Diff, est.Variance, tau2)
	return est, nil
}

// ConfidenceSequence returns the bounds of the always-valid confidence
// sequence of a difference d whose estimated variance is s2, for a normal
// mixture of variance tau2 and the error rate alpha. A zero variance gives
// the degenerate interval [d, d].
func ConfidenceSequence(d, s2, tau2, alpha float64) (lower, upper float64) {
	if s2 <= 0 || tau2 <= 0 {
		return d, d
	}
	radius := math.Sqrt(s2 * (s2 + tau2) / tau2 * math.Log((s2+tau2)/(s2*alpha*alpha)))
	return d - radius, d + radius
}

// AlwaysValidP returns the always-valid p-value min(1, 1/L) of the
// hypothesis of no difference, where L is the likelihood ratio of the
// mixture at the difference d with estimated variance s2. With zero
// variance it is 0 for a difference and 1 for none.
func AlwaysValidP(d, s2, tau2 float64) float64 {
	if s2 <= 0 || tau2 <= 0 {
		if d == 0 {
			return 1
		}
		return 0
	}
	logL := 0.5*math.Log(s2/(s2+tau2)) + tau2*d*d/(2*s2*(s2+tau2))
	return math.Min(1, math.Exp(-logL))
}
