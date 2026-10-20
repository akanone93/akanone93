// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package dsr serves data subject requests (SEC-162, spec section 15): the
// export and the erasure of everything an organisation holds about a device
// or a pseudonymous user, and the map of where personal data is stored.
//
// The personal-data map (Map) is the declarative registry of every store
// that holds, or by its column names could hold, personal data: its device
// and user keys, its personal fields, its retention and what erasure does to
// it. docs/operations/personal-data.md is generated from it (go generate in
// this package), and a test fails when a migration adds a column named like
// personal data to a table the map does not list.
//
// Export reads every store of the subject in one transaction and returns one
// JSON archive with a section per store. The archive is bounded by
// dsr.exportBytes: a subject whose data exceeds it is refused (PLX-1320),
// never truncated, so an archive is always complete. It names columns
// explicitly; key material, secret hashes and the sealed push token are not
// exported.
//
// Erase removes the subject's rows store by store, in a fixed order that
// ends with the device record, one transaction per store. Each store that
// lost rows leaves a tombstone (subject hash, store, count, time), and the
// request is audited. An interrupted erasure is finished by repeating it;
// repeating a finished one changes nothing.
//
// Invariants:
//
//   - A subject is identified in tombstones, requests and audit entries by
//     the SHA-256 of its organisation, kind and identifier, never by the
//     identifier, so the chain and the tombstones cannot bring an erased
//     subject back.
//   - Audit entries are never erased (SEC-140); they carry only pseudonymous
//     identifiers, and the map says so.
//   - Every query is bound to the caller's organisation twice, by row-level
//     security and by its own predicate; a subject of another organisation
//     is indistinguishable from one that has no data (PLX-8501).
//   - Erasing deletes the device record and its credentials, so the device
//     must register again.
package dsr
