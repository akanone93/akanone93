// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Label values of the metrics.
const (
	resultOK            = "ok"
	resultBusinessError = "business_error"
	cacheHit            = "hit"
	cacheMiss           = "miss"
	cacheRefused        = "refused"
)

// metrics are the Prometheus series of the role (FN-032). The label sets
// are fixed here: a series never carries an app, user, function or input.
type metrics struct {
	invocations *prometheus.CounterVec
	duration    prometheus.Histogram
	memoryPeak  prometheus.Histogram
	cache       *prometheus.CounterVec
}

// newMetrics registers the series with reg.
func newMetrics(reg prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		invocations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "plux_function_invocations_total",
			Help: "Function invocations by result: ok, business_error, or the lower-case reason of the system error.",
		}, []string{"result"}),
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "plux_function_duration_seconds",
			Help:    "Wall time of a function invocation, including the module lookup.",
			Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}),
		memoryPeak: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "plux_function_memory_peak_bytes",
			Help:    "Highest linear memory a function instance reached.",
			Buckets: prometheus.ExponentialBuckets(1<<20, 2, 10),
		}),
		cache: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "plux_function_module_cache_total",
			Help: "Compiled-module cache lookups: hit, miss, or refused after failing verification.",
		}, []string{"result"}),
	}
	for _, c := range []prometheus.Collector{m.invocations, m.duration, m.memoryPeak, m.cache} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("register the function metrics: %w", err)
		}
	}
	m.invocations.WithLabelValues(resultOK)
	m.invocations.WithLabelValues(resultBusinessError)
	for _, r := range []string{cacheHit, cacheMiss, cacheRefused} {
		m.cache.WithLabelValues(r)
	}
	return m, nil
}

// resultOf is the label of an invocation that ended in err: the lower-case
// reason of its Plux code.
func resultOf(err error) string {
	def, ok := plxerr.Lookup(codeOf(err))
	if !ok {
		return "internal_server_error"
	}
	return strings.ToLower(string(def.Reason))
}
