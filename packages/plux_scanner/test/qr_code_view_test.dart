// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// QrCodeView: the module matrix of `qr`, painted and read back by the
// offline engine.
//
// Verifies: RT-060.

import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
// The colour value of a bundle has no public constructor.
// ignore: implementation_imports
import 'package:plux_flutter/src/pxl/values.dart' show PxlColor;
import 'package:plux_scanner/plux_scanner.dart';
import 'package:qr/qr.dart';

import 'scanner_widgets_test.dart' show FakeNode;

const _url = 'https://plux.dev/pay?id=1234567890';

/// Paints [painter] on [side] pixels and returns the luminance frame.
Future<ScanFrame> _render(QrPainter painter, int side) async {
  final recorder = ui.PictureRecorder();
  painter.paint(ui.Canvas(recorder), ui.Size(side.toDouble(), side.toDouble()));
  final image = await recorder.endRecording().toImage(side, side);
  final rgba = (await image.toByteData())!.buffer.asUint8List();
  final luma = Uint8List(side * side);
  for (var i = 0; i < luma.length; i++) {
    luma[i] =
        (rgba[i * 4] * 299 + rgba[i * 4 + 1] * 587 + rgba[i * 4 + 2] * 114) ~/
        1000;
  }
  return ScanFrame(luma, side, side);
}

