// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package builderrole is the builder role: it serves the internal
// BuilderService, which compiles an uploaded function source into a
// WebAssembly module (FN-004, FN-005, FN-016, FN-031, ADR-0063).
//
// A build reads the source archive from the request, validates function.json
// against the document schema and the request (PLX-7205), runs the pipeline of
// package builder in a sandbox, reads the schemas out of the schema build on
// the function host, and runs the document's stored test cases against the
// module on the same host. The response carries the module, its provenance,
// the document and the test results; the worker signs the artifact statement
// and stores everything.
//
// # Invariants
//
//   - The role holds no database credential, no object-store credential and no
//     signing key (L-4): the package imports neither storage, signing nor a
//     database driver, which a test enforces with `go list -deps`.
//   - The listener requires and verifies a client certificate (SEC-043).
//   - A build that does not compile is a response with error diagnostics and
//     no module; only a failure of the role itself is an RPC error, which the
//     worker retries.
//   - Where the kernel refuses the sandbox's namespaces the role refuses to
//     build. Building without a sandbox is the explicit setting
//     builder.sandbox: none, for development; the role then logs a distinct
//     warning when it starts and with every build (ADR-0063).
//   - Limits come from the registry (LIM-001): a build runs under the smaller
//     of the request's limits and the registry maximum.
//   - The number of builds at once is bounded; the next is refused as
//     unavailable rather than queued without bound.
//
// A [Service] is safe for concurrent use. It owns the goroutine of its HTTP
// server and the function host engine; Stop releases both.
package builderrole
