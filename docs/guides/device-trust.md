<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Device Trust Guide

How a device earns the server's trust: the keys it creates, how it registers, what each
attestation provider proves, how the server turns that into an assurance level, and how to
develop without real attestation. The decisions are in
[ADR-0012](../adr/0012-dpop-hardware-keys-and-attestation.md) and the rules for assurance
levels, software keys and testing are in [device-trust.md](../security/device-trust.md);
this guide is the working version.

## 1. The keys

The runtime creates its keys on first start. None can be exported.

| Key | Purpose | Where it lives |
|---|---|---|
| DPoP key (P-256) | Signs a proof on every request; the access token is bound to it | StrongBox, else the TEE (Android); the Secure Enclave (iOS) |
| Key-agreement key (P-256, ECDH) | Receives the content key of confidential releases ([guide](confidential-bundles.md)) | Same hardware |
| SCA key (P-256) | Signs transaction confirmations; needs the user's biometric or device credential for each use ([guide](sca.md)). Created only with `strongCustomerAuthentication: true` | Same hardware, user authentication required |

A device with no secure hardware (an emulator, a simulator, a very old phone) gets a
**software key**, flagged `keyStorage: software`. The server accepts it only where the
profile allows it, caps the device at `AL1`, and refuses it under `strict` and `maximum`
with `PLX-6003` (`allowSoftwareKeys`; [device-trust.md](../security/device-trust.md) §2).

If a key is lost (factory reset, a restore to another phone, a new fingerprint
invalidating the SCA key) the device registers again. It never falls back to a software key
where the profile forbids one.

## 2. Registration

```text
device                                              server
  |  CreateRegistrationChallenge (no identity yet)    |
  | ------------------------------------------------> |  single-use challenge, stored hashed,
  | <-------------------------------- challenge ----- |  valid registrationChallengeTtl
  |  creates / loads its keys,                        |
  |  gathers evidence bound to                        |
  |  binding = SHA-256(challenge || jkt || "." ||     |
  |            ka_jkt || "." || sca_jkt)              |
  |  RegisterAttestedDevice(keys, evidence)           |
  | ------------------------------------------------> |  verifies, computes the level,
  | <------------------------------ device record -- |  consumes the challenge
```

- The **binding** ties the evidence to the one-time challenge and to every public key the
  device registers (`jkt`, `ka_jkt`, `sca_jkt` are the RFC 7638 thumbprints of the DPoP,
  key-agreement and SCA keys; a key the device lacks contributes an empty string). A rooted
  device whose DPoP key is attested therefore cannot register a software key-agreement or
  SCA key beside it (ADR-0012, Revisions 2026-10-11).
- A challenge lives at most 5 minutes (`registrationChallengeTtl`: 300 s, 120 s under
  `maximum`) and works once.
- The old flow (`RegisterDevice` with a device secret, `IssueDeviceToken`) is refused with
  `PLX-6008`. Devices from before P6 are not migrated; they register again.
- After registration the device refreshes its access token (5 minutes by default, 15 at most)
  with a fresh DPoP proof. iOS adds an App Attest assertion on every refresh. Android adds
  a Play Integrity token at re-attestation, or on every refresh where
  `androidRefreshRequiresIntegrity` is set. There are no long-lived refresh tokens.

## 3. Attestation providers

| Platform | Evidence | The server checks |
|---|---|---|
| Android | A Key Attestation chain for each hardware key, and a Play Integrity standard token | Chain to Google's attestation roots (embedded); the challenge; security level of the key; verified boot and bootloader state; package name and signing-certificate digest; the leaf key equals the submitted key. For Play Integrity: decrypts and verifies locally with your Play Console keys, checks package, request hash, timestamp, `PLAY_RECOGNIZED` and the device verdict the profile needs |
| iOS | An App Attest attestation of a key, with `clientDataHash` equal to the binding | Chain to Apple's App Attest root (embedded); nonce; App ID hash (`teamID.bundleID`); environment (`development` or `production`); counter 0; key ID equals the credential key's hash. Later assertions must carry a strictly increasing counter |
| Development | The development provider, only in debug and profile builds | Nothing. See §6 |

No request to Google or Apple happens on the request path. The roots are committed and
pinned by fingerprint tests; Play Integrity is decrypted on your server, so an air-gapped
installation works.

Configure each app on the server:

```yaml
attestation:
  developmentProvider: false
  apps:
    "01d0c450-…":                       # the app ID
      androidPackages: ["com.acme.app"]
      androidCertDigests: ["…"]          # hex SHA-256 of the signing certificates
      playIntegrityDecryptionKey: "…"    # from the Play Console; secrets
      playIntegrityVerificationKey: "…"
      iosAppID: "ABCDE12345.com.acme.app"
      appAttestProduction: true
```

An app with no entry cannot register: the server answers `PLX-6009`. Use the Play
Console's own response-encryption keys for the Play Integrity API (the decryption key and
the verification key it shows you), and pass the Google Cloud project number to the
runtime as `playIntegrityCloudProjectNumber`.

If a recorded Android chain or token fails after an app re-signing or a Play App Signing
change, the signing-certificate digest in `androidCertDigests` is the usual cause
([attestation outage](../runbooks/attestation-outage.md)).

