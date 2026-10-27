// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

// validateBuilder checks the builder section: the role's own settings when
// the process runs it, and the worker's route to it. The role compiles
// tenant code, so its listener always requires mutual TLS (SEC-043) and the
// worker reaches it only over mutual TLS (FN-016).
func (c *Config) validateBuilder(p *problems) {
	b := c.Builder
	if (c.Has(RoleWorker) || c.Has(RoleAPI)) && b.Address != "" {
		checkURL(p, "builder.address", b.Address)
		if !c.Server.InternalTLS.Enabled() {
			p.addf("builder.address", "the worker and the api role reach the builder over mutual TLS; set server.internalTLS")
		}
	}
	if !c.Has(RoleBuilder) {
		return
	}
	if b.Listen == "" {
		p.addf("builder.listen", "must be set when the builder role runs")
	}
	if b.InternalTLS.CAFile == "" || b.InternalTLS.CertFile == "" || b.InternalTLS.KeyFile == "" {
		p.addf("builder.internalTLS", "caFile, certFile and keyFile must all be set: the builder listener requires mutual TLS (SEC-043)")
	}
	p.oneOf("builder.sandbox", b.Sandbox, "namespaces", "none")
	if len(b.WasmOpt) == 0 || b.WasmOpt[0] == "" {
		p.addf("builder.wasmOpt", "must name the command that runs the pinned wasm-opt")
	}
	if b.SDKDir == "" {
		p.addf("builder.sdkDir", "must name the pinned SDK crate plux-fn")
	}
	if b.Concurrency < 1 {
		p.addf("builder.concurrency", "must be at least 1")
	}
	if b.LanguageSessions < 0 {
		p.addf("builder.languageSessions", "must not be negative")
	}
	if b.LanguageSessions > 0 && b.LanguageServer == "" {
		p.addf("builder.languageServer", "must name the rust-analyzer executable when builder.languageSessions is set")
	}
	if b.LanguageServer != "" && b.LanguageSessions == 0 {
		p.addf("builder.languageSessions", "must be at least 1 when builder.languageServer is set")
	}
}

// builderWarnings lists the builder settings that are valid but unsafe.
func (c *Config) builderWarnings() []string {
	if c.Has(RoleBuilder) && c.Builder.Sandbox == "none" {
		return []string{"builder.sandbox is none: uploaded Rust sources build without a sandbox, so a build script or procedural macro runs with this process's privileges; it is for development only"}
	}
	return nil
}
