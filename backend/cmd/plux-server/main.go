// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Command plux-server runs the Plux server in the api, worker, fnrunner,
// egress, builder and gateway roles (spec §11). It is the open-source core:
// no enterprise feature is enabled (GOV-033). The enterprise binary
// ee/cmd/plux-server-ee wraps the same code with a licence verifier.
package main

import (
	"os"

	"github.com/nightCode42/plux3/backend/serverapp"
)

// main delegates to serverapp so that the command logic is testable without
// terminating the test process.
func main() {
	os.Exit(serverapp.Main(os.Args[1:], os.Stdout, os.Stderr, serverapp.Options{}))
}
