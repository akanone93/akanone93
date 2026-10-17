// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The offline engine through FFI on the host (decision B21, ADR-0051). The
// measurement draws every code of `support/code_images.dart` in every
// variant and prints one line per variant, the numbers the ADR records.
//
// Verifies: RT-060.

import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_scanner/plux_scanner.dart';

import 'support/code_images.dart';

void main() {
  const decoder = ZxingDecoder();

  group('decoding', () {
    for (final code in testCodes) {
      test('${code.name} is read back exactly', () {
        final got = decoder.decode(code.draw(4).frame);
        expect(got, ZxingResult(code.value, code.format));
      });
    }

    test('an empty image holds no code', () {
      expect(decoder.decode(GrayImage(200, 200).frame), isNull);
    });

    test('a format mask limits what is read', () {
      final qr = testCodes.first.draw(4).frame;
      expect(decoder.decode(qr, formats: formatMask(['ean13'])), isNull);
      expect(
        decoder.decode(qr, formats: formatMask(['ean13', 'qrCode']))?.format,
        'qrCode',
      );
      expect(formatMask(const []), 0);
      expect(formatMask(['qrCode', 'upcE']), 1 | (1 << 12));
      expect(formatMask(['noSuchFormat']), 0);
    });

    test('a row stride wider than the image is honoured', () {
      final image = testCodes.first.draw(4);
      final wide = Uint8List((image.width + 7) * image.height)
        ..fillRange(0, (image.width + 7) * image.height, 255);
      for (var y = 0; y < image.height; y++) {
        wide.setRange(
          y * (image.width + 7),
          y * (image.width + 7) + image.width,
          image.pixels,
          y * image.width,
        );
      }
      final frame = ScanFrame(
        wide,
        image.width,
        image.height,
        stride: image.width + 7,
      );
      expect(decoder.decode(frame)?.value, testCodes.first.value);
    });

    test('a text longer than the buffer is not read', () {
      expect(
        const ZxingDecoder(maxTextBytes: 4)
            .decode(testCodes.first.draw(4).frame),
        isNull,
      );
    });

    test('a frame must match its size', () {
      expect(() => ScanFrame(Uint8List(10), 4, 4), throwsArgumentError);
      expect(
        () => ScanFrame(Uint8List(16), 4, 4, stride: 3),
        throwsArgumentError,
      );
      expect(() => ScanFrame(Uint8List(16), 0, 4), throwsArgumentError);
    });

    test('decodeAsync reads on another isolate', () async {
      final got = await decoder.decodeAsync(testCodes.last.draw(4).frame);
      expect(got, ZxingResult(testCodes.last.value, testCodes.last.format));
    });

    test('the frame the scan window selects is read', () {
      final image = GrayImage(300, 300);
      final qr = testCodes[2].draw(3);
      for (var y = 0; y < qr.height; y++) {
        image.pixels.setRange(
          (y + 20) * 300 + 150,
          (y + 20) * 300 + 150 + qr.width,
          qr.pixels,
          y * qr.width,
        );
      }
      final inside = cropFrame(image.frame, [0.4, 0.0, 0.6, 0.6]);
      expect(inside.width, 180);
      expect(decoder.decode(inside)?.value, 'hi');
      final elsewhere = cropFrame(image.frame, [0.0, 0.5, 0.4, 0.5]);
      expect(decoder.decode(elsewhere), isNull);
      final whole = image.frame;
      expect(identical(cropFrame(whole, [0, 0, 1, 1]), whole), isTrue);
    });
  });

  test('measurement: both-engine test set through ZXing-C++', () {
    final rows = <String>[];
    var all = 0, read = 0;
    final byVariant = <String, (int, int)>{};
    final clock = Stopwatch();
    for (final variant in variants) {
      var ok = 0;
      final failed = <String>[];
      for (final code in testCodes) {
        final frame = variant.make(code).frame;
        clock.start();
        final got = decoder.decode(frame);
        clock.stop();
        if (got == ZxingResult(code.value, code.format)) {
          ok++;
        } else {
          failed.add(code.name);
        }
      }
      byVariant[variant.name] = (ok, testCodes.length);
      all += testCodes.length;
      read += ok;
      rows.add(
        '${variant.name.padRight(13)} $ok/${testCodes.length}'
        '${failed.isEmpty ? '' : '  missed: ${failed.join(', ')}'}',
      );
    }
    final ms = clock.elapsedMicroseconds / 1000 / all;
    // The numbers the ADR records.
    // ignore: avoid_print
    print(
      'ZXing-C++ 2.3.0 on ${variants.length} variants of ${testCodes.length} '
      'codes (host, -Os):\n${rows.join('\n')}\n'
      'total $read/$all, ${ms.toStringAsFixed(1)} ms per frame on average',
    );
    expect(byVariant['clean'], (testCodes.length, testCodes.length));
    expect(byVariant['small']!.$1, greaterThanOrEqualTo(testCodes.length - 1));
    expect(byVariant['bad-light']!.$1, testCodes.length);
    expect(read / all, greaterThanOrEqualTo(0.85));
  });
}
