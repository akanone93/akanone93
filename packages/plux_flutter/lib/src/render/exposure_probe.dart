// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Tells when a subtree is built into the tree, for experiment exposure
/// (ABT-002): exposure means the variant was built for the user, not that
/// it was assigned.
library;

import 'package:flutter/widgets.dart';

/// Calls [onShown] once, when [child] is first mounted.
final class ExposureProbe extends StatefulWidget {
  /// Creates the probe around [child].
  const ExposureProbe({required this.onShown, required this.child, super.key});

  /// Called when the subtree is mounted.
  final VoidCallback onShown;

  /// The subtree being watched.
  final Widget child;

  @override
  State<ExposureProbe> createState() => _ExposureProbeState();
}

final class _ExposureProbeState extends State<ExposureProbe> {
  @override
  void initState() {
    super.initState();
    widget.onShown();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
