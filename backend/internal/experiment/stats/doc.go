// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package stats is the in-house statistics of experiment analysis (ABT-003,
// ADR-0079, maintainer decision Q12): sequential inference that stays valid
// when the result is read at any time, a sample-ratio check, and variance
// reduction with a pre-exposure covariate. It uses the standard library
// only and takes randomness from no source: every function is a pure
// function of the sums it is given.
//
// # Sufficient statistics
//
// An experiment's variants are summarised by [Moments]: the number of
// subjects and the sums of the outcome y, its square, a pre-exposure
// covariate x, its square and the product xy. A proportion is the mean of
// the 0 or 1 outcome "did the event", so one engine serves both kinds of
// metric. The sums can be accumulated by the database, which is why the
// package never sees an individual subject.
//
// # Sequential inference
//
// [Compare] estimates the difference of two means with an always-valid
// confidence sequence from a mixture sequential probability ratio test
// (mSPRT) with a normal mixing distribution N(0, tau^2) over the true
// difference (Johari, Koomen, Pekelis and Walsh, "Always valid inference",
// Operations Research 2022). With the estimate d of the difference and
// s^2 its estimated variance, the likelihood ratio of the mixture is
//
//	L = sqrt(s^2 / (s^2 + tau^2)) * exp(tau^2 d^2 / (2 s^2 (s^2 + tau^2)))
//
// and, by Ville's inequality, the probability that L ever reaches 1/alpha
// when there is no difference is at most alpha, however often and whenever
// it is evaluated. The interval of differences theta whose ratio at d - theta
// stays below 1/alpha is a confidence sequence: it covers the true
// difference at every look at once with probability at least 1 - alpha.
// The always-valid p-value is min(1, 1/L). The variance s^2 is a plug-in
// estimate and tau^2 is set to the variance of the difference at the
// planned sample size, which makes the sequence tightest there; the
// guarantee is asymptotic in the sample size, which is why a result is not
// read before the experiment's minimum sample size (PLX-8201). The tests
// check the type I error under a look after every few observations, on
// simulated data with fixed seeds.
//
// # Sample ratio mismatch
//
// [SampleRatio] compares the subjects per variant with the shares the
// allocation promised by a chi-squared test; a p-value below
// [SampleRatioThreshold] means assignment or exposure logging is broken and
// the result is not to be trusted (PLX-8202).
//
// # CUPED
//
// With [Options.CUPED] the outcome is adjusted by the subject's own
// behaviour in the 14 days before exposure, y - theta (x - mean x), where
// theta is the pooled within-arm regression slope of y on x. The adjustment
// is unbiased because x does not depend on the assignment, and it removes
// the share of variance x explains (Deng, Xu, Kohavi and Walker, "Improving
// the sensitivity of online controlled experiments by utilizing
// pre-experiment data", WSDM 2013).
package stats
