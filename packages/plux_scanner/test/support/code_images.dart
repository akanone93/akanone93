// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Deterministic test images of codes, and the damage a camera does to them:
/// the one set both engines are measured on (decision B21, ADR-0051). QR
/// codes come from `qr`; EAN-13 and Code 128 from the small encoders below.
/// Nothing here uses the clock or a random source.
library;

import 'dart:math' as math;
import 'dart:typed_data';

import 'package:plux_scanner/plux_scanner.dart';
import 'package:qr/qr.dart';

/// A grey image, a byte per pixel, 255 white.
final class GrayImage {
  /// Creates a white image.
  GrayImage(this.width, this.height)
    : pixels = Uint8List(width * height)..fillRange(0, width * height, 255);

  /// The width in pixels.
  final int width;

  /// The height in pixels.
  final int height;

  /// The pixels, row by row.
  final Uint8List pixels;

  /// The pixel at [x], [y].
  int at(int x, int y) => pixels[y * width + x];

  /// Sets the pixel at [x], [y] to [v], limited to 0 to 255.
  void set(int x, int y, int v) => pixels[y * width + x] = v.clamp(0, 255);

  /// A copy.
  GrayImage copy() => GrayImage(width, height)..pixels.setAll(0, pixels);

  /// As a frame for the decoder.
  ScanFrame get frame => ScanFrame(pixels, width, height);
}

/// A code to draw: what it holds and the format it must be read as.
final class TestCode {
  /// Creates a test code.
  const TestCode(this.name, this.value, this.format, this.draw);

  /// A name for reports.
  final String name;

  /// The text the code holds, as it must be read back.
  final String value;

  /// The schema `BarcodeFormat` member.
  final String format;

  /// Draws the code with the given pixels per module on a white canvas.
  final GrayImage Function(int scale) draw;
}

const int _margin = 12;

void _fill(GrayImage image, int x, int y, int w, int h, [int v = 0]) {
  for (var j = y; j < y + h; j++) {
    for (var i = x; i < x + w; i++) {
      image.set(i, j, v);
    }
  }
}

/// A QR code of [data] at [level] on a white canvas.
GrayImage qrImage(
  String data,
  QrErrorCorrectLevel level,
  int scale, {
  int quiet = 4,
}) {
  final qr = QrImage(
    QrCode(payload: QrPayload.fromString(data), errorCorrectLevel: level),
  );
  final cells = qr.moduleCount + 2 * quiet;
  final side = cells * scale + 2 * _margin;
  final image = GrayImage(side, side);
  for (var r = 0; r < qr.moduleCount; r++) {
    for (var c = 0; c < qr.moduleCount; c++) {
      if (!qr.isDark(r, c)) continue;
      _fill(
        image,
        _margin + (quiet + c) * scale,
        _margin + (quiet + r) * scale,
        scale,
        scale,
      );
    }
  }
  return image;
}

/// Bars from a string of `1` (dark) and `0` modules, with a quiet zone of
/// [quiet] modules on both sides.
GrayImage _bars(String modules, int scale, {int height = 30, int quiet = 10}) {
  final w = (modules.length + 2 * quiet) * scale + 2 * _margin;
  final image = GrayImage(w, height * scale + 2 * _margin);
  for (var i = 0; i < modules.length; i++) {
    if (modules[i] != '1') continue;
    _fill(
      image,
      _margin + (quiet + i) * scale,
      _margin,
      scale,
      image.height - 2 * _margin,
    );
  }
  return image;
}

const _eanL = [
  '0001101',
  '0011001',
  '0010011',
  '0111101',
  '0100011',
  '0110001',
  '0101111',
  '0111011',
  '0110111',
  '0001011',
];
const _eanG = [
  '0100111',
  '0110011',
  '0011011',
  '0100001',
  '0011101',
  '0111001',
  '0000101',
  '0010001',
  '0001001',
  '0010111',
];
const _eanParity = [
  'LLLLLL',
  'LLGLGG',
  'LLGGLG',
  'LLGGGL',
  'LGLLGG',
  'LGGLLG',
  'LGGGLL',
  'LGLGLG',
  'LGLGGL',
  'LGGLGL',
];

/// The EAN-13 digits of [digits12], twelve digits, with the check digit.
String ean13Digits(String digits12) {
  var sum = 0;
  for (var i = 0; i < 12; i++) {
    sum += int.parse(digits12[i]) * (i.isEven ? 1 : 3);
  }
  return '$digits12${(10 - sum % 10) % 10}';
}

