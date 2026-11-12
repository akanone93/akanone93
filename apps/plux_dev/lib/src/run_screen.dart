// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// An app as it runs in the Dev app (DEV-001, ADR-0104): straight to its
/// entry screen, then its own routes, with one floating button that says
/// whether the phone is live with Studio and holds the dev actions.
library;

import 'dart:async';

import 'package:flutter/foundation.dart' show ValueListenable;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:go_router/go_router.dart';
import 'package:plux_dev/src/dev_tooling.dart';
import 'package:plux_dev/src/ui/dev_sheet.dart';
import 'package:plux_dev/src/ui/status_pill.dart';
import 'package:plux_dev/src/ui/theme.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_go_router/plux_go_router.dart';

/// How long the launch screen waits for the release to name the entry
/// screen before it says the app has none.
const Duration entryWait = Duration(seconds: 20);

/// The location of the entry screen [route] in the Dev app's router.
String entryLocation(String route) => '${PluxGoRoutes.prefix}/$route';

/// The first route of a running app: it opens the entry screen as soon as
/// the release names it, the dev bundle Studio sends on joining included.
final class DevLaunchScreen extends StatefulWidget {
  /// Creates the screen.
  const DevLaunchScreen({
    super.key,
    required this.appName,
    required this.entryRoute,
    required this.onLeave,
  });

  /// The app's name.
  final String appName;

  /// The app's entry route, as the release names it (`Plux.entryRoute`).
  final ValueListenable<String?> entryRoute;

  /// Leaves the app.
  final VoidCallback onLeave;

  @override
  State<DevLaunchScreen> createState() => _DevLaunchScreenState();
}

final class _DevLaunchScreenState extends State<DevLaunchScreen> {
  Timer? _timer;
  bool _waitedLong = false;

  @override
  void initState() {
    super.initState();
    widget.entryRoute.addListener(_go);
    _timer = Timer(entryWait, () {
      if (mounted) setState(() => _waitedLong = true);
    });
    WidgetsBinding.instance.addPostFrameCallback((_) => _go());
  }

  @override
  void dispose() {
    _timer?.cancel();
    widget.entryRoute.removeListener(_go);
    super.dispose();
  }

  void _go() {
    final route = widget.entryRoute.value;
    if (route == null || !mounted) return;
    context.go(entryLocation(route));
  }

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.all(DevSizes.page),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (!_waitedLong) ...[
                  const CircularProgressIndicator(),
                  const SizedBox(height: 24),
                  Text('Opening ${widget.appName}…', style: text.titleLarge),
                ] else ...[
                  Text(
                    '${widget.appName} has no entry screen yet',
                    style: text.titleLarge,
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Choose the entry screen in Studio, in the app canvas. It opens here as soon as it is set.',
                    style: text.bodyMedium,
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 24),
                  OutlinedButton(
                    onPressed: widget.onLeave,
                    child: const Text('Back to apps'),
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// The running app with the floating dev button above it.
final class DevRunOverlay extends StatelessWidget {
  /// Creates the overlay over [child].
  const DevRunOverlay({
    super.key,
    required this.tooling,
    required this.appName,
    required this.onLeave,
    required this.child,
    this.onReconnect,
    this.notice,
  });

  /// The dev tools of the running app.
  final DevTooling tooling;

  /// The app's name.
  final String appName;

  /// Leaves the app, back to the list.
  final VoidCallback onLeave;

  /// Connects again; by default it tries the socket again at once.
  final VoidCallback? onReconnect;

  /// What the phone is doing about its link, for the sheet.
  final ValueListenable<String?>? notice;

  /// The app.
  final Widget child;

  @override
  Widget build(BuildContext context) => AnnotatedRegion<SystemUiOverlayStyle>(
    // Readable status-bar icons on a screen without a bar of its own; an
    // app bar of the running app sets its own over this.
    value: devSystemBars,
    child: Stack(
      children: [
        Positioned.fill(child: child),
        // Over everything, debug panel included, at the top where the app's
        // own buttons and the panel are not; see-through, so the screen
        // shows beneath it.
        PositionedDirectional(
          end: 12,
          top: MediaQuery.paddingOf(context).top + 8,
          child: ValueListenableBuilder<DevLinkState?>(
            valueListenable: tooling.session.link,
            builder: (context, state, _) => StatusPill(
              key: const Key('dev-button'),
              state: state,
              translucent: true,
              onTap: () => _openSheet(context),
            ),
          ),
        ),
      ],
    ),
  );

  void _openSheet(BuildContext context) {
    final navigator = tooling.navigatorKey.currentState;
    if (navigator == null) return;
    unawaited(
      showModalBottomSheet<void>(
        context: navigator.context,
        useSafeArea: true,
        isScrollControlled: true,
        builder: (sheet) => DraggableScrollableSheet(
          expand: false,
          initialChildSize: 0.6,
          minChildSize: 0.35,
          maxChildSize: 0.95,
          builder: (_, scroll) => DevSheet(
            scroll: scroll,
            appName: appName,
            session: tooling.session,
            diagnostics: tooling.diagnostics,
            notice: notice ?? _quiet,
            onSync: tooling.sync,
            onReconnect: onReconnect ?? tooling.session.reconnectNow,
            onLeave: () {
              Navigator.of(sheet).pop();
              onLeave();
            },
          ),
        ),
      ),
    );
  }
}

final ValueNotifier<String?> _quiet = ValueNotifier(null);
