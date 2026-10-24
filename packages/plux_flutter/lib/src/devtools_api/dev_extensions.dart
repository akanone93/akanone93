// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The device tools of a dev session as command handlers (`DEV-030` to
/// `DEV-034`), registered with `RuntimeDevCommands.extensions`. Only builds
/// with `kPluxDevtools` create them.
library;

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/devtools_api/dev_console.dart';
import 'package:plux_flutter/src/devtools_api/dev_inspect.dart';
import 'package:plux_flutter/src/devtools_api/dev_network.dart';
import 'package:plux_flutter/src/devtools_api/dev_nodes.dart';
import 'package:plux_flutter/src/devtools_api/dev_overlays.dart';
import 'package:plux_flutter/src/devtools_api/dev_screenshot.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/pxl/vm.dart';

/// The handlers of `inspectSelect`, `toggleOverlay`, `evaluatePxl`,
/// `captureScreenshot` and `networkProfile`, by command name.
///
/// The host puts a [DevToolsLayer] and a [DevNodeScope] over the app, a
/// [DevCaptureBoundary] with [captureKey] inside it, and wraps its HTTP
/// clients with [network].
Map<String, DevCommandHandler> devExtensions({
  required DevNodeHost host,
  required DevEventSink sink,
  required Map<String, Object?> Function() roots,
  required GlobalKey captureKey,
  required DevNetworkSimulation network,
  DevPxlCompiler? compile,
  PxlLimits? limits,
  bool Function(RenderObject boundary)? platformViews,
}) => {
  'inspectSelect': DevInspectHandler(host: host, sink: sink),
  'toggleOverlay': DevOverlaysHandler(host: host),
  'evaluatePxl': DevConsoleHandler(
    compile: compile,
    roots: roots,
    limits: limits,
  ),
  'captureScreenshot': DevScreenshotHandler(
    captureKey: captureKey,
    sink: sink,
    platformViews: platformViews,
  ),
  'networkProfile': DevNetworkHandler(simulation: network),
};
