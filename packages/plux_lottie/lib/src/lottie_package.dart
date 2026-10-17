// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package of `plux_lottie`: the builder of the `LottieView`
/// catalogue widget (ADR-0050 Revision). The ID and prop IDs are those of
/// the descriptor in `schema/widgets/layer2`; a test compares them.
library;

import 'dart:typed_data';

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
  // The animation is fetched through the pinned client, and only from an
  // HTTPS domain the plugin declares (SEC-080, SEC-041, AST-002).
  final label = c.decode(10, asString);
  final animate = c.decode(2, asBool) ?? true;
  final repeat = c.decode(3, asBool) ?? true;
  final reverse = c.decode(4, asBool) ?? false;
  final progress = c.decode(5, asDouble);
  final fit =
      BoxFit.values.asNameMap()[c.decode(6, asString)] ?? BoxFit.contain;
  final width = c.decode(7, asDouble);
  final height = c.decode(8, asDouble);
  final reduceMotion = switch (c.decode(9, asString)) {
    'shorten' => PluxReduceMotion.shorten,
    'ignore' => PluxReduceMotion.ignore,
    _ => PluxReduceMotion.skip,
  };
  final onCompleted = c.handles(1) ? () => c.fire(1) : null;
  final view = _Fetched(
    key: ValueKey(url),
    fetch: () => c.remoteBytes(url),
    builder: (bytes) => PluxLottieView(
      provider: MemoryLottie(bytes),
      animate: animate,
      repeat: repeat,
      reverse: reverse,
      progress: progress,
      fit: fit,
      width: width,
      height: height,
      reduceMotion: reduceMotion,
      onCompleted: onCompleted,
    ),
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

/// Shows what [builder] makes of the bytes [fetch] returns; nothing while
/// they load and when they cannot be had (the reason was reported).
final class _Fetched extends StatefulWidget {
  const _Fetched({required this.fetch, required this.builder, super.key});

  final Future<Uint8List?> Function() fetch;
  final Widget Function(Uint8List bytes) builder;

  @override
  State<_Fetched> createState() => _FetchedState();
}

final class _FetchedState extends State<_Fetched> {
  late final Future<Uint8List?> _bytes = widget.fetch();

  @override
  Widget build(BuildContext context) => FutureBuilder<Uint8List?>(
    future: _bytes,
    builder: (context, snapshot) {
      final bytes = snapshot.data;
      return bytes == null ? const SizedBox.shrink() : widget.builder(bytes);
    },
  );
}
