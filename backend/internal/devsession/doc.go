// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package devsession opens dev sessions and pairs devices with them
// (DEV-002, DEV-003, ADR-0087).
//
// A person who holds devsession.create on a non-production environment
// opens a session with CreateSession. The environment must not be a
// production one and must be listed in the server's
// devSession.enabledEnvironments; anything else is refused with PLX-9203.
// When the environment's security configuration sets
// devSessionRequireStepUp, creating a session also needs a recent step-up
// (ADR-0075 Revision).
//
// # Pairing
//
// The creator, and only the creator, asks for pairing tokens: a compact JWS
// of type plux-pair+jwt, signed with the development token key, that names
// the session, the creator, the app and the environment and expires within
// devSession.pairingTokenSeconds (two minutes at most). Only its SHA-256 is
// stored. The device presents it to Pair with its access token and DPoP
// proof; the signature and expiry are checked, the token is spent in one
// statement (PLX-9201 when it expired, PLX-9202 when it was used), the
// session must still be live (PLX-9200) and have room for another device
// (PLX-9208), and the device is recorded bound to the thumbprint of its DPoP
// key (SEC-021). The answer carries a socket ticket the device presents once,
// with the same key, at the WebSocket upgrade; ConsumeSocketTicket spends it
// (PLX-9205).
//
// A session is live while it has not been ended, its lifetime
// (devSession.lifetimeMinutes) has not passed, and it has seen traffic within
// devSession.idleMinutes; Touch records traffic.
//
// Invariants:
//   - No pairing token or socket ticket is stored, logged or returned except
//     in the one response that issues it; the tables hold hashes.
//   - Only the signing package touches keys: tokens are signed through
//     signing.TokenSigner.
//   - Limits come from the registry and time from the injected clock.
//
// Specification: DEV-002, DEV-003, SEC-021, SEC-103, LIM-001; ADR-0087,
// ADR-0075.
package devsession
