// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package fnhost is the Go host of the Plux Function ABI (ADR-0062, FN-015):
// it validates a function module, runs each invocation in a fresh instance on
// wazero and enforces the limits and capabilities of FN-010 to FN-012 and
// FN-052. The server's function runner, the CLI's `plux fn test` and the
// builder use it, so the package imports no storage, signing or network
// package (L-4): everything a function may reach is a Host the caller
// supplies.
//
// # Invariants
//
//   - A module is checked before any guest code runs: exports, imports and
//     the absence of a start function. Only then is plux_abi_version called,
//     in a throwaway instance.
//   - Every invocation has its own instance and linear memory (FN-010); nothing
//     is shared between calls except the compiled code.
//   - A capability that is not granted answers -2 to the guest and the Host
//     is never called (FN-012). A device module cannot import a server-only
//     function (FN-052).
//   - Wall time, memory, input, output, log bytes, HTTP calls and call depth
//     are bounded per call (FN-011). A limit is reported as its registered
//     error, never as an unclassified trap.
//   - Pointers and lengths from the guest are bounds-checked on every host
//     call. A bad one ends the call with PLX-7007; it never panics the host.
//   - WASI is wazero's wasi_snapshot_preview1 with no file system, arguments
//     or environment. Standard output and error become log records, the
//     clocks and random_get read the engine's injected clock and randomness.
//
// The ABI is specified in docs/functions/abi.md and tested by the vectors in
// schema/testdata/functions, which the Dart device host runs too.
package fnhost
