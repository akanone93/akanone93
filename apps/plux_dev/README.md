<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Plux Dev

The Plux Dev app (`DEV-001`, [ADR-0093](../../docs/adr/0093-dev-app-and-host-kit.md)): a Flutter
host for Android and iOS on `plux_flutter`, `plux_devtools` and `plux_host_kit`, with every
first-party optional package registered, so that any plugin can be previewed on a device while
it is edited. A developer connects the phone once with Studio's code and a number match, then
taps an app: it opens at its entry screen, joined to their Studio session, and every edit
arrives as it is made ([ADR-0104](../../docs/adr/0104-dev-app-phone-sign-in.md), `DEV-002`).

It pins **development root keys only**. It does not open reconstruction releases and holds no
production root: one binary never trusts both sets (`SEC-056`). The viewer
([`apps/plux_viewer`](../plux_viewer/README.md)) owns the production roots, and a test here
asserts that nothing in this app names them.

## Build

The app is built with the dev channel in it, which no other build has (`DEV-050`):

```bash
make dev-app-build DEV_APP_TARGET=apk DEV_APP_KEYS=path/to/keys.json
# which writes apps/plux_dev/dev-defines.json (make dev-app-defines) and runs there:
flutter build apk --dart-define-from-file=dev-defines.json
```

To run it on a connected device with hot reload and logs, write the file and run:

```bash
make dev-app-defines DEV_APP_KEYS=path/to/keys.json
cd apps/plux_dev && flutter run --debug --dart-define-from-file=dev-defines.json
```

The settings are a file (ignored by git), not `--dart-define` arguments: they hold quotes and
line breaks, which Windows' `flutter.bat` drops from arguments.

| Define | Meaning |
|---|---|
| `plux.devtools=true` | the build guard: without it `kPluxDevtools` is false in a release build, the app cannot pair and says so |
| `PLUX_DEV_ROOT_KEYS` | the development `keys.json` of the installation, as `plux pull` writes it; without it the app says "No development roots configured" and pairs with nothing |
| `PLUX_DEV_CA` | the PEM of the installation's development certificate authority (`make dev`: `deploy/compose/.secrets/ca/ca.crt`, which `DEV_APP_CA` defaults to); every request and WebSocket of the app trusts it besides the system's authorities. Installing it on the device is not enough: Flutter's `dart:io` trusts only the system's authorities on Android. Without it the app trusts the system's alone; a value that is not a certificate stops the app at start |

On a phone over USB or wireless debugging, `make dev` keeps `adb reverse tcp:8080 tcp:8080`
applied for you (`DEV_ADB_REVERSE=0` turns that off; then run it by hand): the phone reaches
`https://localhost:8080`, the only name the development certificate is valid for.

The Gradle and Xcode builds run in CI and on a developer's machine; they are not part of
`make dart-test`.

## What it does

| Step | Where |
|---|---|
| Connect the phone once: scan the code of Studio's **Profile › Phones › Connect a phone**, pick the number Studio shows; the phone token is bound to a key in the platform keystore and kept in its encrypted store | `lib/src/connect_screen.dart`, `lib/src/phone.dart`, `DevPhone` of the runtime |
| The apps, those open in Studio marked live; **Scan** also opens a session's pairing code (Studio's or `plux dev`'s) at once | `lib/src/apps_screen.dart` |
| Tap an app: join the person's Studio session (a number match the first time), start the runtime on the development roots, open the entry screen (`Plux.entryRoute`) | `lib/src/dev_app.dart`, `lib/src/run_screen.dart`, `lib/optional_packages.dart` |
| The floating status button — Live, Connecting, Reconnecting, Offline — with Reconnect, diagnostics and Leave | `lib/src/run_screen.dart`, `lib/src/ui/status_pill.dart` |
| Overlay, inspector, console, screenshot and network profiles | `lib/src/dev_tooling.dart`: `PluxDevtools`, `DevToolsLayer`, `devExtensions` |

The look is in `lib/src/ui/theme.dart`: white and quiet, rounded, cards raised only where they
group something, touch targets of at least 56 logical pixels. The phone's name in Studio's list
comes from a native call of the app itself (`dev.plux.dev/device`, `MainActivity.kt`,
`AppDelegate.swift`).

## Optional packages

Registered in `lib/optional_packages.dart` (`DevPackages`): `plux_charts`, `plux_components`,
`plux_db_drift`, `plux_functions`, `plux_go_router`, `plux_location`, `plux_lottie`, `plux_maps`,
`plux_markdown`, `plux_media`, `plux_player`, `plux_rive`, `plux_scanner` and `plux_security`.
Two are left out on purpose: `plux_maps_google` answers the same `MapView` as `plux_maps` and
needs an API key a shared development build cannot hold, and `plux_auto_route` is the other
router adapter, of which a host has one. `plux_svgc`, `plux_native_scan` and `plux_widget_api`
are build tools, not runtime packages.

## Platform

Android: application ID `dev.plux.dev`, the camera permission for the QR scans. iOS: bundle ID
`dev.plux.dev`, `NSCameraUsageDescription` for the same.
