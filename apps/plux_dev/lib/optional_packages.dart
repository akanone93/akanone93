// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Every first-party optional package, registered with the runtime
/// (DEV-001): the Dev app can preview any plugin, so it carries them all.
library;

import 'package:go_router/go_router.dart';
import 'package:http/http.dart' as http;
import 'package:plux_charts/plux_charts.dart';
import 'package:plux_components/plux_components.dart';
import 'package:plux_db_drift/plux_db_drift.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_functions/plux_functions.dart';
import 'package:plux_go_router/plux_go_router.dart';
import 'package:plux_location/plux_location.dart';
import 'package:plux_lottie/plux_lottie.dart';
import 'package:plux_maps/plux_maps.dart';
import 'package:plux_markdown/plux_markdown.dart';
import 'package:plux_media/plux_media.dart';
import 'package:plux_player/plux_player.dart';
import 'package:plux_rive/plux_rive.dart';
import 'package:plux_scanner/plux_scanner.dart';
import 'package:plux_security/plux_security.dart';

import 'package:plux_dev/development_roots.dart';

/// The optional packages of the Dev app and the `PluxConfig` that registers
/// them.
///
/// `plux_maps_google` and `plux_auto_route` are not here: the first answers
/// the same `MapView` as `plux_maps` and needs an API key a shared dev build
/// cannot hold, the second is the other router adapter, and a host has one.
final class DevPackages {
  /// Creates the registrations. [router] is the app's `GoRouter`.
  DevPackages({required this.router})
    : widgetPackages = [
        const PluxCharts(),
        const PluxComponents(),
        const PluxMarkdown(),
        const PluxMaps(),
        const PluxMediaWidgets(),
        const PluxPlayer(),
        const PluxScannerWidgets(),
        const PluxLottieWidgets(),
        const PluxRiveWidgets(),
      ],
      devicePackages = [
        PluxLocation(),
        PluxMedia(),
        PluxScanner(navigatorKey: router.routerDelegate.navigatorKey),
      ],
      functionEngine = PluxFunctions(),
      databaseAdapter = PluxDriftAdapter(),
      security = PluxSecurity(
        mode: PluxSecurityMode.developmentReportOnly,
        environment: devEnvironment,
      ) {
    securityModules = [security];
  }

  /// The app's router, which the Plux router adapter wraps.
  final GoRouter router;

  /// Widget packages: charts, components, markdown, maps, media, player,
  /// scanner, Lottie and Rive widgets.
  final List<PluxWidgetPackage> widgetPackages;

  /// Device packages: location, media and scanner actions.
  final List<PluxDevicePackage> devicePackages;

  /// The device function engine.
  final PluxFunctionEngine functionEngine;

  /// The encrypted database.
  final PluxDatabaseAdapter databaseAdapter;

  /// Runtime application self-protection.
  /// `plux_security` in its report-only development mode (DEV-001,
  /// ADR-0057): detections go to the dev stream and lower nothing.
  final PluxSecurity security;

  /// The security modules: [security].
  late final List<PluxSecurityModule> securityModules;

  /// The names of the optional packages these registrations carry, each once
  /// (media and scanner register both widgets and actions), sorted.
  List<String> get registered => {
    for (final p in widgetPackages) p.name,
    for (final p in devicePackages) p.name,
    functionEngine.name,
    if (databaseAdapter is PluxDriftAdapter) 'plux_db_drift',
    for (final m in securityModules) m.name,
    'plux_go_router',
  }.toList()..sort();

  /// The runtime configuration for [appId] on [endpoint]: the development
  /// [rootKeys] and no others, the optional packages, and [client] for every
  /// request (the dev tools' network profiles wrap it).
  PluxConfig configFor({
    required String appId,
    required Uri endpoint,
    required String environment,
    required List<PluxPublicKey> rootKeys,
    required String hostBuild,
    http.Client Function()? client,
  }) => PluxConfig(
    appId: appId,
    endpoint: endpoint,
    environment: environment,
    rootKeys: rootKeys,
    hostBuild: hostBuild,
    baseline: null,
    router: PluxGoRouter(router),
    widgetPackages: widgetPackages,
    devicePackages: devicePackages,
    functionEngine: functionEngine,
    databaseAdapter: databaseAdapter,
    securityModules: securityModules,
    httpClient: client,
  );
}
