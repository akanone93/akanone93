<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0109. The Dev app trusts each app's development key through its organisation's development root

- **Status:** Accepted (maintainer, 2026-11-23)
- **Date:** 2026-11-23
- **Requirements:** `DEV-001`, `DEV-002`, `DEV-003`, `SEC-051`, `SEC-052`, `SEC-056`
- **Revises:** [ADR-0087](0087-dev-session-transport-trust-and-pairing.md) (the Dev app's pinned roots), [ADR-0093](0093-dev-app-and-host-kit.md), [ADR-0104](0104-dev-app-phone-sign-in.md) (the sign-in link)

## Context and problem

The Dev app lists every app its person may preview and opens any of them (`DEV-001`, ADR-0104).
Every dev bundle it applies is verified before load against keys it trusts (`SEC-052`,
ADR-0087). Those keys were pinned at build time (`PLUX_DEV_ROOT_KEYS`, from `plux pull`), but each
environment signs with its own key (`<prefix>-<environment>`), so one build trusted one app: every
other app in the list was refused bundle by bundle and stopped at "no entry screen yet". Getting
the keys also needed a published release, and a new build per app.

The phone already holds a strong identity: it is connected once from Studio after a WebAuthn
step-up, by a QR code read off the person's own screen and a number match, and every call it makes
carries a DPoP proof of a key in its platform keystore (ADR-0104). The trust anchor of the apps'
dev bundles can ride on that same out-of-band step.

## Decision drivers

- Multiple apps from one Dev app build, with no key at build time and nothing pulled by hand.
- No weaker than build-time pinning against a network attacker or a relay; a compromised database
  or API role alone must not be able to make the phone trust a forged key.
- Development keys only: the Dev app never trusts a production key, and a production runtime never
  trusts a development key (`SEC-056`).
- Only `signing/` holds private keys (L-3).

## Considered options

1. **An organisation development root, pinned when the phone is connected**, which certifies each
   app's development key, delivered when the phone joins a session.
2. **Each app's development key returned when the phone joins, kept on first use.** Rejected: the
   anchor would come from the database over the network, so whoever can change the stored public
   keys decides what a first join trusts.
3. **One development key per organisation, built into the Dev app.** Rejected: still a build per
   organisation, and every app of the organisation shares one signing key.
4. **One Dev app build per app** (as before). Rejected: it does not meet `DEV-001`.

## Decision

Chosen option: **1**.

- **The root.** Each organisation has a development root: an Ed25519 key in the signing backend,
  `<targets prefix>-devroot-<organisation>`, created on first use, apart from every environment's
  key. It signs one kind of statement and nothing else.
- **The certificate.** When a phone joins a Studio session — after the number match, or at once on
  a rejoin — the server returns, with the pairing link, the development key of the session's
  environment certified by the root: a canonical JSON statement (RFC 8785) of the organisation,
  app, environment, key ID, public key, role `targets`, type `plux.dev-key-certificate` and
  `environmentType` `development`, and the root's signature of it (`CompleteJoinResponse` and
  `RequestJoinResponse`, field `development_key`, additive). The release service makes it, as it
  signs manifests; a production environment is refused.
- **The anchor.** The sign-in link of **Connect a phone** carries the root's public key,
  `plux-dev://signin?server=…&grant=…&root=<unpadded base64url>`. The phone reads it off the
  person's screen with its camera after their step-up, so the anchor never depends on the network
  or the database, and keeps it with the phone's account in the platform's encrypted store
  (ADR-0029). A link without a root is refused.
- **On the phone.** A join's certificate is verified under the pinned root, then read: it must be
  a development key of role `targets`, for the very app and environment joined. The key is the
  only one that app's runtime and dev channel trust (`PluxConfig.rootKeys`), and it is kept for
  that app, so a session's pairing code scanned later opens it with the same key; an app never
  opened from the list opens from no code. A key the root does not certify refuses the app.
  Connecting the phone again pins the new root and forgets every kept key.
- **No build-time keys.** The Dev app reads no `PLUX_DEV_ROOT_KEYS`; `make dev-app-defines` takes
  only the certificate authority. A phone connected before this change is asked to connect again.
- **Unchanged.** Host apps' debug builds (`DEV-040`, the starter) pin their environment's keys at
  build time as before; the viewer keeps the production roots; the device channel, its tickets and
  its key binding are as ADR-0087 has them.

## Consequences

- One Dev app build previews every app of the person's organisation, with no key handled by hand
  and no release needed first.
- Against a network attacker the phone is as safe as with build-time pins: it trusts only what the
  root it read off its person's screen signed. A compromised database cannot swap the anchor; the
  root's private half stays in the signing backend. A compromised server could sign forged keys,
  as it could already sign forged bundles: it holds the development keys.
- A role that certifies keys, or shows the sign-in code, needs the signer, as signing dev bundles
  already did.
- Replacing an organisation's root (its key lost or rotated in the backend) means connecting each
  phone again; there is no root rotation chain for it.
- Revocation is the phone's: signing a phone out, or connecting it again, forgets its kept keys.
