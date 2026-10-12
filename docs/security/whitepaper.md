<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Plux Security Whitepaper

- **Edition:** 1 (Phase 6, October 2026)
- **Requirements:** `DX-002` (the security whitepaper), `SEC-000`, `SEC-001`–`SEC-009`,
  `SEC-020`–`SEC-032`, `SEC-040`–`SEC-056`, `SEC-070`–`SEC-074`, `SEC-090`–`SEC-094`,
  `SEC-120`–`SEC-122`, `SEC-141`, `SEC-180`–`SEC-182`
- **Audience:** security architects, auditors and engineers who must decide whether to put
  money or personal data behind a Plux app.

This is the first edition. The compliance pack (`DX-002`) completes it in P9. Everything here
is checked against the repository: each claim names the requirement, the decision record or
the test behind it, and Section 9 lists what Plux does not do. Where something is designed
but not yet built, this paper says so.

## 1. What Plux is, and what it protects

Plux is a server-driven UI and plugin platform for Flutter. Developers design apps in Studio.
The Plux server validates, compiles and signs them into bundles. The `plux_flutter` runtime
syncs them to the device, verifies them and renders native widgets. A signed, versioned
release can change what a banking app shows without an app-store release, so **the update
channel is part of the app's attack surface**. Security design starts there.

| Asset | Threat | Section |
|---|---|---|
| What the app does after an update | A forged, replayed or rolled-back release | 4 |
| API calls and the money behind them | A stolen token, a recorded request, a rooted device, a changed amount | 3, 6 |
| Release content (texts, flows, rules) | A CDN, a mirror, a copied bundle | 5 |
| Data on the phone | A lost or backed-up device | 5 |
| Users' secrets on screen | Screenshots, keyboards, shoulder surfing, malware | 8 |
| Signing and wrapping keys | Operator error, a server bug, a stolen backup | 7 |

Security and correctness rank first among the project's goals (spec §1.4); performance,
developer experience, operability and breadth follow. Where they conflict, Plux gives up
speed before it gives up a check.

## 2. Architecture in one picture

```text
                       offline                       online
 root key holders ──signed root──▶  plux-server  ◀───── Studio / CLI (humans, MFA, RBAC)
   (2 of 3, HSM)                    ├ api      ── ConnectRPC, DPoP middleware, gateway
                                    ├ gateway  ── optional separate role
                                    └ worker   ── signs releases, metadata, audit checkpoints
                                         │              │
                        PostgreSQL (RLS) ┘   Valkey (replay cache, challenges)
                                         │
 device ── DPoP + attested hardware keys ─┘──▶ customer API (trusts only Plux-Assertion)
```

Layering rules the code enforces (spec §6.7): handlers hold no business logic; only
`signing/` touches private keys; the `api` and `gateway` roles hold a **sign-only token
capability** and never a release signer; the compiler is a pure library; the runtime does no
network I/O, decompression, bulk crypto or large hashing on the UI isolate; `plux-server`
builds without cgo, and an HSM is reached through a separate helper process.

## 3. The trust chain for a request

Each device request is trusted only as far as four links hold.

### 3.1 Hardware keys

At first start the runtime creates non-exportable P-256 keys (`SEC-001`,
[ADR-0012](../adr/0012-dpop-hardware-keys-and-attestation.md)): a **DPoP key** (StrongBox, else
the TEE, on Android; the Secure Enclave on iOS), a **key-agreement key** for confidential
bundles and an **SCA key** that needs the user's biometric or device credential for every use.
A device with no secure hardware gets a *flagged* software key. The server caps it at `AL1`
under `standard` and refuses it under `strict` and `maximum` (`PLX-6003`).

### 3.2 Attestation

At registration the device presents evidence bound to a single-use server challenge and to
**every public key it registers**: the value is
`SHA-256(challenge ‖ jkt ‖ "." ‖ ka_jkt ‖ "." ‖ sca_jkt)`. A rooted device whose DPoP key is
attested cannot slip a software key beside it.

| Platform | Evidence | Verified |
|---|---|---|
| Android | Key Attestation chain per hardware key; Play Integrity standard token | Chain to Google's roots (embedded); challenge, security level, verified boot, bootloader, package and signing digest; Play Integrity decrypted and checked **locally** with your Play Console keys: no call to Google on the request path (`SEC-002`, `SEC-003`) |
| iOS | App Attest attestation and assertions | Chain to Apple's root (embedded); nonce, App ID, environment, counter 0, then strictly increasing counters (`SEC-004`) |
| Development | Named provider, debug builds | Nothing; accepted only by non-production environments, `PLX-6004` otherwise (`SEC-008`) |

