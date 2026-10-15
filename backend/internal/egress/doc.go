// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package egress is the only route from the function runner to the
// internet (FN-013, FN-017, ADR-0064).
//
// A Server is a forward proxy that understands one HTTP method, CONNECT,
// and nothing else: every request a function makes, plain HTTP included,
// is a tunnel, and the proxy never parses what crosses it. TLS to the
// destination therefore stays end to end between the module's host call
// and the destination, and the proxy holds no certificate for it. The
// listener requires mutual TLS with a certificate from the internal CA
// (SEC-043).
//
// The caller names the function's domain allowlist in a header with each
// tunnel; the proxy keeps no per-tenant state. It decides, in this order:
// the port is one of the configured ports; the target is a DNS name, never
// an IP literal; the name matches the allowlist; and every address the
// name resolves to passes the gateway's SSRF rules (httpx.Policy), so a
// name that resolves to a private address is never reachable. It then
// dials the address it validated, never the name again, which closes the
// DNS-rebinding window. A tunnel ends when either side closes, when it
// has been idle too long, when it has been open too long, or when a
// direction has carried too many bytes.
//
// The Dialer and Transport are the client side, used by fnrunner. A
// refusal is reported as ErrRefused with the PLX-7010 code the proxy
// sent; it never carries upstream detail.
//
// A Server is safe for concurrent use and owns every goroutine it starts;
// Stop closes the listener, lets tunnels finish until its context ends,
// then closes the rest and waits for them.
package egress
