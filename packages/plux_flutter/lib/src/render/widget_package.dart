// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What an optional widget package gives the renderer (RT-060, ADR-0071): the
/// builders of the Layer 2 widgets whose descriptors name the package.
library;

import 'package:plux_flutter/src/render/node_context.dart';

/// A package of catalogue widgets the host registers in
/// `PluxConfig.widgetPackages` (RT-060), such as `plux_charts`: a host pays
/// for a package's dependencies only when it registers the package.
///
/// A package reads its node's props through [NodeContext] and decodes them
/// with the `as*` decoders and `Fields` the library exports; prop, event
/// and slot IDs are those of the widget's descriptor in
/// `schema/widgets/layer2`.
abstract interface class PluxWidgetPackage {
  /// The package's name, such as `plux_charts`; the name the descriptors of
  /// its widgets carry in `package`.
  String get name;

  /// The builders the package provides, by permanent widget ID. They run
  /// after the runtime's own, and win on a clash.
  Map<int, NodeBuilder> get builders;
}
