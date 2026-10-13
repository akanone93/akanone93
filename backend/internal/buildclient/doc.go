// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package buildclient is the worker's side of a function build: the
// function.Builder that calls the builder role over mutual TLS, and the
// function.ArtifactSigner that has signing/ sign the artifact statement with
// the function-artifact key (FN-009, FN-016, ADR-0063).
//
// The worker reads the source archive from the object store and sends it in
// the request, so that the builder role needs neither an object-store
// credential nor a route to the store: a source is bounded by
// function.sourceArchiveSize, whose registry maximum also bounds the request
// the builder accepts. What the role returns is untrusted until the worker
// has judged it (function.RunBuild) and signed its statement.
//
// Both types are safe for concurrent use.
package buildclient
