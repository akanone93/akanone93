// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package: builders for `VideoPlayer` and `AudioPlayer`. The IDs
/// are those of the descriptors in `schema/widgets/layer2`; a test compares
/// them.
library;

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_player/src/players.dart';

/// The widget IDs of the package's descriptors.
abstract final class PluxPlayerIds {
  /// `VideoPlayer`.
  static const int videoPlayer = 203;

  /// `AudioPlayer`.
  static const int audioPlayer = 204;
}

/// The video and audio widgets of Plux pages, to register in
/// `PluxConfig.widgetPackages`.
final class PluxPlayer implements PluxWidgetPackage {
  /// Creates the package.
  const PluxPlayer();

  @override
  String get name => 'plux_player';

  @override
  Map<int, NodeBuilder> get builders => {
    PluxPlayerIds.videoPlayer: _video,
    PluxPlayerIds.audioPlayer: _audio,
  };
}

VoidCallback? _event(NodeContext c, int id) =>
    c.handles(id) ? () => c.fire(id) : null;

Widget _video(NodeContext c) => PluxVideoPlayer(
  url: c.decode(1, asString) ?? c.missing('VideoPlayer.url'),
  captions: c.decode(2, asString),
  autoplay: c.decode(3, asBool) ?? false,
  loop: c.decode(4, asBool) ?? false,
  aspectRatio: (c.decode(5, asDouble) ?? 16 / 9).clamp(0.2, 5.0),
  semanticLabel: c.decode(6, asString),
  playLabel: c.decode(7, asString) ?? 'Play',
  pauseLabel: c.decode(8, asString) ?? 'Pause',
  captionsLabel: c.decode(9, asString) ?? 'Captions',
  positionLabel: c.decode(10, asString) ?? 'Position',
  onEnded: _event(c, 1),
  onError: _event(c, 2),
);

Widget _audio(NodeContext c) => PluxAudioPlayer(
  url: c.decode(1, asString) ?? c.missing('AudioPlayer.url'),
  title: c.decode(2, asString),
  autoplay: c.decode(3, asBool) ?? false,
  loop: c.decode(4, asBool) ?? false,
  playLabel: c.decode(5, asString) ?? 'Play',
  pauseLabel: c.decode(6, asString) ?? 'Pause',
  positionLabel: c.decode(7, asString) ?? 'Position',
  onEnded: _event(c, 1),
  onError: _event(c, 2),
);
