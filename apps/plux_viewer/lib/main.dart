// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/widgets.dart';
import 'package:plux_viewer/viewer.dart';

/// Starts the viewer with the root keys it was built with
/// (`--dart-define=PLUX_VIEWER_ROOT_KEYS=<keys.json>`). The runtime is
/// started later, once per reconstruction the auditor opens, in
/// reconstruction mode.
void main() {
  WidgetsFlutterBinding.ensureInitialized();
  final controller = ViewerController();
  ViewerLinks(controller)
      .attach(WidgetsBinding.instance.platformDispatcher.defaultRouteName);
  runApp(ViewerApp(controller: controller, rootKeys: builtInRootKeys()));
}
