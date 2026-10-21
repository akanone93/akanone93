// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package stats

import (
	"errors"
	"fmt"
	"math"
)

// SampleRatioThreshold is the p-value below which the subjects per variant
// are taken to differ from the promised shares: assignment or exposure
// logging is broken (ABT-003, PLX-8202).
const SampleRatioThreshold = 0.001

// SampleRatioResult is the outcome of the sample-ratio check.
type SampleRatioResult struct {
	// ChiSquare is the statistic and DF its degrees of freedom.
	ChiSquare float64
	DF        int
	// PValue is the probability of a split at least this uneven under the
	// promised shares.
	PValue float64
	// Mismatch is set when PValue is below [SampleRatioThreshold].
	Mismatch bool
}

// SampleRatio compares the observed subjects per variant with the promised
// shares by a chi-squared goodness-of-fit test. The shares need not sum to
// one; they are scaled. A variant with a zero share must have no subjects:
// one that has makes the split impossible (p-value 0).
func SampleRatio(observed []int64, shares []float64) (SampleRatioResult, error) {
	if len(observed) != len(shares) || len(observed) < 2 {
		return SampleRatioResult{}, errors.New("stats: the sample-ratio check needs the same number, at least two, of counts and shares")
	}
	var total int64
	var weight float64
	for i, n := range observed {
		if n < 0 || shares[i] < 0 || math.IsNaN(shares[i]) {
			return SampleRatioResult{}, fmt.Errorf("stats: variant %d has a negative count or share", i)
		}
		total += n
		weight += shares[i]
	}
	if weight <= 0 {
		return SampleRatioResult{}, errors.New("stats: the shares sum to zero")
	}
	if total == 0 {
		return SampleRatioResult{PValue: 1}, nil
	}
	var chi float64
	df := -1
	for i, n := range observed {
		expected := float64(total) * shares[i] / weight
		if expected == 0 {
			if n > 0 {
				return SampleRatioResult{ChiSquare: math.Inf(1), DF: len(observed) - 1, Mismatch: true}, nil
			}
			continue
		}
		df++
		d := float64(n) - expected
		chi += d * d / expected
	}
	res := SampleRatioResult{ChiSquare: chi, DF: df, PValue: 1}
	if df >= 1 {
		res.PValue = ChiSquareSurvival(chi, df)
	}
	res.Mismatch = res.PValue < SampleRatioThreshold
	return res, nil
}

// ChiSquareSurvival is P(X >= x) for X chi-squared with df degrees of
// freedom: the regularised upper incomplete gamma function Q(df/2, x/2).
func ChiSquareSurvival(x float64, df int) float64 {
	if x <= 0 {
		return 1
	}
	return upperGamma(float64(df)/2, x/2)
}

// Iteration bounds and tolerance of the incomplete gamma evaluation; the
// series and the continued fraction converge in far fewer steps for the
// degrees of freedom of an experiment.
const (
	gammaIterations = 500
	gammaEpsilon    = 1e-15
	tiny            = 1e-300
)

// upperGamma is the regularised upper incomplete gamma function Q(a, x),
// from the series for P when x < a + 1 and from Lentz's continued fraction
// for Q otherwise.
func upperGamma(a, x float64) float64 {
	if x < a+1 {
		return 1 - lowerGammaSeries(a, x)
	}
	return upperGammaFraction(a, x)
}

// lowerGammaSeries is P(a, x) by its power series.
func lowerGammaSeries(a, x float64) float64 {
	lg, _ := math.Lgamma(a)
	term := 1 / a
	sum := term
	for n := 1; n <= gammaIterations; n++ {
		term *= x / (a + float64(n))
		sum += term
		if math.Abs(term) < math.Abs(sum)*gammaEpsilon {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-lg)
}

// upperGammaFraction is Q(a, x) by the modified Lentz evaluation of its
// continued fraction.
func upperGammaFraction(a, x float64) float64 {
	lg, _ := math.Lgamma(a)
	b := x + 1 - a
	c := 1 / tiny
	d := 1 / b
	h := d
	for i := 1; i <= gammaIterations; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		delta := d * c
		h *= delta
		if math.Abs(delta-1) < gammaEpsilon {
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-lg) * h
}
