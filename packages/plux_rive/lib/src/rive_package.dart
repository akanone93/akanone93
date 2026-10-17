// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package of `plux_rive`: the builder of the `RiveView`
/// catalogue widget (ADR-0050 Revision). The ID and prop IDs are those of
/// the descriptor in `schema/widgets/layer2`; a test compares them.
library;

import 'dart:typed_data';

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_rive/src/plux_rive.dart';
import 'package:rive/rive.dart' as rive;

/// The widget IDs of the package's descriptors.
abstract final class PluxRiveIds {
  /// `RiveView`.
  static const int riveView = 206;
}

/// The `RiveView` widget, to register in `PluxConfig.widgetPackages`.
final class PluxRiveWidgets implements PluxWidgetPackage {
  /// Creates the package.
  const PluxRiveWidgets();

  @override
  String get name => 'plux_rive';

  @override
  Map<int, NodeBuilder> get builders => {PluxRiveIds.riveView: _riveView};
}

/// A map prop: a plain map, or an object of a bundle.
Map<String, Object?> _map(Object? v) => switch (v) {
  final PluxObject o => o.toMap(),
  final Map<Object?, Object?> m => m.cast<String, Object?>(),
  _ => const {},
};

Widget _riveView(NodeContext c) {
  final url = c.decode(1, asString) ?? c.missing('RiveView.url');
  // Fetched through the pinned client, from an HTTPS domain the plugin
  // declares only (SEC-080, SEC-041, AST-002).
  final label = c.decode(9, asString);
  final view = PluxRiveView(
    // The Rive renderer draws the animation; the loader decodes the bytes
    // the pinned client fetched, once, and caches the file for the widget's
    // life.
    loader: () => _FetchedFileLoader(() => c.remoteBytes(url)),
    cacheKey: url,
    artboard: c.decode(2, asString),
    stateMachine: c.decode(3, asString),
    inputs: {..._map(c.prop(4)), ..._map(c.prop(5))},
    triggers: _map(c.prop(6)),
    fit: rive.Fit.values.asNameMap()[c.decode(7, asString)] ?? rive.Fit.contain,
    ignoreReduceMotion: c.decode(8, asString) == 'ignore',
    onLoaded: c.handles(1) ? () => c.fire(1) : null,
    onFailed: c.handles(2) ? () => c.fire(2) : null,
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

/// A Rive file decoded from the bytes the pinned client fetched; the load
/// fails (and the view reports it) when they cannot be had.
final class _FetchedFileLoader extends rive.FileLoader {
  _FetchedFileLoader(this._fetch)
    : super.fromUrl('', riveFactory: rive.Factory.rive);

  final Future<Uint8List?> Function() _fetch;
  Future<rive.File>? _file;

  @override
  Future<rive.File> file() => _file ??= _load();

  Future<rive.File> _load() async {
    final bytes = await _fetch();
    final file = bytes == null
        ? null
        : await rive.File.decode(bytes, riveFactory: rive.Factory.rive);
    if (file == null) throw const FormatException('the file is not available');
    return file;
  }
}
