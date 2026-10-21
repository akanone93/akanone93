// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package stats_test

import (
	"math"
	"math/rand/v2"
)

// rng is the seeded generator of the simulations: the same seed gives the
// same experiment on every run, which a test of a statistical guarantee
// needs. Nothing it draws reaches a security decision.
type rng struct{ src *rand.PCG }

// newRNG returns the generator for a seed.
func newRNG(hi, lo uint64) *rng { return &rng{src: rand.NewPCG(hi, lo)} }

// Float64 returns a uniform number in [0, 1).
func (r *rng) Float64() float64 { return float64(r.src.Uint64()>>11) / (1 << 53) }

// NormFloat64 returns a standard normal number (Box-Muller).
func (r *rng) NormFloat64() float64 {
	u1, u2 := 1-r.Float64(), r.Float64()
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

// ExpFloat64 returns an exponential number with mean 1.
func (r *rng) ExpFloat64() float64 { return -math.Log(1 - r.Float64()) }
