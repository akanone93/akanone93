// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The offline engine: ZXing-C++ through `dart:ffi` (decision B21,
/// ADR-0051). `hook/build.dart` builds the library from the sources vendored
/// in `third_party/zxing-cpp`; `native/plux_zxing.cpp` is the whole native
/// surface.
///
/// The externals are private and called only from [ZxingDecoder], as in
/// `plux_flutter`'s native bindings.
library;

import 'dart:convert';
import 'dart:ffi';
import 'dart:isolate';

import 'package:flutter/foundation.dart';

/// One luminance frame: a byte per pixel, rows [stride] bytes apart.
@immutable
final class ScanFrame {
  /// Creates a frame; [stride] is [width] when absent.
  ScanFrame(
    this.bytes,
    this.width,
    this.height, {
    int? stride,
    this.rotation = 0,
  }) : stride = stride ?? width {
    if (width <= 0 || height <= 0 || this.stride < width) {
      throw ArgumentError('a frame needs a positive size and stride >= width');
    }
    if (rotation % 90 != 0) {
      throw ArgumentError('rotation is a multiple of 90 degrees');
    }
    if (bytes.length < this.stride * (height - 1) + width) {
      throw ArgumentError('the frame has fewer bytes than its size needs');
    }
  }

  /// The luminance bytes.
  final Uint8List bytes;

  /// The width in pixels.
  final int width;

  /// The height in pixels.
  final int height;

  /// The distance between the starts of two rows, in bytes.
  final int stride;

  /// The degrees clockwise the frame must be turned to be upright; the
  /// engine reads rotated codes, but a scan window is in upright terms.
  final int rotation;
}

/// A code the offline engine read.
@immutable
final class ZxingResult {
  /// Creates a result.
  const ZxingResult(this.value, this.format);

  /// The decoded text.
  final String value;

  /// The schema `BarcodeFormat` member, such as `qrCode`.
  final String format;

  @override
  bool operator ==(Object other) =>
      other is ZxingResult && other.value == value && other.format == format;

  @override
  int get hashCode => Object.hash(value, format);
}

/// The schema's `BarcodeFormat` members in permanent ID order; the ID of a
/// member is its index plus one, and bit (ID - 1) of a format mask selects it.
const List<String> scanFormatNames = [
  'qrCode',
  'aztec',
  'dataMatrix',
  'pdf417',
  'code39',
  'code93',
  'code128',
  'codabar',
  'ean8',
  'ean13',
  'itf',
  'upcA',
  'upcE',
];

/// The format mask of [names], schema `BarcodeFormat` members; zero (every
/// format) when [names] is empty. Names the schema does not know are ignored.
int formatMask(Iterable<String> names) {
  var mask = 0;
  for (final n in names) {
    final i = scanFormatNames.indexOf(n);
    if (i >= 0) mask |= 1 << i;
  }
  return mask;
}

@Native<
  Int32 Function(
    Pointer<Uint8>,
    Int32,
    Int32,
    Int32,
    Int32,
    Pointer<Uint8>,
    Int32,
  )
>(symbol: 'plux_zxing_decode', isLeaf: true)
external int _decode(
  Pointer<Uint8> y,
  int w,
  int h,
  int stride,
  int formats,
  Pointer<Uint8> out,
  int outLen,
);

@Native<Pointer<Uint8> Function(Int32)>(
  symbol: 'plux_zxing_alloc',
  isLeaf: true,
)
external Pointer<Uint8> _alloc(int bytes);

@Native<Void Function(Pointer<Uint8>)>(symbol: 'plux_zxing_free', isLeaf: true)
external void _free(Pointer<Uint8> p);

/// Reads codes from luminance frames with ZXing-C++.
final class ZxingDecoder {
  /// Creates a decoder. [maxTextBytes] bounds the text of a code.
  const ZxingDecoder({this.maxTextBytes = 8192});

  /// The most bytes of text one code may hold; a longer code is not read.
  final int maxTextBytes;

  /// Reads the first code in [frame] among [formats] (every format when
  /// zero, see [formatMask]); null when there is none. Runs on the calling
  /// isolate; use [decodeAsync] from the UI isolate.
  ZxingResult? decode(ScanFrame frame, {int formats = 0}) {
    final size = frame.stride * (frame.height - 1) + frame.width;
    final y = _alloc(size);
    final out = _alloc(maxTextBytes + 2);
    try {
      if (y == nullptr || out == nullptr) {
        throw StateError('the scanner could not allocate a frame buffer');
      }
      y.asTypedList(size).setRange(0, size, frame.bytes);
      final n = _decode(
        y,
        frame.width,
        frame.height,
        frame.stride,
        formats,
        out,
        maxTextBytes + 2,
      );
      if (n < 0) return null;
      final bytes = out.asTypedList(n + 1);
      final id = bytes[0];
      if (id < 1 || id > scanFormatNames.length) return null;
      return ZxingResult(
        utf8.decode(Uint8List.sublistView(bytes, 1), allowMalformed: true),
        scanFormatNames[id - 1],
      );
    } finally {
      if (y != nullptr) _free(y);
      if (out != nullptr) _free(out);
    }
  }

  /// [decode] on a short-lived isolate, so a frame never stalls the UI.
  Future<ZxingResult?> decodeAsync(ScanFrame frame, {int formats = 0}) =>
      Isolate.run(() => decode(frame, formats: formats));
}
