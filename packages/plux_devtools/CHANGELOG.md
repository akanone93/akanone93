<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Changelog

## Unreleased

- `PluxDevSession.reconnectNow()` connects the dev channel again at once when it waits to, for a host to call
  when the app returns to the foreground (DEV-021).

## 0.2.0

Pair your own debug build with a dev session (DEV-040): `PluxDevSession`, a
pairing tab in the overlay with a paste field, and an optional `scan`
callback for QR codes. Inert in builds without `kPluxDevtools` (DEV-050).

## 0.1.0

The first release: the sync, release and log tabs over `Plux.diagnostics`.
