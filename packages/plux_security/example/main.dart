// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_security/plux_security.dart';

/// Starts Plux with runtime self-protection.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // The SHA-256 of the release signing certificate, from
  // `apksigner verify --print-certs`.
  const releaseCertSha256 =
      '00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:'
      '00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF';
  final security = PluxSecurity(
    expectations: PluxRaspExpectations(
      signingCertSha256: const [releaseCertSha256],
      iosTeamId: 'ABCDE12345',
      iosBundleId: 'com.example.app',
    ),
  );
  await Plux.initialize(
    PluxConfig(
      appId: 'app_example',
      endpoint: Uri.parse('https://plux.example.com'),
      securityModules: [security],
    ),
  );
  runApp(
    MaterialApp(
      builder: (context, child) =>
          PluxSecurityGate(security: security, child: child!),
      home: const Scaffold(),
    ),
  );
}
