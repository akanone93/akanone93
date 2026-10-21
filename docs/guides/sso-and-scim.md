<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Single Sign-on and SCIM Guide

How people sign in to Plux with your identity provider, how their groups become Plux teams and
roles, and how SCIM provisions and removes them. The requirements are `GOV-004`, `SEC-100`,
`PLX-6210` to `PLX-6211` and `PLX-6220`, and [ADR-0074](../adr/0074-identity-federation.md).

Edition note: OpenID Connect sign-in and the group mapping are in the core. SAML 2.0 and SCIM 2.0
are enterprise features (`ee/backend`), plugged into the core through its identity seams.

## 1. The idea in one page

Your identity provider authenticates a person. Plux decides who that person is inside the
organisation, using three things:

1. **The link.** An identity is linked to a Plux account by the provider's issuer and subject.
   The first sign-in links it by the address the provider vouches for.
2. **The mapping.** The provider's groups become Plux teams and roles through a mapping document,
   so a person's grants match their groups at every sign-in.
3. **The provisioning rule.** Whether a person with no account may be created on sign-in
   (just-in-time, JIT) or only by SCIM or an invitation.

Plux never trusts a claim the provider did not sign. Signed assertions are required for SAML,
and the ID token is verified against the provider's keys for OpenID Connect.

## 2. Configure a provider

Providers are administered per installation and per organisation: several can exist, each with
its own issuer and client, and each organisation sees only its own. Use the identity provider
RPCs in the [API reference](../reference/api/openapi.yaml) (`FederationService`):

- `CreateIdentityProvider`, `UpdateIdentityProvider`, `DeleteIdentityProvider`,
  `GetIdentityProvider`, `ListIdentityProviders`
- `ListSignInProviders` returns what the sign-in page offers to an unauthenticated visitor.
- `GetGroupMapping` and `SetGroupMapping` read and write the group mapping.

Once a provider exists, password sign-in can be turned off with `auth.studio.allowPasswordLogin`
set to false, so the provider becomes the only way in. Keep at least one Owner with a working
second factor before you do this (see the [sessions guide](sessions-and-step-up.md)).

## 3. OpenID Connect (core)

The client follows the standard authorisation code flow with the provider. What Plux records and
checks:

- The `groups` claim (or the claim you configure) is read and mapped as section 5 describes.
- The authentication context (`acr` and `amr`) the provider reports is recorded on the session.
- Back-channel logout: a logout token from the provider ends the matching sessions.

## 4. SAML 2.0 (enterprise)

The service provider speaks SAML 2.0 with the identity provider's metadata, imported with the
provider. Sign-ins may start at Plux or at the provider (IdP-initiated), and assertions must be
signed. Encrypted assertions are supported.

Plux verifies every response itself and reads nothing from it before its signature is checked,
which closes the XML signature wrapping attack. Only the first assertion is accepted, and the
clock skew comes from the limits registry. An assertion that was already used is refused by a
replay cache keyed by its identifier (`PLX-6211`). An invalid response is `PLX-6210`. The service provider's own metadata is available from `FederationService.GetSamlMetadata`
for the provider's administrator to import.

## 5. Group mapping

A mapping document says which groups give which teams and roles. For each sign-in Plux:

1. Reads the groups the provider sent.
2. Makes the person's team memberships match the mapped teams.
3. Makes the person's role grants match the mapped roles, within the scopes the mapping names.

Mapping is applied at every sign-in, so removing a person from a group in the provider changes
their grants at their next sign-in. As with any grant, a role can never grant more than its
creator holds (ADR-0073).

## 6. Just-in-time provisioning

Just-in-time (JIT) provisioning creates an account the first time a person signs in. It is **off
by default**. When it is on, the new person receives only the roles the mapping gives them, and
nothing else. Leave it off where SCIM provisions people, so that there is one source of truth for
who exists.

## 7. SCIM 2.0 (enterprise)

SCIM provisions people and teams from the provider. The server offers:

- `/scim/v2/Users`: list, create, get, replace, patch and delete. A user is a person of the
  organisation.
- `/scim/v2/Groups`: list, create, get, replace, patch and delete. A group is a team, and its
  members are the team's members.
- `/scim/v2/ServiceProviderConfig`, `/Schemas` and `/ResourceTypes`, which clients read to
  discover the server.

Requests are authenticated with a bearer token for the organisation. Administer the tokens with
`FederationService.CreateScimToken`, `ListScimTokens`, `RotateScimToken` and `RevokeScimToken`.
A token is stored hashed. Rotation keeps the previous token valid for
`auth.scimTokenOverlapSeconds` (default 86400 seconds, one day), so the provider can switch over
without a failed sync.

SCIM decides nothing about who may exist. Every create, change, removal and group change is
applied by the core's own rules for the organisation, so a SCIM client can do nothing an
administrator could not do. An invalid SCIM request is `PLX-6220`.

### Deprovisioning

Removing a user in the provider removes their access. The person's tokens in that organisation
are revoked at once. Their sessions end at once too, unless they belong to another organisation,
where those sessions stay. Removal takes effect across every server replica, not only the one
that received the request (ADR-0074).

## 8. Checklist for a new provider

- Register Plux as a client with the provider, with the redirect URL of your installation.
- Decide the mapping before you let anyone sign in: which groups give which teams and roles.
- Decide JIT (off unless you have a reason) and whether SCIM provisions people.
- Test a sign-in with a non-administrator account, then a removal, and confirm the session ends.
- Only then turn off password sign-in.

## 9. Errors

| Code | Meaning |
|---|---|
| `PLX-6210` | A SAML response is invalid (signature, audience, time bounds). |
| `PLX-6211` | A SAML assertion was already used (replay). |
| `PLX-6220` | A SCIM request is invalid. |

Sign-in and session errors are in the [sessions guide](sessions-and-step-up.md).
