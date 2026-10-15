// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"strings"
	"testing"
)

// builderSection is a valid builder section for a process that runs only the
// role.
const builderSection = "builder:\n  listen: \":8443\"\n  sdkDir: /opt/plux/sdk/plux-fn\n  wasmOpt: [bun, /opt/binaryen/wasm-opt]\n" +
	"  internalTLS:\n    caFile: /etc/plux/ca.pem\n    certFile: /etc/plux/builder.pem\n    keyFile: /etc/plux/builder.key\n"

// Verifies: FN-016.
func TestBuilderRoleParsesWithSecureDefaults_FN_016(t *testing.T) {
	c := parse(t, "server:\n  roles: [builder]\n"+builderSection)
	if !c.Has(RoleBuilder) || !c.NoDataAccess() {
		t.Errorf("roles = %v; a builder-only process holds no data access", c.Server.Roles)
	}
	if c.Builder.Sandbox != "namespaces" || c.Builder.Concurrency != 1 {
		t.Errorf("defaults = %+v; the sandbox is the secure default", c.Builder)
	}
	if c.Signing.Keys.FunctionArtifact != "function-artifact" {
		t.Errorf("function artifact key = %q", c.Signing.Keys.FunctionArtifact)
	}
	if w := c.Warnings(); len(w) != 0 {
		t.Errorf("warnings for the default sandbox: %v", w)
	}
}

// Verifies: FN-016, SEC-043.
func TestBuilderRoleValidation_FN_016(t *testing.T) {
	mentions(t, []struct{ name, src, want string }{
		{"no listener", "server:\n  roles: [builder]\nbuilder:\n  sdkDir: /sdk\n  wasmOpt: [wasm-opt]\n  internalTLS:\n    caFile: a\n    certFile: b\n    keyFile: c\n", "builder.listen"},
		{"no mutual TLS", "server:\n  roles: [builder]\nbuilder:\n  listen: \":1\"\n  sdkDir: /sdk\n  wasmOpt: [wasm-opt]\n", "builder.internalTLS"},
		{"no wasm-opt", "server:\n  roles: [builder]\nbuilder:\n  listen: \":1\"\n  sdkDir: /sdk\n  internalTLS:\n    caFile: a\n    certFile: b\n    keyFile: c\n", "builder.wasmOpt"},
		{"no SDK", "server:\n  roles: [builder]\nbuilder:\n  listen: \":1\"\n  wasmOpt: [wasm-opt]\n  internalTLS:\n    caFile: a\n    certFile: b\n    keyFile: c\n", "builder.sdkDir"},
		{"unknown sandbox", "server:\n  roles: [builder]\n" + strings.Replace(builderSection, "builder:\n", "builder:\n  sandbox: chroot\n", 1), "builder.sandbox"},
		{"no concurrency", "server:\n  roles: [builder]\n" + strings.Replace(builderSection, "builder:\n", "builder:\n  concurrency: 0\n", 1), "builder.concurrency"},
		{"worker without mutual TLS", withServer("  roles: [api, worker]\n") + "builder:\n  address: \"https://builder:8443\"\n", "server.internalTLS"},
	})
}

// Verifies: FN-016.
func TestBuilderWithoutSandboxWarns_FN_016(t *testing.T) {
	c := parse(t, "server:\n  roles: [builder]\n"+strings.Replace(builderSection, "builder:\n", "builder:\n  sandbox: none\n", 1))
	w := c.Warnings()
	if len(w) != 1 || !strings.Contains(w[0], "builder.sandbox is none") {
		t.Errorf("warnings = %v; want the distinct sandbox warning", w)
	}
}

// Verifies: FN-016.
func TestWorkerReachesTheBuilderOverMutualTLS_FN_016(t *testing.T) {
	c := parse(t, withServer("  roles: [api, worker]\n  internalListen: \":9443\"\n  internalTLS:\n    caFile: a\n    certFile: b\n    keyFile: c\n")+
		"builder:\n  address: \"https://builder:8443\"\n")
	if c.Builder.Address != "https://builder:8443" {
		t.Errorf("address = %q", c.Builder.Address)
	}
}
