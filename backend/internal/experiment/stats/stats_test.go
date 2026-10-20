// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package stats_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/experiment/stats"
)

// sample draws one subject's outcome y and pre-exposure covariate x.
type sample func(r *rand.Rand, treated bool) (y, x float64)

// outcome of one simulated experiment, read at every look.
type outcome struct {
	// everSignificant: some look had a confidence sequence excluding zero.
	everSignificant bool
	// alwaysCovered: the confidence sequence held the true difference at every look.
	alwaysCovered bool
	// final is the estimate at the last look.
	final stats.Estimate
	looks int
}

// simulate runs one experiment of maxN subjects per variant, looking at the
// result after every step subjects per variant from minN on. truth is the true
// difference the confidence sequence is checked against.
func simulate(seed uint64, draw sample, truth float64, minN, maxN, step int, cuped bool) outcome {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	var c, t stats.Moments
	out := outcome{alwaysCovered: true}
	for n := 1; n <= maxN; n++ {
		add(&c, draw, r, false)
		add(&t, draw, r, true)
		if n < minN || (n-minN)%step != 0 {
			continue
		}
		est, err := stats.Compare(c, t, stats.Options{PlannedPerArm: float64(minN), CUPED: cuped})
		if err != nil {
			panic(err)
		}
		out.looks++
		out.final = est
		out.everSignificant = out.everSignificant || est.Significant()
		if truth < est.Lower || truth > est.Upper {
			out.alwaysCovered = false
		}
	}
	return out
}

func add(m *stats.Moments, draw sample, r *rand.Rand, treated bool) {
	y, x := draw(r, treated)
	m.N++
	m.SumY += y
	m.SumYY += y * y
	m.SumX += x
	m.SumXX += x * x
	m.SumXY += x * y
}

// rate runs sims experiments and returns the share for which pick holds.
func rate(sims int, run func(seed uint64) outcome, pick func(outcome) bool) float64 {
	hits := 0
	for i := range sims {
		if pick(run(uint64(i) + 1)) {
			hits++
		}
	}
	return float64(hits) / float64(sims)
}

func normal(effect float64) sample {
	return func(r *rand.Rand, treated bool) (float64, float64) {
		y := r.NormFloat64()
		if treated {
			y += effect
		}
		return y, 0
	}
}

func bernoulli(base, lift float64) sample {
	return func(r *rand.Rand, treated bool) (float64, float64) {
		p := base
		if treated {
			p += lift
		}
		if r.Float64() < p {
			return 1, 0
		}
		return 0, 0
	}
}

// skewed is an exponential outcome with mean 1, a heavy right tail like a
// revenue or latency metric.
func skewed(lift float64) sample {
	return func(r *rand.Rand, treated bool) (float64, float64) {
		y := r.ExpFloat64()
		if treated {
			y += lift
		}
		return y, 0
	}
}

// history draws an outcome that depends on the subject's pre-exposure
// covariate: y = slope * x + noise (+ effect when treated).
func history(slope, effect float64) sample {
	return func(r *rand.Rand, treated bool) (float64, float64) {
		x := 10 + 2*r.NormFloat64()
		y := slope*x + r.NormFloat64()
		if treated {
			y += effect
		}
		return y, x
	}
}

// TestTypeIErrorUnderPeeking_ABT_003 runs A/A experiments, looks at the
// confidence sequence after every ten subjects per variant from the minimum
// sample size to the end, and expects the share of experiments that ever
// declare a difference to stay at or below alpha: the property of sequential
// inference that a fixed-horizon test lacks.
//
// Verifies: ABT-003.
func TestTypeIErrorUnderPeeking_ABT_003(t *testing.T) {
	t.Parallel()
	for name, draw := range map[string]sample{
		"normal":      normal(0),
		"proportion":  bernoulli(0.1, 0),
		"skewed mean": skewed(0),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := rate(1000, func(seed uint64) outcome { return simulate(seed, draw, 0, 200, 3000, 10, false) },
				func(o outcome) bool { return o.everSignificant })
			t.Logf("type I error under peeking: %.3f", got)
			if got > stats.DefaultAlpha {
				t.Fatalf("type I error under continuous peeking = %.3f, want <= %.2f", got, stats.DefaultAlpha)
			}
		})
	}
}

