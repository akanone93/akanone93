// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Plux Dev app (DEV-001, DEV-002, ADR-0104): connect the phone once,
/// see your apps, tap one and it opens at its entry screen, joined to your
/// Studio session and live: every edit arrives as you make it.
library;

import 'dart:async';

import 'package:flutter/foundation.dart' show ValueListenable;
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:plux_dev/dev_http.dart';
import 'package:plux_dev/development_roots.dart';
import 'package:plux_dev/optional_packages.dart';
import 'package:plux_dev/src/apps_screen.dart';
import 'package:plux_dev/src/connect_screen.dart';
import 'package:plux_dev/src/dev_tooling.dart';
import 'package:plux_dev/src/pairing_scanner.dart';
import 'package:plux_dev/src/phone.dart';
import 'package:plux_dev/src/run_screen.dart';
import 'package:plux_dev/src/ui/number_picker.dart';
import 'package:plux_dev/src/ui/theme.dart';
import 'package:plux_devtools/plux_devtools.dart'
    show PluxDevError, PluxDevSession;
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_go_router/plux_go_router.dart';

/// Starts the runtime with [config]; `Plux.initialize` unless a test
/// replaces it.
typedef RuntimeStarter = Future<Object?> Function(PluxConfig config);

/// Stops the runtime; `Plux.dispose` unless a test replaces it.
typedef RuntimeStopper = Future<void> Function();

/// Gives the running app the runtime's providers; `PluxScope` unless a test
/// replaces it (the scope needs a started runtime).
typedef RuntimeScope = Widget Function(Widget child);

/// The entry route of the running app; `Plux.entryRoute` unless a test
/// replaces it.
typedef EntryRouteOf = ValueListenable<String?> Function();

Widget _pluxScope(Widget child) => PluxScope(child: child);

ValueListenable<String?> _pluxEntryRoute() => Plux.entryRoute;

/// The words for a failed join of [app].
String joinFailure(Object error, String app) => switch (error) {
  DevPhoneException(failure: DevPhoneFailure.noLiveSession) =>
    'Open $app in Studio first (any of its plugins), then tap it here again.',
  DevPhoneException(failure: DevPhoneFailure.wrongNumber) =>
    "That wasn't the number Studio showed. In Studio, choose Let it try again, then tap $app again.",
  DevPhoneException(failure: DevPhoneFailure.closed) =>
    'The request expired before a number was picked. Tap $app to try again.',
  DevPhoneException(failure: DevPhoneFailure.unreachable) => "Studio's server can't be reached. Check the phone's connection (adb reverse for a USB phone).",
  DevPhoneException(failure: DevPhoneFailure.rateLimited) =>
    'Too many attempts. Wait a few minutes and try again.',
  DevPhoneException(:final message) => '$app could not join: $message',
  _ => '$app could not start: $error',
};

/// The Dev app.
final class DevApp extends StatefulWidget {
  /// Creates the app. [rootKeys] are the development roots it was built
  /// with; without any it opens nothing. [phones] gives the phone's sign-in;
  /// [startRuntime], [stopRuntime], [scope], [entryRoute] and [scan]
  /// replace the runtime and the camera in tests.
  const DevApp({
    super.key,
    required this.rootKeys,
    required this.phones,
    this.deviceName = '',
    this.startRuntime = Plux.initialize,
    this.stopRuntime = Plux.dispose,
    this.scope = _pluxScope,
    this.entryRoute = _pluxEntryRoute,
    this.scan,
    this.newSession,
  });

  /// Makes the dev session of a running app; the runtime's unless a test
  /// replaces it.
  final PluxDevSession Function()? newSession;

  /// The development roots.
  final List<PluxPublicKey> rootKeys;

  /// The phone's sign-in.
  final PhoneDirectory phones;

  /// The phone's name in the session's device list.
  final String deviceName;

  /// Starts the runtime.
  final RuntimeStarter startRuntime;

  /// Stops the runtime.
  final RuntimeStopper stopRuntime;

  /// Provides the runtime to the app once it runs.
  final RuntimeScope scope;

  /// The running app's entry route.
  final EntryRouteOf entryRoute;

  /// Scans a code; the camera unless a test replaces it.
  final Future<String?> Function(NavigatorState navigator)? scan;

