// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The zoom buttons of a map (A11Y-001, A11Y-004): a screen-reader user
/// cannot pinch, so the map offers the same zoom as two labelled buttons.
library;

import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// Zoom-in and zoom-out buttons for [controller], each a 48 x 48 target
/// labelled with the runtime's `plux.map.zoomIn` / `plux.map.zoomOut`.
final class PluxMapZoomControls extends StatelessWidget {
  /// Creates the buttons; zoom stays within [minZoom] and [maxZoom].
  const PluxMapZoomControls({
    required this.controller,
    required this.minZoom,
    required this.maxZoom,
    super.key,
  });

  /// The controller of the map the buttons drive.
  final MapController controller;

  /// The lowest zoom the buttons reach.
  final double minZoom;

  /// The highest zoom the buttons reach.
  final double maxZoom;

  void _zoom(double delta) {
    final camera = controller.camera;
    controller.move(
      camera.center,
      (camera.zoom + delta).clamp(minZoom, maxZoom),
    );
  }

  @override
  Widget build(BuildContext context) {
    final strings = RuntimeStrings.of(context);
    final theme = Theme.of(context);
    Widget button(IconData icon, String label, double delta) => IconButton(
      icon: Icon(icon, semanticLabel: label),
      tooltip: label,
      constraints: const BoxConstraints.tightFor(width: 48, height: 48),
      onPressed: () => _zoom(delta),
    );
    return Material(
      color: theme.colorScheme.surface.withValues(alpha: 0.9),
      borderRadius: BorderRadius.circular(8),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          button(Icons.add, strings['plux.map.zoomIn'], 1),
          button(Icons.remove, strings['plux.map.zoomOut'], -1),
        ],
      ),
    );
  }
}
