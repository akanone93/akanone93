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
///
/// The button opens the debug panel in place, over the app, as a light
/// overlay: no route is pushed on the app's navigator (the panel used to be a
/// modal bottom sheet there, which stalled on its first open and added an
/// entry to the app's own navigation). The panel is built on its first open
/// and kept afterwards, so it opens at once from then on; its handle drags it
/// taller or shorter, and dragging it low or tapping beside it closes it.
final class DevRunOverlay extends StatefulWidget {
  /// Creates the overlay over [child].
  const DevRunOverlay({
    super.key,
    required this.tooling,
    required this.appName,
    required this.onLeave,
    required this.child,
    this.onReconnect,
    this.notice,
    this.picking,
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

  /// Whether a number picker is up on the app's navigator, under the panel:
  /// the panel and its scrim step aside meanwhile and come back after.
  final ValueListenable<bool>? picking;

  /// The app.
  final Widget child;

  @override
  State<DevRunOverlay> createState() => _DevRunOverlayState();
}

final class _DevRunOverlayState extends State<DevRunOverlay> {
  /// The panel's share of the screen's height, and its limits.
  static const double _initial = 0.6;
  static const double _min = 0.35;
  static const double _max = 0.92;

  bool _open = false;
  bool _built = false;
  double _height = _initial;
  final ScrollController _scroll = ScrollController();

  @override
  void dispose() {
    _scroll.dispose();
    super.dispose();
  }

  void _show() => setState(() {
    _open = true;
    _built = true;
    _height = _initial;
  });

  void _hide() => setState(() => _open = false);

  void _drag(DragUpdateDetails d, double screen) => setState(
    () => _height = (_height - d.delta.dy / screen).clamp(0.2, _max),
  );

  void _release() {
    if (_height < _min) {
      _hide();
    }
  }

  @override
  Widget build(BuildContext context) {
    final media = MediaQuery.of(context);
    final screen = media.size.height;
    return AnnotatedRegion<SystemUiOverlayStyle>(
      // Readable status-bar icons on a screen without a bar of its own; an
      // app bar of the running app sets its own over this.
      value: devSystemBars,
      child: Stack(
        children: [
          Positioned.fill(child: widget.child),
          Positioned.fill(
            child: ValueListenableBuilder<bool>(
              valueListenable: widget.picking ?? _notPicking,
              builder: (context, picking, _) => Offstage(
                offstage: picking,
                child: Stack(
                  children: [
                    if (_open)
                      Positioned.fill(
                        child: GestureDetector(
                          key: const Key('dev-scrim'),
                          behavior: HitTestBehavior.opaque,
                          onTap: _hide,
                          child: const ColoredBox(color: Color(0x33000000)),
                        ),
                      ),
                    if (_built)
                      AnimatedPositioned(
                        duration: const Duration(milliseconds: 180),
                        curve: Curves.easeOutCubic,
                        left: 0,
                        right: 0,
                        bottom: _open ? 0 : -screen,
                        height: screen * _height,
                        child: Offstage(
                          offstage: !_open,
                          child: TickerMode(
                            enabled: _open,
                            child: _panel(context, screen),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
          // Over everything, debug panel included, at the top where the
          // app's own buttons and the panel are not; see-through, so the
          // screen shows beneath it.
          PositionedDirectional(
            end: 12,
            top: media.padding.top + 8,
            child: ValueListenableBuilder<DevLinkState?>(
              valueListenable: widget.tooling.session.link,
              builder: (context, state, _) => StatusPill(
                key: const Key('dev-button'),
                state: state,
                translucent: true,
                onTap: _open ? _hide : _show,
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// The panel: the Dev app's own theme (never the running app's), an
  /// overlay of its own for what needs one, and a handle to resize it.
  Widget _panel(BuildContext context, double screen) => Theme(
    data: devTheme(systemBars: false),
    child: Overlay(
      initialEntries: [
        OverlayEntry(
          builder: (context) => Material(
            key: const Key('dev-panel'),
            elevation: 8,
            borderRadius: const BorderRadius.vertical(top: Radius.circular(20)),
            clipBehavior: Clip.antiAlias,
            child: Column(
              children: [
                GestureDetector(
                  key: const Key('dev-panel-handle'),
                  behavior: HitTestBehavior.opaque,
                  onVerticalDragUpdate: (d) => _drag(d, screen),
                  onVerticalDragEnd: (_) => _release(),
                  child: const SizedBox(
                    height: 22,
                    child: Center(
                      child: SizedBox(
                        width: 36,
                        height: 4,
                        child: DecoratedBox(
                          decoration: BoxDecoration(
                            color: Color(0x55000000),
                            borderRadius: BorderRadius.all(Radius.circular(2)),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
                Expanded(
                  child: RepaintBoundary(
                    child: DevSheet(
                      scroll: _scroll,
                      appName: widget.appName,
                      session: widget.tooling.session,
                      diagnostics: widget.tooling.diagnostics,
                      notice: widget.notice ?? _quiet,
                      onSync: widget.tooling.sync,
                      onReconnect:
                          widget.onReconnect ??
                          widget.tooling.session.reconnectNow,
                      onLeave: () {
                        _hide();
                        widget.onLeave();
                      },
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    ),
  );
}

final ValueNotifier<String?> _quiet = ValueNotifier(null);
final ValueNotifier<bool> _notPicking = ValueNotifier(false);
