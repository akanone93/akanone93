<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Changelog

## Unreleased

- The debug panel is redesigned: a dark, slightly see-through sheet with a header, tabs (Sync,
  Release, Problems with a count, Session with the link state) and monospace rows that scroll.
  `PluxDevtools(showBadge: false, initiallyOpen: true, onClose: …)` lets a host open it from a
  button of its own; showing or hiding the overlay no longer rebuilds the app beneath it.
- `PluxDevSession.link` follows the pairing's `DevChannel.link` (live, reconnecting, ended; null while nothing
  is paired), and `PluxDevSession(deviceName: …)` names the device in the session's device list (DEV-020).

- `PluxDevSession.reconnectNow()` connects the dev channel again at once when it waits to, for a host to call
  when the app returns to the foreground (DEV-021).

## 0.2.0

Pair your own debug build with a dev session (DEV-040): `PluxDevSession`, a
pairing tab in the overlay with a paste field, and an optional `scan`
callback for QR codes. Inert in builds without `kPluxDevtools` (DEV-050).

## 0.1.0

The first release: the sync, release and log tabs over `Plux.diagnostics`.
