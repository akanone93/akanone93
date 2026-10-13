// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import "strconv"

// validateEgress checks the egress section when the process runs the
// role. The role carries tenant traffic to the internet, so its listener
// always requires mutual TLS (SEC-043) and its limits are never unbounded
// (FN-017).
func (c *Config) validateEgress(p *problems) {
	if !c.Has(RoleEgress) {
		return
	}
	e := c.Egress
	if e.Listen == "" {
		p.addf("egress.listen", "must be set when the egress role runs")
	}
	if e.InternalTLS.CAFile == "" || e.InternalTLS.CertFile == "" || e.InternalTLS.KeyFile == "" {
		p.addf("egress.internalTLS", "caFile, certFile and keyFile must all be set: the egress listener requires mutual TLS (SEC-043)")
	}
	p.positive("egress.connectTimeout", int64(e.ConnectTimeout))
	p.positive("egress.idleTimeout", int64(e.IdleTimeout))
	p.positive("egress.maxTunnelDuration", int64(e.MaxTunnelDuration))
	p.positive("egress.maxTunnelBytes", e.MaxTunnelBytes)
	p.positive("egress.maxTunnels", int64(e.MaxTunnels))
	if len(e.Ports) == 0 {
		p.addf("egress.ports", "must name at least one port")
	}
	for i, port := range e.Ports {
		if port < 1 || port > 65535 {
			p.addf("egress.ports["+strconv.Itoa(i)+"]", "%d is not a TCP port", port)
		}
	}
	if e.AllowLoopback && c.Server.Production {
		p.addf("egress.allowLoopback", "a production installation does not let functions reach loopback addresses; the setting is for tests and development")
	}
}

// egressWarnings lists the egress settings that are valid but unsafe.
func (c *Config) egressWarnings() []string {
	if c.Has(RoleEgress) && c.Egress.AllowLoopback {
		return []string{"egress.allowLoopback is set: functions can reach services on this host's loopback interface; it is for tests and development only"}
	}
	return nil
}
