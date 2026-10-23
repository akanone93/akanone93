// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package identity is the extension seam of identity federation (GOV-004,
// ADR-0074): the interfaces through which the enterprise edition adds SAML
// 2.0 single sign-on and SCIM 2.0 provisioning to the open-source core.
//
// The core owns everything that decides who a person is and what they may
// do: account linking, group mapping, just-in-time provisioning, sessions,
// deprovisioning and audit. The enterprise module owns only the protocols.
// SAMLProvider turns an HTTP POST into a verified Identity, and nothing
// else; the core then treats that identity exactly as it treats one from an
// OpenID Connect provider. SCIMService serves the SCIM REST protocol over
// the Directory the core implements, so a SCIM client can change nothing the
// core's own rules would refuse.
//
// The package is outside internal/ on purpose: the enterprise module is a
// separate Go module and may import only public packages of the core. It
// holds interfaces, plain data and community defaults that report PLX-8600
// (the feature is not licensed), so the core builds, tests and runs without
// the enterprise module.
//
// Invariants:
//   - An Identity is returned only after every check of its protocol has
//     passed; an implementation that cannot verify returns an error that
//     wraps ErrInvalidResponse or ErrReplayed, never a partial identity.
//   - Time and the replay cache are passed in, never read from globals, so
//     the core controls the clock and the skew bound (the limits registry).
//   - Nothing here carries a secret except SAMLConfig.DecryptionKeyPEM, which
//     the core opens from sealed storage for one call and the implementation
//     must not retain, log or return.
//
// Specification: GOV-004, GOV-030, GOV-033, SEC-103; ADR-0074.
package identity
