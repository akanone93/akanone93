<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Security at a functional level

You look at what a device and a person must prove: the device key and attestation level, DPoP on
every request, pinning, signed and verified updates, the security profile, sessions and step-up, and
the secure UI. Each check shows where the proof is visible; the deep checks are in layer 3.
Guides: [security](../../guides/security.md), [device trust](../../guides/device-trust.md),
[sessions and step-up](../../guides/sessions-and-step-up.md), [secure UI](../../guides/secure-ui.md).

On an emulator the device has a software key and level `AL0` or `AL1`. That is expected; a real phone
shows a hardware key and `AL2`.

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-PASSKEY | A virtual or real passkey registered for R-ADMIN, usable for step-up | L2-SEC-010 |
| R-REVOKED | A device that was revoked and registered again | L2-SEC-005 |

### L2-SEC-001 — The device key and its level in Studio

- **Needs:** R-DEVICE-SHOP, R-STUDIO
- **Do:** open **Devices** of `shop`, then the device (`/devices/<id>`).
- **Expect:** key storage (hardware or software), key thumbprint, attestation provider, assurance level (`AL0` to `AL3`) and last attestation.
- **Why:** the server computes the level, the device never claims one (SEC-001, SEC-003).
- **Look at:** the thumbprint matches the `jkt` claim of the device's access token (decode it with a debugger; never share it).
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-002 — Every device request carries a DPoP proof

- **Needs:** R-API, R-DEVICE-SHOP
- **Do:** replay a captured device request (copy as curl from the server log or a proxy) a second time.
- **Expect:** refused: the proof's unique id was already used, or the nonce is stale (`PLX-6012` with a fresh `DPoP-Nonce` header).
- **Why:** a recorded request cannot be sent again (SEC-020, SEC-024).
- **Look at:** `plux_dpop_rejections_total` rises in `/metrics`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-003 — A copied token is useless

- **Needs:** R-API
- **Do:** send a device access token with `Authorization: DPoP <token>` but no `DPoP` proof header.
- **Expect:** refused with a DPoP error; the token alone proves nothing.
- **Why:** tokens are bound to the hardware key (SEC-020).
- **Look at:** the response `WWW-Authenticate: DPoP` header.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-004 — Software keys are accepted or refused by profile

- **Needs:** R-DEVICE-SHOP, R-CLI
- **Do:**
  1. `plux security config show -C ~/plux-checks/shop --env development` and note `allowSoftwareKeys`.
  2. `plux security config set --profile strict --expected-version <n> -C ~/plux-checks/shop --env development` (needs a step-up, see L2-SEC-011).
  3. Restart the emulator app.
- **Expect:** under `standard` the emulator registers and is capped at `AL1`; under `strict` it is refused (`PLX-6003`). Set the profile back.
- **Why:** deny by default where the profile says so (SEC-004, SEC-180).
- **Look at:** `plux security config diff` shows the change and the config hash.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-005 — Revoke a device: create R-REVOKED

- **Needs:** R-DEVICE-SHOP, R-STUDIO
- **Do:** in **Devices** open the device, **Revoke device**, give a reason. Then use the app and sync.
- **Expect:** the device loses access at once (calls fail); it shows revoked in the list; it can register again with a new challenge after a restart or reinstall.
- **Why:** a lost device is cut off immediately (SEC-006).
- **Look at:** the reason in **Audit log**; [device revocation runbook](../../runbooks/device-revocation.md).
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-006 — Registration challenge is single-use

