// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nightCode42/plux3/backend/internal/egress"
	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// buildEgress prepares the egress role: the CONNECT-only proxy that is
// the one route from the function runner to the internet (FN-017,
// ADR-0064). It runs as a component, so it starts with the process and
// drains on shutdown like the other roles.
func (s *Server) buildEgress() error {
	e := s.cfg.Egress
	tlsConfig, err := httpx.InternalServerTLS(httpx.InternalTLSOptions{
		CAFile: e.InternalTLS.CAFile, CertFile: e.InternalTLS.CertFile, KeyFile: e.InternalTLS.KeyFile,
	})
	if err != nil {
		return fmt.Errorf("server: egress: %w", err)
	}
	var reg prometheus.Registerer
	if s.metrics != nil {
		reg = s.metrics.Registry()
	}
	proxy, err := egress.New(egress.Options{
		Listen:            e.Listen,
		TLS:               tlsConfig,
		Policy:            httpx.Policy{AllowLoopback: e.AllowLoopback},
		Ports:             e.Ports,
		ConnectTimeout:    e.ConnectTimeout.Duration(),
		IdleTimeout:       e.IdleTimeout.Duration(),
		MaxTunnelDuration: e.MaxTunnelDuration.Duration(),
		MaxTunnelBytes:    e.MaxTunnelBytes,
		MaxTunnels:        e.MaxTunnels,
		Log:               s.log.With("component", "egress"),
		Registerer:        reg,
	})
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	s.components = append(s.components, proxy)
	return nil
}
