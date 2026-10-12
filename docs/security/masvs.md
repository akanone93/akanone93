# OWASP MASVS v2: L2 and RESILIENCE Checklist

The mobile half of `SEC-190`: the runtime (`plux_flutter`, `plux_security`) is tested against the
OWASP MASTG test cases that apply to MASVS **L2** and **MASVS-RESILIENCE**, and the results are
recorded per release. This page maps every MASVS v2 control to the Plux requirements that meet
it, the automated evidence in this repository, what an emulator shows, and what only a real
device shows. Where a control is not fully covered, the row says so.

The emulator and real-device columns say what each pass is able to show and must check; the
results themselves are recorded per release in the last section, and none is recorded yet.

How to read the evidence columns:

- **Automated** is a test or CI job that runs on every pull request, named by file or job. A
  requirement ID in a test name or a `Verifies:` comment ties it to the spec (`make trace`).
- **Emulator** is what the device jobs (`e2e-android`, `e2e-ios`, [QA-006]) and the manual
  emulator pass show with the development attestation provider
  ([device-trust.md](device-trust.md) §3.2). An emulator proves flows, not hardware.
- **Real device (maintainer)** is the part only a physical phone proves: hardware-backed keys,
  real Play Integrity and App Attest verdicts, biometrics, capture and overlay protection,
  root/jailbreak and hooking detection. The maintainer runs it once per phase and before every
  release ([device-trust.md](device-trust.md) §3.3), from the checklist
  `docs/security/real-device-checklist.md`. **Until that run is recorded for a release, every
  entry in that column is open.**

Plux is a runtime that host apps embed, so some controls are shared: the runtime provides the
mechanism and the host app (or the operator, through the security profile, `SEC-180`) decides
the policy. The host's own code is outside this checklist.

## MASVS-STORAGE

| Control | Plux requirements | Automated | Emulator | Real device (maintainer) |
|---|---|---|---|---|
| STORAGE-1 The app securely stores sensitive data | `SEC-072`, `SEC-073`, `SEC-001`, `SEC-053` | `test/security/storage_secrets_test.dart`, `test/security/device_keys_test.dart`; encrypted cache, outbox and database: `test/data/sealed_cache_test.dart`, `test/data/outbox_test.dart`, `plux_db_drift/test/drift_adapter_test.dart` | Secrets land in Keystore/Keychain-backed storage (flows of `e2e-android`); keys are software-backed on emulators | Keys non-exportable in StrongBox/TEE/Secure Enclave; storage contents inspected on a rooted/jailbroken test device |
| STORAGE-2 The app prevents leakage of sensitive data | `SEC-092`, `SEC-091`, `SEC-072`, `SEC-074` | `sensitive` fields excluded from telemetry (`test/telemetry/telemetry_test.dart`, `[SCH-012]`); secure pages (`test/secure/secure_pages_test.dart`); release-build inspection for dev logging (`make release-inspect`, `tools/internal/policy/release_test.go`) | Logcat/console free of tokens during the flows | Screenshots, app switcher snapshot and screen recording blocked on secure pages; backups and clipboard checked |

## MASVS-CRYPTO

| Control | Plux requirements | Automated | Emulator | Real device (maintainer) |
|---|---|---|---|---|
| CRYPTO-1 The app employs current strong cryptography | `SEC-021` (ES256), `SEC-053` (AES-256-GCM), `SEC-050`, `SEC-122` | `test/security/dpop_test.dart`, `aead_test.dart`, `p256_test.dart`, `confidential_test.dart`, `record_cipher_test.dart`; verify vectors `test/verify/` | Same code as a device | Platform AES-GCM and P-256 implementations on real hardware (`platform_aead_test.dart` covers the Dart side) |
| CRYPTO-2 The app performs key management according to industry best practices | `SEC-001`, `SEC-053`, `SEC-056`, `SEC-121`, `SEC-122` | `test/security/key_wrap_test.dart`, `software_keys_test.dart`, `confidential_sync_test.dart`; key rotation and roles: `test/verify/update_metadata_test.dart`; server `backend/internal/signing` | Software keys with a distinct assurance level (AL1) | Non-exportable, hardware-bound DPoP and SCA keys; user-authentication-bound keys (`SEC-027`) |

## MASVS-AUTH

