// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Package saml is SAML 2.0 single sign-on for the enterprise edition
// (GOV-004, ADR-0074): the implementation of the core's
// identity.SAMLProvider seam. It turns a posted SAMLResponse into a verified
// identity.Identity, builds the AuthnRequest that starts a sign-in, reads an
// identity provider's metadata and writes the service provider's own. It
// decides nothing about who a person is: the core links, provisions, maps
// groups and opens the session.
//
// The XML types, the AuthnRequest and the metadata documents come from
// github.com/crewjam/saml, the signatures are checked with
// github.com/russellhaering/goxmldsig and the encrypted assertions are
// opened with crewjam's xmlenc. The response verification itself is this
// package's own, because the library's reads the clock and its skew from
// package variables, trusts a signed response's assertions without looking at
// them and accepts the first of several assertions, none of which a
// multi-tenant server can allow.
//
// Invariants of Verify:
//
//   - Nothing is read from a message before a trusted signature has been
//     verified over it: the response is read through the element the
//     signature digest was computed over, never through the document that
//     was posted (XML signature wrapping).
//   - A signature of the identity provider's is required on the response or
//     on the assertion. Only the certificates in the stored descriptor are
//     trusted; a certificate a message carries is only ever compared with
//     them. Signatures and digests with SHA-1 are refused.
//   - Exactly one assertion is accepted. A document with comments, or with a
//     repeated attribute, is refused, so text cannot be split in two.
//   - Issuer, audience, recipient, destination, request, NotBefore and
//     NotOnOrAfter are checked, with the skew the core passes in, against the
//     clock the core passes in. An assertion that carries a condition this
//     package does not understand is refused.
//   - An assertion is accepted once: its identifier is recorded in the
//     core's replay cache, last, so that only an assertion that passed every
//     other check can fill it. A cache that cannot answer is a refusal.
//   - A failure wraps identity.ErrInvalidResponse or identity.ErrReplayed
//     and says why in its text for the log; the core shows the caller only
//     the code (PLX-6210, PLX-6211).
//
// The package keeps no state and reads no clock: the instant of every
// check and of every request is the one the core passes in. The only
// randomness it does not receive is the identifier the library gives an
// AuthnRequest. The decryption key it is given is used within the call.
//
// Specification: GOV-004, GOV-033, SEC-103; ADR-0074.
package saml