  @override
  State<DevApp> createState() => _DevAppState();
}

/// A running app: its router and dev tools.
final class _Running {
  _Running(this.name, this.router, this.tooling);

  final String name;
  final GoRouter router;
  final DevTooling tooling;
}

final class _DevAppState extends State<DevApp> {
  final GlobalKey<NavigatorState> _launcher = GlobalKey();
  final GlobalKey<ScaffoldMessengerState> _messenger = GlobalKey();
  bool _restoring = true;
  SignedInPhone? _phone;
  String? _signedOutNotice;
  List<DevPhoneApp> _apps = const [];
  _Running? _running;

  @override
  void initState() {
    super.initState();
    unawaited(_restore());
  }

  @override
  void dispose() {
    final running = _running;
    if (running != null) {
      running.router.dispose();
      unawaited(running.tooling.dispose());
    }
    super.dispose();
  }

  Future<void> _restore() async {
    SignedInPhone? phone;
    try {
      phone = await widget.phones.restore();
    } on Object {
      phone = null;
    }
    if (mounted) {
      setState(() {
        _phone = phone;
        _restoring = false;
      });
    }
  }

  Future<String?> _scan() async {
    final navigator = _launcher.currentState;
    if (navigator == null) return null;
    return (widget.scan ?? scanPairingCode)(navigator);
  }

