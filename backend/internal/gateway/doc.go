// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package gateway is the secure gateway: the one path a device request to
// a customer's API takes through the Plux server (SEC-030–SEC-032,
// ADR-0056).
//
// A device that has proved its key (DPoP, S3) calls a registered upstream
// by name. The gateway checks, before it forwards anything, the method and
// path the upstream and operation allow, the plugin's capability, the
// assurance level the operation needs, the device's rate, and, for a
// transaction, the strong customer authentication signature and its
// dynamic linking. It then forwards over a pooled connection that
// authenticates to the upstream (mutual TLS, OAuth 2.0 client credentials
// or a key held encrypted), with a short-lived signed assertion in
// Plux-Assertion that tells the upstream who is calling. The upstream
// trusts that assertion and nothing the device sent.
//
// The package also holds the upstream registry (SEC-031), the exchange of
// a host's user token for the token the gateway forwards (SEC-026,
// RFC 8693) and the challenges and verification of transaction signing
// (SEC-027, SEC-028, SEC-032). Bodies are never logged, traced or stored;
// only a transaction's body is read, bounded by gateway.requestBytes.
//
// Only internal/signing touches private keys: assertions and user tokens
// are signed through a signing.TokenSigner (L-3).
package gateway
