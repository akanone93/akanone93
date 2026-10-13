// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// The results of a tunnel request, the label values of
// plux_egress_tunnels_total.
const (
	resultOK              = "ok"
	resultRefusedDomain   = "refused_domain"
	resultRefusedAddress  = "refused_address"
	resultRefusedPort     = "refused_port"
	resultRefusedIPLit    = "refused_ip_literal"
	resultBadRequest      = "bad_request"
	resultUpstreamFailed  = "upstream_failed"
	resultLimited         = "limited"
	directionToUpstream   = "to_upstream"
	directionFromUpstream = "from_upstream"
)

// results lists every result so that each series exists from the first
// scrape.
var results = []string{
	resultOK, resultRefusedDomain, resultRefusedAddress, resultRefusedPort,
	resultRefusedIPLit, resultBadRequest, resultUpstreamFailed, resultLimited,
}

// metrics are the counters of the egress role. The label sets are fixed
// here, so a caller cannot grow their cardinality.
type metrics struct {
	tunnels *prometheus.CounterVec
	bytes   *prometheus.CounterVec
}

// newMetrics registers the counters with reg.
func newMetrics(reg prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		tunnels: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "plux_egress_tunnels_total",
			Help: "Tunnel requests by result.",
		}, []string{"result"}),
		bytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "plux_egress_bytes_total",
			Help: "Bytes carried through tunnels, by direction.",
		}, []string{"direction"}),
	}
	for _, c := range []prometheus.Collector{m.tunnels, m.bytes} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("register the egress metrics: %w", err)
		}
	}
	for _, r := range results {
		m.tunnels.WithLabelValues(r)
	}
	m.bytes.WithLabelValues(directionToUpstream)
	m.bytes.WithLabelValues(directionFromUpstream)
	return m, nil
}

// result counts one tunnel request.
func (m *metrics) result(r string) { m.tunnels.WithLabelValues(r).Inc() }

// carried counts bytes copied in one direction.
func (m *metrics) carried(direction string, n int64) {
	m.bytes.WithLabelValues(direction).Add(float64(n))
}
