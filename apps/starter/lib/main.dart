// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_devtools/plux_devtools.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_starter/starter.dart';

/// Starts Plux with the server named by `--dart-define`s, then the app.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final config = StarterConfig.fromEnvironment();
  final host = StarterHost();
  final startup = await Plux.initialize(config.toPluxConfig(host: host));
  // A debug build can pair with a dev session; a release build has none.
  final devSession = kDebugMode
      ? PluxDevSession(devRootKeys: config.devRootKeys)
      : null;
  runApp(
    StarterApp(
      config: config,
      startup: startup,
      host: host,
      devSession: devSession,
    ),
  );
}
