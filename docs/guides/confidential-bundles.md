<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Confidential Bundles Guide

How to keep the content of a release (texts, flows, business rules in PXL) unreadable to a
CDN, to storage and to anyone who copies a bundle, and what that does not cover. This is
`SEC-053`; the design is [ADR-0055](../adr/0055-confidential-bundles.md) and the container
is described in the [bundle format](../reference/bundle-format.md#7-confidential-bundles).

## 1. What you get

A confidential release ships every bundle **sealed**: each section is encrypted with
AES-256-GCM under a content key drawn for that release only. The key is wrapped to each
eligible device's hardware key-agreement key and delivered with the manifest. Hashes in the
signed manifest cover the ciphertext, so a device verifies before it decrypts (`SEC-052`).

| Threat | Covered? |
|---|---|
| A CDN, object store or mirror reads or edits bundles | Yes: they hold ciphertext, and an edit fails the hash |
| A user copies the bundle files from the phone's storage | Yes: sections stay encrypted at rest; the key is wrapped to a key that never leaves hardware |
| A device that does not meet the assurance level asks for the content | Yes: it gets no key (`PLX-6050`) |
| A key leaked for one device | Contained: it opens that release for that device only |
| The user of an eligible, genuine device reads what the app shows | No. The content is decrypted into memory to render |
| A rooted device with a debugger reads memory | Reduced, not prevented. RASP and the assurance level exist for this |

## 2. Turn it on

The environment's `confidentialBundles` setting decides, **at the time the server signs a
manifest** for a release:

| Value | Effect | Profile default |
|---|---|---|
| `off` | No release is sealed; creating a confidential release is refused | `standard` |
| `optional` | Only releases created as confidential are sealed | `strict` |
| `required` | Every release is sealed | `maximum` |

```bash
plux security config set --env production --profile strict --expected-version 4 \
  --set confidentialBundles=required
```

(`--set` replaces the override set: include the overrides `plux security config diff` shows.)
Under `optional`, ask for a confidential release with `ReleaseService.CreateRelease` and
`"confidential": true`. A release is built once and promoted: the environment it is signed
for decides, so the same release can be sealed in production and plain in staging. The worker
seals a copy the first time it signs a manifest for it; the published versions are untouched.

`confidentialMinAssurance` (server-only, `AL2` in every profile) is the lowest level a device
needs to receive the key. A device below it, a revoked device, and a device that registered no
key-agreement key get none.

The runtime needs the key-agreement key, which it creates and registers like the DPoP key
([device trust](device-trust.md)). Releases require the feature `security.confidential.v1`, so
an older runtime refuses the release before it downloads anything.

## 3. What changes for you

- **No deltas.** Ciphertext does not diff. A changed bundle downloads whole
  (`confidential.bundleBytes`, 16 MiB by default, bounds one).
- **Decryption happens once per launch**, off the UI isolate, on first use, with the
  platform's hardware AES. The budget is 10 ms per MiB p95 on a mid-tier device (`NFR-013`).
  The decrypted sections live in memory and are never written to disk; the reader works on
  the buffer instead of a memory mapping.
- **Offline.** A device keeps the sealed release and its wrapped key on disk and opens it at
  launch without the network.
- **The embedded baseline is not sealed.** `plux pull` writes the published bundles. If the
  content must stay confidential, do not ship a baseline of it in the app (`baseline: null`);
  the first launch then needs the network.
- **Errors.** `PLX-6050` means the device cannot obtain or use the key: its level is below
  `confidentialMinAssurance`, it has no key-agreement key, or the key does not open the
  content (wrapped for another device or release, or altered). Hash failures are `PLX-3041`
  and happen before any decryption.

## 4. Verifying a release

`schema/testdata/confidential/vectors.json` pins a sealed bundle, a wrapped key and the damage
a device must refuse. The Go writer and the Dart reader are both tested against it, with
tests for edits that keep the hashes consistent, a key for another release or device, and
reused nonces (`bundle/confidential_test.go`, `security/confidential_test.dart`). The
benchmark `security/confidential_bench_test.dart` measures decryption.
