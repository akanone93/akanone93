// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

// validateRenderWorker checks the worker's route to the render worker. The
// render worker compiles and draws tenant projects, so the worker reaches it
// only over mutual TLS (SEC-043, O-9).
func (c *Config) validateRenderWorker(p *problems) {
	if !c.Has(RoleWorker) || c.RenderWorker.Address == "" {
		return
	}
	checkURL(p, "renderWorker.address", c.RenderWorker.Address)
	if !c.Server.InternalTLS.Enabled() {
		p.addf("renderWorker.address", "the worker role reaches the render worker over mutual TLS; set server.internalTLS")
	}
}
