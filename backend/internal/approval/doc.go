// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package approval is the approval engine: it decides which changes need a
// second person's agreement, records who agreed to exactly what, and lets a
// guarded change proceed only on an approval that still counts (GOV-020 to
// GOV-023, TST-005, LIM-006, SEC-080, FN-007, FN-055, ADR-0076).
//
// # Policies
//
// A policy is a document (schema/json/policy.schema.json) that says which
// changes it matches (resource type, environment, action and risk
// attributes) and what approval they need (a number of approvers, roles,
// named approvers, distinct teams, stages, an expiry). Policies belong to an
// app, are versioned and audited, and are exported as the files of the Git
// layout. The first time an app is asked about, the default policies of its
// security profile are written as ordinary policy documents that the
// organisation can tighten; a default an administrator deleted is not
// written again.
//
// A change that matches several policies needs every one of them satisfied.
//
// # Artifacts and requests
//
// A guarded domain service describes what it is about to do as an Artifact
// and asks Require. The artifact names what changes (its subject) and the
// content it changes to by a hash, so an approval is about one exact
// artifact: when the content changes the hash changes, the earlier approvals
// stop counting and Require answers PLX-8002 (GOV-022). When nothing
// approved is on file, Require opens a request, in a transaction of its own
// so that it survives the refusal that rolls the caller back, and answers
// PLX-8001 naming it; a request that outlived its policy's expiry is
// PLX-8004. An approval is spent by the change it approves.
//
// Require must be called before the caller appends to the audit log, because
// the request is opened in a second transaction that appends to the same
// chain.
//
// # Decisions
//
// An approver decides a request with approvals.decide, which needs a fresh
// step-up (SEC-103). The requester and the author of the change are never
// eligible (PLX-8003), nor is anyone a policy's roles and named approvers
// leave out. Each decision is a record whose canonical payload is written in
// the audit chain's own transaction; the worker role, which alone holds the
// audit key, signs the payload later, and VerifyRecord checks it offline.
// Policies with stages (sequential or parallel) and distinct-team rules are
// enterprise features; the engine enforces them wherever a policy states
// them, but a policy can only be written with them under a licence that
// allows them.
//
// # Evidence
//
// The review bundle (GOV-023) is assembled from a Reviewer per resource
// type: the release service supplies the diff, reports, golden-image
// differences, install base and changelog of a release.
package approval
