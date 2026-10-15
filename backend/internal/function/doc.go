// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package function is the server's function domain: the sources, builds,
// immutable versions and aliases of Plux Functions, and the key-value
// store a server function reads and writes (FN-005, FN-006, FN-009,
// FN-016, FN-018, spec section 16, ADR-0063).
//
// A developer uploads a source archive, asks for a build and deploys the
// build that succeeded. The build itself runs on the builder role: the
// package only queues it (a River job, enqueued in the transaction that
// records it) and, when the worker runs the job, hands the source to an
// injected Builder, asks an injected ArtifactSigner to sign the artifact
// statement and stores the result. Neither interface is implemented here:
// the builder role's client and the worker's signing adapter are, so that
// no private key and no compiler reach this package (L-3, L-4).
//
// Invariants:
//
//   - Only a succeeded build that carries a signature becomes a version
//     (FN-009). Failing stored test cases block a deploy, a promotion and
//     a release only in a production environment; elsewhere the version
//     is deployed and marked as failing its tests (FN-031).
//   - A version is immutable: the database refuses an update of its row
//     (FN-006). Aliases move; versions never do.
//   - Version numbers are assigned inside the deploy transaction, one
//     above the highest of the function in its app, whatever the
//     environment, so a number names one signed build everywhere. A
//     version reaches another environment only by Promote, which copies
//     the row without a rebuild (FN-006).
//   - Every table is bound to its organisation by row-level security, the
//     key-value store included (FN-018, SRV-022).
//   - Deploying to a production environment needs function.deploy.production,
//     which requires a second factor, until the approval engine of P9
//     replaces it (FN-007); every other deploy needs function.deploy.
//   - Limits come from the registry: function.sourceArchiveSize,
//     function.moduleSize, function.kvBytes, function.kvKeys and
//     function.kvValueBytes (LIM-001).
//
// The administrative methods authorise a principal. ResolveVersion and the
// key-value methods are for the server's own callers (the release service,
// the host service the function runner reaches); they take the tenant
// from the caller, who has already authorised the request with a grant.
package function