// TestFixedHorizonInflatesUnderPeeking_ABT_003 shows the simulation can fail:
// the same peeking at a plain 95% z interval declares a difference in far more
// than 5% of A/A experiments, so the test above is not vacuous.
//
// Verifies: ABT-003.
func TestFixedHorizonInflatesUnderPeeking_ABT_003(t *testing.T) {
	t.Parallel()
	hits := 0
	for seed := uint64(1); seed <= 500; seed++ {
		r := rand.New(rand.NewPCG(seed, seed))
		var c, tr stats.Moments
		for n := 1; n <= 3000; n++ {
			add(&c, normal(0), r, false)
			add(&tr, normal(0), r, true)
			if n >= 200 && n%10 == 0 {
				se := math.Sqrt(c.VarianceY()/c.N + tr.VarianceY()/tr.N)
				if math.Abs(tr.MeanY()-c.MeanY()) > 1.96*se {
					hits++
					break
				}
			}
		}
	}
	t.Logf("fixed-horizon interval under peeking: %.3f", float64(hits)/500)
	if got := float64(hits) / 500; got < 0.15 {
		t.Fatalf("a fixed-horizon interval under peeking declared a difference in %.3f of A/A runs; the simulation is too weak to show the problem", got)
	}
}

// TestDetectsKnownEffect_ABT_003 runs A/B experiments with a known effect and
// expects the confidence sequence to exclude zero by the end in nearly all of
// them, to hold the true difference at every look, and to centre on it.
//
// Verifies: ABT-003.
func TestDetectsKnownEffect_ABT_003(t *testing.T) {
	t.Parallel()
	const effect = 0.2
	run := func(seed uint64) outcome { return simulate(seed, normal(effect), effect, 500, 8000, 50, false) }
	got := rate(300, run, func(o outcome) bool { return o.final.Significant() && o.final.Diff > 0 })
	t.Logf("power: %.3f", got)
	if got < 0.95 {
		t.Fatalf("power at a 0.2 sd effect = %.3f, want >= 0.95", got)
	}
	if got := rate(300, run, func(o outcome) bool { return o.alwaysCovered }); got < 1-stats.DefaultAlpha {
		t.Fatalf("the confidence sequence held the true difference at every look in %.3f of runs, want >= 0.95", got)
	}
	if got := rate(300, run, func(o outcome) bool { return math.Abs(o.final.Diff-effect) < 0.05 }); got < 0.95 {
		t.Fatalf("the estimate was within 0.05 of the true effect in %.3f of runs", got)
	}
}

// TestDetectsProportionLift_ABT_003 does the same for a conversion rate: 10%
// to 12%.
//
// Verifies: ABT-003.
func TestDetectsProportionLift_ABT_003(t *testing.T) {
	t.Parallel()
	run := func(seed uint64) outcome { return simulate(seed, bernoulli(0.10, 0.02), 0.02, 2000, 30000, 500, false) }
	got := rate(200, run, func(o outcome) bool { return o.final.Significant() && o.final.Diff > 0 })
	t.Logf("power: %.3f", got)
	if got < 0.9 {
		t.Fatalf("power for a 10%% to 12%% lift = %.3f, want >= 0.9", got)
	}
}

// TestSequenceIsWiderThanFixedHorizon_ABT_003 states the price of validity:
// at the planned sample size the sequence is wider than the fixed-horizon
// interval, and it narrows as data arrives.
//
// Verifies: ABT-003.
func TestSequenceIsWiderThanFixedHorizon_ABT_003(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(7, 7))
	var c, tr stats.Moments
	var earlier stats.Estimate
	for n := 1; n <= 4000; n++ {
		add(&c, normal(0), r, false)
		add(&tr, normal(0), r, true)
		if n == 500 || n == 4000 {
			est, err := stats.Compare(c, tr, stats.Options{PlannedPerArm: 500})
			if err != nil {
				t.Fatal(err)
			}
			if fixed := 1.96 * math.Sqrt(est.Variance); est.Upper-est.Diff <= fixed {
				t.Fatalf("n=%d: radius %.4f is not wider than the fixed-horizon %.4f", n, est.Upper-est.Diff, fixed)
			}
			if n == 4000 && est.Upper-est.Lower >= earlier.Upper-earlier.Lower {
				t.Fatalf("the sequence did not narrow: %.4f at n=500, %.4f at n=4000", earlier.Upper-earlier.Lower, est.Upper-est.Lower)
			}
			earlier = est
		}
	}
}

