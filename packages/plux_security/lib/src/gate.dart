// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `PluxSecurityGate`: the app-level widget that shows the `warn` message
/// and the `block` screen (SEC-071).
library;

import 'package:flutter/widgets.dart';
import 'package:plux_security/src/detection.dart';
import 'package:plux_security/src/security.dart';

/// Builds the message shown for [warnings], the detections whose response
/// is `warn` or stricter.
typedef PluxWarningBuilder = Widget Function(
  BuildContext context,
  List<PluxDetection> warnings,
);

/// Builds the blocking screen shown for the detections in force.
typedef PluxBlockedBuilder = Widget Function(
  BuildContext context,
  List<PluxDetection> detections,
);

/// Wraps the app's content and reacts to [security]'s status (SEC-071):
/// shows a message while a detection's response is `warn` or `degrade`,
/// and replaces the content with the blocking screen while it is `block`.
///
/// The content stays mounted under the blocking screen, so navigation state
/// survives a block that ends; it is hidden, receives no touches and is
/// excluded from accessibility. Place it where it covers every route, for
/// example in `MaterialApp.builder`. The default screens are plain and
/// English; give [blockedBuilder] and [warningBuilder] the app's own,
/// localised ones.
final class PluxSecurityGate extends StatelessWidget {
  /// Creates the gate.
  const PluxSecurityGate({
    required this.security,
    required this.child,
    this.blockedBuilder,
    this.warningBuilder,
    super.key,
  });

  /// The module whose status the gate follows.
  final PluxSecurity security;

  /// The app's content.
  final Widget child;

  /// The blocking screen; a plain one when null.
  final PluxBlockedBuilder? blockedBuilder;

  /// The warning message; a plain bar when null.
  final PluxWarningBuilder? warningBuilder;

  @override
  Widget build(BuildContext context) =>
      ValueListenableBuilder<PluxSecurityStatus>(
        valueListenable: security.status,
        child: child,
        builder: (context, status, content) {
          final detections = status.detections.values.toList();
          final warnings = status.warnings;
          return Stack(
            fit: StackFit.passthrough,
            children: [
              Visibility(
                visible: !status.blocked,
                maintainState: true,
                child: content!,
              ),
              if (status.blocked)
                (blockedBuilder ?? _defaultBlocked)(context, detections)
              else if (warnings.isNotEmpty)
                Align(
                  alignment: Alignment.bottomCenter,
                  child: (warningBuilder ?? _defaultWarning)(context, warnings),
                ),
            ],
          );
        },
      );

  static Widget _defaultBlocked(
    BuildContext context,
    List<PluxDetection> detections,
  ) => const ColoredBox(
    color: Color(0xFF000000),
    child: Center(
      child: Padding(
        padding: EdgeInsets.all(24),
        child: Text(
          'This app cannot run safely on this device.',
          textAlign: TextAlign.center,
          style: TextStyle(color: Color(0xFFFFFFFF), fontSize: 18),
        ),
      ),
    ),
  );

  static Widget _defaultWarning(
    BuildContext context,
    List<PluxDetection> warnings,
  ) => Semantics(
    liveRegion: true,
    child: const ColoredBox(
      color: Color(0xFFB00020),
      child: Padding(
        padding: EdgeInsets.all(12),
        child: Text(
          'This device is not secure. Do not enter sensitive information.',
          style: TextStyle(color: Color(0xFFFFFFFF), fontSize: 14),
        ),
      ),
    ),
  );
}
