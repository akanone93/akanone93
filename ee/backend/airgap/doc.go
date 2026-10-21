// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Package airgap builds and verifies the air-gapped installation bundle
// (DEP-040, ADR-0083): one tar archive with the container images as a single
// OCI image layout, the packaged Helm chart, the operations documents
// (including the egress list and the install guide), and a signed
// manifest.json that lists every other file with its size and SHA-256.
//
// Invariants:
//
//   - The manifest is signed with Ed25519 in the envelope the licence files
//     use ({"signed", "signatures"}); verification checks the signature
//     before it decodes the manifest, and then refuses any file that is
//     missing, extra or altered, and any entry that is not a regular file.
//   - Building is deterministic: entries are sorted and carry no time,
//     owner or mode of the source, and blobs are deduplicated by digest, so
//     the same inputs and key give the same archive.
//   - Nothing here touches the network, and the private key is only read
//     from the file the operator names; it is never logged or written.
//
// The tool in ee/backend/cmd/plux-airgap is the only caller.
package airgap
