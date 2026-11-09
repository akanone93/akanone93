// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"slices"
	"testing"
)

// Studio on an origin of its own: security keys are accepted from it as
// well as from the public URL, under the same relying party, and the links
// to Studio point at it.
//
// Verifies: SEC-100, SEC-103, CLI-002.
func TestStudioOrigin(t *testing.T) {
	t.Parallel()
	same := testConfig(t, base)
	if w := webAuthn(same); w.RPID != "plux.example" || !slices.Equal(w.Origins, []string{"https://plux.example"}) {
		t.Errorf("without studioBaseURL: %+v", w)
	}
	if verificationURI(same) != "https://plux.example/device" || studioURL(same) != "https://plux.example" {
		t.Errorf("links: %q %q", verificationURI(same), studioURL(same))
	}
	local := testConfig(t, "server:\n  publicBaseURL: \"https://localhost:8080\"\n  studioBaseURL: \"http://localhost:8081/\"\n"+
		"database:\n  url: \"postgres://plux@db/plux\"\n")
	if w := webAuthn(local); w.RPID != "localhost" || !slices.Equal(w.Origins, []string{"https://localhost:8080", "http://localhost:8081"}) {
		t.Errorf("with studioBaseURL: %+v", w)
	}
	if verificationURI(local) != "http://localhost:8081/device" {
		t.Errorf("verification URI = %q", verificationURI(local))
	}
	// The same origin is not listed twice.
	dup := testConfig(t, "server:\n  publicBaseURL: \"https://plux.example\"\n  studioBaseURL: \"https://plux.example\"\n"+
		"database:\n  url: \"postgres://plux@db/plux\"\n")
	if w := webAuthn(dup); len(w.Origins) != 1 {
		t.Errorf("origins = %v", w.Origins)
	}
}
