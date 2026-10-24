// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The viewer host's app (SEC-142, ADR-0082): sign in, list the
/// reconstructions, open one. A `plux-viewer://` link opens its
/// reconstruction as soon as the auditor is signed in to the link's server.
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:plux_flutter/plux_flutter.dart' show PluxPublicKey;
import 'package:plux_host_kit/plux_host_kit.dart';
import 'package:plux_viewer/src/list_screen.dart';
import 'package:plux_viewer/src/production_roots.dart';
import 'package:plux_viewer/src/reconstruction_screen.dart';
import 'package:plux_viewer/src/sign_in_screen.dart';

/// The viewer host.
final class ViewerApp extends StatefulWidget {
  /// Creates the app over [controller], which trusts the root keys
  /// [rootKeys] pinned at build time and opens nothing without them.
  /// [openerFor] builds the opener for a session; tests replace it.
  const ViewerApp({
    super.key,
    required this.controller,
    required this.rootKeys,
    this.openerFor,
    this.apiFactory,
  });

  /// The viewer's state.
  final ViewerController controller;

  /// The root keys the viewer was built with (SEC-051).
  final List<PluxPublicKey> rootKeys;

  /// Builds the opener of a signed-in client, or null for the default.
  final ReconstructionOpener Function(ViewerApi api)? openerFor;

  /// Creates the client sign-in uses, or null for the default.
  final ApiFactory? apiFactory;

  @override
  State<ViewerApp> createState() => _ViewerAppState();
}

final class _ViewerAppState extends State<ViewerApp> {
  final GlobalKey<NavigatorState> _navigator = GlobalKey<NavigatorState>();
  final GlobalKey<ScaffoldMessengerState> _messenger =
      GlobalKey<ScaffoldMessengerState>();
  ReconstructionOpener? _opener;

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_changed);
    WidgetsBinding.instance.addPostFrameCallback((_) => _changed());
  }

  @override
  void dispose() {
    widget.controller.removeListener(_changed);
    unawaited(_opener?.close());
    super.dispose();
  }

  ReconstructionOpener _newOpener(ViewerApi api) =>
      widget.openerFor?.call(api) ??
      ReconstructionOpener(
        api,
        rootKeys: widget.rootKeys,
        hostBuild: viewerHostBuild,
        missingKeysMessage: viewerNoRootKeysMessage,
      );

  void _changed() {
    final controller = widget.controller;
    if (controller.hasRejection) {
      _messenger.currentState?.showSnackBar(
        SnackBar(content: Text(controller.takeRejection()!)),
      );
    }
    final session = controller.session;
    if (session == null) {
      unawaited(_opener?.close());
      _opener = null;
    }
    final link = controller.takeLink();
    if (link != null && session != null) {
      final opener = _opener ??= _newOpener(session.api());
      _navigator.currentState
        ?..popUntil((route) => route.isFirst)
        ..push(_route(link.id, opener));
    }
  }

  MaterialPageRoute<void> _route(String id, ReconstructionOpener opener) =>
      MaterialPageRoute<void>(
        builder: (_) => ReconstructionScreen(id: id, opener: opener),
      );

  Widget _home() {
    final controller = widget.controller;
    final session = controller.session;
    if (session == null) {
      final link = controller.pendingLink;
      return SignInScreen(
        key: ValueKey(link),
        controller: controller,
        server: link?.server,
        notice: link == null
            ? null
            : 'This link opens a reconstruction on ${link.server.host}. '
                  'Sign in to that server to open it.',
        apiFactory: widget.apiFactory ?? defaultApiFactory,
      );
    }
    return ListScreen(
      key: ValueKey(session.server),
      api: session.api(),
      onOpen: (item) {
        final opener = _opener ??= _newOpener(session.api());
        unawaited(_navigator.currentState!.push(_route(item.id, opener)));
      },
      onSignOut: controller.signOut,
    );
  }

  @override
  Widget build(BuildContext context) => MaterialApp(
    title: 'Plux Viewer',
    navigatorKey: _navigator,
    scaffoldMessengerKey: _messenger,
    // The engine's default route is the launch link, not a route of the
    // app: the viewer reads it itself (ViewerLinks).
    initialRoute: '/',
    theme: ThemeData(colorSchemeSeed: Colors.indigo),
    darkTheme: ThemeData(
      colorSchemeSeed: Colors.indigo,
      brightness: Brightness.dark,
    ),
    home: ListenableBuilder(
      listenable: widget.controller,
      builder: (context, _) => _home(),
    ),
  );
}
