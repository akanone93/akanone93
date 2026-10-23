<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0091. Devtools in host apps, release absence and the `plux_security` development mode

- **Status:** Accepted (maintainer, 2026-10-22; P10 plan D-8, §5.3, §5.6, §5.9)
- **Date:** 2026-10-22
- **Requirements:** `DEV-001`, `DEV-040`, `DEV-050`, `SEC-074`, `SEC-052`, `SEC-070`, `SEC-071`

## Context and problem

A device that a remote client can drive — set state, fake the clock, evaluate PXL, push sections —
is a remote-control surface. It belongs in the Dev app and in a developer's own debug build
(`DEV-040`) and nowhere else: not in a release build, not in a store binary (`DEV-050`,
`SEC-074`). `plux_devtools` is today a 315-line overlay that renders nothing in release; P10 adds
a transport, a command handler, an event encoder, a hit test, a network simulator and pinned
development roots, each of which must also be absent. Separately, `plux_security` (ADR-0057)
lowers the assurance level on root, hooking, debugger and emulator detections, which would make
the Dev app unusable on the emulators and debuggers developers use, and plugin authors need to
see how secure pages and SCA steps react to a detection.

## Decision drivers

- Release absence is proven, not asserted: a CI test inspects a release build (`SEC-074`).
- Security defaults stay secure: production behaviour of `plux_security` does not change.
- The Dev app has the capabilities a developer's own host gets (`DEV-040`).
- Nothing on the UI isolate does network I/O, decompression, patching or large hashing (L-6).

## Considered options

1. Compile-time guards with tree shaking, an extended release inspection with canary strings, and a report-only development mode for `plux_security`.
2. A runtime flag that switches the devtools off in release.
3. A separate, stripped-down package for release builds.
4. `plux_security` switched off, or enforcing as in production, in the Dev app.

## Decision

Chosen option: **1**.

- **Guard.** All dev-channel code in `packages/plux_flutter` (`devtools_api`) and
  `plux_devtools` sits behind `kDebugMode` and `bool.fromEnvironment('plux.devtools')`, so the
  compiler removes it from release builds. The socket, verification, patching and event encoding
  run in a background isolate (L-6). Pinned development roots exist only in debug and Dev-app
  builds (ADR-0087, ADR-0093).
- **`plux_devtools` in host apps.** It gains the pairing client (QR scan through the host's
  scanner, or a pasted link), the command and stream adapter over the runtime's dev API, and the
  overlay. The starter app wires it for debug builds, so a developer's own app has the Dev app's
  capabilities (`DEV-040`).
- **Release absence.** `make release-inspect` for Android and iOS gains every P10 symbol
  (transport, event schema, hit test, simulator, commands, development roots) and canary strings
  compiled only into debug code, and fails if one appears in a release build. A production runtime
  also refuses a dev bundle (`SEC-056`) and a manifest with `development_session`.
- **`plux_security` development mode (D-8).** In the Dev app, in development environments only,
  `plux_security` runs report-only: every detection (root, hooking, debugger, emulator,
  repackaging, installer, screen capture, overlay, accessibility) appears in the dev stream but
  does not lower the assurance level and does not block. A dev command simulates a detection of a
  chosen kind, so authors can test how secure pages and SCA steps react. The mode is a property of
  the development environment and the debug or Dev-app build, not a setting an app or an operator
  can enable in production; production behaviour (ADR-0057) is unchanged.

## Consequences

- **Positive:** one inspection proves the absence on both platforms; the Dev app is usable on
  emulators and with a debugger; authors can test detection handling without rooting a phone.
- **Negative:** `release-inspect` grows with every devtools symbol and needs maintenance; a mode
  that does not lower the level must be kept out of production by construction, which the
  inspection and the environment check both guard.
- **Follow-up:** the extended symbol list and canaries (D9); the Revision of ADR-0057 that records the development mode;
  the version bump of `plux_flutter` and `plux_devtools`.

## Options in detail

### Option 1 — compile-time guards

Described above. Costs inspection maintenance; gives proof.

### Option 2 — a runtime flag

The code ships and a flag decides: one wrong default or one memory patch away from a
remote-control surface in a store binary. Rejected.

### Option 3 — separate packages

Two packages to keep in step, and a release dependency that can still pull the dev one in
transitively. Rejected in favour of the guards plus inspection.

### Option 4 — `plux_security` off or enforcing

Off hides the detections plugin authors must be able to see; enforcing makes the Dev app refuse
to run where developers work. Rejected.
