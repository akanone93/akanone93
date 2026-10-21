// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Command plux-load runs a fleet of synthetic devices against a Plux server
// and prints a JSON report of request rates, latency percentiles and error
// counts (DEP-051, NFR-023).
//
//	plux-load -server https://localhost:8080 -app <app-id> -env staging \
//	    -devices 1000 -duration 10m -speedup 10 -forwarded-for -ca-file ca.pem
//
// The devices register with development evidence, so the server needs
// attestation.developmentProvider and the environment must not be a
// production one. They check the manifest, fetch the signed control
// document, send telemetry batches and, with -function, call a function, at
// the runtime's pacing divided by -speedup. Nothing a device sends comes
// from a person or a real device. The process exits 1 when more than
// -max-error-ratio of the requests fail. The generator and its report are
// described in docs/operations/sizing.md.
package main
