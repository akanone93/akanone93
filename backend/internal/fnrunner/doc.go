// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package fnrunner is the fnrunner role: it runs the WebAssembly modules of
// Plux Functions for the api role (FN-009 to FN-013, SEC-043, ADR-0064). It
// implements the internal FunctionRunnerService on top of the function host
// in package fnhost.
//
// # Invariants
//
//   - The role holds no database credential, no object-storage credential
//     and no signing capability (L-4, FN-013). It reaches modules, the
//     key-value store, secrets and nested invocations only through
//     FunctionHostService, authorised by the grant of the invocation in
//     flight, and the internet only through the egress role. A test fails
//     the build if the package imports storage, signing or a database
//     driver.
//   - A module is compiled only after its SHA-256 equals the requested one,
//     its signature verifies against a key from the role's own
//     configuration, and the signed statement names that SHA-256 (FN-009).
//     Keys never arrive from the api role. Any failure is PLX-7019 and
//     nothing is compiled.
//   - Every invocation runs in a fresh instance (FN-010); only compiled code
//     is shared, in a cache bounded in modules and in bytes.
//   - The number of concurrent invocations is bounded; the next is refused
//     with PLX-8040 instead of queueing.
//   - Outbound HTTP follows no redirect: a 3xx answer is the function's to
//     see. The body of a response is bounded; a larger one is a limit
//     failure, never silently truncated.
//   - Nothing from an invocation but its outcome leaves the role: labels
//     carry no app, user or input, and a log line carries identifiers and
//     the PLX code, never input, output, claims or secrets (SEC-092).
//
// The Runner is safe for concurrent use. A Service owns the listener of the
// role and the goroutines of its HTTP server; Stop ends them.
package fnrunner