| Control | Plux requirements | Automated | Emulator | Real device (maintainer) |
|---|---|---|---|---|
| AUTH-1 The app uses secure authentication and authorization protocols | `SEC-020`–`SEC-025`, `SEC-026`, `SEC-029`, `SEC-002`–`SEC-009` | DPoP negative suite `backend/internal/api/dpop_negative_integration_test.go` (`SEC-029`); `test/sync/device_auth_test.dart`, `test/sync/gateway_agent_test.dart`; attestation verifiers `backend/internal/attest/…` | Registration and refresh with the development provider in `e2e-android`/`e2e-ios` | Real Play Integrity and App Attest verdicts accepted; a modified app refused |
| AUTH-2 The app performs local authentication securely | `SEC-027`, `SEC-094`, `SEC-090` | `test/security/sca_test.dart`, `test/secure/secure_pages_test.dart`, `test/secure/secure_widgets_test.dart` | Biometric prompt simulated by the emulator (fingerprint/face enrolment); key use gated by the result | The key operation fails closed without a fresh biometric or device credential; enrolment change invalidates the key |
| AUTH-3 The app secures sensitive operations with additional authentication | `SEC-027`, `SEC-028`, `SEC-032` | `test/security/sign_transaction_test.dart`, `test/data/gateway_routing_test.dart` (dynamic linking, gateway-only acceptance) | Plux Bank SCA transfer through the gateway (`e2e-android`, shard `bank`) | Transaction signed with the user-authenticated hardware key on the displayed amount and payee |

## MASVS-NETWORK

| Control | Plux requirements | Automated | Emulator | Real device (maintainer) |
|---|---|---|---|---|
| NETWORK-1 The app secures all network traffic according to current best practices | `SEC-040`, `SEC-021`, `SEC-030` | Server TLS 1.3 and HSTS: `backend/internal/httpx/tls_test.go`, `backend/internal/server/tls_test.go`; no cleartext: HTTPS-only clients in `test/platform/` | Flows run over TLS to the test server | Traffic captured with a proxy shows no cleartext and no payload outside the gateway path |
| NETWORK-2 The app verifies the identity of the remote endpoint | `SEC-041`, `SEC-042` (SHOULD, `WIP`) | `test/platform/pinned_http_test.dart`, `native_pinned_http_test.dart`; `pin_failure` event: `test/telemetry/runtime_telemetry_test.dart` | Pin mismatch refused against a proxy certificate | Pin mismatch with a user-installed CA on a real device (Android network security config, iOS ATS) |

## MASVS-PLATFORM

| Control | Plux requirements | Automated | Emulator | Real device (maintainer) |
|---|---|---|---|---|
| PLATFORM-1 The app uses IPC mechanisms securely | `SEC-080` (capabilities), `SEC-054` | Capability checks: `test/navigation/routing_test.dart`; the plugin declares no exported component beyond the host's launcher (checked by review of the plugin manifests) | Deep-link and intent probing of the example apps | Intent fuzzing with `drozer`/`adb` against the built app |
| PLATFORM-2 The app uses WebViews securely | `SEC-054`, `SEC-080` | Plux renders native widgets and loads no web content in the runtime; no WebView dependency in `plux_flutter` (dependency allowlist, [dependencies.md](../engineering/dependencies.md)) | Not applicable | Not applicable unless a host app adds a WebView plugin |
| PLATFORM-3 The app uses the user interface securely | `SEC-091`, `SEC-093`, `SEC-090`, `SEC-092` | `test/secure/secure_pages_test.dart`, `secure_runtime_test.dart`, `secure_widgets_test.dart`, `kyc_test.dart`; touch filtering flag on Android | `FLAG_SECURE` set in the window flags (readable on an emulator) | Screenshot, recording, overlay (tap-jacking) and accessibility-service attacks on a secure page |

## MASVS-CODE

| Control | Plux requirements | Automated | Emulator | Real device (maintainer) |
|---|---|---|---|---|
| CODE-1 The app requires an up-to-date platform version | `SEC-007`, `REL-080` | Runtime and OS versions reported at registration and used for assurance: `backend/internal/device`; minimum versions in the host app's build files | n/a | Minimum OS version enforced on the oldest supported device |
| CODE-2 The app has a mechanism for enforcing app updates | `REL-080`, `SEC-006`, `SEC-055` | Devices on an old runtime are counted per release (`backend/internal/device`); re-attestation on app update (`test/sync/device_auth_test.dart`) | n/a | Forced update path of the host app (host-owned) |
| CODE-3 The app only uses software components without known vulnerabilities | `SEC-109`, `CI-007`, dependency policy | `go-vuln`, dependency review, SBOM and Dependabot jobs ([ci.md](../engineering/ci.md)); `tools/internal/policy` (`CI-007`) | n/a | n/a |
| CODE-4 The app validates and sanitizes untrusted inputs | `SEC-052`, `SEC-054`, `SEC-104` | Verify-before-load: `test/verify/`, `test/bundle/golden_bundles_test.dart`; PXL robustness `test/pxl/robustness_test.dart`; fuzzing of bundles, metadata and CBOR (`QA-004`) | Tampered and rolled-back bundles refused in the flows | n/a |

