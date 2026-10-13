// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// registry reads series back from a private Prometheus registry.
type registry struct{ reg *prometheus.Registry }

func newRegistry() *registry { return &registry{reg: prometheus.NewRegistry()} }

// counter reads the counter of one result label, zero when the series does not exist.
func (r *registry) counter(t *testing.T, name, result string) float64 {
	t.Helper()
	families, err := r.reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "result" && l.GetValue() == result {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// samples reads the observation count of a histogram.
func (r *registry) samples(t *testing.T, name string) uint64 {
	t.Helper()
	families, err := r.reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == name && len(f.GetMetric()) > 0 {
			return f.GetMetric()[0].GetHistogram().GetSampleCount()
		}
	}
	t.Fatalf("no histogram %s", name)
	return 0
}
