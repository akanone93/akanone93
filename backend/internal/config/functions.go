// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import "strings"

// Functions configures how the api role reaches the function runner
// (FN-020, ADR-0064).
type Functions struct {
	// RunnerURL is the base URL of the fnrunner role's mutual-TLS
	// listener. Empty leaves FunctionService.Invoke unmounted: the
	// installation runs no server functions.
	RunnerURL string `json:"runnerURL"`
}

// validateFunctions checks the functions section. The api role calls the
// runner over mutual TLS with its own internal identity, so the URL must be
// https and the identity configured (SEC-043).
func (c *Config) validateFunctions(p *problems) {
	if c.Functions.RunnerURL == "" {
		return
	}
	if !c.Has(RoleAPI) {
		p.addf("functions.runnerURL", "needs the api role")
	}
	checkURL(p, "functions.runnerURL", c.Functions.RunnerURL)
	if !strings.HasPrefix(c.Functions.RunnerURL, "https://") {
		p.addf("functions.runnerURL", "must be an https URL: the runner requires mutual TLS (SEC-043)")
	}
	if !c.Server.InternalTLS.Enabled() {
		p.addf("functions.runnerURL", "needs server.internalTLS: the api role presents its certificate to the runner (SEC-043)")
	}
}
