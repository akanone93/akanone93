// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package of `plux_lottie`: the builder of the `LottieView`
/// catalogue widget (ADR-0050 Revision). The ID and prop IDs are those of
/// the descriptor in `schema/widgets/layer2`; a test compares them.
library;

import 'package:flutter/widgets.dart';
import 'package:lottie/lottie.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_lottie/src/plux_lottie.dart';

/// The widget IDs of the package's descriptors.
abstract final class PluxLottieIds {
  /// `LottieView`.
  static const int lottieView = 205;
}

/// The `LottieView` widget, to register in `PluxConfig.widgetPackages`.
final class PluxLottieWidgets implements PluxWidgetPackage {
  /// Creates the package.
  const PluxLottieWidgets();

  @override
  String get name => 'plux_lottie';

  @override
  Map<int, NodeBuilder> get builders => {PluxLottieIds.lottieView: _lottieView};
}

Widget _lottieView(NodeContext c) {
  final url = c.decode(1, asString) ?? c.missing('LottieView.url');
  // Only https addresses play: an animation is data, but never over a
  // connection that anyone can change.
  if (!url.startsWith('https://')) return const SizedBox.shrink();
  final label = c.decode(10, asString);
  final view = PluxLottieView(
    provider: NetworkLottie(url),
    animate: c.decode(2, asBool) ?? true,
    repeat: c.decode(3, asBool) ?? true,
    reverse: c.decode(4, asBool) ?? false,
    progress: c.decode(5, asDouble),
    fit: BoxFit.values.asNameMap()[c.decode(6, asString)] ?? BoxFit.contain,
    width: c.decode(7, asDouble),
    height: c.decode(8, asDouble),
    reduceMotion: switch (c.decode(9, asString)) {
      'shorten' => PluxReduceMotion.shorten,
      'ignore' => PluxReduceMotion.ignore,
      _ => PluxReduceMotion.skip,
    },
    onCompleted: c.handles(1) ? () => c.fire(1) : null,
  );
  return label == null
      ? ExcludeSemantics(child: view)
      : Semantics(
          label: label,
          image: true,
          excludeSemantics: true,
          child: view,
        );
}
