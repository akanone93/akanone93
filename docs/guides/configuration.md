<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Security Configuration Guide

How security settings work: the profile presets, how an operator tightens them per app and
environment, how a change reaches devices, and how to read and set them with the CLI and the
API. The design is [ADR-0053](../adr/0053-remote-security-configuration.md); the settings are
defined once in `schema/security/settings.json`.

## 1. One source of settings

Every setting has a key, a type (`bool`, `seconds`, `count` or an ordered `enum`), hard
bounds, a *tighter direction*, a preset per profile and a flag saying whether it travels to the
device. `make gen` writes the Go and Dart defaults from that file, so the server and the
runtime always agree on the built-in values.

| Where it is read | Settings |
|---|---|
| On the device (travel in the configuration) | `allowDirectDataSources`, `allowSoftwareKeys`, `androidRefreshRequiresIntegrity`, `encryptLocalStores`, `inactivityLock`, `inactivityLockTimeout`, `minAssuranceForSync`, `rasp*Response` (eight), `raspScanInterval`, `reattestationInterval`, `screenshotBlockingDefault` |
| On the server only (never sent) | `accessTokenLifetime`, `androidDeviceVerdictAL2`, `attestationOutageGrace`, `confidentialBundles`, `confidentialMinAssurance`, `defaultFunctionAssurance`, `dpopIatWindow`, `dpopIatWindowFallback`, `dpopNonceRotation`, `registrationChallengeTtl`, `replayCacheFallback`, `requiredTests`, `riskMetricMaxAL3`, `tls12Allowed` |

The presets per profile are in the [security guide](security.md) §2.

## 2. The effective configuration

```text
profile preset  →  operator overrides (per app and environment)  →  document tightening
```

1. **The profile.** `standard`, `strict` or `maximum`, set per environment.
2. **Operator overrides.** A JSON object of settings that differ from the preset.
3. **The app document's `security.settings`** may tighten further at compile time: a subset
   (`allowDirectDataSources`, `minAssuranceForSync`, `screenshotBlockingDefault`,
   `inactivityLock`, `inactivityLockTimeout`, `raspRootHookingResponse`). Plugins and pages
   tighten what they cover (`SEC-181`); a page cannot opt out of screenshot blocking.

**Overrides only tighten.** "Tighter" is part of the setting's definition: lower for
`inactivityLockTimeout`, `true` for `encryptLocalStores`, a stricter step for a RASP
response. A value that loosens its preset is refused with `PLX-6041`
(`SECURITY_CONFIG_LOOSENS_PRESET`); a value outside the hard bounds with `PLX-6042`
(`SECURITY_CONFIG_OUT_OF_BOUNDS`). To loosen anything, choose a lower profile. That is a
visible, audited act, and each release records the effective configuration it was published
under (`SEC-180`).

The installation's `auth.device.accessTokenTTL` is an override like any other: it can
shorten the token lifetime of every environment, never lengthen it.

## 3. Read and set with the CLI

```bash
plux security config show --env production
```

```text
profile strict, version 3, device configuration sha256 9f2c…

SETTING                 VALUE   SOURCE
accessTokenLifetime     300     profile
inactivityLockTimeout   120     override (profile: 300)
minAssuranceForSync     AL1     profile
…
```

```bash
plux security config diff --env production        # only the overrides
plux security config set --env production --profile strict \
  --expected-version 3 --set inactivityLockTimeout=120 --set raspScanInterval=30
```

- `set` **replaces** the profile and the whole override set; start from `diff` so you do not
  drop existing overrides. `--file overrides.json` reads an object of keys and values;
  `--set` wins over the file.
- `--expected-version` is the version `show` printed (`0` when none was set yet). If someone
  else changed the configuration meanwhile you get a revision conflict; read again.
- Every command takes `--json`, `--app` and `-C <project-dir>`.
- Each change makes a new version (an increasing integer per app and environment) and an audit
  entry. Approvals for production changes arrive in P9.

## 4. Read and set with the API

`SecurityAdminService`:

```bash
curl -sS "$PLUX_SERVER/plux.v1.SecurityAdminService/GetEffectiveSecurityConfig" \
  -H "Authorization: Bearer $PLUX_TOKEN" -H 'Content-Type: application/json' \
  -d '{"appId": "…", "environmentId": "…"}'
# {"profile":"strict","version":"3","effectiveJson":"…","sha256":"…"}

curl -sS "$PLUX_SERVER/plux.v1.SecurityAdminService/SetSecurityConfig" \
  -H "Authorization: Bearer $PLUX_TOKEN" -H 'Content-Type: application/json' \
  -d '{"appId":"…","environmentId":"…","profile":"strict",
       "overridesJson":"eyJyYXNwU2NhbkludGVydmFsIjozMH0=","expectedVersion":"3"}'
```

`overridesJson` is bytes, so base64 in JSON (the example is `{"raspScanInterval":30}`). The
size of the overrides and of the device document are limited by `securityConfig.bytes`
(64 KiB).

## 5. How a change reaches devices

The device sends the configuration version it holds with the manifest request it already
makes at start. Nothing extra is requested.

1. Manifest unchanged (the ETag matches): `not_modified`; nothing about configuration is sent.
2. Otherwise the **signed** manifest carries `config: {version, sha256}` of the canonical
   (RFC 8785) device document.
3. If the device already holds that version, nothing more is sent. If not, the response
   (outside the signed part) carries `config_patch`, an RFC 7396 merge patch from the
   device's version to the current one. A device on built-in defaults (`0`) gets the
   operator's overrides.
4. The device applies the patch, canonicalises, and checks the SHA-256 against the signed
   manifest. A mismatch is `PLX-6040`: it discards the patch, keeps its old configuration and
   asks again with `config_full_required`, and the server sends the whole device document.
5. The server keeps a bounded history of versions; a device older than that gets the full
   document (`securityConfig.patchBytes` bounds a patch).

So nobody can alter settings in transit: the hash is under the manifest's signature, and a
manifest cannot be rolled back (`SEC-055`).

**When a change applies.** Settings that shape a running session (RASP responses, screenshot
blocking, the inactivity lock) apply as soon as the device has applied them. Settings bound
to keys or registration (`allowSoftwareKeys`, attestation) apply at the next registration or
refresh. Stored settings are verified again against the last verified manifest at launch.
Server-only settings take effect as the server reads them.

Device-specific state (revoked, level, re-attestation due) does not travel as configuration.
It arrives in token responses and errors.

## 6. Tightening in the app and in plugins

```json
{"securityProfile": "strict",
 "security": {"settings": {"inactivityLock": true, "inactivityLockTimeout": 90,
                           "raspRootHookingResponse": "block"}}}
```

The compiler and the server reject a value that loosens the effective profile. In the host
app, `PluxSecurity` takes a `PluxRaspPolicy` and a scan interval that may also only
tighten the settings.

## 7. Other places configuration lives

| What | Where |
|---|---|
| Pins of the Plux server | `PluxConfig.pins`; later from signed root metadata ([pin rotation](../runbooks/pin-rotation.md)) |
| Pins for customer domains reached directly | `capabilities.networkPins` in the app document |
| Attestation trust per app | `attestation.apps.<appID>` in `plux-server.yaml` |
| Update-metadata thresholds and expiries | `updateMetadata` in `plux-server.yaml` |
| TLS, HSTS, internal mTLS | `server.tls`, `server.hsts`, `server.internalTLS` |
| Gateway sizes, timeout, rate; SCA challenge lifetime | the `limits:` section ([limits reference](../reference/limits.md)) |

See the [server reference](../reference/server.md) for the file.
