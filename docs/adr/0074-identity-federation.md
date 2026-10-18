# 0074. Identity federation: OIDC providers, SAML 2.0, SCIM 2.0, JIT provisioning, group mapping

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `GOV-004`, `GOV-005`, `GOV-030`, `GOV-032`, `GOV-033`, `SEC-100`, `SEC-103`

## Context and problem

Organisations want single sign-on through their identity provider, automatic provisioning and
deprovisioning, and session control. P6 shipped an in-house OpenID Connect client
(`backend/internal/auth/oidc.go`: discovery, authorization code flow with PKCE S256 and a nonce,
ID-token verification through the existing `go-jose` dependency, account linking). `GOV-004` adds
SAML 2.0 and SCIM 2.0; `GOV-005` adds session and token administration. Which libraries do we
use, what is open source and what is enterprise, and how does a leaver lose access at once?

## Decision drivers

- No new dependency where the existing code and `go-jose` suffice (`dependencies.md`).
- XML signature handling is a known attack surface (wrapping, replay); do not write it ourselves.
- Deprovisioning must end access immediately, across replicas.
- The edition split (plan §2.6): OIDC in the core, SAML and SCIM in `ee/`.

## Considered options

1. Extend the in-house OIDC client; SAML with `crewjam/saml`; SCIM in-house.
2. Replace the in-house client with `coreos/go-oidc` (plus `golang.org/x/oauth2`).
3. Delegate federation to an external broker (Keycloak, Dex) and support only OIDC.

## Decision

Chosen option: **1**, because the client already does what `go-oidc` would, and replacing it
buys no security while adding a dependency.

- **OIDC (core).** Extend the client: several providers per installation and per organisation;
  the `groups` claim (or a configured one) mapped to roles and teams through mapping documents;
  just-in-time provisioning behind a setting, off by default (a new person gets only the
  mapping's roles); ID-token `acr` and `amr` recorded; back-channel logout.
  `auth.studio.allowPasswordLogin` may be false once a provider exists.
- **SAML 2.0 (enterprise).** Service provider with `github.com/crewjam/saml` (with
  `russellhaering/goxmldsig` and `beevik/etree`), SP- and IdP-initiated. Signed assertions are
  required; encrypted assertions are supported. A replay cache in Valkey keyed by assertion ID
  (`PLX-6211`); metadata import; the clock-skew bound comes from the limit registry. An invalid
  response gives `PLX-6210`.
- **SCIM 2.0 (enterprise, in-house).** `/scim/v2/Users`, `/Groups`, `ServiceProviderConfig`,
  `Schemas`, `ResourceTypes`; PATCH semantics; bearer tokens per organisation, stored hashed and
  rotatable. Groups map to teams (ADR-0073). Invalid requests give `PLX-6220`.
- **Sessions (core).** A sessions table (device, IP prefix, user agent, created, last seen);
  idle and absolute timeouts per organisation (`PLX-6231`, `PLX-6232`); administrators list and
  revoke sessions and personal tokens (`PLX-6230`). Revocation propagates through Valkey within
  seconds. Deprovisioning through SCIM or a provider revokes sessions and tokens at once.
- The SAML library is the single place XML signatures are checked; wrapping and replay tests are
  fuzzed (plan §8).

## Consequences

- **Positive:** no new dependency for OIDC; the riskiest parser is a maintained library; leavers
  lose access in seconds; the core keeps SSO, which regulated customers need to be safe.
- **Negative:** three new dependencies under `ee/` to track and patch; an in-house SCIM server to
  keep conformant; the Valkey dependency for replay and revocation.
- **Follow-up:** `dependencies.md` rows for the SAML stack (E0); migrations 0024 to 0027; a runbook
  for IdP key rotation; SAML fuzz corpus.

## Options in detail

### Option 1 — extend in-house, add `crewjam/saml`

Smallest surface. The in-house client already uses `go-jose`, so token verification is reviewed
code. SAML in-house would mean writing XML canonicalisation and signature verification; rejected.

### Option 2 — `coreos/go-oidc`

A well-known library, but it duplicates working, tested code, adds a dependency beyond the
allowlist, and does not provide the group mapping, JIT or back-channel logout we need on top.

### Option 3 — an external broker

Pushes SAML and SCIM into another system the operator must run, breaks air-gapped installs
(ADR-0083), and moves a security-critical decision out of the audited code. Rejected.