## 4. Assurance levels

The server computes the level from the evidence, the profile and RASP reports. It is
written into the access token (`al` claim).

| Level | Android | iOS |
|---|---|---|
| `AL0` | Nothing verified (development provider), or the app is not recognised by Google Play | The same |
| `AL1` | App recognised; a software key, or a device verdict below what the profile needs | App Attest valid, software key |
| `AL2` | `AL1` plus a hardware key and the profile's device verdict | App Attest valid and a Secure Enclave key |
| `AL3` | `AL2` plus `MEETS_STRONG_INTEGRITY` and no RASP finding | not reached yet (Apple's fraud-metric receipt is not checked) |

Where a level is required:

- the app's `minAssuranceForSync` gates the manifest;
- a page, route, data source or function can set `requiresAssurance` (`AL0` to `AL3`);
- a gateway upstream or operation can set it too, and an operation can ask for more than
  its upstream, never less.

A device below the requirement is refused with `PLX-6002`. Nothing is silently degraded.
RASP findings cap the level at once on the device (`degrade` caps it at `AL2`, `block` at
`AL0`) and on the server (a modified environment caps it at `AL1` until a clean
re-attestation); see the [secure UI guide](secure-ui.md) and `plux_security`.

Pages and routes read the level through guards (`NAV-009`); a guard that needs a higher level
sends the user to its fallback, it does not hide the problem.

## 5. Re-attestation and revocation

- **Re-attestation** happens every 24 hours (`reattestationInterval`), on a host app update
  (a new `hostBuild`), after a RASP report, and when the server answers a refresh with
  `PLX-6007`. A device whose attestation is older than the interval cannot refresh until it
  has attested again.
- **Revocation** is an administrator action: `DeviceService.RevokeDevice` (needs
  `app.manage` on the app). The device's tokens stop working on every replica at once
  (`PLX-6006`) and it must register again. The call is audited with the reason. The steps
  are in the [device revocation runbook](../runbooks/device-revocation.md).
- Telemetry: the runtime reports `attestation_result` (provider, result, level; no device
  identifiers) and `rasp_detection` as `necessary` events.

## 6. Developing without real attestation

Simulators and emulators have no App Attest and no genuine Play Integrity. The
**development provider** stands in:

1. On the server, set `attestation.developmentProvider: true` **in a development or test
   installation**. The default is `false`.
2. Use a non-production environment. A production environment refuses development evidence
   with `PLX-6004`, whatever the setting.
3. In the app, a debug or profile build offers development evidence automatically where the
   platform has no attestation (or, on Android, when `playIntegrityCloudProjectNumber` is
   not set). A release build never does.
4. The device gets `AL0`. It can sync only where `minAssuranceForSync` is `AL0`. Give your
   development environment the `standard` profile; under `strict` and `maximum` an `AL0`
   device is refused at sync (`PLX-6002`).
5. Software keys apply here too: `standard` accepts one and caps the level at `AL1` where
   attestation did prove the app.

The provider is a named provider in the evidence, never a skipped check. The device jobs in
CI register through it on every pull request, so registration, DPoP on every request,
refresh, revocation, the gateway and SCA are tested on emulators
([device-trust.md](../security/device-trust.md) §3.2).

Do not claim more than the layers prove. CI shows that the verifiers accept valid
evidence and refuse forgeries, and that the protocol works end to end. Only a physical
phone shows that Google's and Apple's real evidence verifies and that the keys are really in
hardware. That check is the maintainer's, once per phase and before a release.

## 7. Errors you will meet

| Code | Reason | Usual cause |
|---|---|---|
| `PLX-6001` | `ATTESTATION_FAILED` | Evidence did not verify: wrong challenge, key not attested, app not recognised, unknown package or certificate digest |
| `PLX-6002` | `ASSURANCE_INSUFFICIENT` | The device's level is below what the page, route, source or sync needs |
| `PLX-6003` | `KEY_NOT_HARDWARE_BACKED` | A software key under `strict`, `maximum`, or `allowSoftwareKeys: false` |
| `PLX-6004` | `DEV_PROVIDER_IN_PRODUCTION` | Development evidence sent to a production environment |
| `PLX-6005` | `REGISTRATION_CHALLENGE_INVALID` | Used, expired or unknown challenge |
| `PLX-6006` | `DEVICE_REVOKED` | An administrator revoked the device |
| `PLX-6007` | `REATTESTATION_REQUIRED` | The attestation is older than `reattestationInterval` |
| `PLX-6008` | `LEGACY_REGISTRATION_REFUSED` | An old runtime using the device-secret flow |
| `PLX-6009` | `ATTESTATION_UNAVAILABLE` | The app has no attestation configuration on the server, or the platform's attestation service could not produce evidence on the device |
| `PLX-6017` | `DEVICE_KEY_UNAVAILABLE` | A key is missing, was invalidated (a new biometric enrolment, a restore onto another phone) or cannot be created (no screen lock for the SCA key). The runtime registers again; a transaction fails until the SCA key exists |

Every code, with cause and fix, is in the [error reference](../reference/errors.md).