void main() {
  group('the module matrix', () {
    test('known data gives the known version and module count', () {
      // Version 1 is 21 modules; each version adds 4.
      expect(qrModules('HELLO', QrErrorCorrectLevel.medium).moduleCount, 21);
      expect(qrModules('HELLO', QrErrorCorrectLevel.medium).typeNumber, 1);
      // 34 bytes do not fit version 2 at level M (26 bytes), so version 3.
      final url = qrModules(_url, QrErrorCorrectLevel.medium);
      expect(url.typeNumber, 3);
      expect(url.moduleCount, 29);
    });

    test('a higher error correction level needs a larger code', () {
      final low = qrModules(_url, QrErrorCorrectLevel.low).moduleCount;
      final high = qrModules(_url, QrErrorCorrectLevel.high).moduleCount;
      expect(high, greaterThan(low));
    });

    test('finder patterns are in three corners and not in the fourth', () {
      final qr = qrModules('HELLO', QrErrorCorrectLevel.medium);
      final n = qr.moduleCount;
      for (final (r, c) in [(0, 0), (0, n - 7), (n - 7, 0)]) {
        // The 7x7 finder: dark ring, light ring, dark 3x3 centre.
        expect(qr.isDark(r, c), isTrue);
        expect(qr.isDark(r + 1, c + 1), isFalse);
        expect(qr.isDark(r + 3, c + 3), isTrue);
      }
    });

    test('data too long for any version is refused', () {
      expect(
        () => qrModules('x' * 4000, QrErrorCorrectLevel.high),
        throwsA(isA<InputTooLongException>()),
      );
    });
  });

  group('the painter', () {
    final image = qrModules('HELLO', QrErrorCorrectLevel.medium);
    final painter = QrPainter(
      image: image,
      foreground: Colors.black,
      background: Colors.white,
      quietZone: 4,
    );

    test('the quiet zone surrounds the modules', () {
      expect(painter.cells, 21 + 8);
      final bounds = painter.modulesPath(290).getBounds();
      // Cells are 10 pixels: the finder at module 0 starts after 4 cells.
      expect(bounds.left, 40);
      expect(bounds.top, 40);
      expect(bounds.right, 250);
      expect(bounds.bottom, 250);
    });

    test('a path holds exactly the dark modules', () {
      final path = painter.modulesPath(290);
      for (var r = 0; r < image.moduleCount; r++) {
        for (var c = 0; c < image.moduleCount; c++) {
          final centre = Offset((4 + c) * 10 + 5, (4 + r) * 10 + 5);
          expect(path.contains(centre), image.isDark(r, c), reason: '$r,$c');
        }
      }
    });

    test('it repaints for another matrix, colour or quiet zone', () {
      QrPainter with_({
        QrImage? i,
        Color fg = Colors.black,
        Color bg = Colors.white,
        int q = 4,
      }) => QrPainter(
        image: i ?? image,
        foreground: fg,
        background: bg,
        quietZone: q,
      );
      expect(painter.shouldRepaint(with_()), isFalse);
      expect(
        painter.shouldRepaint(
          with_(i: qrModules('HELLO', QrErrorCorrectLevel.low)),
        ),
        isTrue,
      );
      expect(painter.shouldRepaint(with_(fg: Colors.red)), isTrue);
      expect(painter.shouldRepaint(with_(bg: Colors.red)), isTrue);
      expect(painter.shouldRepaint(with_(q: 0)), isTrue);
    });

    testWidgets('what is painted is read back by the offline engine', (
      tester,
    ) async {
      final p = QrPainter(
        image: qrModules(_url, QrErrorCorrectLevel.medium),
        foreground: Colors.black,
        background: Colors.white,
        quietZone: 4,
      );
      final frame = (await tester.runAsync(() => _render(p, 290)))!;
      final got = const ZxingDecoder().decode(frame);
      expect(got, const ZxingResult(_url, 'qrCode'));
    });

    testWidgets('colours and the quiet zone reach the pixels', (tester) async {
      final p = QrPainter(
        image: qrModules('HELLO', QrErrorCorrectLevel.medium),
        foreground: const Color(0xFF000000),
        background: const Color(0xFFFFFFFF),
        quietZone: 2,
      );
      final frame = (await tester.runAsync(() => _render(p, 250)))!;
      // 25 cells of 10 pixels: the corner is quiet, the finder starts at 20.
      expect(frame.bytes[0], 255);
      expect(frame.bytes[25 * 250 + 25], 0);
    });
  });

  group('the builder', () {
    final package = PluxScannerWidgets();

    Future<void> pump(WidgetTester tester, Map<int, Object?> props) =>
        tester.pumpWidget(
          MaterialApp(
            home: Scaffold(
              body: Builder(
                builder: (context) =>
                    package.builders[PluxScannerIds.qrCodeView]!(
                      FakeNode(context, props),
                    ),
              ),
            ),
          ),
        );

    testWidgets('draws a square of the given size with the label', (
      tester,
    ) async {
      final handle = tester.ensureSemantics();
      await pump(tester, {1: _url, 2: 150.0, 7: 'Payment code'});
      final box = tester.getSize(find.byType(PluxQrCodeView));
      expect(box, const Size(150, 150));
      expect(find.bySemanticsLabel('Payment code'), findsOneWidget);
      // The label never holds the data (SEC-092).
      expect(find.bySemanticsLabel(RegExp('plux.dev')), findsNothing);
      final view = tester.widget<PluxQrCodeView>(find.byType(PluxQrCodeView));
      expect(view.image.moduleCount, 29);
      expect(view.quietZone, 4);
      expect(view.errorCorrection, QrErrorCorrectLevel.medium);
      handle.dispose();
    });

    testWidgets('error correction, colours and quiet zone come from props', (
      tester,
    ) async {
      await pump(tester, {
        1: _url,
        3: 'high',
        4: const PxlColor(0xff0000ff),
        5: const PxlColor(0x00ff00ff),
        6: 1,
        7: 'Code',
      });
      final view = tester.widget<PluxQrCodeView>(find.byType(PluxQrCodeView));
      expect(view.errorCorrection, QrErrorCorrectLevel.high);
      expect(view.foreground, const Color(0xFFFF0000));
      expect(view.background, const Color(0xFF00FF00));
      expect(view.quietZone, 1);
      // By permanent value ID too: 1 low, 2 medium, 3 quartile, 4 high.
      await pump(tester, {1: _url, 3: 3, 7: 'Code'});
      expect(
        tester
            .widget<PluxQrCodeView>(find.byType(PluxQrCodeView))
            .errorCorrection,
        QrErrorCorrectLevel.quartile,
      );
    });

    testWidgets('data and label are required', (tester) async {
      await pump(tester, {7: 'Code'});
      expect(tester.takeException(), isA<DecodeError>());
      await pump(tester, {1: _url});
      expect(tester.takeException(), isA<DecodeError>());
    });

    testWidgets('data that fits no QR code fails without echoing it', (
      tester,
    ) async {
      await pump(tester, {1: 'secret' * 1000, 3: 'high', 7: 'Code'});
      final error = tester.takeException();
      expect(error, isA<DecodeError>());
      expect('$error', isNot(contains('secret')));
    });
  });
}
