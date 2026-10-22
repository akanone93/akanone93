<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Sessions and Step-up Guide

How a person's sign-in to Plux Studio and the API lasts, how an administrator ends it, and how
a sensitive action asks for a second factor again, in the browser and from the CLI. The
requirements are `SEC-100`, `SEC-103`, `GOV-004`, the session rules of `PLX-6230` to `PLX-6232`,
and [ADR-0075](../adr/0075-webauthn-step-up.md).

## 1. The idea in one page

A session is a person's sign-in in one browser or one CLI login. It ends when it is idle for too
long, when it reaches its absolute lifetime, or when an administrator revokes it. Separately, a
few permissions are **step-up** permissions: they need a second factor presented recently, not
just a live session. Approving a change, deciding an approval, and creating a restriction are
examples (`approvals.decide`, `restrictions.manage`). A session that has signed in with a password
alone can read and edit, but cannot use those.

## 2. Sessions

Each session records the device, a network prefix, the user agent, when it was created and when
it was last seen. Administrators list and revoke sessions and personal tokens with
`IdentityService.ListSessions` and `IdentityService.RevokeSession`. A revoked session is refused
on its next request (`PLX-6230`); the revocation reaches every server replica through Valkey
within a few seconds.

Two timeouts apply, each set per installation and tightened per organisation:

| Limit | Default | Meaning |
|---|---|---|
| `auth.sessionIdleTimeout` | 30 minutes | A session not seen for this long ends (`PLX-6231`). |
| `auth.sessionAbsoluteTimeout` | 12 hours | A session older than this ends, however active (`PLX-6232`). |

When an organisation sets a timeout, the shorter of its value and the installation's applies.
Both come from the limits registry ([the limits reference](../reference/limits.md)).

Sign-in itself comes from one of three sources: a password (when `auth.studio.allowPasswordLogin`
is true), an OpenID Connect provider, or SAML in the enterprise edition. A person's sign-in
through a provider is recorded with the authentication context the provider reported. Back-channel
logout from a provider ends the matching sessions. See the [SSO and SCIM guide](sso-and-scim.md).

## 3. Second factor

The second factor is a WebAuthn credential (a hardware key or a platform passkey) that the person
registers (`IdentityService.BeginWebAuthnRegistration`, `FinishWebAuthnRegistration`). Plux requires **user verification** for a step-up, so the
authenticator must check the person (a PIN, biometric or equivalent), not only be present.

## 4. Step-up

A step-up is a fresh, user-verified assertion recorded on the session. The rules are:

- The assertion must have been made within the **last 5 minutes** (`StepUpWindow`). A session
  holds one step-up time, the one from the most recent successful assertion.
- A permission marked step-up in the catalogue ([permissions reference](../reference/permissions.md))
  is refused without a fresh assertion.
- A request to step up accepts at most `auth.stepUpAttempts` ceremonies (default 5, after which
  it is closed). A ceremony that does not verify is refused and counts as an attempt.
- A step-up request itself expires after five minutes if no ceremony completes.
- The session records the time of its latest successful assertion; the window is measured from
  there.

In the browser, a guarded action that needs a step-up asks the server to begin one
(`IdentityService.BeginStepUp`), the browser performs the WebAuthn ceremony, and the server
completes it (`IdentityService.CompleteStepUp`). The retry then succeeds while the window is open.

## 5. Step-up from the CLI

The CLI has no authenticator of its own, so it borrows the person's browser session. The grant
is bound to the CLI's key, so a copied token cannot use it.

1. The CLI asks the server for a step-up request. The request is bound to the CLI's access token
   and to the RFC 7638 thumbprint of its DPoP key. The server returns a request identifier and a
   secret; only a hash of the secret is stored, and the secret is returned once.
2. The CLI opens `https://<server>/stepup?request=<id>` in the browser. The person signs in there
   (in their own session) and performs the WebAuthn ceremony.
3. The CLI polls with a proof signed by the same DPoP key and the secret. When the ceremony has
   been approved, the server issues a **step-up grant valid for 5 minutes** and bound to that
   key thumbprint.
4. The CLI presents the grant with the guarded request, together with its token and a proof of
   the same key. The grant counts only while it is fresh, and only with that key.

A grant used with another key, presented after it expires, or replayed is refused. The tests of
this flow cover expiry, replay and a grant used with another key.

Every proof names the request URL in its `htu` claim, and the server compares it with its
configured `server.publicBaseURL`, not with the address the request arrived at (a proxy or a
port-forward changes that). When you reach the server by another address, for example with
`kubectl port-forward`, set `PLUX_PUBLIC_URL` to the public base URL and the CLI names it in its
proofs; without it the CLI names the address it calls.

The CLI reference ([CLI reference](../reference/cli.md)) lists the command that starts the flow
when the command ships. The server side is `CreateCLIStepUp`, `PollCLIStepUp` and
`CheckStepUpGrant`.

## 6. Errors

| Code | Meaning |
|---|---|
| `PLX-6200` | A step-up is required and none is on the session. |
| `PLX-6201` | The step-up is too old; repeat the ceremony. |
| `PLX-6202` | The assertion did not verify. |
| `PLX-6230` | The session was revoked. |
| `PLX-6231` | The session was idle for too long. |
| `PLX-6232` | The session passed its absolute lifetime. |

## 7. Operating it

- Revoke a leaver's sessions and personal tokens first, then remove their grants. Deprovisioning
  through SCIM or a provider revokes the person's tokens in that organisation at once; their
  sessions end only when they belong to no other organisation (ADR-0074).
- Watch for repeated `PLX-6202`: a run of failed ceremonies on one account can be an attempt to
  guess or relay a second factor, and is worth a look in the audit log.
