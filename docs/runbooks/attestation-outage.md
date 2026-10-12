# Attestation Outage Runbook

Devices cannot register or re-attest: Play Integrity, Android Key Attestation or App Attest
evidence is failing, or the platform cannot produce it (`SEC-002` to `SEC-006`, `SEC-009`,
[ADR-0012](../adr/0012-dpop-hardware-keys-and-attestation.md)).

## What can actually fail

The server makes **no call to Google or Apple on the request path**. Roots are embedded,
Play Integrity is decrypted locally with your Play Console keys, and App Attest is checked
against Apple's embedded root. So an "outage" is one of these:

| Cause | Where | Shows as |
|---|---|---|
| The platform's service is down or throttled for devices (Google Play services, the Play Integrity API quota, Apple's DeviceCheck) | On the device | The runtime raises `PLX-6009` locally and cannot produce evidence |
| The app has no `attestation.apps.<appID>` entry (a config deploy dropped it) | Server | Registrations fail with `PLX-6009` ("attestation is not configured for this app") |
| Play Integrity keys changed in the Play Console, or were pasted wrong | Server config | Every Android registration fails with `PLX-6001` |
| The app's signing certificate changed (Play App Signing, a new keystore) | Server config | Android evidence fails with `PLX-6001`; `androidCertDigests` lacks the new digest |
| Google rotated an attestation root | Server build | Android Key Attestation chains stop verifying (`PLX-6001`); the roots are embedded and need a server release |
| The challenge store is down | Valkey | Registration fails; see [replay-cache outage](replay-cache-outage.md) |

**As built (P6).** The setting `attestationOutageGrace` (24 h, 1 h, `0` fail closed) is in
the registry but the server does not read it yet (`SEC-009`), and there is no grace
period. The server does not fetch Google's certificate status list; the verifier supports
one, but nothing loads it. Apple's fraud-metric receipt is not checked, so iOS stops at
`AL2`. This runbook describes what happens today.

## Signals

| Where | What to look at |
|---|---|
| `plux_rpc_requests_total{service="plux.v1.DeviceService", method=~"RegisterAttestedDevice\|ReattestDevice", code!="ok"}` | Registration and re-attestation failures. A step change is the signal |
| `plux_rpc_requests_total{service="plux.v1.TokenService", method="RefreshDeviceToken", code!="ok"}` | Devices that cannot refresh (after 24 h without re-attestation: `PLX-6007`) |
| `plux_rpc_duration_seconds` for the same methods | Slowness points at the challenge store or database |
| `plux_attestation_results_total{platform, verdict}` | Catalogued in Appendix G.1; not yet exported by the server. Check `/metrics`; until it appears use the RPC counters above |
| `attestation_result` telemetry events | Provider, result, level, no device identifiers. Group by provider |
| Server logs (JSON) | The one line with the incident ID of an internal failure; verification failures name the check that failed, never the evidence |
| Device `onError` | `PLX-6009` and `PLX-6001` from the host app's error hook |

Alert when failures of `RegisterAttestedDevice` or `ReattestDevice` exceed 5% of calls for 10
minutes, or when `RefreshDeviceToken` returns `failed_precondition` for more than 1% of
calls for 10 minutes.

## Diagnosis

1. **One platform or both?** Group failures by `platform`. One platform means that platform's
   service or keys; both means shared infrastructure (config, cache, database).
2. **Which code?** `PLX-6009` is "cannot attest" (device side or missing configuration).
   `PLX-6001` is "evidence did not verify" (keys, digests, roots, clock).
3. **Did something change?** A deploy of `plux-server.yaml`, a Play Console key change, an app
   re-signing, a new app build with a new bundle ID.
4. **Check the configuration offline:** `plux-server config validate -config plux-server.yaml`
   and read the `attestation.apps` entry for the app: package names, certificate digests
   (hex SHA-256 of the *signing* certificate, the Play App Signing one for store builds),
   the two Play Integrity keys, `iosAppID` (`teamID.bundleID`) and `appAttestProduction`.
5. **Is it the device?** If only some devices fail and the server configuration verifies
   others, the cause is on those devices: Play services missing or outdated, no network to
   Google, a Play Integrity quota exhausted for the Cloud project. Check the Google Cloud
   project's Play Integrity API quota and the Play and Apple status pages.
6. **Clock.** Evidence is time-checked. Compare the server clock with NTP.

## Mitigation

- **Configuration fault:** correct the `attestation.apps` entry and restart or roll the
  replicas. Failed registrations are not stored; devices register again on their next start.
- **Platform outage:** wait. Devices that attested within `reattestationInterval` (24 h in
  every profile) keep refreshing their tokens. Devices that cannot re-attest get `PLX-6007`
  on refresh once the interval has passed, then cannot obtain tokens; they keep running the
  last good release offline (expiry never stops a running app), but gateway calls fail until
  they attest again. `reattestationInterval` is a "lower is tighter" setting, so it cannot be
  extended past the profile's value. Do not lower `minAssuranceForSync` to hide the outage:
  that is loosening the profile, which is audited and deliberate.
- **Google rotated a root:** ship a server build with the new root (the fingerprint test
  fails first, in CI).
- **Do not enable the development provider in production.** It is refused there (`PLX-6004`)
  and enabling it anywhere is a security decision.

## Recovery

Nothing per device. Devices re-attest on their next refresh, on app start, or when the server
answers `PLX-6007`. Watch `ReattestDevice` success climb back and `RefreshDeviceToken` failures
fall. If the outage was longer than 24 h, expect a burst of re-attestations: Play Integrity
quota is the limit. The runtime asks Play Integrity only at re-attestation, not on every
refresh (unless `androidRefreshRequiresIntegrity` is set), to keep within quota.

## Prevention

- Test every configuration change on a staging app with a real Play-installed build and a
  real iPhone (the maintainer's real-device check, [device-trust.md](../security/device-trust.md) §3.3).
- Record the signing-certificate digests for upload and Play App Signing keys before changing
  signing.
- Keep the Play Integrity keys in the secret store, with a note of their rotation owner.
- Alert on the signals above. Subscribe to Google's and Apple's status feeds.
- Request a higher Play Integrity quota before a large launch.
