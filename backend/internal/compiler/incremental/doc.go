// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package incremental compiles a project again after an edit by redoing only
// what the edit changed (CMP-051, ADR-0089): the edited pages and the pages
// that refer to them, found through the reference graph. The bundles are
// byte-identical to the ones the full compiler makes for the same project
// (CMP-002); a Cache that cannot show that falls back to a full
// compilation and says why.
//
// The compiler stays a pure library (layering rule L-5): a Cache is a value
// the caller owns and passes in, nothing is global, and the only I/O is
// reading the fs.FS the caller supplies. The package serves the development
// loop (DEV-010): Studio saves a draft, the dev session compiles it here,
// and the devices receive the sections that changed together with a
// verdict, per page, on whether the page keeps its state.
//
// The mechanism is in the compiler package (Memo, Recompile); this package
// decides what changed. Per call it hashes the project's files and compares
// them with the previous call's, parses the changed documents only, takes
// the pages they belong to, adds the pages that refer to them (to the
// fixpoint), and asks the compiler to recompile those. Anything it cannot
// reason about is a full compilation: a document that is not a page or an
// action graph of a page, a file added or removed, a changed page identity,
// a diagnostic.
//
// Releases are made by compiler.Compile: an incremental Result carries the
// bundles, the files, the reference graph and no diagnostics, but not the
// reports of the whole project (accessibility, localisation, stored state,
// collections, gateway sources), which Recompile does not recompute.
package incremental
