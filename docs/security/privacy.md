# Privacy: Processing Activities, Legal Bases and Retention

Plux is designed for compliance with the EU GDPR and comparable laws (`SEC-160`): data
minimisation, purpose limitation, pseudonymous device identifiers, configurable retention and
documented processing activities. This page is the record of the processing activities of the
Plux Server and the runtime. It describes what the software does; the operator who runs a server
and the developer who ships an app are the controllers and complete the parts only they can (the
privacy notice, the processor agreements, the choice of legal basis for their own analytics).
Consent gating is `SEC-161`; erasure and export of one data subject's data is `SEC-162` (P9).

## What Plux holds about a device

A device is known by a **pseudonymous identifier**: a random UUIDv7 the server mints at
registration (`internal/schema/uuid7`, entropy from `crypto/rand`). It is not derived from any
hardware or user identifier, and the runtime reads none: IMEI, serial number, `ANDROID_ID`, IDFV,
IDFA, advertising ID and MAC address appear nowhere in `plux_flutter`. Tests:

- `TestDeviceIdentifiersAreRandomUUIDv7` (`backend/internal/server`) and
  `TestRegistrationCarriesNoHardwareIdentifier` (`backend/internal/device`): the identifier is a
  UUIDv7 whose random part comes from the entropy source, and the registration request has no
  field for a hardware identifier (`SEC-160`).
- `test/privacy/no_hardware_identifiers_test.dart` (`plux_flutter`): the Dart, Kotlin and Swift
  sources contain none of the identifier APIs above (`SEC-160`).

The server links a device to no end user. The end-user token a host forwards to a function or
data source (`SEC-026`) is not stored with the device.

## Processing activities

| # | Activity | Data | Source | Purpose | Legal basis (GDPR Art. 6) | Retention |
|---|---|---|---|---|---|---|
| 1 | Device registration and trust | Device ID; app, environment and platform; OS version, runtime version, host build; assurance level; DPoP public key and its thumbprint; key storage class; attestation provider, verdicts and risk metric; App Attest key ID, public key, counter and receipt (iOS); key-agreement public key; registration, last-seen and attestation times; revocation time and reason (`devices`, migrations `0007`, `0009`, `0013`, `0014`) | The device and the platform attestation services | Authenticate requests, decide which releases a device may hold, deny revoked devices (`SEC-001`–`SEC-009`) | Legitimate interest (f): securing the service and the customer's app. Where the controller relies on consent, it is the app's | While the device is registered; removed with the app, environment or organisation (cascade). Erasure of one device on request: `SEC-162`, P9 |
| 2 | Release sync | Device ID; the plugin keys and bundle hashes it holds (`device_bundles`); installed release sequence | The device | Choose deltas, count devices a release would strand (`REL-022`, `REL-080`) | Legitimate interest (f) | With the device |
| 3 | Access tokens and proofs | 5-minute access token (hash only); DPoP `jti` values in the shared replay cache | The device | Authorise requests (`SEC-020`–`SEC-025`) | Legitimate interest (f) | Token lifetime; expired tokens are purged by the maintenance sweep; `jti` for the proof window. Proofs and tokens are never logged (`SEC-092`) |
| 4 | RASP detections | Event `rasp_detection`: `kind`, `severity`, `response`, `indicators` as codes; the device keeps `rasp_detected_at` and `rasp_kinds` | The device (`plux_security`) | Lower a device's assurance level, trigger re-attestation, let the operator see attack trends (`SEC-070`, `SEC-071`) | Legitimate interest (f): fraud prevention and network and information security (recital 49) | Event: `telemetry.retentionDays`; device columns: with the device |
| 5 | Necessary telemetry | `session_start` (runtime version, host build, platform, OS version, device class; no locale), `sync_result`, `error`, `rasp_detection`, `pin_failure` (the pinned host name), `attestation_result` (catalogued, not yet emitted); release sequence, plugin key, route. Fields are flat scalars; names that look sensitive are dropped on the device and refused by the server (`SCH-012`); no page parameters, state, input, URLs or message text | The device | Operate releases, diagnose failures (`ANL-001`–`ANL-003`) | Legitimate interest (f); contains no personal data beyond the pseudonymous ID | `telemetry.retentionDays` (default 30) |
| 6 | Analytics telemetry | `session_end`, `screen_view`, `render_perf`, `action_run`, `api_call`, `function_call`, `custom`; the locale with `session_start` | The device, only after `PluxConsent(analytics: true)` | Product analytics for the app's developer | Consent (a), collected by the host app through `Plux.setConsent`; withdrawing deletes the buffered events at once | `telemetry.retentionDays` |
| 7 | Experiment exposures | `experiment_exposure` | The device, only after `PluxConsent(experiments: true)` | Evaluate experiments (P9) | Consent (a) | `telemetry.retentionDays` |
| 8 | Session replay | Category `replay` is reserved by `SEC-161`; nothing is collected or sent today | — | — | Consent (a) when it arrives | — |
| 9 | Gateway | Metrics (counts, latencies, status classes per upstream) and audit entries for administrative changes. **No request or response bodies are kept and no payloads are logged.** | The device's request in transit | Forward calls to the customer's upstreams (`SEC-030`–`SEC-032`) | Contract (b) with the controller; legitimate interest (f) for the metrics | Metrics: as the operator's monitoring; audit: see 10 |
| 10 | Audit log | Actor, action, target, time, source IP, user agent, request ID, hashes of before and after content, hash chain and signed checkpoints (`SEC-140`, `SEC-141`) | Studio users and API clients | Accountability, tamper evidence | Legal obligation (c) and legitimate interest (f) | `retention.auditYears`, 10 by default |

