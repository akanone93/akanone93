// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package stats

import "math"

// Moments are the sufficient statistics of one variant: the number of
// subjects, the sums of the outcome y and its square, and, where the
// subjects have a pre-exposure covariate x, the sums of x, its square and
// the product xy. A subject without pre-exposure data has x = 0.
type Moments struct {
	N                  float64
	SumY, SumYY        float64
	SumX, SumXX, SumXY float64
}

// Proportion returns the moments of n subjects of whom converted did the
// event: the outcome is 1 for them and 0 for the rest.
func Proportion(n, converted int64) Moments {
	c := float64(converted)
	return Moments{N: float64(n), SumY: c, SumYY: c}
}

// Add returns the moments of both groups together.
func (m Moments) Add(o Moments) Moments {
	return Moments{
		N: m.N + o.N, SumY: m.SumY + o.SumY, SumYY: m.SumYY + o.SumYY,
		SumX: m.SumX + o.SumX, SumXX: m.SumXX + o.SumXX, SumXY: m.SumXY + o.SumXY,
	}
}

// MeanY is the mean outcome; zero for an empty group.
func (m Moments) MeanY() float64 { return div(m.SumY, m.N) }

// MeanX is the mean covariate; zero for an empty group.
func (m Moments) MeanX() float64 { return div(m.SumX, m.N) }

// centred are the sums of squares and products about the group's means.
type centred struct{ yy, xx, xy float64 }

func (m Moments) centred() centred {
	if m.N == 0 {
		return centred{}
	}
	return centred{
		yy: math.Max(m.SumYY-m.SumY*m.SumY/m.N, 0),
		xx: math.Max(m.SumXX-m.SumX*m.SumX/m.N, 0),
		xy: m.SumXY - m.SumX*m.SumY/m.N,
	}
}

// VarianceY is the sample variance of the outcome; zero below two subjects.
func (m Moments) VarianceY() float64 {
	if m.N < 2 {
		return 0
	}
	return m.centred().yy / (m.N - 1)
}

// div divides, giving zero for a zero denominator.
func div(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