// TestCUPEDReducesVariance_ABT_003 simulates outcomes that follow the
// pre-exposure covariate and expects the adjustment to remove most of the
// variance, keep the estimate unbiased and narrow the sequence.
//
// Verifies: ABT-003.
func TestCUPEDReducesVariance_ABT_003(t *testing.T) {
	t.Parallel()
	const effect = 0.5
	draw := history(0.8, effect)
	r := rand.New(rand.NewPCG(11, 3))
	var c, tr stats.Moments
	for range 5000 {
		add(&c, draw, r, false)
		add(&tr, draw, r, true)
	}
	plain, err := stats.Compare(c, tr, stats.Options{PlannedPerArm: 1000})
	if err != nil {
		t.Fatal(err)
	}
	adjusted, err := stats.Compare(c, tr, stats.Options{PlannedPerArm: 1000, CUPED: true})
	if err != nil {
		t.Fatal(err)
	}
	if !adjusted.CUPED || plain.CUPED {
		t.Fatalf("CUPED flags: plain %v, adjusted %v", plain.CUPED, adjusted.CUPED)
	}
	// Var(y) = 0.8^2 * 4 + 1 = 3.56 and the residual variance is 1, so about 72% goes.
	if adjusted.VarianceReduction < 0.6 {
		t.Fatalf("variance reduction %.3f, want >= 0.6", adjusted.VarianceReduction)
	}
	if got, want := adjusted.Variance, plain.Variance*(1-adjusted.VarianceReduction); math.Abs(got-want) > 1e-9 {
		t.Fatalf("variance %.6f does not match the reported reduction (%.6f)", got, want)
	}
	if math.Abs(adjusted.Diff-effect) > 0.1 {
		t.Fatalf("CUPED estimate %.3f, want about %.1f", adjusted.Diff, effect)
	}
	if adjusted.Upper-adjusted.Lower >= (plain.Upper-plain.Lower)*0.7 {
		t.Fatalf("the adjusted sequence (%.3f) is not much narrower than the plain one (%.3f)",
			adjusted.Upper-adjusted.Lower, plain.Upper-plain.Lower)
	}
}

// TestCUPEDKeepsTypeIError_ABT_003 repeats the peeking A/A simulation on
// outcomes with a covariate and expects the adjusted sequences to hold alpha.
//
// Verifies: ABT-003.
func TestCUPEDKeepsTypeIError_ABT_003(t *testing.T) {
	t.Parallel()
	got := rate(1000, func(seed uint64) outcome { return simulate(seed, history(0.8, 0), 0, 200, 3000, 10, true) },
		func(o outcome) bool { return o.everSignificant })
	t.Logf("type I error with CUPED: %.3f", got)
	if got > stats.DefaultAlpha {
		t.Fatalf("type I error with CUPED under peeking = %.3f, want <= %.2f", got, stats.DefaultAlpha)
	}
}

// TestCUPEDWithoutCovariate_ABT_003 falls back to the plain estimate where
// no subject has pre-exposure data.
//
// Verifies: ABT-003.
func TestCUPEDWithoutCovariate_ABT_003(t *testing.T) {
	t.Parallel()
	c, tr := stats.Proportion(1000, 100), stats.Proportion(1000, 130)
	est, err := stats.Compare(c, tr, stats.Options{PlannedPerArm: 1000, CUPED: true})
	if err != nil {
		t.Fatal(err)
	}
	if est.CUPED || est.VarianceReduction != 0 {
		t.Fatalf("CUPED applied without a covariate: %+v", est)
	}
	if math.Abs(est.Diff-0.03) > 1e-12 {
		t.Fatalf("diff = %v, want 0.03", est.Diff)
	}
}

// TestCompareRefusals_ABT_003 refuses data and settings the maths cannot use.
//
// Verifies: ABT-003.
func TestCompareRefusals_ABT_003(t *testing.T) {
	t.Parallel()
	ok := stats.Proportion(100, 10)
	if _, err := stats.Compare(stats.Proportion(1, 0), ok, stats.Options{PlannedPerArm: 100}); err == nil {
		t.Error("one subject in the control was accepted")
	}
	for _, o := range []stats.Options{{}, {PlannedPerArm: 100, Alpha: 1.5}, {PlannedPerArm: math.NaN()}} {
		if _, err := stats.Compare(ok, ok, o); err == nil {
			t.Errorf("options %+v were accepted", o)
		}
	}
	est, err := stats.Compare(stats.Proportion(100, 0), stats.Proportion(100, 0), stats.Options{PlannedPerArm: 100})
	if err != nil {
		t.Fatal(err)
	}
	if est.Significant() || est.PValue != 1 || est.Lower != 0 || est.Upper != 0 {
		t.Errorf("no variation and no difference gave %+v", est)
	}
}

