// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/widgets.dart';
import 'package:plux_bank/bank.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_security/testing.dart';

/// The app the accessibility-tree test walks (QA-011): `lib/main.dart`, built
/// with the flows' defines, on a device the flows treat as clean. Emulators
/// look rooted to the bank (SEC-071), which would cover every page, as in
/// `bank_flows.dart`.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final config = BankConfig.fromEnvironment();
  final host = BankHost(detector: FakeRaspDetector());
  final startup = await Plux.initialize(config.toPluxConfig(host: host));
  runApp(BankApp(config: config, startup: startup, host: host));
}
