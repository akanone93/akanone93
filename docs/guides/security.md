<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Security Guide

What Plux does to keep an app, its content and its users' money safe, which security
profile to choose, and where each part is explained. Plux ships the controls; a profile is
the switch that turns on the right set for your app. The design is in the
[security whitepaper](../security/whitepaper.md); the attacks it considers are in the
[threat model](../security/threat-model.md).

## 1. The idea in one page

A device that talks to a Plux server has to prove three things on every request:

1. **Which device it is.** It holds a P-256 key that cannot leave secure hardware (StrongBox
   or the TEE on Android, the Secure Enclave on iOS). Every request carries a proof signed by
   that key (DPoP, RFC 9449), and the access token is bound to the key. A copied token is
   useless.
2. **That the app and the device are genuine.** At registration the server checks Android Key
   Attestation and Play Integrity, or iOS App Attest, and computes an *assurance level*
   (`AL0` to `AL3`). The device never claims one.
3. **That the request is fresh.** Proofs carry a server nonce and a unique ID the server
   remembers, so a recorded request cannot be sent again.

On top of that:

- **Updates are signed and cannot be rolled back.** Four metadata roles (`root`, `targets`,
  `snapshot`, `timestamp`) with an offline root. A device verifies before it loads anything.
- **Data at rest is encrypted** under keys held in secure hardware, and a release can be
  **confidential**: its content is encrypted for each eligible device.
- **API calls go through the Plux gateway** by default. The gateway checks the device,
  forwards to your API and adds a signed assertion your API can trust. Money movements are
  confirmed by the user with a hardware key (**SCA**) and verified by the server alone.
- **The UI protects secrets**: hardened inputs, secure pages, masking, an inactivity lock.
- **The runtime watches itself** (RASP) and reports root, hooking, debuggers and more. This
  raises an attacker's cost. It does not stop one who owns the device.

Every behaviour in this guide is a *setting* with a default per profile.
[Configuration](configuration.md) shows how to read and tighten them.

## 2. The three profiles

A profile bundles settings so you do not tune thirty switches. Each maps to an OWASP MASVS
level. You choose one per app and environment, in the app document
(`"securityProfile": "maximum"`) and with `plux security config set`.

| Setting | `standard` (MASVS L1) | `strict` (MASVS L2) | `maximum` (MASVS L2 + R) |
|---|---|---|---|
| DPoP, attestation, signed updates, certificate pinning | on | on | on |
| Software DPoP key (`allowSoftwareKeys`) | accepted, capped at `AL1` | refused (`PLX-6003`) | refused |
| Lowest level that may sync (`minAssuranceForSync`) | `AL0` | `AL1` | `AL2` |
| Android verdict for `AL2` (`androidDeviceVerdictAL2`) | `MEETS_BASIC_INTEGRITY` | `MEETS_DEVICE_INTEGRITY` | `MEETS_DEVICE_INTEGRITY` |
| Default level a function needs (`defaultFunctionAssurance`) | `AL1` | `AL2` | `AL3` |
| Attestation outage (`attestationOutageGrace`) † | 24 h | 1 h | fail closed |
| Proof clock window (`dpopIatWindow`) | 60 s | 60 s | 30 s |
| DPoP nonce rotation (`dpopNonceRotation`) | 300 s | 300 s | 120 s |
| Replay cache down (`replayCacheFallback`) † | per-replica memory | per-replica memory | fail closed |
| Direct data sources (`allowDirectDataSources`) | allowed | allowed | refused (`PLX-6063`) |
| Confidential bundles (`confidentialBundles`) | off | optional | required |
| Encrypted local stores (`encryptLocalStores`) | off | on | on |
| RASP: root or hooking (`raspRootHookingResponse`) | report | degrade | block |
| RASP: debugger, repackaging | report / report | degrade / degrade | block / block |
| RASP: emulator, installer, capture, overlay | report | warn | degrade |
| Screenshot blocking on every page (`screenshotBlockingDefault`) | per secure page | per secure page | on |
| Inactivity lock on secure pages (`inactivityLock`) | off | on, 300 s | on, 120 s |
| Registration challenge lifetime | 300 s | 300 s | 120 s |

† **Defined, not applied yet.** The registry and the presets above are final, but the server
does not read `attestationOutageGrace` yet (`SEC-009`), and it takes `replayCacheFallback`
from the installation's default (`memory`), not from each environment, so `maximum` does not
yet fail closed when the replay cache is down ([replay-cache outage](../runbooks/replay-cache-outage.md)).
`riskMetricMaxAL3` waits for Apple's fraud-metric receipt, and `defaultFunctionAssurance`
for functions (P7).

The full list, with units, hard bounds and which settings travel to the device, is
`schema/security/settings.json` and `plux security config show`. Settings that later phases
build (publish approvals, required Plux tests, accessibility blocking, device-placed
functions) join their profile in that phase.

Two rules hold in every profile:

- **Overrides only tighten.** You can set `inactivityLockTimeout` lower than the preset,
  never higher. To loosen something, choose a lower profile. That choice is audited and
  shows in each release's effective configuration (`SEC-181`, `SEC-182`).
- **Security is not an edition feature.** Everything here is in the open core (`GOV-032`).

## 3. How to choose

