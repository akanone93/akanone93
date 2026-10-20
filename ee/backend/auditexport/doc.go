// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Package auditexport is the enterprise edition's audit export sinks: CEF
// over syslog with TLS, OTLP logs and signed webhooks (SEC-143).
//
// The core owns the destinations, their credentials, the cursors and the
// retries. This package only renders a batch of audit records for one kind
// and delivers it through the connections and HTTP client the core passes
// in, so every outbound request still passes the core's egress guard. A sink
// refuses every write unless the licence lists FeatureSIEMExtended for
// writing (GOV-030), and a refused write is returned unchanged so the core
// keeps the destination's cursor where it was.
package auditexport
