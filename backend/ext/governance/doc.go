// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package governance is the public seam through which the enterprise
// edition adds change freezes and compliance reports to the core's approval
// and break-glass machinery (GOV-025, GOV-026, ADR-0076).
//
// The core stores the freeze windows an organisation defines, gathers the
// production changes of a period and signs a report with the audit key; the
// enterprise edition decides what a window means and renders the report as
// a document. Two interfaces carry that: FreezeCalendar evaluates windows,
// and ReportRenderer writes a report's PDF. The core ships the community
// defaults, NoFreezes and no renderer, and refuses the features with
// PLX-8600 unless the licence lists them (GOV-030, GOV-033). The package
// holds standard-library and public types only, so the separate enterprise
// module can import it.
//
// # Freeze windows
//
// A Window is a calendar entry in an IANA time zone: a start as wall-clock
// time, a duration, and an optional recurrence. The recurrence language is
// a subset of RFC 5545's RRULE, chosen because a freeze calendar needs
// nothing more: FREQ=DAILY or FREQ=WEEKLY;BYDAY=MO,TU,..., optionally with
// UNTIL=YYYYMMDD as the last local date a window may start on. Wall-clock
// starts follow daylight-saving time: a start that does not exist on a
// date, because the clocks jumped over it, begins when the gap ends, and an
// ambiguous start, repeated when the clocks fall back, takes its first
// occurrence. A window lasts Duration of elapsed time, not of wall-clock
// time.
//
// # Compliance reports
//
// A Report lists every production change of a period. Its statement is the
// report's JSON, and the signed form is a DSSE envelope (PayloadType) over
// that statement, so that an auditor verifies it with nothing but the
// audit public key: ParseEnvelope and Envelope.Open do so and are part of
// this package, not of the enterprise module, because trust in evidence must
// not depend on a licence. Open checks the signature before it parses the
// payload.
//
// Invariants:
//   - Time is passed in, never read from a global clock.
//   - Nothing here holds a private key; the core signs, through
//     internal/signing, and hands the signature to NewEnvelope.
//   - An implementation that cannot decide returns an error and never a
//     guess: a freeze that cannot be evaluated is not silently lifted.
//
// Specification: GOV-025, GOV-026, GOV-030, GOV-033; ADR-0076.
package governance
