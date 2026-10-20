// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package gitsync exports an app to a Git repository on every publish
// (GOV-011, SCH-006, ADR-0082).
//
// An app manager configures a remote (HTTPS only), a branch and a
// credential, which is the name of an environment secret held under
// envelope encryption (SEC-106). From then on the publish that records a
// version also enqueues an export job in the same transaction (SRV-024).
// The job, which runs in the worker, writes the SCH-006 layout of the app,
// as document.Service.Export renders it, into one commit on top of the
// remote's branch, authored by the publisher under a no-reply address, with
// the release identifier in the message, and pushes it.
//
// Invariants:
//
//   - An export is idempotent per published version: a row with a unique
//     key records it, a finished one is never repeated, and a commit whose
//     tree equals its parent's is not made, so the several versions one
//     app publish records give one commit.
//   - A push the remote refuses is PLX-8503 and fails the job, which the
//     job system retries with backoff; the failure is recorded as a code,
//     never as the remote's reply.
//   - The credential is opened in the worker for one push, handed to the
//     transport and never logged, returned or stored (SEC-092).
//   - A remote is reached only through a loopback CONNECT proxy that owns
//     the connections of one push. go-git offers no way to give one
//     operation its own dialer, and its HTTP transport follows a first
//     redirect, so a check made before connecting would be undone by a
//     redirect or a second DNS answer. The proxy resolves the name itself,
//     applies the httpx.Policy to every address, and dials the address it
//     checked, so the connection that is made is the one that was
//     validated, redirects included (SEC-163).
//   - Nothing is written to disk: the repository lives in memory for the
//     length of one job.
//
// The import side is the CLI's: it reads a repository's working tree and
// sends it through the ordinary draft import, which validates every
// document.
package gitsync
