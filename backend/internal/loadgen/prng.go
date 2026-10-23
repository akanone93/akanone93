// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen

import (
	"crypto/rand"
	"time"
)

// prng is a SplitMix64 generator. It decides pacing jitter and synthetic
// identifiers, never anything a secret depends on, and its stream is a
// function of the seed and the device's index so that a run is repeatable.
type prng struct{ state uint64 }

// newPRNG returns the stream of one device.
func newPRNG(seed, index uint64) *prng {
	return &prng{state: seed ^ (index+1)*0x9e3779b97f4a7c15}
}

func (p *prng) next() uint64 {
	p.state += 0x9e3779b97f4a7c15
	z := p.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// float returns a number in [0, 1).
func (p *prng) float() float64 { return float64(p.next()>>11) / (1 << 53) }

// randomID returns a random identifier that differs between runs, for
// idempotency keys and push tokens.
func randomID() string { return rand.Text() }

// jittered returns the interval varied by up to the given fraction either
// way.
func (p *prng) jittered(every time.Duration, jitter float64) time.Duration {
	return time.Duration(float64(every) * (1 + jitter*(2*p.float()-1)))
}

// within returns a moment in [0, every), the offset that spreads the first
// run of a task across the fleet.
func (p *prng) within(every time.Duration) time.Duration {
	return time.Duration(p.float() * float64(every))
}