// TestChiSquareSurvival_ABT_003 checks the distribution function against
// published critical values.
//
// Verifies: ABT-003.
func TestChiSquareSurvival_ABT_003(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		x    float64
		df   int
		want float64
	}{
		{3.841458820694124, 1, 0.05},
		{5.991464547107979, 2, 0.05},
		{16.26623619626627, 3, 0.001},
		{10.827566170662733, 1, 0.001},
		{18.307038053275146, 10, 0.05},
		{0, 4, 1},
		{200, 3, 0},
	} {
		if got := stats.ChiSquareSurvival(c.x, c.df); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("ChiSquareSurvival(%v, %d) = %v, want %v", c.x, c.df, got, c.want)
		}
	}
}

// TestSampleRatioDetectsMismatch_ABT_003 flags a 51/47 split of 9800 subjects
// that should be even and accepts a split that chance explains.
//
// Verifies: ABT-003.
func TestSampleRatioDetectsMismatch_ABT_003(t *testing.T) {
	t.Parallel()
	bad, err := stats.SampleRatio([]int64{5100, 4700}, []float64{50, 50})
	if err != nil {
		t.Fatal(err)
	}
	if !bad.Mismatch || bad.PValue >= stats.SampleRatioThreshold || bad.DF != 1 {
		t.Fatalf("a 5100/4700 split of an even allocation was not flagged: %+v", bad)
	}
	good, err := stats.SampleRatio([]int64{4950, 4850}, []float64{50, 50})
	if err != nil {
		t.Fatal(err)
	}
	if good.Mismatch {
		t.Fatalf("a 4950/4850 split was flagged: %+v", good)
	}
	uneven, err := stats.SampleRatio([]int64{7000, 2000, 1000}, []float64{70, 20, 10})
	if err != nil || uneven.Mismatch || uneven.DF != 2 {
		t.Fatalf("a 70/20/10 split of a 70/20/10 allocation: %+v, %v", uneven, err)
	}
}

// TestSampleRatioFalsePositiveRate_ABT_003 draws subjects by the promised
// shares and expects the check to flag about one run in a thousand.
//
// Verifies: ABT-003.
func TestSampleRatioFalsePositiveRate_ABT_003(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(5, 5))
	flagged := 0
	const runs = 4000
	for range runs {
		counts := make([]int64, 3)
		for range 1000 {
			switch u := r.Float64(); {
			case u < 0.5:
				counts[0]++
			case u < 0.8:
				counts[1]++
			default:
				counts[2]++
			}
		}
		res, err := stats.SampleRatio(counts, []float64{50, 30, 20})
		if err != nil {
			t.Fatal(err)
		}
		if res.Mismatch {
			flagged++
		}
	}
	if flagged > 12 {
		t.Fatalf("flagged %d of %d correct allocations; expected about %d", flagged, runs, runs/1000)
	}
}

// TestSampleRatioEdges_ABT_003 covers a zero share, no subjects and bad input.
//
// Verifies: ABT-003.
func TestSampleRatioEdges_ABT_003(t *testing.T) {
	t.Parallel()
	res, err := stats.SampleRatio([]int64{10, 5}, []float64{100, 0})
	if err != nil || !res.Mismatch || res.PValue != 0 {
		t.Errorf("subjects in a variant with no share: %+v, %v", res, err)
	}
	if res, err = stats.SampleRatio([]int64{0, 0}, []float64{1, 1}); err != nil || res.Mismatch || res.PValue != 1 {
		t.Errorf("no subjects: %+v, %v", res, err)
	}
	for _, c := range []struct {
		observed []int64
		shares   []float64
	}{
		{[]int64{1}, []float64{1}},
		{[]int64{1, 2}, []float64{1}},
		{[]int64{-1, 2}, []float64{1, 1}},
		{[]int64{1, 2}, []float64{0, 0}},
	} {
		if _, err := stats.SampleRatio(c.observed, c.shares); err == nil {
			t.Errorf("SampleRatio(%v, %v) was accepted", c.observed, c.shares)
		}
	}
}
