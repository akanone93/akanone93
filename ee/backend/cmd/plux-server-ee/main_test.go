// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/serverapp"
	"github.com/nightCode42/plux3/ee/backend/license"
)

// Verifies: GOV-033.
// The enterprise binary is the core's server with the licence verifier
// registered.
func TestEnterpriseBinaryRegistersTheLicenceVerifier_GOV_033(t *testing.T) {
	t.Parallel()
	if _, ok := options().Edition.(license.Verifier); !ok {
		t.Fatalf("Edition = %T, want license.Verifier", options().Edition)
	}
	var out, errOut bytes.Buffer
	if code := serverapp.Run(t.Context(), []string{"version"}, &out, &errOut, options()); code != 0 || !strings.Contains(out.String(), "plux-server") {
		t.Errorf("version: %d %q %q", code, out.String(), errOut.String())
	}
}
