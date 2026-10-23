<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Plux Dev

The Plux Dev app (`DEV-001`, [ADR-0093](../../docs/adr/0093-dev-app-and-host-kit.md)): a Flutter
host for Android and iOS on `plux_flutter`, `plux_devtools` and `plux_host_kit`, with every
first-party optional package registered, so that any plugin can be previewed on a device while
it is edited. A developer signs in, chooses the app to preview, pairs the device with a dev
session by QR code or link, and the session's dev bundles render on the device (`DEV-040`).

It pins **development root keys only**. It does not open reconstruction releases and holds no
production root: one binary never trusts both sets (`SEC-056`). The viewer
([`apps/plux_viewer`](../plux_viewer/README.md)) owns the production roots, and a test here
asserts that nothing in this app names them.

## Build

The app is built with the dev channel in it, which no other build has (`DEV-050`):

```bash
make dev-app-build DEV_APP_TARGET=apk DEV_APP_KEYS=path/to/keys.json
# which runs, in apps/plux_dev:
flutter build apk --dart-define=plux.devtools=true \
  --dart-define=PLUX_DEV_ROOT_KEYS="$(cat path/to/keys.json)"
```

| Define | Meaning |
|---|---|
| `plux.devtools=true` | the build guard: without it `kPluxDevtools` is false in a release build, the app cannot pair and says so |
| `PLUX_DEV_ROOT_KEYS` | the development `keys.json` of the installation, as `plux pull` writes it; without it the app says "No development roots configured" and pairs with nothing |

The Gradle and Xcode builds run in CI and on a developer's machine; they are not part of
`make dart-test`.

## What it does

| Step | Where |
|---|---|
| Sign in with a device code (client name `plux-dev`, scope `app.read`) | `lib/src/sign_in_screen.dart`, `DeviceSignIn` of the host kit |
| Choose the app to preview; start the runtime on the development roots | `lib/src/dev_app.dart`, `lib/optional_packages.dart` |
| Show the pairing status; pair by **Scan QR** (`plux_scanner`) or **Paste link** | `lib/src/home_screen.dart`, `lib/src/pairing_scanner.dart` |
| Overlay, inspector, console, screenshot and network profiles | `lib/src/dev_tooling.dart`: `PluxDevtools`, `DevToolsLayer`, `devExtensions` |

## Optional packages

Registered in `lib/optional_packages.dart` (`DevPackages`): `plux_charts`, `plux_components`,
`plux_db_drift`, `plux_functions`, `plux_go_router`, `plux_location`, `plux_lottie`, `plux_maps`,
`plux_markdown`, `plux_media`, `plux_player`, `plux_rive`, `plux_scanner` and `plux_security`.
Two are left out on purpose: `plux_maps_google` answers the same `MapView` as `plux_maps` and
needs an API key a shared development build cannot hold, and `plux_auto_route` is the other
router adapter, of which a host has one. `plux_svgc`, `plux_native_scan` and `plux_widget_api`
are build tools, not runtime packages.

## Platform

Android: application ID `dev.plux.dev`, the camera permission for the QR scan. iOS: bundle ID
`dev.plux.dev`, `NSCameraUsageDescription` for the same.