The **server** computes an assurance level, `AL0` to `AL3`, from the evidence, the profile and
RASP reports ([device-trust.md](device-trust.md) §1). The device never claims one. A page, route,
data source, gateway upstream or function can require a level; the refusal is `PLX-6002`.
Re-attestation happens every 24 hours by default, on an app update, after a RASP report and
on request.

### 3.3 Sender-constrained tokens

Access tokens are RFC 9068 JWTs, ES256, five minutes by default (15 at most), with
`cnf.jkt` binding them to the DPoP key (`SEC-020`). Every request carries an RFC 9449 proof
whose `htm`, normalised `htu`, `iat`, `jti`, `ath` and server `nonce` are checked in the order
of ADR-0012 by one middleware for every device route (`SEC-021`, `SEC-022`, `SEC-024`).
There are no long-lived refresh tokens: a refresh needs a fresh proof by the bound key, plus
an App Attest assertion on iOS and, on Android, a Play Integrity token at re-attestation (or on
every refresh by setting). Nonces are stateless (`HMAC(key, epoch)`), so every replica accepts
them with no shared state.

### 3.4 Replay protection

Each proof's `jti` is recorded in Valkey with `SET NX` for twice the `iat` window plus 60
seconds, shared by all replicas (`SEC-023`). If Valkey is down, `standard` and `strict`
fall back to a bounded per-replica store with a narrower window and an alert, never
accepting requests unchecked; `maximum` is specified to fail closed. Section 9 records how
far the build is from that.

### 3.5 Revocation

`RevokeDevice` deletes a device's tokens and publishes its key thumbprint to every replica;
a replica whose cache is down checks the database (`SEC-006`).

## 4. Update integrity

[ADR-0004](../adr/0004-tuf-style-update-security.md) and
[ADR-0054](../adr/0054-update-metadata-roles.md) adopt TUF's design in Plux's formats:

| Role | Signs | Keys | Expiry |
|---|---|---|---|
| `root` | The keys and thresholds of the other roles, and the server's certificate pins | 2 of 3 **offline** keys (HSM) | 1 year |
| `targets` | The manifest: bundle hashes and release contents | Online, per environment | 30 days |
| `snapshot` | The current metadata versions | Online | 7 days |
| `timestamp` | The latest snapshot | Online, re-signed by the worker before expiry | 24 hours |

A device verifies `timestamp → snapshot → targets → manifest` with a threshold per role,
non-decreasing versions and expiry, and then verifies every section hash, **before it parses
anything beyond a container header** (`SEC-050`, `SEC-052`, `SEC-055`). A root rotates only
when the previous root's threshold and the new one's both signed it (`SEC-051`), without an
app release. Development keys are refused by production runtimes (`SEC-056`). Every key and
signature names its algorithm, with ECDSA P-256 as the second algorithm and a documented
path to post-quantum signatures (`SEC-122`). Offline, a device keeps running its last good
release: expiry is checked at sync and never stops a running app.

Executable code is never downloaded except PXL bytecode, action graphs and sandboxed
WebAssembly run by an interpreter (`SEC-054`).

Online keys live in a signing backend: Vault Transit, or any HSM and cloud KMS through the
PKCS#11 helper, tested with SoftHSM2 (`SEC-120`, [ADR-0060](../adr/0060-signing-backends-and-audit-checkpoints.md)).
The file backend is for development and refuses production. The root ceremony is a rehearsed
runbook (`SEC-121`, [key ceremony](../runbooks/key-ceremony.md)).

### Security configuration travels the same way

Every security setting has a built-in default on the server and the device, generated from
`schema/security/settings.json`. Operators change settings per app and environment, within hard
bounds and **only by tightening** the profile's preset (`SEC-181`, `SEC-182`,
[ADR-0053](../adr/0053-remote-security-configuration.md)). A change reaches a device on the
manifest request it already makes, as an RFC 7396 patch whose result must hash to the value
in the **signed** manifest; a mismatch is discarded (`PLX-6040`).

## 5. Data protection

**On the device** (`SEC-072`, `SEC-073`, [ADR-0058](../adr/0058-encryption-at-rest-native-aes-gcm.md)).
Under `strict` and `maximum` every store is encrypted: the database (SQLCipher), the response
cache, the outbox, `persisted` state and the key-value store. Records are AES-256-GCM with the
store and key as associated data. Each store's data key is wrapped by a hardware key and unwrapped
once per launch. Bulk crypto runs off the UI isolate on the platform's hardware-accelerated
AES. Tokens and keys sit only in Keystore- or Keychain-protected storage, and a test scans the
app's files after a session.

