// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Package scim is SCIM 2.0 provisioning for the enterprise edition (GOV-004,
// ADR-0074): the implementation of the core's identity.SCIMService seam. It
// speaks the protocol of RFC 7643 and RFC 7644 over HTTP and decides nothing
// else. Every question of who may be created, changed, removed or put in a
// group is the core's identity.Directory's, which applies the organisation's
// own rules, so a SCIM client can do nothing the core would refuse and
// deprovisioning ends a person's access, tokens and sessions in the core, not
// here.
//
// The server offers /Users and /Groups (list, create, get, replace, patch,
// delete), /ServiceProviderConfig, /Schemas and /ResourceTypes. A user is a
// person of the organisation, a group is a team. The core mounts the handler
// at /scim/v2/ and strips that prefix, bounds the request body and checks that
// the licence lets the feature be read.
//
// Behaviour worth knowing:
//
//   - Every request, including a request for a path that does not exist, is
//     authenticated first by the bearer token the directory resolves to an
//     organisation. Nothing is revealed about a path to a caller without one.
//   - Writes are refused with PLX-8600 unless the licence lets SCIM be
//     written (edition.EnabledForWrite); reads follow the core's check.
//   - Filters are a single equality on userName, externalId or displayName
//     (users) and externalId or displayName (groups), which is what
//     provisioning clients use to find what they created. Anything else is an
//     invalidFilter error. Sorting is not supported and is ignored, as the
//     specification allows.
//   - PATCH supports add, replace and remove on the attributes Plux stores,
//     with the forms Microsoft Entra ID and Okta send: operation names in any
//     case, no path with an object value, booleans as strings, and
//     members[value eq "id"] paths. Attributes of the core schema that Plux
//     does not store are accepted and ignored; any other attribute is an
//     invalidPath error.
//   - Every resource carries a weak ETag (its version), honoured in
//     If-Match on PUT, PATCH and DELETE and in If-None-Match on GET.
//   - Failures are SCIM error documents. The detail of a rejected request
//     starts with PLX-6220; an unexpected failure says only that the request
//     could not be processed and is logged.
//
// Specification: GOV-004, GOV-005, GOV-033; RFC 7643, RFC 7644; ADR-0074.
package scim
