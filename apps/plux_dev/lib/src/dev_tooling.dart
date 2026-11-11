// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The dev tools of the Dev app (DEV-030 to DEV-034, DEV-040): the pairing
/// controller, the node host, the overlay and the command handlers, wired
/// to one another.
library;

import 'package:flutter/material.dart';
import 'package:plux_dev/src/pairing_scanner.dart';
import 'package:plux_devtools/plux_devtools.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_security/plux_security.dart';

/// Everything the dev tools share, created once per app.
final class DevTooling {
  /// Creates the tools. [rootKeys] are the development roots; [deviceName]
  /// is the device's name in the session's device list; [scan] and
  /// [session] replace the camera and the runtime's pairing in tests.
  DevTooling({
    required List<PluxPublicKey> rootKeys,
    required this.navigatorKey,
    Future<String?> Function(NavigatorState navigator)? scan,
    String deviceName = '',
    PluxDevSession? session,
  }) : _scan = scan ?? scanPairingCode,
       session =
           session ??
           PluxDevSession(
             devRootKeys: rootKeys,
             extensions: <String, DevCommandHandler>{},
             deviceName: deviceName,
           ) {
    // Back in the foreground, the dev channel connects at once rather than
    // after its backoff: the phone slept, or its cable was plugged back.
    _lifecycle = AppLifecycleListener(onResume: this.session.reconnectNow);
    this.session.extensions.addAll(
      devExtensions(
        host: host,
        sink: this.session.events,
        roots: () => const {},
        captureKey: captureKey,
        network: network,
      ),
    );
  }

  /// Reports [security]'s detections to the session and lets the session
  /// simulate one (DEV-001, ADR-0057).
  void attachSecurity(PluxSecurity security) {
    security.devEventSink = session.events;
    session.extensions['simulateSecurityDetection'] =
        pluxSecuritySimulateDetectionHandler(security);
  }

  /// The navigator the scan page opens on.
  final GlobalKey<NavigatorState> navigatorKey;

  /// This build's pairing with a dev session.
  final PluxDevSession session;

  /// The state of the overlays and the inspector.
  final DevNodeHost host = DevNodeHost();

  /// The network profile the tools simulate.
  final DevNetworkSimulation network = DevNetworkSimulation();

  /// The boundary `captureScreenshot` captures.
  final GlobalKey captureKey = GlobalKey();

  /// Whether the diagnostics overlay (sync, release, problems, log) shows;
  /// off by default, so the app looks as it will, with the dev button only.
  final ValueNotifier<bool> diagnostics = ValueNotifier(false);

  final Future<String?> Function(NavigatorState navigator) _scan;

  /// Scans a pairing QR code; null when the user gave up or no navigator is
  /// up.
  Future<String?> scan(BuildContext context) async {
    final navigator = navigatorKey.currentState;
    return navigator == null ? null : _scan(navigator);
  }

  /// The app under the tools: the overlay with its pairing tab, the layer
  /// that draws the inspector, and the capture boundary.
  Widget wrap(Widget app) => DevNodeScope(
    host: host,
    child: DevToolsLayer(
      host: host,
      child: ValueListenableBuilder<bool>(
        valueListenable: diagnostics,
        builder: (_, shown, child) => PluxDevtools(
          enabled: shown,
          session: session,
          scan: scan,
          child: child!,
        ),
        child: DevCaptureBoundary(captureKey: captureKey, child: app),
      ),
    ),
  );

  late final AppLifecycleListener _lifecycle;

  /// Releases the tools.
  Future<void> dispose() async {
    _lifecycle.dispose();
    await session.dispose();
    host.dispose();
    diagnostics.dispose();
  }
}