**Confidential bundles** (`SEC-053`, [ADR-0055](../adr/0055-confidential-bundles.md)). A release
can ship every section encrypted with AES-256-GCM under a per-release key. The key is wrapped
to each eligible device's key-agreement key with ECDH-ES, HKDF bound to the device and
release, and AES-KW, and is delivered only to devices at or above `confidentialMinAssurance`.
Signed hashes cover the ciphertext, so verification precedes decryption. Sections stay
encrypted on disk and are decrypted into memory, once per launch. Deltas are off for these
releases.

**On the server.** Environment secrets (upstream credentials, Play Integrity keys, content
keys) are sealed under a wrapping key held by the signing backend (`SEC-106`). PostgreSQL
uses row-level security as a second barrier behind application checks. Logs, traces and
metrics carry no keys, tokens, proofs or `sensitive` fields (`SEC-092`).

**Privacy.** Device identifiers are random UUIDs; necessary telemetry carries no personal data;
analytics and experiments need consent (`SEC-160`, `SEC-161`). Data subject export and erasure
arrive in P9 (`SEC-162`).

## 6. The gateway and strong customer authentication

Customer APIs cannot check DPoP, attestation or SCA signatures without reimplementing Plux. So
a device sends API calls to the Plux gateway ([ADR-0056](../adr/0056-secure-gateway-and-sca.md)),
which checks them and forwards to a registered *upstream* with a signed `Plux-Assertion` (60
seconds, ES256, naming the device, app, environment, assurance level, verified user and, for a
transaction, the confirmation).

**SSRF.** Only registered upstreams are reachable; the path may not leave the base URL;
private, loopback, link-local and metadata addresses are refused unless the upstream allows
private ones (never link-local or metadata), checked after DNS and after redirects;
redirects are not followed. Bodies are not logged or stored (`SEC-031`, `SEC-105`).

**SCA with dynamic linking.** The user confirms the amount, currency and payee exactly as they
will be signed. The hardware SCA key signs the single-use challenge and the canonical
summary only after a biometric or device-credential check. **Only the Plux server verifies the
signature**, and a transaction travels only through the gateway; it compares the request's
fields with the signed ones before forwarding. The customer's service sees a verified
assertion, never a device signature (`SEC-027`, `SEC-028`, `SEC-032`). A transaction data
source cannot be `direct`; the compiler refuses it.

**User identity.** The host's user token is exchanged (RFC 8693) after verification against the
customer's identity provider, bound to the device key (`SEC-026`).

The gateway adds at most 5 ms p95 at 1,000 requests per second per replica, excluding the
upstream (`NFR-025`); it can run as its own role.

## 7. Operations that protect the system

- **Key custody.** Root: offline, 2 of 3. Online metadata, token and audit keys: HSM or
  Vault Transit. Roles separate duties: only the worker can sign a release; the `api` can
  only sign tokens.
- **Audit.** Every administrative action is in a per-organisation hash chain. The worker signs
  checkpoints of the chain's head on a schedule; `plux-server audit verify` recomputes the
  chain and every checkpoint and reports the first broken entry (`SEC-140`, `SEC-141`).
- **Transport.** TLS 1.3 only by default (1.2 with AEAD suites by configuration), HSTS on every
  response, TLS to PostgreSQL and Valkey required in production, mutual TLS between roles
  (`SEC-040`, `SEC-043`). The device pins the server's SPKI hashes, at least two with a backup
  key, updated only through signed root metadata (`SEC-041`).
- **Runbooks.** Attestation outage, replay-cache outage, device revocation, key compromise,
  pin rotation, gateway upstream failure, key ceremony ([index](../runbooks/README.md)).

## 8. Detection and the user interface

**RASP** ([ADR-0057](../adr/0057-plux-security-rasp.md), `plux_security`). Plux's own
Kotlin and Swift checks detect root and jailbreak, hooking frameworks, debuggers,
emulators, repackaging, untrusted installers, screen capture, overlays and risky accessibility
services, off the UI thread at start and periodically. Each kind has a response from the
configuration: `report`, `warn`, `degrade` (pages needing `AL3` disabled) or `block`. Every
detection is reported without personal data, and the server caps the device's level.

**RASP is a signal, not a boundary.** Whoever controls the device can hide root, rename or move Frida, clear debugger
flags, answer the checks falsely or patch them out. RASP raises the cost of an attack and
gives the server evidence. The protection that matters is server-side: hardware-bound keys,
attestation, assurance levels, and SCA on the displayed amount.

**Secure UI** ([ADR-0059](../adr/0059-secure-ui.md)). Hardened inputs (no suggestions,
keyboard learning or clipboard), a PIN pad with an optional randomised layout, one-time-code
entry with platform autofill, a biometric button, ID capture with quality checks, secure
pages (`FLAG_SECURE`, covering views on iOS, touch filtering against overlays), masking of
`sensitive` values and an inactivity lock.

## 9. Verification

