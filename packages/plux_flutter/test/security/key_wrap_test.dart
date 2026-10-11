// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:math';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/security/key_wrap.dart';
import 'package:plux_flutter/src/store/kv_store.dart';

import 'package:plux_flutter/src/sync/sync_engine.dart' show MemorySecretStore;

import '../support/fake_hardware.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final keyText = base64Encode(List.generate(32, (i) => 255 - i));
  late MemorySecretStore inner;
  late FakeHardware hardware;
  late HardwareWrappedSecretStore secrets;

  setUp(() {
    inner = MemorySecretStore();
    hardware = FakeHardware();
    secrets = HardwareWrappedSecretStore(inner, hardware);
  });

  group('HardwareWrappedSecretStore', () {
    test('keeps only the wrapped form, and unwraps a value once per '
        'launch [SEC-073]', () async {
      await secrets.write('data-key.a', keyText);
      final stored = inner.values['data-key.a']!;
      expect(stored, startsWith(HardwareWrappedSecretStore.prefix));
      expect(stored, isNot(contains(keyText)));

      // A new launch: nothing cached.
      final next = HardwareWrappedSecretStore(inner, hardware);
      expect(await next.read('data-key.a'), keyText);
      expect(await next.read('data-key.a'), keyText);
      expect(await next.read('data-key.a'), keyText);
      expect(hardware.unwraps, 1);
      expect(await next.read('absent'), isNull);
    });

    test('the name is bound: a blob copied under another name does not '
        'unwrap [SEC-073]', () async {
      await secrets.write('data-key.a', 'value');
      inner.values['data-key.b'] = inner.values['data-key.a']!;
      final fresh = HardwareWrappedSecretStore(inner, hardware);
      expect(await fresh.read('data-key.b'), isNull);
      expect(inner.values.containsKey('data-key.b'), isFalse);
      expect(await fresh.read('data-key.a'), 'value');
    });

    test('a key the hardware lost reads as absent and is removed, so the '
        'caller makes a new one [SEC-073]', () async {
      await secrets.write('k', 'value');
      hardware.lost = true;
      final fresh = HardwareWrappedSecretStore(inner, hardware);
      expect(await fresh.read('k'), isNull);
      expect(inner.values, isEmpty);
    });

    test('a transient platform failure is an error, and the wrapped value '
        'is kept [SEC-073]', () async {
      await secrets.write('k', 'value');
      hardware.failing = true;
      final fresh = HardwareWrappedSecretStore(inner, hardware);
      await expectLater(fresh.read('k'), throwsA(isA<StoreException>()));
      expect(inner.values, contains('k'));
      hardware.failing = false;
      expect(await fresh.read('k'), 'value');
    });

    test('a value written before the stores were encrypted is wrapped in '
        'place on its first read [SEC-073]', () async {
      final key = base64Encode(List.generate(32, (i) => i));
      inner.values['data-key.a'] = key;
      expect(await secrets.read('data-key.a'), key);
      expect(inner.values['data-key.a'], startsWith('hw1.'));
      expect(hardware.wraps, 1);
      final fresh = HardwareWrappedSecretStore(inner, hardware);
      expect(await fresh.read('data-key.a'), key);
    });

    test('a blob that is not base64, or another hardware key wrapped, '
        'reads as absent [SEC-073]', () async {
      inner.values['a'] = 'hw1.!!!';
      expect(await secrets.read('a'), isNull);
      await secrets.write('b', 'value');
      hardware.replaceKey();
      final fresh = HardwareWrappedSecretStore(inner, hardware);
      expect(await fresh.read('b'), isNull);
    });

    test('delete removes the value and what is cached [SEC-073]', () async {
      await secrets.write('k', 'value');
      await secrets.delete('k');
      expect(await secrets.read('k'), isNull);
      expect(inner.values, isEmpty);
    });

    test('installationKey makes a 256-bit key that only the hardware can '
        'open [SEC-073]', () async {
      final key = (await installationKey(secrets, 'data-key.x', create: true))!;
      expect(key, hasLength(32));
      expect(inner.values['data-key.x'], isNot(contains(base64Encode(key))));
      final fresh = HardwareWrappedSecretStore(inner, hardware);
      expect(await installationKey(fresh, 'data-key.x', create: false), key);
    });
  });

  group('PlatformKeyWrapper', () {
    const wrapper = PlatformKeyWrapper();
    const channel = MethodChannel('dev.plux/runtime');
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    tearDown(() => messenger.setMockMethodCallHandler(channel, null));

    test(
      'wrap and unwrap send the bytes and read the reply [SEC-073]',
      () async {
        final calls = <MethodCall>[];
        messenger.setMockMethodCallHandler(channel, (call) async {
          calls.add(call);
          return call.method == 'dataKeyWrap'
              ? Uint8List.fromList([9, 9, 9])
              : Uint8List.fromList([1, 2, 3]);
        });
        expect(await wrapper.wrap(Uint8List.fromList([1, 2, 3])), [9, 9, 9]);
        expect(await wrapper.unwrap(Uint8List.fromList([9, 9, 9])), [1, 2, 3]);
        expect(calls.map((c) => c.method), ['dataKeyWrap', 'dataKeyUnwrap']);
        expect(calls[0].arguments, {
          'data': Uint8List.fromList([1, 2, 3]),
        });
        expect(calls[1].arguments, {
          'wrapped': Uint8List.fromList([9, 9, 9]),
        });
      },
    );

    test('PLUX_KEY_MISSING is a lost key; any other failure is an error '
        '[SEC-073]', () async {
      var code = 'PLUX_KEY_MISSING';
      messenger.setMockMethodCallHandler(
        channel,
        (call) async => throw PlatformException(code: code),
      );
      expect(await wrapper.unwrap(Uint8List(40)), isNull);
      code = 'PLUX_PLATFORM';
      await expectLater(
        wrapper.unwrap(Uint8List(40)),
        throwsA(
          isA<StoreException>().having(
            (e) => e.failure,
            'failure',
            StoreFailure.unavailable,
          ),
        ),
      );
      await expectLater(
        wrapper.wrap(Uint8List(32)),
        throwsA(isA<StoreException>()),
      );
    });

    test('a platform without the methods, or a malformed reply, is '
        'unavailable [SEC-073]', () async {
      await expectLater(
        wrapper.wrap(Uint8List(32)),
        throwsA(isA<StoreException>()),
      );
      messenger.setMockMethodCallHandler(channel, (call) async => 'text');
      await expectLater(
        wrapper.wrap(Uint8List(32)),
        throwsA(isA<StoreException>()),
      );
      await expectLater(
        wrapper.unwrap(Uint8List(40)),
        throwsA(isA<StoreException>()),
      );
    });
  });
}
