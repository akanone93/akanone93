// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package of `plux_media`: the builder of `SignaturePad`. The ID
/// is that of the descriptor in `schema/widgets/layer2`; a test compares it.
library;

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_media/src/signature_pad.dart';

/// The widget IDs of the package's descriptors.
abstract final class PluxMediaWidgetIds {
  /// `SignaturePad`.
  static const int signaturePad = 202;
}

/// The widgets of `plux_media`, to register in
/// `PluxConfig.widgetPackages` (beside `PluxMedia` in `devicePackages`).
final class PluxMediaWidgets implements PluxWidgetPackage {
  /// Creates the package.
  const PluxMediaWidgets();

  @override
  String get name => 'plux_media';

  @override
  Map<int, NodeBuilder> get builders => {
    PluxMediaWidgetIds.signaturePad: _signaturePad,
  };
}

Widget _signaturePad(NodeContext c) => PluxSignaturePad(
  format: c.decode(1, asString) == 'png'
      ? SignatureFormat.png
      : SignatureFormat.svg,
  strokeWidth: (c.decode(2, asDouble) ?? 2).clamp(0.5, 12.0),
  height: (c.decode(3, asDouble) ?? 160).clamp(80.0, double.infinity),
  clearLabel: c.decode(4, asString) ?? 'Clear',
  semanticLabel: c.decode(5, asString),
  onChanged: c.handles(1) ? (value) => c.fire(1, value) : null,
);
