// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package: builders for `QrScanner`, `BarcodeScanner` and
/// `QrCodeView`. The IDs are those of the descriptors in
/// `schema/widgets/layer2`; a test compares them.
library;

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_scanner/src/engine.dart';
import 'package:plux_scanner/src/qr_view.dart';
import 'package:plux_scanner/src/scan_view.dart';
import 'package:plux_scanner/src/zxing_ffi.dart';
import 'package:qr/qr.dart' show InputTooLongException, QrErrorCorrectLevel;

/// The widget IDs of the package's descriptors.
abstract final class PluxScannerIds {
  /// `QrScanner`.
  static const int qrScanner = 199;

  /// `BarcodeScanner`.
  static const int barcodeScanner = 200;

  /// `QrCodeView`.
  static const int qrCodeView = 201;
}

/// The scanner and QR widgets of Plux pages, to register in
/// `PluxConfig.widgetPackages`.
final class PluxScannerWidgets implements PluxWidgetPackage {
  /// Creates the package; [config] says which engine reads the camera.
  const PluxScannerWidgets({this.config = const PluxScannerConfig()});

  /// How the scanners of this host behave.
  final PluxScannerConfig config;

  @override
  String get name => 'plux_scanner';

  @override
  Map<int, NodeBuilder> get builders => {
    PluxScannerIds.qrScanner: (c) => _scanner(c, config, barcode: false),
    PluxScannerIds.barcodeScanner: (c) => _scanner(c, config, barcode: true),
    PluxScannerIds.qrCodeView: _qrCodeView,
  };
}

/// The member of an enum prop, from its permanent value ID or its name.
String? _member(List<String> members, Object? v) => switch (v) {
  final int i when i >= 1 && i <= members.length => members[i - 1],
  final String s when members.contains(s) => s,
  _ => null,
};

String? _facing(Decoding d, Object? v) => _member(const ['back', 'front'], v);

String? _level(Decoding d, Object? v) =>
    _member(const ['low', 'medium', 'quartile', 'high'], v);

/// A `BarcodeFormat` member from its permanent value ID or its name.
String? _format(Decoding d, Object? v) => _member(scanFormatNames, v);

List<double>? _window(Decoding d, Object? v) {
  final w = asDoubles(d, v);
  if (w == null || w.length != 4) return null;
  // Fractions of the preview; the window must have an area.
  final l = w[0], t = w[1], ww = w[2], hh = w[3];
  if (l < 0 || t < 0 || ww <= 0 || hh <= 0 || l + ww > 1 || t + hh > 1) {
    return null;
  }
  return w;
}

Widget _scanner(
  NodeContext c,
  PluxScannerConfig config, {
  required bool barcode,
}) {
  // `BarcodeScanner` has `formats` first, so its other props shift by one.
  final o = barcode ? 1 : 0;
  final formats = barcode
      ? (c.decode(1, (d, v) => asList(d, v, _format)) ?? const <String>[])
      : const ['qrCode'];
  return PluxScannerView(
    config: config,
    formats: formats,
    torch: c.decode(1 + o, asBool) ?? false,
    backCamera: (c.decode(2 + o, _facing) ?? 'back') == 'back',
    scanWindow: c.decode(3 + o, _window),
    continuous: c.decode(4 + o, asBool) ?? false,
    overlayHint: c.decode(5 + o, asString),
    semanticLabel: c.decode(6 + o, asString),
    onDetected: c.handles(1)
        ? (hit) => c.fire(1, {'value': hit.value, 'format': hit.format})
        : null,
    onError: c.handles(2) ? (code) => c.fire(2, code) : null,
  );
}

Widget _qrCodeView(NodeContext c) {
  final data = c.decode(1, asString) ?? c.missing('QrCodeView.data');
  final label = c.decode(7, asString) ?? c.missing('QrCodeView.semanticLabel');
  final level = switch (c.decode(3, _level)) {
    'low' => QrErrorCorrectLevel.low,
    'quartile' => QrErrorCorrectLevel.quartile,
    'high' => QrErrorCorrectLevel.high,
    _ => QrErrorCorrectLevel.medium,
  };
  final size = c.decode(2, asDouble) ?? 200;
  try {
    return PluxQrCodeView(
      data: data,
      semanticLabel: label,
      size: size < 16 ? 16 : size,
      errorCorrection: level,
      foreground: c.decode(4, asColor) ?? const Color(0xFF000000),
      background: c.decode(5, asColor) ?? const Color(0xFFFFFFFF),
      quietZone: (c.decode(6, asInt) ?? 4).clamp(0, 16),
    );
  } on InputTooLongException {
    // The message holds no data (SEC-092).
    throw const DecodeError('QrCodeView.data is too long for a QR code');
  }
}