  void _say(String message) => _messenger.currentState
    ?..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(message)));

  Future<List<DevPhoneApp>> _loadApps() async {
    final phone = _phone;
    if (phone == null) return const [];
    try {
      final apps = await phone.apps();
      _apps = apps;
      return apps;
    } on DevPhoneException catch (e) {
      if (e.failure == DevPhoneFailure.signedOut) _signedOut();
      rethrow;
    }
  }

  void _signedOut() {
    if (!mounted) return;
    setState(() {
      _phone = null;
      _signedOutNotice =
          'This phone was signed out. Connect it again from Studio.';
    });
  }

  Future<void> _signOut() async {
    await _phone?.signOut();
    if (mounted) setState(() => _phone = null);
  }

  /// Opens [app]: joins the person's Studio session of it, after the number
  /// match the first time, then starts it at its entry screen.
  Future<void> _open(DevPhoneApp app) async {
    final phone = _phone;
    final context = _launcher.currentContext;
    if (phone == null || context == null) return;
    try {
      final join = await phone.requestJoin(app.id);
      var link = join.pairingLink;
      if (link == null) {
        if (!context.mounted) return;
        final picked = await pickNumber(
          context,
          options: join.options,
          title: 'Pick the number shown in Studio',
          detail: 'Joining ${app.name} on this phone',
          expiresAt: join.expiresAt,
        );
        if (picked == null) return;
        link = await phone.completeJoin(join, picked);
      }
      await _launch(app.name, app.id, join.environment, link);
    } on DevPhoneException catch (e) {
      if (e.failure == DevPhoneFailure.signedOut) {
        _signedOut();
      } else {
        _say(joinFailure(e, app.name));
      }
    }
  }

  /// Scans a code from the app list: a session's pairing code opens its app
  /// at once; a sign-in code connects the phone again.
  Future<void> _scanCode() async {
    final text = await _scan();
    if (text == null || text.trim().isEmpty) return;
    if (DevSignInLink.matches(text)) {
      setState(() => _phone = null);
      _say('Connect the phone again: scan the code once more.');
      return;
    }
    final appId = pairingAppOf(text);
    if (appId == null) {
      _say("That isn't a Plux Dev code.");
      return;
    }
    final name =
        _apps.where((a) => a.id == appId).firstOrNull?.name ?? 'The app';
    await _launch(name, appId, devEnvironment, text);
  }

  /// Starts the runtime for [appId] in [environment] and pairs it with
  /// [link]; the app then opens at its entry screen.
  Future<void> _launch(
    String name,
    String appId,
    String environment,
    String link,
  ) async {
    final phone = _phone;
    if (phone == null) return;
    final key = GlobalKey<NavigatorState>();
    final tooling = DevTooling(
      rootKeys: widget.rootKeys,
      navigatorKey: key,
      deviceName: widget.deviceName,
      scan: widget.scan,
      session: widget.newSession?.call(),
    );
    final router = GoRouter(
      navigatorKey: key,
      routes: [
        GoRoute(
          path: '/',
          builder: (context, _) => DevLaunchScreen(
            appName: name,
            entryRoute: widget.entryRoute(),
            onLeave: _leave,
          ),
        ),
        ...PluxGoRoutes().routes,
      ],
    );
    final packages = DevPackages(router: router);
    tooling.attachSecurity(packages.security);
    try {
      await widget.startRuntime(
        packages.configFor(
          appId: appId,
          endpoint: phone.account.server,
          environment: environment.isEmpty ? devEnvironment : environment,
          rootKeys: widget.rootKeys,
          hostBuild: devHostBuild,
          // Top-level functions: the runtime calls them on its sync and data
          // isolates too, and a closure over this state cannot be sent there
          // ("object is unsendable"). The network profiles of the dev tools
          // (DevNetworkSimulation) therefore do not reach these requests.
          client: devHttpClient,
          webSocketClient: devIoClient,
        ),
      );
      await tooling.session.pair(link);
      final paired = tooling.session.state.value;
      if (paired is PluxDevError) {
        throw StateError('the session refused the phone (${paired.code})');
      }
    } on Object catch (e) {
      router.dispose();
      await tooling.dispose();
      await _stopQuietly();
      _say('$name could not start: $e');
      return;
    }
    if (mounted) setState(() => _running = _Running(name, router, tooling));
  }

  Future<void> _stopQuietly() async {
    try {
      await widget.stopRuntime();
    } on Object {
      // Nothing was started.
    }
  }

  /// Leaves the running app: the pairing ends, the runtime stops, the list
  /// shows again.
  void _leave() {
    final running = _running;
    if (running == null) return;
    setState(() => _running = null);
    unawaited(() async {
      await running.tooling.session.end();
      await running.tooling.dispose();
      running.router.dispose();
      await _stopQuietly();
    }());
  }

  @override
  Widget build(BuildContext context) {
    final running = _running;
    if (running != null) {
      return widget.scope(
        MaterialApp.router(
          title: running.name,
          debugShowCheckedModeBanner: false,
          routerConfig: running.router,
          theme: devTheme(),
          builder: (context, child) => DevRunOverlay(
            tooling: running.tooling,
            appName: running.name,
            onLeave: _leave,
            child: running.tooling.wrap(child!),
          ),
        ),
      );
    }
    final phone = _phone;
    return MaterialApp(
      title: 'Plux Dev',
      debugShowCheckedModeBanner: false,
      navigatorKey: _launcher,
      scaffoldMessengerKey: _messenger,
      theme: devTheme(),
      home: widget.rootKeys.isEmpty
          ? const _NoRoots()
          : _restoring
          ? const Scaffold(body: Center(child: CircularProgressIndicator()))
          : phone == null
          ? _Connect(
              notice: _signedOutNotice,
              child: ConnectScreen(
                scan: (_) => _scan(),
                claim: widget.phones.claim,
                onSignedIn: (p) => setState(() {
                  _phone = p;
                  _signedOutNotice = null;
                }),
              ),
            )
          : AppsScreen(
              key: ValueKey(phone.account.phoneId),
              account: phone.account,
              load: _loadApps,
              onOpen: _open,
              onScan: _scanCode,
              onSignOut: _signOut,
            ),
    );
  }
}

/// The connect screen, with the notice of a phone that was signed out.
final class _Connect extends StatelessWidget {
  const _Connect({required this.notice, required this.child});

  final String? notice;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final n = notice;
    if (n == null) return child;
    return Stack(
      children: [
        child,
        Positioned(
          left: 16,
          right: 16,
          bottom: 16 + MediaQuery.paddingOf(context).bottom,
          child: DevCard(child: Text(n)),
        ),
      ],
    );
  }
}

final class _NoRoots extends StatelessWidget {
  const _NoRoots();

  @override
  Widget build(BuildContext context) => const Scaffold(
    body: SafeArea(
      child: Padding(
        padding: EdgeInsets.all(DevSizes.page),
        child: Text(noDevelopmentRootsMessage),
      ),
    ),
  );
}
