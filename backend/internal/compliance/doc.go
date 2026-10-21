// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package compliance produces the compliance reports of GOV-026: for a
// period, every production change with its author, approvers, timestamps,
// content hashes and the evidence that proves them, as a signed JSON DSSE
// envelope and as a PDF (ADR-0076).
//
// A report is a reading of the ledger of production changes (package
// prodchange), the decisions of the approval engine and the break-glass
// invocations, taken when it is requested and stored as its statement: the
// canonical JSON of a governance.Report. The statement is what is signed, so
// a report can be fetched again, byte for byte, long after the data it was
// built from has been archived.
//
// # Signing
//
// The API role that takes a request holds no private key (L-3, ADR-0006). The
// worker role, which alone holds the audit key, signs the statement later:
// Sign wraps it as DSSE (governance.PAE) and stores the signature, the way
// the approval engine signs its decision records. A report that is not yet
// signed has no content to fetch. Verification needs the audit public key
// only, and is part of the public package ext/governance, so that trust in
// the evidence does not depend on a licence.
//
// # Editions
//
// Requesting and fetching reports need the licence's
// FeatureComplianceReports (PLX-8600 otherwise); the PDF is written by the
// enterprise edition's governance.ReportRenderer from the signed envelope,
// at the time it is fetched.
//
// Invariants:
//   - A report lists a period completely or is refused: more changes than
//     governance.reportChanges is an error, never a silent truncation.
//   - Only internal/signing touches the key; this package hands it a message.
//   - Nothing the report holds is a secret: people appear by identifier and
//     display name, never by email address.
//
// Specification: GOV-026, GOV-030, SEC-103, LIM-001; ADR-0076.
package compliance
