# Device Revocation Runbook

Withdraw trust from one device, or from many (`SEC-006`). Use it for a lost or stolen phone, a
device reported as compromised, a RASP finding you want to act on, or an app build you must
stop trusting.

## What revocation does, and does not do

`DeviceService.RevokeDevice` marks the device revoked, deletes its tokens, and publishes the
revoked key thumbprint to the shared cache (`device:revoked:<jkt>`, kept for the longest token
lifetime, 15 minutes). Every replica refuses the device's tokens from then on with
`PLX-6006`. If the cache is down, replicas check the database. The call is audited with your
reason. Repeating it is safe: it keeps the first reason and publishes the flag again.

It does **not** ban the installation. A genuine runtime that gets `PLX-6006` deletes its
keys and registers again, once per start, with new keys and fresh attestation. That gives a
new device identity and a new SCA key, and it needs a device that still passes attestation
at the profile's level. For a stolen but attested phone, revocation ends the old identity
and its SCA enrolment; keeping the thief out needs the user's own credentials to change at
your identity provider.

## Signals

| Signal | Meaning |
|---|---|
| Reports of lost phones; fraud alerts from your backend | Reason to revoke |
| `rasp_detection` events, device `rasp_detected_at` and `rasp_kinds` on the device record | The server has already capped the device at `AL1` |
| `plux_rpc_requests_total{service="plux.v1.TokenService", method="RefreshDeviceToken", code="unauthenticated"}` | `PLX-6006` on refresh (also unknown devices). Expect a short rise after a mass revocation |
| Audit log entry for the revocation | `ListAuditEntries`, filtered by the device |

## Revoke one device

You need `app.manage` on the app. Find the device, then revoke it:

```bash
curl -sS "$PLUX_SERVER/plux.v1.DeviceService/ListDevices" \
  -H "Authorization: Bearer $PLUX_TOKEN" -H "X-Plux-Organization: $ORG" \
  -H 'Content-Type: application/json' \
  -d '{"appId":"…","environmentId":"…","page":{"pageSize":50,"filter":"…"}}'

curl -sS "$PLUX_SERVER/plux.v1.DeviceService/RevokeDevice" \
  -H "Authorization: Bearer $PLUX_TOKEN" -H "X-Plux-Organization: $ORG" \
  -H 'Content-Type: application/json' \
  -d '{"deviceId":"01d0c450-…","reason":"reported stolen, ticket SEC-1234"}'
```

The reason is limited in length and is shown in the audit log; keep personal data out of it.
The response carries the device with `revokedAt` set. The device record shows platform, OS,
runtime and host build, key storage, provider, level and when it last attested, which is
enough to pick the right one. Filters are equalities on the fields `ListDevices` documents
in the [API reference](../reference/api.md).

## Revoke many

There is no bulk call. Loop over `ListDevices` pages and call `RevokeDevice` for each; each
call is idempotent. Use an `Idempotency-Key` header per call so a retried batch does not
repeat work. Rate limits apply (`api.requestsPerMinute`); spread large batches.

When the problem is a *build* or a *class* of devices rather than named devices, revoke is
the wrong tool. Prefer:

- raise `minAssuranceForSync` or set `allowSoftwareKeys` to false (tighten only), which
  blocks sync and gateway use for devices below it at their next token;
- the kill switch and mandatory-update flag of `ControlService` for a release or build;
- `raspRootHookingResponse` and the other RASP responses for detections.

## After revoking

1. Confirm: `GetDevice` shows `revokedAt`. A refresh by that device returns `PLX-6006`.
2. A short-lived access token issued before the revocation is refused too, because the
   thumbprint flag is checked on every request, not only at issue time.
3. If the device was used for transactions, treat transactions in the last 15 minutes as
   suspect and review them at your backend. Each carries `sca.cid` in the assertion.
4. If the cause was a stolen phone, also end the user's session at your identity provider.
   Plux does not hold user credentials.

## Recovery

A legitimate user who regains the phone opens the app; the runtime registers again and the
user re-enrols biometrics for the SCA key if needed. No operator action. If the device keeps
failing registration, see the [attestation outage runbook](attestation-outage.md).

## Prevention

- Give support a documented path to the `app.manage` role holder; revocation should take
  minutes, not hours.
- Keep access tokens at 5 minutes (the default); the exposure after a revocation is bounded by
  that, and the thumbprint flag closes it anyway.
- Use `maximum` for apps where a stolen phone moves money: every transaction needs the user's
  biometric on the displayed amount.
