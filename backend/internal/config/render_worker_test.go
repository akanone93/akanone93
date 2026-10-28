// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import "testing"

// Verifies: STU-100, SEC-043.
// The worker reaches the render worker only over mutual TLS, by an absolute
// URL; without an address the installation renders no visual diffs.
func TestWorkerReachesTheRenderWorkerOverMutualTLS_STU_100(t *testing.T) {
	mentions(t, []struct{ name, src, want string }{
		{"no mutual TLS", withServer("  roles: [api, worker]\n") + "renderWorker:\n  address: \"https://render-worker:8443\"\n", "server.internalTLS"},
		{"not a URL", withServer("  roles: [api, worker]\n  internalListen: \":9443\"\n  internalTLS:\n    caFile: a\n    certFile: b\n    keyFile: c\n") + "renderWorker:\n  address: render-worker\n", "renderWorker.address"},
	})
	c := parse(t, withServer("  roles: [api, worker]\n  internalListen: \":9443\"\n  internalTLS:\n    caFile: a\n    certFile: b\n    keyFile: c\n")+
		"renderWorker:\n  address: \"https://render-worker:8443\"\n")
	if c.RenderWorker.Address != "https://render-worker:8443" {
		t.Errorf("address = %q", c.RenderWorker.Address)
	}
	if none := parse(t, withServer("  roles: [api, worker]\n")); none.RenderWorker.Address != "" {
		t.Errorf("default address = %q", none.RenderWorker.Address)
	}
}
