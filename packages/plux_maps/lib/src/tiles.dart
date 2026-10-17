// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The tile provider: every tile goes through the runtime's pinned client,
/// for a domain the plugin declares (SEC-080, AST-002).
library;

import 'dart:ui' as ui;

import 'package:flutter/foundation.dart';
import 'package:flutter/painting.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:http/http.dart' as http;

/// The client for one tile URL: the pinned client, or null when the URL is
/// blocked (the runtime has reported it).
typedef TileClientFor = http.Client? Function(String url);

/// A [TileProvider] that fetches tiles with the client of [clientFor].
///
/// The first blocked tile marks the provider blocked: later tiles are not
/// asked for, so the runtime reports the domain once (PLX-6030), and no tile
/// is drawn.
final class PluxTileProvider extends TileProvider {
  /// Creates a provider over [clientFor].
  PluxTileProvider(this.clientFor);

  /// Resolves the client of a tile URL; replaced when the node rebuilds.
  TileClientFor clientFor;

  bool _blocked = false;

  /// Whether a tile was blocked.
  bool get blocked => _blocked;

  @override
  ImageProvider getImage(TileCoordinates coordinates, TileLayer options) {
    final url = getTileUrl(coordinates, options);
    final client = _blocked ? null : clientFor(url);
    if (client == null) _blocked = true;
    return PluxTileImage(url, client);
  }
}

/// A tile image fetched with [client]; without one it fails to load, which
/// leaves the tile blank.
@immutable
final class PluxTileImage extends ImageProvider<PluxTileImage> {
  /// Creates a tile image of [url].
  const PluxTileImage(this.url, this.client);

  /// The tile URL.
  final String url;

  /// The pinned client; null when the URL is blocked.
  final http.Client? client;

  @override
  Future<PluxTileImage> obtainKey(ImageConfiguration configuration) =>
      SynchronousFuture<PluxTileImage>(this);

  @override
  ImageStreamCompleter loadImage(
    PluxTileImage key,
    ImageDecoderCallback decode,
  ) => MultiFrameImageStreamCompleter(
    codec: _load(decode),
    scale: 1,
    debugLabel: url,
  );

  Future<ui.Codec> _load(ImageDecoderCallback decode) async {
    final c = client;
    if (c == null) throw StateError('the tile $url is blocked');
    final response = await c.get(Uri.parse(url));
    if (response.statusCode != 200 || response.bodyBytes.isEmpty) {
      throw StateError('the tile $url answered ${response.statusCode}');
    }
    return decode(await ui.ImmutableBuffer.fromUint8List(response.bodyBytes));
  }

  @override
  bool operator ==(Object other) =>
      other is PluxTileImage && other.url == url && other.client == client;

  @override
  int get hashCode => Object.hash(url, client);
}