/// An EAN-13 symbol of [digits12] on a white canvas.
GrayImage ean13Image(String digits12, int scale) {
  final d = ean13Digits(digits12);
  final parity = _eanParity[int.parse(d[0])];
  final b = StringBuffer('101');
  for (var i = 0; i < 6; i++) {
    final n = int.parse(d[1 + i]);
    b.write(parity[i] == 'L' ? _eanL[n] : _eanG[n]);
  }
  b.write('01010');
  for (var i = 0; i < 6; i++) {
    final left = _eanL[int.parse(d[7 + i])];
    b.write(left.split('').map((c) => c == '1' ? '0' : '1').join());
  }
  b.write('101');
  return _bars(b.toString(), scale);
}

/// The element widths of Code 128 symbol characters 0 to 105 and of the
/// stop pattern, as ZXing-C++ lists them (ODCode128Patterns.cpp).
const _code128 = [
  '212222',
  '222122',
  '222221',
  '121223',
  '121322',
  '131222',
  '122213',
  '122312',
  '132212',
  '221213',
  '221312',
  '231212',
  '112232',
  '122132',
  '122231',
  '113222',
  '123122',
  '123221',
  '223211',
  '221132',
  '221231',
  '213212',
  '223112',
  '312131',
  '311222',
  '321122',
  '321221',
  '312212',
  '322112',
  '322211',
  '212123',
  '212321',
  '232121',
  '111323',
  '131123',
  '131321',
  '112313',
  '132113',
  '132311',
  '211313',
  '231113',
  '231311',
  '112133',
  '112331',
  '132131',
  '113123',
  '113321',
  '133121',
  '313121',
  '211331',
  '231131',
  '213113',
  '213311',
  '213131',
  '311123',
  '311321',
  '331121',
  '312113',
  '312311',
  '332111',
  '314111',
  '221411',
  '431111',
  '111224',
  '111422',
  '121124',
  '121421',
  '141122',
  '141221',
  '112214',
  '112412',
  '122114',
  '122411',
  '142112',
  '142211',
  '241211',
  '221114',
  '413111',
  '241112',
  '134111',
  '111242',
  '121142',
  '121241',
  '114212',
  '124112',
  '124211',
  '411212',
  '421112',
  '421211',
  '212141',
  '214121',
  '412121',
  '111143',
  '111341',
  '131141',
  '114113',
  '114311',
  '411113',
  '411311',
  '113141',
  '114131',
  '311141',
  '411131',
  '211412',
  '211214',
  '211232',
  '233111',
];

/// A Code 128 symbol of [text] in code set B (printable ASCII) on a white
/// canvas.
GrayImage code128Image(String text, int scale) {
  final values = <int>[104, for (final u in text.codeUnits) u - 32];
  var check = 104;
  for (var i = 1; i < values.length; i++) {
    check += values[i] * i;
  }
  values
    ..add(check % 103)
    ..add(106);
  final b = StringBuffer();
  for (final v in values) {
    final pattern = _code128[v];
    for (var i = 0; i < pattern.length; i++) {
      b.write((i.isEven ? '1' : '0') * int.parse(pattern[i]));
    }
  }
  // The stop pattern ends with a bar of two modules.
  b.write('11');
  return _bars(b.toString(), scale);
}

/// The codes of the measurement.
final List<TestCode> testCodes = [
  TestCode(
    'qr-url-M',
    'https://plux.dev/pay?id=1234567890',
    'qrCode',
    (s) => qrImage(
      'https://plux.dev/pay?id=1234567890',
      QrErrorCorrectLevel.medium,
      s,
    ),
  ),
  TestCode(
    'qr-ticket-H',
    'PLUX-TICKET-0042-GATE-7',
    'qrCode',
    (s) => qrImage('PLUX-TICKET-0042-GATE-7', QrErrorCorrectLevel.high, s),
  ),
  TestCode(
    'qr-short-L',
    'hi',
    'qrCode',
    (s) => qrImage('hi', QrErrorCorrectLevel.low, s),
  ),
  TestCode(
    'ean13',
    ean13Digits('590123412345'),
    'ean13',
    (s) => ean13Image('590123412345', s),
  ),
  TestCode(
    'code128',
    'PLUX-1234',
    'code128',
    (s) => code128Image('PLUX-1234', s),
  ),
];

