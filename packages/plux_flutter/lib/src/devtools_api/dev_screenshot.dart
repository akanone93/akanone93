// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Screenshots of a dev session (`DEV-033`): `captureScreenshot` renders the
/// app inside a [DevCaptureBoundary] at the device's pixel ratio, encodes it
/// as PNG on a background isolate and reports it as a `screenshot` event.
/// Platform views are drawn by the platform and are not in the image; the
/// event says so. Only builds with `kPluxDevtools` create it.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:isolate';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/rendering.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/devtools_api/dev_nodes.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';

/// Marks the part of the app a screenshot captures.
final class DevCaptureBoundary extends StatelessWidget {
  /// Creates the boundary, found through [captureKey].
  const DevCaptureBoundary({
    super.key,
    required this.captureKey,
    required this.child,
  });

  /// The key [DevScreenshotHandler] captures.
  final GlobalKey captureKey;

  /// The app.
  final Widget child;

  @override
  Widget build(BuildContext context) =>
      RepaintBoundary(key: captureKey, child: child);
}

/// Carries out `captureScreenshot`.
final class DevScreenshotHandler implements DevCommandHandler {
  /// Creates the handler for the boundary [captureKey] names.
  /// [platformViews] says whether the boundary holds a platform view; it
  /// looks for one in the render tree unless a caller replaces it.
  DevScreenshotHandler({
    required this.captureKey,
    required this.sink,
    bool Function(RenderObject boundary)? platformViews,
  }) : platformViews = platformViews ?? hasPlatformView;

  /// The key of the [DevCaptureBoundary].
  final GlobalKey captureKey;

  /// Receives the `screenshot` event.
  final DevEventSink sink;

  /// Whether the render tree below the boundary holds a platform view.
  final bool Function(RenderObject boundary) platformViews;

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    final context = captureKey.currentContext;
    final boundary = context?.findRenderObject();
    if (context == null || boundary is! RenderRepaintBoundary) {
      return devFailure(devStatusFailed, 'there is no screen to capture');
    }
    final ratio = View.of(context).devicePixelRatio;
    final hasPlatformViews = platformViews(boundary);
    final ui.Image image = await boundary.toImage(pixelRatio: ratio);
    try {
      final data = await image.toByteData(format: ui.ImageByteFormat.rawRgba);
      if (data == null) {
        return devFailure(devStatusFailed, 'the screen could not be read');
      }
      final width = image.width;
      final height = image.height;
      final pixels = TransferableTypedData.fromList([
        data.buffer.asUint8List(data.offsetInBytes, data.lengthInBytes),
      ]);
      final png = await Isolate.run(
        () => encodePng(width, height, pixels.materialize().asUint8List()),
      );
      sink.emit({
        'screenshot': {
          'png': base64.encode(png),
          'width': width,
          'height': height,
          'pixelRatio': ratio,
          'platformViewsNotCaptured': hasPlatformViews,
        },
      });
      return {'status': devStatusOk};
    } finally {
      image.dispose();
    }
  }
}

/// Whether [root] or a render object below it is a platform view.
bool hasPlatformView(RenderObject root) {
  var found = false;
  void visit(RenderObject o) {
    if (found) return;
    if (o is PlatformViewRenderBox || o is RenderAndroidView) {
      found = true;
      return;
    }
    o.visitChildren(visit);
  }

  visit(root);
  return found;
}

/// The PNG of the 8-bit RGBA [pixels] of an image [width] by [height].
Uint8List encodePng(int width, int height, Uint8List pixels) {
  final stride = width * 4;
  final raw = BytesBuilder(copy: false);
  for (var y = 0; y < height; y++) {
    raw
      ..addByte(0)
      ..add(Uint8List.sublistView(pixels, y * stride, (y + 1) * stride));
  }
  final header = ByteData(13)
    ..setUint32(0, width)
    ..setUint32(4, height)
    ..setUint8(8, 8)
    ..setUint8(9, 6);
  final out = BytesBuilder()
    ..add(const [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  void chunk(String type, List<int> data) {
    final length = ByteData(4)..setUint32(0, data.length);
    final body = [...ascii.encode(type), ...data];
    final crc = ByteData(4)..setUint32(0, _crc32(body));
    out
      ..add(length.buffer.asUint8List())
      ..add(body)
      ..add(crc.buffer.asUint8List());
  }

  chunk('IHDR', header.buffer.asUint8List());
  chunk('IDAT', ZLibEncoder().convert(raw.takeBytes()));
  chunk('IEND', const []);
  return out.takeBytes();
}

int _crc32(List<int> bytes) {
  var crc = 0xffffffff;
  for (final b in bytes) {
    crc ^= b;
    for (var k = 0; k < 8; k++) {
      crc = (crc & 1) != 0 ? (crc >> 1) ^ 0xedb88320 : crc >> 1;
    }
  }
  return crc ^ 0xffffffff;
}
