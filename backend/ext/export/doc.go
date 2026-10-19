// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package export is the public seam through which audit entries leave Plux
// for a SIEM (SEC-143, ADR-0082).
//
// The core delivers entries in order, at least once, from a durable cursor
// that moves only after the sink has acknowledged a batch. What a sink does
// with a batch is the sink's business: the core ships the syslog sink
// (RFC 5424 over TLS, RFC 5425), and the enterprise edition adds CEF, OTLP
// logs and signed webhooks by implementing Sink and registering a Provider
// (GOV-033). The package holds standard-library and public types only, so the
// separate enterprise module can import it.
//
// The core owns every outbound connection: it hands a sink a dialer that
// refuses private, loopback and metadata addresses and an HTTP client with
// the same guard (SEC-104), so no sink can reach what the policy forbids.
// The package also carries the wire helpers the syslog-based sinks share:
// the RFC 5424 formatter, the RFC 5425 octet-counting framing and a TLS
// stream that acknowledges a batch by the receiver closing the connection.
//
// A Record carries the entry's hash and the previous entry's hash. The hash
// is the hex SHA-256 over the length-prefixed fields in the order of the
// Record's declaration, with the timestamp as RFC 3339 in UTC with
// nanoseconds, so a SIEM can recompute it and follow the chain.
package export