- **Needs:** R-API
- **Do:** call `DeviceService/CreateRegistrationChallenge` (no credential) and use the same challenge in two registrations (reuse the first request body).
- **Expect:** the second is refused; a challenge older than five minutes is refused too.
- **Why:** registration cannot be replayed (SEC-002, SEC-005).
- **Look at:** the `registrationChallengeTtl` setting (300 s; 120 s under `maximum`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-007 — Signed manifest and bundles

- **Needs:** R-CLI, R-RELEASE1
- **Do:** `plux keys -C ~/plux-checks/shop --env development`; `plux pull … -o /tmp/baseline`; flip one byte of a `.pxb` in `/tmp/baseline` and run `plux pull` verification again or open it in a test app.
- **Expect:** the keys are listed; the unmodified pull verifies; the changed bundle is refused by hash.
- **Why:** nothing is parsed before signature and hashes verify (SEC-050, SEC-052).
- **Look at:** exit code 1 and the hash message; `PLX-3040`–`3045` on a device.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-008 — Replayed or older manifest is refused

- **Needs:** R-DEVICE-SHOP, R-RELEASE2
- **Do:** after the device runs R-RELEASE2, point the channel back with a hand-made old manifest (serve a saved copy with a local proxy) or read the runtime test that does it: `flutter test` in `packages/plux_flutter` with name filter `rollback`.
- **Expect:** the device refuses a lower sequence (`PLX-3003`).
- **Why:** updates cannot be rolled back by an attacker (SEC-050).
- **Look at:** the legitimate rollback of L2-RUN-009 is a new, higher sequence.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-009 — Certificate pinning

- **Needs:** R-DEVICE-SHOP
- **Do:** in Studio open **Certificate pins** (`/security/pins`) and **Signing keys** (`/security/keys`); then run the device through a proxy with its own certificate.
- **Expect:** both pages are read-only and explain the key ceremony; the proxied connection fails closed (`PLX-6020`).
- **Why:** an interposed certificate must not work (SEC-041).
- **Look at:** [pin rotation runbook](../../runbooks/pin-rotation.md).
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-010 — Second factor: register a passkey, create R-PASSKEY

- **Needs:** R-STUDIO
- **Do:** in Chrome open DevTools, More tools, WebAuthn, enable a virtual authenticator (CTAP2, internal, user verification on). In Studio look under **Profile** (`/me`) for the place to register a passkey; register one. Sign out and sign in again.
- **Expect:** sign-in asks for **Second factor** and **Use a passkey**, and accepts the virtual authenticator. If Studio has no registration screen, write that down as a gap.
- **Why:** sensitive actions rest on a verified second factor (SEC-100, SEC-101).
- **Look at:** the audit entry for the registration.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-011 — Step-up is required for sensitive actions

- **Needs:** R-PASSKEY
- **Do:** sign in with a password only if the form allows; in **Devices**, **Security profile** choose **Change profile**.
- **Expect:** a security-key check is asked first; without it the change fails with `PLX-6200`; with it the change goes through and the window lasts about 5 minutes (`PLX-6201` after).
- **Why:** step-up permissions need a fresh user-verified assertion (SEC-101).
- **Look at:** the environment's overrides stay as they were.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-012 — Step-up from the CLI

- **Needs:** R-CLI, R-PASSKEY
- **Do:** run a guarded command, for example `plux security config set --profile standard --expected-version <n> …`.
- **Expect:** the CLI opens the browser at `/stepup?request=<id>`; after the passkey ceremony the command completes; a second command within 5 minutes needs no new ceremony.
- **Why:** CLI step-up borrows the browser session, bound to the CLI's key (SEC-101, ADR-0075).
- **Look at:** set `PLUX_PUBLIC_URL` if you reach the server by another address.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-013 — Sessions: list and revoke

- **Needs:** R-STUDIO, R-USER2 (signed in, private window)
- **Do:** **Administration**, **Sessions** (`/admin/sessions`): find R-USER2's session; revoke it; in the private window click anything. Also `plux sessions list`.
- **Expect:** the revoked window is refused at its next request (`PLX-6230`); it must sign in again.
- **Why:** administrators can end sessions at once (SEC-102, SEC-103).
- **Look at:** Valkey spreads the revocation to all replicas within seconds.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-014 — Idle timeout

- **Needs:** R-STUDIO
- **Do:** leave a signed-in tab idle past `auth.sessionIdleTimeout` (30 minutes by default; lower it in `deploy/compose/plux-server.yaml` for the check), then click.
- **Expect:** the session is over (`PLX-6231`) and Studio asks you to sign in.
- **Why:** sessions end when idle (SEC-103).
- **Look at:** the absolute limit (12 hours) works the same way (`PLX-6232`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-015 — Tokens

- **Needs:** R-STUDIO
- **Do:** **Administration**, **Tokens**: create a token with a short expiry, use it with the CLI (`PLUX_TOKEN=… plux whoami`), revoke it (`plux tokens revoke <id>`).
- **Expect:** works until revoked or expired; the secret is shown once.
- **Why:** automation credentials are scoped and revocable (SRV-064).
- **Look at:** the audit log lists creation and revocation.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-016 — Secure pages hide the screen

- **Needs:** R-DEVICE-SHOP, a page marked `"security":{"secure":true}`
- **Do:** publish such a page, open it on an Android device, take a screenshot, open the app switcher.
- **Expect:** the screenshot is blocked or black and the switcher card is blank. On iOS the window is covered while captured.
- **Why:** screen protection on secure pages (SEC-093).
- **Look at:** `FLAG_SECURE` is set while the page shows and cleared after the last secure page.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-017 — Secure widgets

- **Needs:** R-DEVICE-SHOP
- **Do:** add `SecureTextField`, `SecurePinPad` (randomised) and `OtpInput` to a secure page; publish; type.
- **Expect:** the text is obscured with no suggestions or clipboard; the pad reshuffles; OTP accepts paste of the right length only.
- **Why:** hardened inputs for secrets (SEC-090, SEC-091).
- **Look at:** **Process flow** shows no values for the sensitive events.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-018 — Masking of sensitive fields

- **Needs:** R-DEVICE-SHOP, the sensitive state of L2-STU-020
- **Do:** show the sensitive value in a text; tap to unmask; send the app to the background and back.
- **Expect:** bullets until you unmask; masked again after backgrounding; no value in logs, traces or telemetry.
- **Why:** sensitive data never leaks (SCH-012, SEC-092).
- **Look at:** `plux dev logs` shows no value; try `plux dev set` on it: `PLX-9206`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-019 — Inactivity lock and biometrics

- **Needs:** R-DEVICE-SHOP, profile `strict` or `maximum`
- **Do:** open a secure page, wait for `inactivityLockTimeout`; unlock; add a `biometricAuth` step to a button and tap it with and without enrolled biometrics.
- **Expect:** a lock screen covers the page and keeps typed input; unlock asks for the device credential; with nothing enrolled the step fails with `PLX-5403`.
- **Why:** shoulder-surfing and unattended devices (SEC-094).
- **Look at:** the lock only runs while a secure page shows.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-020 — Simulated detections

- **Needs:** R-DEVICE-SHOP, R-SESSION, `plux_security`
- **Do:** `plux dev security-detect hooking`, `emulator`, `screen-capture`.
- **Expect:** the app's response follows its profile; the device's level may drop.
- **Why:** the runtime watches itself (SEC-070, SEC-072).
- **Look at:** the assurance column in Studio **Devices**.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-021 — Confidential release

- **Needs:** R-CLI, a device at `AL2` (real phone); otherwise read only
- **Do:** set `confidentialBundles` to `optional` for `development`; publish a release as confidential; inspect the stored bundle in the object store.
- **Expect:** the stored bundle is ciphertext; a device below the required level gets no key (`PLX-6050`).
- **Why:** content is unreadable to CDN and storage (SEC-053).
- **Look at:** SeaweedFS object contents (`http://localhost:8180`): no readable JSON.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-022 — The reference bank app (gateway and SCA)

- **Needs:** R-DEVICE or `make e2e-starter` (host machine)
- **Do:** run `make e2e-starter`; read the `plux_bank` README in `apps/plux_bank`.
- **Expect:** the starter and both reference apps' flows pass against a server built from source; the bank app uses the gateway and the SCA key.
- **Why:** end-to-end proof of the money-movement path (SEC-027, SEC-030, QA-006).
- **Look at:** the gateway route `/gateway/v1/{upstream}/{path}` in the server log.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SEC-023 — Audit chain verifies

- **Needs:** R-STACK
- **Do:** `docker compose -f deploy/compose/compose.yaml exec plux-server plux-server audit verify -config /etc/plux/plux.yaml`.
- **Expect:** exit 0; the chain and its signed checkpoints verify.
- **Why:** the audit log is tamper-evident (SEC-140, SEC-141).
- **Look at:** `audit.checkpointInterval` (1 hour) in the server config.
- **Result:** ☐ pass ☐ fail — notes:
