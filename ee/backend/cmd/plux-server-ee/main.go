// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Command plux-server-ee is the Plux server with the enterprise edition
// linked in (GOV-033, ADR-0084). It is the core's server, unchanged, plus a
// licence verifier: the file named by PLUX_LICENSE_FILE is verified offline
// against the public keys compiled into this build, and the enterprise
// features it lists are enabled. It also carries the enterprise identity
// federation, SAML 2.0 single sign-on and SCIM 2.0 provisioning (GOV-004),
// which the licence switches on. Without a valid licence it behaves as the
// open-source core.
package main

import (
	"log/slog"
	"os"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/serverapp"
	"github.com/nightCode42/plux3/ee/backend/auditexport"
	"github.com/nightCode42/plux3/ee/backend/license"
	"github.com/nightCode42/plux3/ee/backend/saml"
	"github.com/nightCode42/plux3/ee/backend/scim"
)

// options registers the licence verifier with the keys compiled into this
// build, the enterprise audit export sinks (SEC-143), and the SAML and
// SCIM implementations. The SCIM server logs the failures it does not tell
// its client about to the process's error stream.
func options() serverapp.Options {
	return serverapp.Options{
		Edition:    license.Verifier{Keys: licensefile.TrustedKeys()},
		AuditSinks: auditexport.Provider{},
		Identity: identity.Extensions{
			SAML: saml.Provider{},
			SCIM: scim.New(scim.Options{Log: slog.New(slog.NewJSONHandler(os.Stderr, nil))}),
		},
	}
}

func main() {
	os.Exit(serverapp.Main(os.Args[1:], os.Stdout, os.Stderr, options()))
}
