// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package storetest is the conformance suite of analytics stores (ANL-010).
// It writes one fixed set of events to a store, rolls them up, runs a fixed
// set of queries, and compares the series with the ones the PostgreSQL store
// returns for the same events, which are committed with the events in
// testdata/store_conformance.json. A store that passes answers like the
// core's.
//
// The fixture holds the edge cases a store must agree on: an event delivered
// twice, a duration that is a string or a boolean and so counts as none,
// render_perf reading build_ms, values on either side of a histogram bound,
// and events either side of an hour and a day boundary. Identifiers in the
// expected series are symbolic ("env"), so any store can run it. It is for
// tests only.
package storetest