| Layer | What | Where |
|---|---|---|
| Unit and integration | Every `MUST` set `DONE` is verified by a test that names its ID; `make trace` enforces it | `docs/requirements.md`, `make trace` |
| Negative suites | 16 DPoP cases; forged and mismatched attestation evidence; revoked devices; development evidence in production; tamper, rollback and mix-and-match against every metadata role; altered and mis-keyed confidential bundles; gateway refusals; SCA failures | `TestDPoPNegativeSuite`, verifier tests, `update_metadata_test.dart`, `confidential_test.dart`, `gateway_test.go`, `sca_test.go` |
| Fuzzing | DPoP proofs, access tokens, Key Attestation extensions, App Attest objects and assertions, Play Integrity tokens, CBOR, bundles and sections, deltas, the compiler and PXL; briefly on every pull request and longer nightly | `FuzzVerify` (dpop, devtoken, playintegrity), `FuzzParseKeyDescription`, `FuzzVerifyAttestation`, `FuzzVerifyAssertion`, cbor `FuzzDecode`, `FuzzRead`, `FuzzVerifyPage`, `FuzzApply`, `make go-fuzz` (`QA-004`) |
| Cross-language vectors | The Go server and the Dart runtime agree on the attestation binding, the confidential container, update metadata and the canonical SCA summary | `schema/testdata/attest/binding.json`, `schema/testdata/confidential/vectors.json`, `schema/testdata/manifest` |
| Standards | OWASP ASVS 5.0 L2 and API Top 10 for the server ([asvs-server.md](asvs-server.md)); OWASP MASVS v2 L2 and RESILIENCE for the runtime, with the MASTG results per release (`docs/security/masvs.md`) | `SEC-109`, `SEC-190` |
| Scans | OWASP ZAP baseline against the API in CI; dependency, vulnerability, secret and licence checks on every change | `QA-008`, `CI-007` |
| Performance | DPoP proof ≤ 15 ms p95 on the device (`NFR-012`), confidential decryption ≤ 10 ms per MiB (`NFR-013`), gateway ≤ 5 ms added (`NFR-025`), encrypted stores within 10% of plain | [P6 benchmarks](../benchmarks/p6-security.md) |
| Reference apps | Plux Bank under `maximum` (attestation, SCA through the gateway, secure widgets, confidential bundles); Plux Express under `standard` | `apps/plux_bank`, `apps/plux_express` |

**Attestation is proved in three layers** ([device-trust.md](device-trust.md) §3): verifiers
against recorded and synthetic vectors on every pull request; the development provider
end to end on emulators and simulators; and the maintainer's real-device check on a Pixel-class
phone and an iPhone, once per phase and before a release. No document may claim a layer-3 result from
layer-1 or layer-2 evidence.

## 10. Honest limits

This list is part of the design.

1. **Real-device results are pending.** `SEC-002`–`SEC-004`, `SEC-190` and the reference-device
   numbers of `NFR-012` and `NFR-013` stay open until the maintainer's check is recorded.
2. **A person who controls an unlocked, compromised device sees what the app shows** and
   what is decrypted in its memory. Hardware keys stop *copying* credentials, not *using* them
   while the attacker holds the device; that is why money movements need the user's biometric
   on the displayed amount, and why RASP and assurance levels exist.
3. **iOS cannot block a screenshot**; Plux covers recording and the app switcher. An iPhone
   reaches `AL2`, not `AL3`: Apple's fraud-metric receipt is not checked yet.
4. **Three behaviours are specified but not applied by the server yet:** the attestation outage
   grace period (`SEC-009`); a per-environment replay-cache policy, so `maximum` does not yet
   fail closed on its own (readiness drops replicas whose cache is down); and Google's attestation
   status list.
5. **Revocation forces fresh attestation; it does not ban a device that still passes it.**
6. **The replay fallback** lets a captured proof be replayed once per other replica inside a 15-second
   window, only while the shared cache is down.
7. **An assertion is valid for 60 seconds.** An upstream that moves money must check `jti`, `rh`
   and the `sca` claim.
8. **Direct data sources bypass the gateway:** no per-call assurance enforcement, no SCA. A
   profile can refuse them; `maximum` does.
9. **Two compromised root keys** need an app release to re-anchor devices. Losing both pinned TLS
   keys strands devices until an app update.
10. **Not in P6:** function-side user claims and mutual TLS to the function runner (P7),
    publish approvals and approval of security-configuration changes, data subject export and
    erasure, SIEM export, the compliance pack and the pre-1.0 penetration test (P9), and Studio's
    views of all this (P11).

Report a vulnerability as described in [SECURITY.md](../../SECURITY.md). The threat model with
every threat, mitigation and test is [threat-model.md](threat-model.md).