Plux collects nothing for advertising and shares nothing with a third party by itself. Calls that
leave an installation are the ones the operator configures: attestation verification (Google,
Apple), push, AI providers and webhooks. The complete list of egress destinations is `SEC-163` (P9).

## Retention

| Data | Setting | Default | Enforced by |
|---|---|---|---|
| Telemetry events | `telemetry.retentionDays` (`SEC-160`) | 30 | The maintenance sweep (`internal/server`, `Events.Purge`) deletes older events per organisation, hourly; ingest refuses an event older than the window |
| Audit entries | `retention.auditYears` | 10 | Configuration check (must be positive) |
| Document snapshots | `retention.snapshotDays` | 90, at least 90 | Maintenance sweep |
| Trash | `retention.trashDays` | 30, at least 30 | Maintenance sweep |
| Development releases | `retention.developmentReleaseDays` | 90 | Maintenance sweep |
| Device access tokens | fixed by `auth.device.accessTokenTTL` | 5 minutes | Expiry, then the sweep |

The settings are server configuration ([server.md](../reference/server.md)); the value must be
greater than zero, so retention cannot be switched off by mistake. `TestTelemetryRetentionIsConfigurable`
(`backend/internal/config`) checks the default and the override.

## Data-subject rights

- **Information, objection, withdrawal of consent.** The host app owns the consent screen. The
  runtime applies the state it is given: `necessary` events are always sent, all others only
  with consent, and withdrawal deletes the buffered events of the category at once (`SEC-161`;
  [telemetry.md](../reference/telemetry.md)).
- **Access and erasure.** Today the server can revoke a device (`RevokeDevice`: its tokens stop
  working) and removes its data with the app, environment or organisation (cascade). A request
  to export or erase everything about one device or pseudonymous user ID, with a view of where
  personal data is stored, is `SEC-162` and arrives in P9; until then an administrator with
  database access deletes the `devices` row, which cascades to its bundles, tokens and
  telemetry events. A device cannot be tied to a person by Plux alone: the controller must
  tell the data subject's device ID from the app.
- **Minimisation by design.** No hardware identifiers, no locale in necessary telemetry, no
  user input, no URLs, no message text, no gateway bodies.

## Verification

| Claim | Evidence |
|---|---|
| Device IDs are random UUIDv7 and the registration carries no hardware identifier | `TestDeviceIdentifiersAreRandomUUIDv7`, `TestRegistrationCarriesNoHardwareIdentifier` |
| The runtime reads no hardware identifier | `no_hardware_identifiers_test.dart` |
| Retention is a setting | `TestTelemetryRetentionIsConfigurable`; the purge is exercised by `backend/internal/telemetry` integration tests |
| Only `necessary` events are sent without consent, including the P6 events | `telemetry_test.dart` (`the P6 security events are necessary …`, `[SEC-161]`) |
| Sensitive field names never leave the device | `redactFields` test (`SCH-012`) |
