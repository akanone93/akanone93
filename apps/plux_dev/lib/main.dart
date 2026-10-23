// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/widgets.dart';
import 'package:plux_dev/dev.dart';

/// Starts the Plux Dev app with the development root keys it was built with
/// (`--dart-define=PLUX_DEV_ROOT_KEYS=<keys.json>`) and the dev channel
/// (`--dart-define=plux.devtools=true`). The runtime starts once the
/// developer has chosen the app to preview.
void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(DevApp(rootKeys: developmentRootKeys()));
}
