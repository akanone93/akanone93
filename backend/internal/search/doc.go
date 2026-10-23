// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package search is the server's full-text search over apps, plugins,
// pages, components, templates, translation keys and functions
// (SRV-061, ADR-0082).
//
// The index is PostgreSQL's own: GIN expression indexes over
// plux_search_vector, which uses the language-neutral "simple"
// configuration and blanks the separators of paths. Every word of a query
// is a prefix term and all of them must match, so "calc" finds "calculator"
// and "loan calc" finds a page that has both. Results are ranked with
// ts_rank_cd and carry a snippet with the hits marked « and ».
//
// Invariants:
//
//   - What a caller may see is decided inside the SQL statement, not after
//     it. The statement takes the apps the caller holds search.read on and
//     restricts every branch of the search by them, so a row of another
//     app is never read, ranked or sent to the service, and row-level
//     security keeps every other organisation out underneath.
//   - The words of a query reach the database only as letters and digits
//     joined by the tsquery operators this package writes, never as query
//     syntax.
//   - The page size comes from search.resultsPerPage, in force for the
//     organisation (LIM-001).
//
// The index follows the data: the indexed text of a document is written
// by the document service in the statement that writes the document, and
// apps and plugins are indexed by their own columns.
package search
