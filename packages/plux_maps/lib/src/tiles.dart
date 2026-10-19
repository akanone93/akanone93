// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The tile provider: every tile is a remote image of the runtime, so it goes
/// through the pinned client, the domain check, the size limit and the disk
/// cache (SEC-080, AST-002, LIM-001).
library;

import 'dart:typed_data';

import 'package:flutter/painting.dart';
import 'package:flutter_map/flutter_map.dart';

/// The image of one tile URL: the runtime's remote image, or null when the
/// URL is blocked (the runtime has reported it).
typedef TileImageFor = ImageProvider<Object>? Function(String url);

/// A transparent 1x1 PNG, drawn for a blocked tile.
final Uint8List transparentTile = Uint8List.fromList(const [
  0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, //
  0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
  0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
  0x0b, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x60, 0x00, 0x02, 0x00,
  0x00, 0x05, 0x00, 0x01, 0x7a, 0x5e, 0xab, 0x3f, 0x00, 0x00, 0x00, 0x00,
  0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
]);

/// A [TileProvider] over the tile images of [imageFor].
///
/// The first blocked tile marks the provider blocked: later tiles are not
/// asked for, so the runtime reports the domain once (PLX-6030), and every
/// tile is transparent.
final class PluxTileProvider extends TileProvider {
  /// Creates a provider over [imageFor].
  PluxTileProvider(this.imageFor);

  /// Resolves the image of a tile URL; replaced when the node rebuilds.
  TileImageFor imageFor;

  bool _blocked = false;

  /// Whether a tile was blocked.
  bool get blocked => _blocked;

  @override
  ImageProvider getImage(TileCoordinates coordinates, TileLayer options) {
    if (!_blocked) {
      final image = imageFor(getTileUrl(coordinates, options));
      if (image != null) return image;
      _blocked = true;
    }
    return MemoryImage(transparentTile);
  }
}
