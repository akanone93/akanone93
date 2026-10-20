// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/serverapp"
	"github.com/nightCode42/plux3/ee/backend/license"
	"github.com/nightCode42/plux3/ee/backend/saml"
	"github.com/nightCode42/plux3/ee/backend/scim"
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

// Verifies: GOV-004, GOV-033.
// The enterprise binary carries the enterprise SAML and SCIM, not the
// community defaults that refuse with PLX-8600.
func TestEnterpriseBinaryRegistersSAMLAndSCIM_GOV_004(t *testing.T) {
	t.Parallel()
	ext := options().Identity
	if _, ok := ext.SAML.(saml.Provider); !ok {
		t.Errorf("SAML = %T, want saml.Provider", ext.SAML)
	}
	if _, ok := ext.SCIM.(scim.Service); !ok {
		t.Errorf("SCIM = %T, want scim.Service", ext.SCIM)
	}
	if _, ok := ext.OrDefault().SAML.(identity.NoSAML); ok {
		t.Error("a community default replaced the enterprise SAML")
	}
}
