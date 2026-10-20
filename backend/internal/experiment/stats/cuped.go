// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package stats

// adjusted is the outcome of two variants after the optional covariate
// adjustment: the means and the per-subject variances of the outcome.
type adjusted struct {
	controlMean, treatmentMean float64
	varianceC, varianceT       float64
	// applied says the covariate was used; reduction is the share of the
	// variance of the difference it removed.
	applied   bool
	reduction float64
}

// adjust computes the means and variances of two variants, with CUPED when
// asked for and the covariate varies. theta is the pooled within-variant
// slope of y on x; each mean is moved by theta times the distance of its
// covariate mean from the pooled covariate mean, which removes the part of
// a difference that the subjects' history explains.
func adjust(control, treatment Moments, useCUPED bool) adjusted {
	raw := adjusted{
		controlMean: control.MeanY(), treatmentMean: treatment.MeanY(),
		varianceC: control.VarianceY(), varianceT: treatment.VarianceY(),
	}
	if !useCUPED {
		return raw
	}
	c, t := control.centred(), treatment.centred()
	if c.xx+t.xx <= 0 {
		return raw
	}
	theta := (c.xy + t.xy) / (c.xx + t.xx)
	pooledX := control.Add(treatment).MeanX()
	out := adjusted{
		controlMean:   control.MeanY() - theta*(control.MeanX()-pooledX),
		treatmentMean: treatment.MeanY() - theta*(treatment.MeanX()-pooledX),
		varianceC:     adjustedVariance(control.N, c, theta),
		varianceT:     adjustedVariance(treatment.N, t, theta),
		applied:       true,
	}
	before := raw.varianceC/control.N + raw.varianceT/treatment.N
	after := out.varianceC/control.N + out.varianceT/treatment.N
	if before > 0 {
		out.reduction = max(0, 1-after/before)
	}
	return out
}

// adjustedVariance is the sample variance of y - theta x from the centred
// sums of a variant of n subjects.
func adjustedVariance(n float64, c centred, theta float64) float64 {
	v := (c.yy - 2*theta*c.xy + theta*theta*c.xx) / (n - 1)
	return max(v, 0)
}
