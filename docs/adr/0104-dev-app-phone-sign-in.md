<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0104. The Dev app signs a phone in once and joins Studio sessions with a number match

- **Status:** Accepted (maintainer, 2026-11-11)
- **Date:** 2026-11-11
- **Requirements:** `DEV-001`, `DEV-002`, `DEV-003`, `DEV-020`, `DEV-021`, `SEC-021`, `SEC-103`, `LIM-001`

## Context and problem

Opening an app on a phone took eight steps before this decision: type the server's address,
`adb reverse` by hand, sign in with a device code in a browser, choose the app, open the pairing
tab, scan the session's QR code, open a page, and start again after every unplugged cable, app
restart or Studio reload (a reload opened a new dev session). The maintainer asked for one or two
steps, a remembered pairing, the app list kept, a tap on an app that opens it at its entry screen
with no page in between, and no reduction in security; the product is used for financial apps.
`DEV-002` required the pairing token to arrive as a QR code, which made any flow without a scan
per session non-compliant.

## Decision drivers

- No weaker than the QR pairing: whoever joins a session must be its person, with that person's
  consent, on a device bound to a hardware key; a copied QR code or a phone in other hands must
  not be enough.
- One step a day: no scan, no typing, no browser once the phone is connected.
- The device channel itself (Pair, socket tickets, DPoP, verify-before-load) is unchanged.
- Everything the phone may do is the least it needs, and revocable.

## Considered options

1. **A one-time phone sign-in and a number match on joining** (chosen).
2. A long-lived pairing token in the QR code that pairs with any session of the app. Rejected: a
   photographed code would pair a stranger's phone for its lifetime.
3. The existing device-code sign-in kept, with a QR code per session. Rejected by the maintainer:
   it is the flow that took eight steps.

## Decision

**Connecting a phone (once).** In Studio, under **Profile › Phones**, the person chooses
**Connect a phone**: a WebAuthn step-up first (`SEC-103`), then a QR code with a sign-in grant —
32 random bytes, stored as their hash, single-use, living `devSession.phoneSignInSeconds` (60).
The Dev app claims the grant with a DPoP proof of a P-256 key it creates in the platform keystore
(hardware where the phone has one; alias `dev.plux.phone`). The claim binds the grant to that key
and draws a number challenge: Studio shows a two-digit number, the phone offers it among three
(uniform random, from the server's CSPRNG). The right pick, sent with a proof of the same key,
mints a **phone token**: an access token of its own source (`phone`) and prefix (`plux_phn`),
bound to the key's thumbprint, with the scopes `app.read` and `devsession.create`, stored by the
server as a hash and by the phone in the platform's encrypted secret store (ADR-0029). A wrong
pick ends the sign-in; a new code needs a new step-up.

**Using it.** A phone token is never a bearer token: the people's authentication refuses it, and
it is accepted only by the four procedures of a signed-in phone (`ListPhoneApps`, `RequestJoin`,
`CompleteJoin`, `SignOutPhone`), each with a DPoP proof of the bound key, the server's nonce and
replay protection, rate limited per phone. It lapses after `devSession.phoneIdleDays` (30) of no
use. The person lists and signs out their phones in Studio; the phone can sign itself out.

**Joining a session.** Tapping an app asks to join the person's live **Studio** session of that
app (never a CLI session, never another person's; none open is `PLX-9211`). The server draws a
new number challenge and writes a `JoinRequestEvent` into the session's stream — the phone's name
and state, never the number. Studio, seeing it, reads the number with `GetJoinRequest`, which only
the session's creator may call, and shows it with a **Deny** button. The right pick returns a
pairing token exactly as `GetPairing` issues it, and the runtime pairs as after a scan, so the
device channel, its tickets and its key binding are unchanged. The same phone joins the same live
session again without a number. A wrong pick (`PLX-9210`) keeps the phone out of that session
until the person chooses **Let it try again** in Studio, so a phone in other hands gets one guess
in three per session, visibly. Join requests per phone are limited by `devSession.phoneAttempts`
in ten minutes and live `devSession.joinRequestSeconds` (60).

**One Studio session per person, app and environment.** `CreateSession` of the Studio kind
returns the person's live session of the app in the environment instead of opening another, so a
reload, a second tab or reopening the editor finds the same devices and joined phones.

**The connection.** `make dev` keeps `adb reverse tcp:8080` applied to every phone adb sees,
over USB or wireless debugging, through `scripts/adb-reverse-watch.sh` (from WSL it uses Windows'
`adb.exe`). `DEV_ADB_REVERSE=0` turns it off for those who run `adb reverse` by hand. It only
forwards the phone's own localhost port; the development certificate stays issued for `localhost`
and nothing opens a port on a network.

**The Dev app.** Three screens: connect (scan Studio's code, pick the number), the app list
(apps open in Studio marked live), and the app itself at its entry screen (`Plux.entryRoute`),
with one floating status button — Live, Connecting, Reconnecting, Offline — holding Reconnect,
diagnostics and Leave. A scanned session QR code (Studio's or `plux dev`'s) still pairs at once,
as `DEV-002` always allowed.

## Consequences

- **Positive:** a connected phone opens an app in one tap, without a number while it stays in the
  same Studio session. Joining is stronger than before: the device-code sign-in had no number
  match, and a session QR code needs none. The phone's credential is bound to hardware, scoped to
  four procedures, idle-lapsing, listed and revocable; every sign-in, join and sign-out is audited
  (`devphone.signed_in`, `devsession.joined`, `devphone.revoked`).
- **Negative:** a new credential kind and three tables (`access_tokens` gains `dpop_thumbprint`
  and `platform`; `dev_phone_sign_ins`; `dev_join_requests`) to keep correct. Number matching is
  a check of presence, not a secret: its strength is the one guess per code or session and the
  person seeing every request, not the three choices.
- **Follow-up:** iOS and Android key attestation of the phone key, as devices do, would bind the
  phone token to a genuine build; not needed for development, recorded here.