## MASVS-RESILIENCE

| Control | Plux requirements | Automated | Emulator | Real device (maintainer) |
|---|---|---|---|---|
| RESILIENCE-1 The app validates the integrity of the platform | `SEC-070`, `SEC-071`, `SEC-002`–`SEC-004`, `SEC-006`, `SEC-007` | `packages/plux_security/test/` (`detector_test.dart`, `policy_test.dart`, `gate_test.dart`, `security_test.dart`); `test/security/rasp_host_test.dart`; server side `backend/internal/device/rasp_test.go` | Emulator/simulator detection and the policy responses (`report`, `warn`, `degrade`, `block`) are visible; root and hooking detectors are exercised by fakes only | **Open until run:** root/jailbreak (Magisk, unc0ver), Frida, Xposed/LSPosed, debugger, untrusted installer detection on real devices; real attestation verdicts |
| RESILIENCE-2 The app implements anti-tampering mechanisms | `SEC-070` (repackaging), `SEC-002`–`SEC-004`, `SEC-052`, `SEC-050`, `SEC-055`, `SEC-056` | Signature/certificate digest check by the attestation verifiers; bundle and metadata signature, rollback and key-role tests (`test/verify/`, `test/sync/sync_metadata_test.dart`) | Repackaged-app detection by fake; tampered bundles refused | **Open until run:** a re-signed APK and a patched IPA are refused by Play Integrity and App Attest and flagged by the runtime |
| RESILIENCE-3 The app implements anti-static analysis mechanisms | `SEC-074`, `SEC-054` | `make release-inspect` (`test/release/inspect.sh`): no devtools code, dev-only logging, source maps or debug artifacts in the release snapshot; Plux Bank is built with `--obfuscate --split-debug-info`; no secret in documents (`SEC-107`) | Release APK/IPA inspected in CI | Static review of the store build (strings, manifest) with MobSF/`apktool` |
| RESILIENCE-4 The app implements anti-dynamic analysis techniques | `SEC-070` (hooking, debugger, emulator), `SEC-071`, `SEC-006` | As RESILIENCE-1 | As RESILIENCE-1 | **Open until run:** Frida/objection attach to a release build is detected and answered by the profile's response |

## MASVS-PRIVACY

| Control | Plux requirements | Automated | Emulator | Real device (maintainer) |
|---|---|---|---|---|
| PRIVACY-1 The app minimizes access to sensitive data and resources | `SEC-160`, `SEC-080` | No hardware identifier read (`test/privacy/no_hardware_identifiers_test.dart`); capabilities denied by default (`test/navigation/routing_test.dart`); permission prompts only for declared capabilities | Permission prompts observed in the flows | Install-time and runtime permissions of the store build reviewed |
| PRIVACY-2 The app prevents identification of the user | `SEC-160`, `SEC-092` | Random UUIDv7 device IDs (`TestDeviceIdentifiersAreRandomUUIDv7`); necessary telemetry carries no personal data (`telemetry_test.dart`, `redactFields`) | Telemetry batches inspected in the flows | Network capture shows no identifier beyond the device ID |
| PRIVACY-3 The app is transparent about data collection and usage | `SEC-160`, `SEC-161`, `ANL-003` | [privacy.md](privacy.md) and [telemetry.md](../reference/telemetry.md) list every processing activity and field | n/a | Host app's privacy notice and store data-safety/nutrition labels match (host-owned) |
| PRIVACY-4 The app offers user control over their data | `SEC-161`, `SEC-162` (P9) | Consent gating and withdrawal deleting buffered events (`telemetry_test.dart`, `[SEC-161]`, including `rasp_detection`, `attestation_result`, `pin_failure` as `necessary`); export and erasure arrive in P9 | n/a | n/a |

## MASTG results per release

Each release records the MASTG test cases run against the controls above, with their result. A
test case that applies to a control is run on the emulator where it can be and on the
maintainer's real devices where it cannot; the release record lists the MASTG test IDs, tool
versions, devices, and the verdict (`pass`, `fail` with a finding, `not applicable` with the
reason).

| Release | Emulator pass | Real-device pass (maintainer) | Findings |
|---|---|---|---|
| none yet | not run | not run | — |

`SEC-190` stays open until the first release record is in this table, with the real-device
column filled by the maintainer.
