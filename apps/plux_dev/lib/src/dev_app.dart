// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Dev app: sign in, choose the app to preview, start the runtime on
/// the development roots, then pair with a dev session (DEV-001).
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:http/http.dart' as http;
import 'package:plux_dev/development_roots.dart';
import 'package:plux_dev/optional_packages.dart';
import 'package:plux_dev/src/dev_tooling.dart';
import 'package:plux_dev/src/home_screen.dart';
import 'package:plux_dev/src/sign_in_screen.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_go_router/plux_go_router.dart';
import 'package:plux_host_kit/plux_host_kit.dart';

/// Starts the runtime with [config]; `Plux.initialize` unless a test
/// replaces it.
typedef RuntimeStarter = Future<Object?> Function(PluxConfig config);

/// The Dev app.
final class DevApp extends StatefulWidget {
  /// Creates the app. [rootKeys] are the development roots it was built
  /// with; without any it pairs with nothing.
  const DevApp({
    super.key,
    required this.rootKeys,
    this.apiFactory = defaultDevApi,
    this.startRuntime = Plux.initialize,
  });

  /// The development roots.
  final List<PluxPublicKey> rootKeys;

  /// Creates the client of a server.
  final DevApiFactory apiFactory;

  /// Starts the runtime.
  final RuntimeStarter startRuntime;

  @override
  State<DevApp> createState() => _DevAppState();
}

final class _DevAppState extends State<DevApp> {
  ViewerSession? _session;
  List<AppSummary>? _apps;
  String? _error;
  DevTooling? _tooling;
  GoRouter? _router;

  @override
  void dispose() {
    _router?.dispose();
    final tooling = _tooling;
    if (tooling != null) unawaited(tooling.dispose());
    super.dispose();
  }

  Future<void> _signedIn(ViewerSession session) async {
    setState(() {
      _session = session;
      _error = null;
    });
    try {
      final apps = await widget.apiFactory(session.server)._withToken(session);
      if (mounted) setState(() => _apps = apps);
    } on ViewerApiException catch (e) {
      if (mounted) {
        setState(() {
          _session = null;
          _error = e.toString();
        });
      }
    }
  }

  Future<void> _start(AppSummary app) async {
    final session = _session;
    if (session == null) return;
    final key = GlobalKey<NavigatorState>();
    late final DevTooling tooling;
    final router = GoRouter(
      navigatorKey: key,
      routes: [
        GoRoute(
          path: '/',
          builder: (context, _) => DevHomeScreen(
            session: tooling.session,
            hasRoots: widget.rootKeys.isNotEmpty,
            scan: tooling.scan,
          ),
        ),
        ...PluxGoRoutes().routes,
      ],
    );
    tooling = DevTooling(rootKeys: widget.rootKeys, navigatorKey: key);
    final packages = DevPackages(router: router);
    tooling.attachSecurity(packages.security);
    try {
      await widget.startRuntime(
        packages.configFor(
          appId: app.id,
          endpoint: session.server,
          environment: devEnvironment,
          rootKeys: widget.rootKeys,
          hostBuild: devHostBuild,
          client: () => tooling.network.wrap(http.Client()),
        ),
      );
    } on Object catch (e) {
      router.dispose();
      await tooling.dispose();
      if (mounted) setState(() => _error = 'The app could not start: $e');
      return;
    }
    if (mounted) {
      setState(() {
        _router = router;
        _tooling = tooling;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final router = _router;
    final tooling = _tooling;
    if (router != null && tooling != null) {
      return PluxScope(
        child: MaterialApp.router(
          title: 'Plux Dev',
          debugShowCheckedModeBanner: false,
          routerConfig: router,
          theme: ThemeData(colorSchemeSeed: const Color(0xFF5B3DF5)),
          builder: (context, child) => tooling.wrap(child!),
        ),
      );
    }
    final apps = _apps;
    return MaterialApp(
      title: 'Plux Dev',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(colorSchemeSeed: const Color(0xFF5B3DF5)),
      home: widget.rootKeys.isEmpty
          ? const _NoRoots()
          : apps == null
          ? _session == null
                ? DevSignInScreen(
                    onSignedIn: _signedIn,
                    apiFactory: widget.apiFactory,
                  )
                : const Scaffold(
                    body: Center(child: CircularProgressIndicator()),
                  )
          : _AppPicker(apps: apps, error: _error, onChoose: _start),
    );
  }
}

extension on ViewerApi {
  Future<List<AppSummary>> _withToken(ViewerSession session) => ViewerApi(
    server,
    token: session.token,
    clientName: clientName,
    scopes: scopes,
  ).listApps();
}

final class _NoRoots extends StatelessWidget {
  const _NoRoots();

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Plux Dev')),
    body: const SafeArea(
      child: Padding(
        padding: EdgeInsets.all(16),
        child: Text(noDevelopmentRootsMessage),
      ),
    ),
  );
}

final class _AppPicker extends StatelessWidget {
  const _AppPicker({
    required this.apps,
    required this.error,
    required this.onChoose,
  });

  final List<AppSummary> apps;
  final String? error;
  final ValueChanged<AppSummary> onChoose;

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Choose the app to preview')),
    body: SafeArea(
      child: ListView(
        children: [
          if (error case final error?)
            Padding(
              padding: const EdgeInsets.all(16),
              child: Text(
                error,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
          for (final app in apps)
            ListTile(
              title: Text(app.name),
              subtitle: Text(app.id),
              onTap: () => onChoose(app),
            ),
          if (apps.isEmpty)
            const Padding(
              padding: EdgeInsets.all(16),
              child: Text('This organisation has no apps yet.'),
            ),
        ],
      ),
    ),
  );
}
