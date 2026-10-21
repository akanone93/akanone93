// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package walkthrough drives the scripted regulated-industry audit
// walkthrough, the exit criterion of phase 9 (spec §5.1, plan §5.12): a
// self-hosted HA installation on kind is exercised end to end with the plux
// CLI and a synthetic device fleet, and every step leaves evidence.
//
// The package has four parts. [Runner] runs an ordered list of [Step]s,
// stops at the first failure and keeps a transcript. [Store] and [Seal] turn
// the evidence the steps wrote into a signed bundle: every evidence file is
// named with its SHA-256 in a statement that is signed, with a key generated
// for the run, and checked before anyone parses it ([Verify]). [Steps]
// defines the nine steps against small interfaces ([Commander], [Fleet],
// [Hooks]) so that the step logic is tested without a cluster. The OIDC test
// provider ([IDP]) and the TLS syslog sink ([Sink]) are the two services the
// installation talks to during the run.
//
// Invariants: no key, token or secret is ever written to the transcript or
// to an evidence file (commands are recorded as arguments only; the
// credential travels in the environment); the signing key lives in memory
// for the run and only its public half is written out; evidence file names
// are plain names, never paths.
package walkthrough
