// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `QrCodeView`: a QR code painted from the module matrix of `qr`, offline.
library;

import 'package:flutter/material.dart';
import 'package:qr/qr.dart';

/// The modules of a QR code for [data] at [level]: the smallest version that
/// holds the data. Throws [InputTooLongException] when no version does.
QrImage qrModules(String data, QrErrorCorrectLevel level) => QrImage(
  QrCode(payload: QrPayload.fromString(data), errorCorrectLevel: level),
);

/// A QR code for [data], drawn as one path of squares.
final class PluxQrCodeView extends StatelessWidget {
  /// Creates the view. The code is [size] logical pixels square including
  /// [quietZone] modules of [background] on every side.
  PluxQrCodeView({
    required String data,
    required this.semanticLabel,
    this.size = 200,
    this.errorCorrection = QrErrorCorrectLevel.medium,
    this.foreground = const Color(0xFF000000),
    this.background = const Color(0xFFFFFFFF),
    this.quietZone = 4,
    super.key,
  }) : image = qrModules(data, errorCorrection);

  /// The modules.
  final QrImage image;

  /// What a screen reader announces; never the data.
  final String semanticLabel;

  /// The side of the square.
  final double size;

  /// The error correction level the code was made at.
  final QrErrorCorrectLevel errorCorrection;

  /// The colour of the modules.
  final Color foreground;

  /// The colour behind the modules.
  final Color background;

  /// The empty border, in modules.
  final int quietZone;

  @override
  Widget build(BuildContext context) => Semantics(
    image: true,
    label: semanticLabel,
    child: ExcludeSemantics(
      child: SizedBox.square(
        dimension: size,
        child: CustomPaint(
          painter: QrPainter(
            image: image,
            foreground: foreground,
            background: background,
            quietZone: quietZone,
          ),
        ),
      ),
    ),
  );
}

/// Paints a [QrImage] to fill the canvas.
final class QrPainter extends CustomPainter {
  /// Creates the painter.
  const QrPainter({
    required this.image,
    required this.foreground,
    required this.background,
    required this.quietZone,
  });

  /// The modules.
  final QrImage image;

  /// The colour of the modules.
  final Color foreground;

  /// The colour behind the modules.
  final Color background;

  /// The empty border, in modules.
  final int quietZone;

  /// The modules plus the quiet zone on both sides, per side.
  int get cells => image.moduleCount + 2 * quietZone;

  /// The dark modules as rectangles in a canvas of [side] logical pixels.
  Path modulesPath(double side) {
    final cell = side / cells;
    final path = Path();
    for (var row = 0; row < image.moduleCount; row++) {
      var col = 0;
      while (col < image.moduleCount) {
        if (!image.isDark(row, col)) {
          col++;
          continue;
        }
        // A run of dark modules is one rectangle: fewer path segments.
        final start = col;
        while (col < image.moduleCount && image.isDark(row, col)) {
          col++;
        }
        path.addRect(
          Rect.fromLTWH(
            (quietZone + start) * cell,
            (quietZone + row) * cell,
            (col - start) * cell,
            cell,
          ),
        );
      }
    }
    return path;
  }

  @override
  void paint(Canvas canvas, Size size) {
    final side = size.shortestSide;
    canvas
      ..drawRect(Offset.zero & size, Paint()..color = background)
      ..drawPath(
        modulesPath(side),
        Paint()
          ..color = foreground
          ..isAntiAlias = false,
      );
  }

  @override
  bool shouldRepaint(QrPainter old) =>
      old.image != image ||
      old.foreground != foreground ||
      old.background != background ||
      old.quietZone != quietZone;
}
