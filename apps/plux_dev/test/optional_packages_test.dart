// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// Every first-party optional package is registered with the runtime
/// [DEV-001].
void main() {
  const optionalPackages = {
    'plux_charts',
    'plux_components',
    'plux_db_drift',
    'plux_functions',
    'plux_go_router',
    'plux_location',
    'plux_lottie',
    'plux_maps',
    'plux_markdown',
    'plux_media',
    'plux_player',
    'plux_rive',
    'plux_scanner',
    'plux_security',
  };

  late DevPackages packages;
  late PluxConfig config;

  setUp(() {
    packages = DevPackages(router: GoRouter(routes: []));
    config = packages.configFor(
      appId: 'app_1',
      endpoint: Uri.parse('https://plux.example.org/'),
      environment: devEnvironment,
      rootKeys: const [],
      hostBuild: devHostBuild,
    );
  });

  test('lists every first-party optional package once [DEV-001]', () {
    expect(packages.registered.toSet(), optionalPackages);
    expect(packages.registered, hasLength(optionalPackages.length));
  });

  test('the configuration carries them [DEV-001]', () {
    expect(config.widgetPackages.map((p) => p.name), {
      'plux_charts',
      'plux_components',
      'plux_lottie',
      'plux_maps',
      'plux_markdown',
      'plux_media',
      'plux_player',
      'plux_rive',
      'plux_scanner',
    });
    expect(config.devicePackages.map((p) => p.name), {
      'plux_location',
      'plux_media',
      'plux_scanner',
    });
    expect(config.functionEngine, isNotNull);
    expect(config.databaseAdapter, isNotNull);
    expect(config.router, isNotNull);
    expect(config.securityModules.map((m) => m.name), ['plux_security']);
  });

  test('pins the development roots it is given and no baseline [DEV-001]', () {
    expect(config.rootKeys, isEmpty);
    expect(config.baseline, isNull);
    expect(config.environment, 'development');
    expect(config.hostBuild, 'plux-dev');
  });
}
