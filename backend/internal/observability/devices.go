// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package observability

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// devicesReadTimeout bounds the read a scrape may wait for.
	devicesReadTimeout = 3 * time.Second
	// devicesMaxAge is how long a scrape reuses the counts, so that
	// frequent scrapes do not each query the database.
	devicesMaxAge = 30 * time.Second
)

// DeviceAssurance is the number of devices of one app at one assurance
// level.
type DeviceAssurance struct {
	App   string
	Level string
	Count int64
}

// devicesByAssurance is the collector of plux_devices_by_assurance. It
// reads on scrape, at most once per devicesMaxAge, and keeps the last
// counts when a read fails.
type devicesByAssurance struct {
	desc *prometheus.Desc
	now  func() time.Time

	mu   sync.Mutex
	read func(context.Context) ([]DeviceAssurance, error)
	at   time.Time
	last []DeviceAssurance
}

func (c *devicesByAssurance) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *devicesByAssurance) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	if c.read != nil && (c.at.IsZero() || c.now().Sub(c.at) >= devicesMaxAge) {
		ctx, cancel := context.WithTimeout(context.Background(), devicesReadTimeout)
		if rows, err := c.read(ctx); err == nil {
			c.last = rows
		}
		cancel()
		c.at = c.now()
	}
	rows := c.last
	c.mu.Unlock()
	totals := map[[2]string]int64{}
	for _, r := range rows {
		totals[[2]string{r.App, assuranceLabel(r.Level)}] += r.Count
	}
	for k, n := range totals {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(n), k[0], k[1])
	}
}

// assuranceLabel keeps the level label to the four levels; a stored value
// that is none of AL1 to AL3 is AL0.
func assuranceLabel(level string) string {
	switch level {
	case "AL1", "AL2", "AL3":
		return level
	default:
		return "AL0"
	}
}

// DevicesByAssurance exports plux_devices_by_assurance, read from read at
// most every thirty seconds when the endpoint is scraped. The app label is
// bounded by what read returns. Calling it again replaces the reader.
func (m *Metrics) DevicesByAssurance(read func(context.Context) ([]DeviceAssurance, error)) {
	m.devicesMu.Lock()
	defer m.devicesMu.Unlock()
	if m.devices == nil {
		m.devices = &devicesByAssurance{
			desc: prometheus.NewDesc("plux_devices_by_assurance", "Registered devices that are not revoked, by app and assurance level.", []string{"app", "level"}, nil),
			now:  time.Now,
		}
		m.registry.MustRegister(m.devices)
	}
	m.devices.mu.Lock()
	m.devices.read, m.devices.at = read, time.Time{}
	m.devices.mu.Unlock()
}
