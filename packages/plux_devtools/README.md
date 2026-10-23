<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# plux_devtools

A debug overlay for Flutter apps that host [Plux](https://nightcode42.github.io/plux3/)
pages with [`plux_flutter`](https://pub.dev/packages/plux_flutter): the sync status with
a sync button, the active release and its plugins, and the problems the runtime reported.
It uses only `plux_flutter`'s public diagnostics and renders nothing in release builds.

## Usage

Wrap the app's content, for example in `MaterialApp.builder`:

```dart
MaterialApp(
  builder: (context, child) => PluxDevtools(child: child!),
  home: const HomeScreen(),
)
```

## Pairing your own debug build

`PluxDevSession` (DEV-040) joins a dev session from your own debug build, so the
build gets the Dev app's capabilities: hot-applied bundles, commands such as
opening a screen or faking the clock, and the event stream. Pin the
**development** roots of your environment, never the production ones: dev
bundles are verified against them alone.

```dart
final session = PluxDevSession(devRootKeys: devRootKeys);

MaterialApp(
  builder: (context, child) => PluxDevtools(
    session: session,
    // Optional: scan the session's QR code, for example with plux_scanner.
    scan: (context) => scanPairingCode(context),
    child: child!,
  ),
)
```

Open the overlay, choose the *session* tab, then *Pair with dev session* and paste the
`plux-dev://pair?...` link or scan it. `session.state` is a `ValueListenable` of
`PluxDevIdle`, `PluxDevPairingNow`, `PluxDevPaired` or `PluxDevError`; `session.end()` closes
the channel and discards the overlay, so pages render the active release again. Fill
`session.extensions` to carry out commands of other packages.

### Release builds

Every API here is a no-op unless `kPluxDevtools` is true, which it is in debug builds
and with `--dart-define=plux.devtools=true` (DEV-050). In a release build `PluxDevtools`
renders only its child, `PluxDevSession.pair` does nothing, and the compiler removes the
dev channel; guard your construction with `kDebugMode` so the pinned development keys
are removed too.

## Licence

Apache-2.0.