| Your app | Profile | Why |
|---|---|---|
| Content, catalogue or community app; no money, little personal data | `standard` | You get DPoP, attestation, signed updates and pinning with no friction for rooted-device users |
| Accounts with personal data, health, HR, B2B tools | `strict` | Hardware keys are mandatory, local data is encrypted, a rooted device is degraded |
| Payments, banking, anything under strong customer authentication | `maximum` | Fail closed on outages, confidential bundles, root and hooking block the app, no direct calls |

Rules of thumb:

- **Start from the data.** If a stolen phone or a rooted phone must not expose it, choose
  `strict` or higher. The cost is that older and modified devices stop working.
- **Do not run development under `maximum`.** The development attestation provider yields
  `AL0`, which cannot sync where `minAssuranceForSync` is above `AL0`. Use a `standard`
  development environment ([device trust](device-trust.md) §6).
- **`maximum` needs the pieces behind it.** A Valkey with Sentinel, a signing backend that is
  not the file backend, the Play Integrity keys, an App Attest team. [Configuration](configuration.md)
  lists them.
- **Mix profiles by environment, not by hope.** Staging may be `strict` while production is
  `maximum`; a release is built once and promoted, so staging then tests what production
  runs.

Plux Bank, the reference app, runs `maximum`. Plux Express runs `standard`, with direct and
gateway data sources side by side.

## 4. Setting up a host app

```dart
final security = PluxSecurity(
  expectations: PluxRaspExpectations(
    signingCertSha256: [releaseCertSha256],
    iosTeamId: 'ABCDE12345',
    iosBundleId: 'com.acme.app',
  ),
);

await Plux.initialize(PluxConfig(
  appId: '<your app ID>',
  endpoint: Uri.parse('https://plux.acme.example'),
  hostBuild: '1.4.2+57',
  pins: ['<current SPKI pin>', '<backup SPKI pin>'],
  playIntegrityCloudProjectNumber: 123456789012,
  strongCustomerAuthentication: true,
  securityModules: [security],
));
```

| Field | What it is |
|---|---|
| `pins` | SPKI SHA-256 pins of the Plux server, base64 (RFC 7469). At least two distinct ones, one a backup key not yet in use. A release build with an `https` endpoint and no pins refuses to start. Later updates come only through signed root metadata ([pin rotation](../runbooks/pin-rotation.md)) |
| `playIntegrityCloudProjectNumber` | The Google Cloud project Play Integrity runs under. Required on Android release builds |
| `strongCustomerAuthentication` | Creates the SCA key at registration. Set it when any data source declares a `transaction` ([SCA](sca.md)) |
| `securityModules` | `PluxSecurity()` from `plux_security`. Without it nothing is detected and a device never reaches `AL3` |
| `hostBuild` | Your build string. A change triggers re-attestation |

On Android the host activity must extend `FlutterFragmentActivity` (the biometric prompt
needs a `FragmentActivity`). On iOS add `NSFaceIDUsageDescription` to `Info.plist`. A
pin is the base64 SHA-256 of a certificate's SubjectPublicKeyInfo:

```bash
openssl x509 -in server.pem -pubkey -noout | openssl pkey -pubin -outform der \
  | openssl dgst -sha256 -binary | base64
```

The server side needs the app's attestation settings (`attestation.apps.<appID>`),
`cache.valkeyURL` for more than one replica, and a production signing backend
([server reference](../reference/server.md)).

## 5. What each part does, and where to read more

| Part | Guide or reference |
|---|---|
| Keys, registration, attestation, assurance levels | [Device trust](device-trust.md) |
| Routing API calls through the server; what your API receives | [Gateway](gateway.md) |
| Confirming a payment with a hardware key | [SCA](sca.md) |
| Reading and tightening settings, the CLI and the API | [Configuration](configuration.md) |
| Encrypting release content for devices | [Confidential bundles](confidential-bundles.md) |
| Secure inputs and pages, masking, the lock | [Secure UI](secure-ui.md) |
| Failures and what to do | [Runbooks](../runbooks/README.md) |
| Error codes `PLX-6001` to `PLX-6100` | [Errors](../reference/errors.md) |

## 6. What Plux does not promise

- **It does not make a compromised device safe.** Someone who controls the phone can read
  what the app shows and what is decrypted in memory. Hardware keys stop *copying* a
  credential; they do not stop *use* while the device is theirs. That is why money
  movements need the user's biometric on the displayed amount.
- **RASP is a signal.** An attacker who owns the device can hide root, rename Frida or patch
  the checks out. The server lowers the device's level on a report; it does not trust the
  app to police itself.
- **Real attestation is proved in layers.** CI proves the verifiers against vectors and the
  development provider on emulators. A real Play Integrity verdict, a real App Attest
  attestation and StrongBox or Secure Enclave keys are proved only by the maintainer's
  real-device check. Until that is recorded, `SEC-002` to `SEC-004` stay `WIP`
  ([device-trust.md](../security/device-trust.md) §3).
- **iOS never reaches `AL3` yet.** Apple's fraud-metric receipt is not checked, so an
  iPhone's ceiling is `AL2`.
- **Customer backends are theirs.** Plux protects the path to your API and tells it who is
  calling. What your API does with a verified request is outside Plux.
