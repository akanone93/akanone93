// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/key_wrap.dart';
import 'package:plux_flutter/src/security/platform_aead.dart';
import 'package:plux_flutter/src/security/record_cipher.dart';
import 'package:plux_flutter/src/store/kv_store.dart';
import 'package:plux_flutter/src/store/sealed_store.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart' show MemorySecretStore;

import '../support/fake_hardware.dart';

/// The encrypted store of the `strict` and `maximum` profiles and its
/// migration from the stores of `standard` (SEC-073, ADR-0058).
void main() {
  late Directory dir;
  late MemorySecretStore platform;
  late FakeHardware hardware;
  late HardwareWrappedSecretStore wrapped;

  setUp(() {
    dir = Directory.systemTemp.createTempSync('plux_sealed');
    platform = MemorySecretStore();
    hardware = FakeHardware();
    wrapped = HardwareWrappedSecretStore(platform, hardware);
  });
  tearDown(() => dir.deleteSync(recursive: true));

  Future<R> inline<R>(FutureOr<R> Function() work) async => work();

  SealedFileStore store({
    String file = 's.pxk',
    String name = 'plux-state/secure/a',
    bool plain = false,
    bool legacy = false,
    int maxBytes = 1024,
    OffIsolate? off,
    SecretStore? keys,
  }) => SealedFileStore(
    path: '${dir.path}/$file',
    name: name,
    secrets: keys ?? wrapped,
    keyName: 'data-key.secure',
    maxBytes: maxBytes,
    cipher: const RecordCipher(DartAead()),
    plainMigration: plain,
    legacySecrets: legacy ? platform : null,
    legacyKeyName: legacy ? 'state-secure.a' : null,
    legacyLabel: legacy ? 'plux-state/secure/a' : null,
    offIsolate: off ?? inline,
  );

  Matcher corrupt() => throwsA(
    isA<StoreException>().having(
      (e) => e.failure,
      'failure',
      StoreFailure.corrupt,
    ),
  );

  test('round-trips entries as a record, under a data key that only the '
      'hardware can open [SEC-073]', () async {
    final s = store();
    expect(await s.load(), isEmpty);
    expect(platform.values, isEmpty, reason: 'no key until something is saved');
    await s.save({'a': 1, 'secret': 's3cr3t-value'});

    final raw = File('${dir.path}/s.pxk').readAsBytesSync();
    expect(raw[0], recordVersion);
    expect(latin1.decode(raw), isNot(contains('s3cr3t-value')));
    // The key sits in secure storage only as the hardware wrapped it.
    final stored = platform.values['data-key.secure']!;
    expect(stored, startsWith('hw1.'));
    final dataKey = await installationKey(
      wrapped,
      'data-key.secure',
      create: false,
    );
    expect(dataKey, hasLength(32));
    expect(stored, isNot(contains(base64Encode(dataKey!))));

    // A later launch unwraps it once.
    final next = HardwareWrappedSecretStore(platform, hardware);
    expect(await store(keys: next).load(), {'a': 1, 'secret': 's3cr3t-value'});
    expect(hardware.unwraps, 1);
  });

  test('the default isolate runs the cipher, the record is the same '
      '[SEC-073]', () async {
    final s = SealedFileStore(
      path: '${dir.path}/i.pxk',
      name: 'n',
      secrets: wrapped,
      keyName: 'data-key.i',
      maxBytes: 1024,
      cipher: RecordCipher(platformAead),
    );
    await s.save({'k': 'v'});
    expect(await s.load(), {'k': 'v'});
  });

  test('a tampered file, a file of another store and a file without its key '
      'fail authentication, are removed and reported as corrupt '
      '[SEC-073]', () async {
    final f = File('${dir.path}/s.pxk');
    await store().save({'a': 1});

    final bytes = f.readAsBytesSync();
    f.writeAsBytesSync(bytes.sublist(0, bytes.length)..[bytes.length - 3] ^= 1);
    await expectLater(store().load(), corrupt());
    expect(f.existsSync(), isFalse);

    // The same bytes under another store's name do not open.
    await store().save({'a': 1});
    File('${dir.path}/o.pxk').writeAsBytesSync(f.readAsBytesSync());
    await expectLater(
      store(file: 'o.pxk', name: 'plux-data/outbox/a').load(),
      corrupt(),
    );
    expect(File('${dir.path}/o.pxk').existsSync(), isFalse);

    // The hardware key was lost with a restore: the file cannot be opened.
    hardware.lost = true;
    final restored = HardwareWrappedSecretStore(platform, hardware);
    await expectLater(store(keys: restored).load(), corrupt());
    expect(f.existsSync(), isFalse);
    hardware.lost = false;
    await store(keys: restored).save({'b': 2});
    expect(await store(keys: restored).load(), {'b': 2});
  });

  test('a hardware that fails makes the store unavailable and writes '
      'nothing, in the clear or otherwise [SEC-073]', () async {
    hardware.failing = true;
    await expectLater(
      store().save({'a': 1}),
      throwsA(
        isA<StoreException>().having(
          (e) => e.failure,
          'failure',
          StoreFailure.unavailable,
        ),
      ),
    );
    expect(File('${dir.path}/s.pxk').existsSync(), isFalse);
    expect(platform.values, isEmpty);
  });

  test('entries over the limit are refused and the file is kept; wipe '
      'removes the file and the key [LIM-001] [SEC-073]', () async {
    final s = store(maxBytes: 40);
    await s.save({'a': 1});
    await expectLater(
      s.save({'a': 'x' * 100}),
      throwsA(
        isA<StoreException>().having(
          (e) => e.failure,
          'failure',
          StoreFailure.tooLarge,
        ),
      ),
    );
    expect(await store(maxBytes: 40).load(), {'a': 1});
    await s.wipe();
    expect(File('${dir.path}/s.pxk').existsSync(), isFalse);
    expect(platform.values, isEmpty);
  });

  group('migration in place', () {
    test('a plain file is sealed under the same name; nothing of it is '
        'left readable [SEC-073]', () async {
      final f = File('${dir.path}/s.pxk')
        ..writeAsStringSync(jsonEncode({'a': 'plain-value', 'n': 3}));
      // A temporary file a crashed plain store left behind.
      File('${f.path}.tmp').writeAsStringSync('{"a":"plain-value"}');

      expect(await store(plain: true).load(), {'a': 'plain-value', 'n': 3});

      final raw = f.readAsBytesSync();
      expect(raw[0], recordVersion);
      expect(latin1.decode(raw), isNot(contains('plain-value')));
      expect(File('${f.path}.tmp').existsSync(), isFalse);
      expect(await store(plain: true).load(), {'a': 'plain-value', 'n': 3});
    });

    test('a plain file is not read by a store that does not migrate; it is '
        'removed and reported [SEC-073]', () async {
      File('${dir.path}/s.pxk').writeAsStringSync('{"a":1}');
      await expectLater(store().load(), corrupt());
      expect(File('${dir.path}/s.pxk').existsSync(), isFalse);
    });

    test(
      'a file that is neither plain nor a record is corrupt [SEC-073]',
      () async {
        File('${dir.path}/s.pxk').writeAsBytesSync([0xff, 0x00, 0x01]);
        await expectLater(store(plain: true).load(), corrupt());
        File('${dir.path}/s.pxk').writeAsStringSync('{not json');
        await expectLater(store(plain: true).load(), corrupt());
      },
    );

    test(
      'an encrypted file of the standard profile is read with its '
      'software-wrapped key, sealed, and the old key removed [SEC-073]',
      () async {
        final old = EncryptedFileStore(
          path: '${dir.path}/s.pxk',
          secrets: platform,
          keyName: 'state-secure.a',
          label: 'plux-state/secure/a',
          maxBytes: 1024,
          offIsolate: inline,
        );
        await old.save({'token': 'secret-token'});
        final legacyKey = platform.values['state-secure.a']!;
        expect(legacyKey, isNot(startsWith('hw1.')));

        expect(await store(legacy: true).load(), {'token': 'secret-token'});

        expect(File('${dir.path}/s.pxk').readAsBytesSync()[0], recordVersion);
        expect(platform.values.containsKey('state-secure.a'), isFalse);
        expect(platform.values['data-key.secure'], startsWith('hw1.'));
        expect(await store(legacy: true).load(), {'token': 'secret-token'});
      },
    );

    test('an encrypted file of the standard profile is corrupt when its '
        'key is gone [SEC-073]', () async {
      final old = EncryptedFileStore(
        path: '${dir.path}/s.pxk',
        secrets: platform,
        keyName: 'state-secure.a',
        label: 'plux-state/secure/a',
        maxBytes: 1024,
        offIsolate: inline,
      );
      await old.save({'a': 1});
      platform.values.remove('state-secure.a');
      await expectLater(store(legacy: true).load(), corrupt());
    });

    test('a failed migration leaves the old file whole, so a crash costs '
        'nothing [SEC-073]', () async {
      final f = File('${dir.path}/s.pxk')..writeAsStringSync('{"a":"keep"}');
      // The sealed form cannot be written: the limit is under its size.
      await expectLater(
        store(plain: true, maxBytes: 4).load(),
        throwsA(
          isA<StoreException>().having(
            (e) => e.failure,
            'failure',
            StoreFailure.tooLarge,
          ),
        ),
      );
      expect(f.readAsStringSync(), '{"a":"keep"}');
      expect(File('${f.path}.tmp').existsSync(), isFalse);
    });
  });
}
