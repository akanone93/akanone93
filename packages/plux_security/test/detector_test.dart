// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_security/plux_security.dart';

const _channel = MethodChannel('dev.plux/security');

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  void answer(Future<Object?> Function(MethodCall call) handler) {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(_channel, handler);
  }

  tearDown(
    () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(_channel, null),
  );

  test(
    'a scan sends the expectations and reads the findings [SEC-070]',
    () async {
      MethodCall? seen;
      answer((call) async {
        seen = call;
        return [
          {
            'kind': 'hooking',
            'indicators': ['frida_port', 'frida_maps'],
          },
          {'kind': 'emulator'},
        ];
      });
      final found = await const PlatformRaspDetector().scan(
        PluxRaspExpectations(
          signingCertSha256: ['AA:BB'],
          trustedInstallers: ['com.android.vending'],
          iosBundleId: 'dev.example.app',
        ),
      );
      expect(seen!.method, 'scan');
      expect(seen!.arguments, {
        'signingCertSha256': ['aabb'],
        'trustedInstallers': ['com.android.vending'],
        'iosTeamId': null,
        'iosBundleId': 'dev.example.app',
      });
      expect(found.map((d) => d.kind), [
        PluxDetectionKind.hooking,
        PluxDetectionKind.emulator,
      ]);
      expect(found.first.indicators, ['frida_port', 'frida_maps']);
      expect(found.last.indicators, isEmpty);
    },
  );

  test('a kind this version does not know, and malformed entries, are '
      'ignored [SEC-070]', () async {
    answer(
      (_) async => [
        {'kind': 'time_travel'},
        'not a map',
        {
          'kind': 'root',
          'indicators': ['su_binary', 7],
        },
      ],
    );
    final found = await const PlatformRaspDetector().scan(
      PluxRaspExpectations(),
    );
    expect(found, hasLength(1));
    expect(found.single.indicators, ['su_binary']);
  });

  test('a platform without the checks finds nothing [SEC-070]', () async {
    final found = await const PlatformRaspDetector().scan(
      PluxRaspExpectations(),
    );
    expect(found, isEmpty);
  });

  test('an empty answer finds nothing [SEC-070]', () async {
    answer((_) async => null);
    expect(
      await const PlatformRaspDetector().scan(PluxRaspExpectations()),
      isEmpty,
    );
  });

  test('a failure of the platform reaches the caller, which contains it '
      '[SEC-070]', () async {
    answer((_) async => throw PlatformException(code: 'PLUX_PLATFORM'));
    expect(
      const PlatformRaspDetector().scan(PluxRaspExpectations()),
      throwsA(isA<PlatformException>()),
    );
  });

  test('the default trusted installers are the major stores [SEC-070]', () {
    expect(
      PluxRaspExpectations().trustedInstallers,
      contains('com.android.vending'),
    );
    expect(PluxRaspExpectations().signingCertSha256, isEmpty);
  });
}
