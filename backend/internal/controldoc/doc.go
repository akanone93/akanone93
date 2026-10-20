// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package controldoc is the Go view of the control document of
// schema/control.fbs (SYN-060, REL-060, REL-070, ADR-0077): the kill
// switches, the mandatory update and the forced rollback of one app,
// environment and channel, and the environment's feature flags with their
// per-segment values (ABT-006), its running experiments with their
// allocations and its global holdout (ABT-001, ABT-004).
//
// Encode writes a [Document] as a FlatBuffers buffer and is deterministic:
// the same document always yields the same bytes, whatever the order in
// which its kill switches, flags or experiments were given (CMP-002). Decode
// reads one back.
// Neither signs or verifies anything. The server wraps the encoded bytes
// in the update-metadata envelope of the control role (package updatemeta)
// and a device verifies that envelope before it calls anything here
// (SEC-052); Decode bounds its work and reports a buffer that does not fit
// the schema, but it is not a verifier of hostile input.
//
// The accessors flatc generates are in package controlfbs.
package controldoc