/// A deterministic sequence of bytes (a linear congruential generator).
Iterator<int> _noise(int seed) {
  Iterable<int> gen() sync* {
    var x = seed;
    while (true) {
      x = (x * 1103515245 + 12345) & 0x7fffffff;
      yield (x >> 16) & 0xff;
    }
  }

  return gen().iterator;
}

/// A box blur of [radius] pixels, [passes] times.
GrayImage blur(GrayImage src, {int radius = 1, int passes = 1}) {
  var cur = src;
  for (var p = 0; p < passes; p++) {
    final out = GrayImage(cur.width, cur.height);
    for (var y = 0; y < cur.height; y++) {
      for (var x = 0; x < cur.width; x++) {
        var sum = 0, n = 0;
        final j1 = math.min(cur.height - 1, y + radius);
        final i1 = math.min(cur.width - 1, x + radius);
        for (var j = math.max(0, y - radius); j <= j1; j++) {
          for (var i = math.max(0, x - radius); i <= i1; i++) {
            sum += cur.at(i, j);
            n++;
          }
        }
        out.set(x, y, sum ~/ n);
      }
    }
    cur = out;
  }
  return cur;
}

/// Squeezes the grey range into [low], [high].
GrayImage lowContrast(GrayImage src, {int low = 90, int high = 150}) {
  final out = src.copy();
  for (var i = 0; i < out.pixels.length; i++) {
    out.pixels[i] = low + out.pixels[i] * (high - low) ~/ 255;
  }
  return out;
}

/// Light that falls off from [from] per cent on the left to [to] on the
/// right: a dim room with one lamp.
GrayImage badLight(GrayImage src, {int from = 100, int to = 60}) {
  final out = src.copy();
  for (var y = 0; y < out.height; y++) {
    for (var x = 0; x < out.width; x++) {
      final pct = from + (to - from) * x ~/ (out.width - 1);
      out.set(x, y, out.at(x, y) * pct ~/ 100);
    }
  }
  return out;
}

/// Adds grain of up to +-[amplitude].
GrayImage grain(GrayImage src, {int amplitude = 40, int seed = 7}) {
  final out = src.copy();
  final it = _noise(seed);
  for (var i = 0; i < out.pixels.length; i++) {
    it.moveNext();
    final delta = it.current * 2 * amplitude ~/ 255 - amplitude;
    out.pixels[i] = (out.pixels[i] + delta).clamp(0, 255);
  }
  return out;
}

/// Wipes out parts of the code: a band across the middle third of its width
/// ([bandPct] per cent of its height, so a one-dimensional code keeps rows
/// it can be read from) and eight grey dots, as a scratch and dirt do.
GrayImage damage(GrayImage src, {int bandPct = 12, int seed = 3}) {
  final out = src.copy();
  final top = out.height * (50 - bandPct ~/ 2) ~/ 100;
  final bottom = out.height * (50 + bandPct ~/ 2) ~/ 100;
  for (var y = top; y < bottom; y++) {
    for (var x = out.width ~/ 3; x < out.width * 2 ~/ 3; x++) {
      out.set(x, y, 255);
    }
  }
  final it = _noise(seed);
  for (var k = 0; k < 8; k++) {
    it.moveNext();
    final x = it.current * (out.width - 3) ~/ 255;
    it.moveNext();
    final y = it.current * (out.height - 3) ~/ 255;
    _fill(out, x, y, 3, 3, 128);
  }
  return out;
}

/// A variant of a code image: a name and how to make it from a code.
final class Variant {
  /// Creates a variant.
  const Variant(this.name, this.make);

  /// A name for reports.
  final String name;

  /// Draws a code with the variant's scale and damage.
  final GrayImage Function(TestCode code) make;
}

/// The variants of the measurement: clean, small, blurred, damaged, in bad
/// and harsh light, low contrast and grainy.
final List<Variant> variants = [
  Variant('clean', (c) => c.draw(4)),
  Variant('small', (c) => c.draw(2)),
  Variant('blurred', (c) => blur(c.draw(4), passes: 2)),
  Variant('damaged', (c) => damage(c.draw(4))),
  Variant('bad-light', (c) => badLight(c.draw(4))),
  Variant('harsh-light', (c) => badLight(c.draw(4), to: 35)),
  Variant('low-contrast', (c) => lowContrast(c.draw(4))),
  Variant('grainy', (c) => grain(c.draw(4))),
];
