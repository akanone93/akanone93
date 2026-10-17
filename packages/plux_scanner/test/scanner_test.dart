// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mobile_scanner/mobile_scanner.dart' as ms;
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_scanner/plux_scanner.dart';
import 'package:plux_scanner/src/scanner.dart' show formatName, scannerFormats;

import 'scanner_widgets_test.dart' show FakeProbe, FakeSession;

void main() {
  final key = GlobalKey<NavigatorState>();
  late ScanSink sink;
  late List<String> asked;

  Future<PluxCodeScanner> pump(WidgetTester tester) async {
    asked = [];
    await tester.pumpWidget(
      MaterialApp(navigatorKey: key, home: const Scaffold()),
    );
    final scanner = PluxScanner(
      navigatorKey: key,
      pageBuilder: (context, formats, s) {
        asked = formats;
        sink = s;
        return const Scaffold(body: Text('camera'));
      },
    );
    return scanner.services[PluxCodeScanner]! as PluxCodeScanner;
  }

  test(
    'the package registers itself as plux_scanner and provides the scanner',
    () {
      final scanner = PluxScanner(navigatorKey: key);
      expect(scanner, isA<PluxDevicePackage>());
      expect(scanner.name, 'plux_scanner');
      expect(scanner.services.keys, [PluxCodeScanner]);
    },
  );

  testWidgets('a code that is read closes the page and is returned', (
    tester,
  ) async {
    final scanner = await pump(tester);
    final result = scanner.scan(['qrCode', 'ean13']);
    await tester.pumpAndSettle();
    expect(find.text('camera'), findsOneWidget);
    expect(asked, ['qrCode', 'ean13']);
    sink.found(const PluxScanResult(value: 'hello', format: 'qrCode'));
    await tester.pumpAndSettle();
    final got = await result;
    expect(got?.value, 'hello');
    expect(got?.format, 'qrCode');
    expect(find.text('camera'), findsNothing);
  });

  testWidgets('cancelling, or leaving the page, returns null', (tester) async {
    final scanner = await pump(tester);
    var result = scanner.scan(const []);
    await tester.pumpAndSettle();
    sink.cancel();
    await tester.pumpAndSettle();
    expect(await result, isNull);

    result = scanner.scan(const []);
    await tester.pumpAndSettle();
    key.currentState!.pop();
    await tester.pumpAndSettle();
    expect(await result, isNull);
  });

  testWidgets('a failure is typed, and the first report wins', (tester) async {
    final scanner = await pump(tester);
    final result = scanner.scan(const []);
    final typed = expectLater(
      result,
      throwsA(
        isA<PluxDeviceException>().having(
          (e) => e.failure,
          'failure',
          PluxDeviceFailure.denied,
        ),
      ),
    );
    await tester.pumpAndSettle();
    sink.fail(const PluxDeviceException.denied('camera denied'));
    sink.found(const PluxScanResult(value: 'late', format: 'qrCode'));
    await tester.pumpAndSettle();
    await typed;
  });

  testWidgets('without a navigator the scan is unavailable', (tester) async {
    final scanner =
        PluxScanner(navigatorKey: GlobalKey<NavigatorState>())
                .services[PluxCodeScanner]!
            as PluxCodeScanner;
    await expectLater(
      scanner.scan(const []),
      throwsA(isA<PluxDeviceException>()),
    );
  });

  test('schema formats map to the scanner formats and back', () {
    expect(scannerFormats(['qrCode', 'itf']), [
      ms.BarcodeFormat.qrCode,
      ms.BarcodeFormat.itf14,
      ms.BarcodeFormat.itf2of5,
    ]);
    expect(scannerFormats(const []), isEmpty);
    expect(formatName(ms.BarcodeFormat.ean13), 'ean13');
    expect(formatName(ms.BarcodeFormat.itf14), 'itf');
    expect(formatName(ms.BarcodeFormat.maxiCode), isNull);
  });

  group('the camera page runs on the shared scanner view', () {
    late FakeSession session;
    late PluxCodeScanner scanner;

    Future<void> open(WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(navigatorKey: key, home: const Scaffold()),
      );
      scanner =
          PluxScanner(
                navigatorKey: key,
                config: PluxScannerConfig(
                  engine: ScannerEngine.platform,
                  probe: FakeProbe(MlKitState.ready),
                  sessionFactory: (request, engine) =>
                      session = FakeSession(request, engine),
                ),
              ).services[PluxCodeScanner]!
              as PluxCodeScanner;
    }

    testWidgets('a code that is read is returned', (tester) async {
      await open(tester);
      final result = scanner.scan(['ean13']);
      await tester.pumpAndSettle();
      await tester.pump();
      expect(session.request.formats, ['ean13']);
      session.hitController.add(const ScanHit('5901234123457', 'ean13'));
      await tester.pumpAndSettle();
      final got = await result;
      expect(got?.value, '5901234123457');
      expect(got?.format, 'ean13');
    });

    testWidgets('a denied camera is a typed denial', (tester) async {
      await open(tester);
      final result = scanner.scan(const []);
      final typed = expectLater(
        result,
        throwsA(
          isA<PluxDeviceException>().having(
            (e) => e.failure,
            'failure',
            PluxDeviceFailure.denied,
          ),
        ),
      );
      await tester.pumpAndSettle();
      await tester.pump();
      session.errorController.add(ScanErrorCodes.permissionDenied);
      await tester.pumpAndSettle();
      await typed;
    });

    testWidgets('any other failure is unavailable and holds only the code', (
      tester,
    ) async {
      await open(tester);
      final result = scanner.scan(const []);
      final typed = expectLater(
        result,
        throwsA(
          isA<PluxDeviceException>()
              .having(
                (e) => e.failure,
                'failure',
                PluxDeviceFailure.unavailable,
              )
              .having((e) => e.message, 'message', contains('failed')),
        ),
      );
      await tester.pumpAndSettle();
      await tester.pump();
      session.errorController.add(ScanErrorCodes.failed);
      await tester.pumpAndSettle();
      await typed;
    });
  });
}
