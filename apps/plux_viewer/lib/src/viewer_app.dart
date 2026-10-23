// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The viewer host's app (SEC-142, ADR-0082): sign in, list the
/// reconstructions, open one. A `plux-viewer://` link opens its
/// reconstruction as soon as the auditor is signed in to the link's server.
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:plux_flutter/plux_flutter.dart' show PluxPublicKey;
import 'package:plux_viewer/src/api.dart';
import 'package:plux_viewer/src/controller.dart';
import 'package:plux_viewer/src/link.dart';
import 'package:plux_viewer/src/list_screen.dart';
import 'package:plux_viewer/src/opener.dart';
import 'package:plux_viewer/src/reconstruction_screen.dart';
import 'package:plux_viewer/src/sign_in_screen.dart';

/// Hands the links the platform delivers to a [ViewerController].
///
/// A custom-scheme link reaches Flutter as route information, once at launch
/// (the engine's default route) and again whenever the app is open. The
/// viewer claims every link of its scheme, valid or not, so that the
/// framework's own navigator never tries to push it as a route.
final class ViewerLinks with WidgetsBindingObserver {
  /// Creates the receiver for [controller].
  ViewerLinks(this.controller);

  /// Receives the links.
  final ViewerController controller;

  /// Starts listening, and offers the link the app was launched with, if
  /// any. [initialRoute] is the engine's default route.
  void attach(String initialRoute) {
    WidgetsBinding.instance.addObserver(this);
    final uri = Uri.tryParse(initialRoute);
    if (uri != null) offer(uri);
  }

  /// Stops listening.
  void detach() => WidgetsBinding.instance.removeObserver(this);

  /// Offers [uri]; whether it was a viewer link, and so is handled.
  bool offer(Uri uri) {
    if (!ViewerLink.isViewerUri(uri)) return false;
    try {
      controller.offerLink(ViewerLink.parse(uri));
    } on FormatException catch (e) {
      controller.rejectLink('That link cannot be opened: ${e.message}.');
    }
    return true;
  }

  @override
  Future<bool> didPushRouteInformation(RouteInformation routeInformation) =>
      Future.value(offer(routeInformation.uri));
}

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
      ReconstructionOpener(api, rootKeys: widget.rootKeys);

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
