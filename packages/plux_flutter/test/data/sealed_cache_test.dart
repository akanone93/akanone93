// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/data/cache.dart';
import 'package:plux_flutter/src/data/worker.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/key_wrap.dart';
import 'package:plux_flutter/src/security/record_cipher.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart' show MemorySecretStore;

import '../support/fake_hardware.dart';

/// The response cache under `strict` and `maximum` (SEC-073, ADR-0058).
void main() {
  const cipher = RecordCipher(DartAead());
  final key = Uint8List.fromList(List.generate(32, (i) => i));
  late Directory dir;

  setUp(() => dir = Directory.systemTemp.createTempSync('plux_sealed_cache'));
  tearDown(() => dir.deleteSync(recursive: true));

  FileCacheStore sealed({Uint8List? k, String name = 'data-cache/plain'}) =>
      FileCacheStore(dir.path, key: k ?? key, cipher: cipher, storeName: name);

  test('an entry is a record bound to the store and its key; the time stays '
      'readable for the index [SEC-073]', () async {
    final store = sealed();
    final k = cacheKey('x');
    await store.write(k, {'name': 'Ada Lovelace'}, 5);
    final file = File('${dir.path}/$k').readAsBytesSync();
    expect(
      utf8.decode(file, allowMalformed: true),
      isNot(contains('Lovelace')),
    );
    expect(file[8], recordVersion);
    expect((await store.read(k))!.json, {'name': 'Ada Lovelace'});
    expect((await store.entries()).single.storedAtMs, 5);
  });

  test('an entry moved to another key, another store or opened with another '
      'key is forgotten [SEC-073]', () async {
    final k = cacheKey('x');
    final moved = cacheKey('y');
    await sealed().write(k, [1], 5);
    File('${dir.path}/$k').copySync('${dir.path}/$moved');
    expect(await sealed().read(moved), isNull);
    expect(File('${dir.path}/$moved').existsSync(), isFalse);
    expect(await sealed(name: 'data-cache/secure').read(k), isNull);
    expect(File('${dir.path}/$k').existsSync(), isFalse);
    await sealed().write(k, [1], 5);
    expect(await sealed(k: Uint8List(32)).read(k), isNull);
  });

  test('entries stored in the clear are sealed in place, and a stale '
      'temporary file is removed [SEC-073]', () async {
    final plain = FileCacheStore(dir.path);
    final a = cacheKey('a');
    final b = cacheKey('b');
    await plain.write(a, {'secret': 'plain-value'}, 7);
    await plain.write(b, [2], 8);
    File('${dir.path}/.$a.tmp').writeAsStringSync('plain-value');
    File('${dir.path}/${cacheKey('junk')}')
        .writeAsBytesSync([0, 0, 0, 0, 0, 0, 0, 1, 0xff, 0xfe]);

    final store = sealed();
    await store.migrate();

    for (final k in [a, b]) {
      final bytes = File('${dir.path}/$k').readAsBytesSync();
      expect(bytes[8], recordVersion);
      expect(latin1.decode(bytes), isNot(contains('plain-value')));
    }
    expect(File('${dir.path}/.$a.tmp').existsSync(), isFalse);
    expect(File('${dir.path}/${cacheKey('junk')}').existsSync(), isFalse);
    expect((await store.read(a))!.json, {'secret': 'plain-value'});
    expect((await store.entries()).map((e) => e.storedAtMs).toSet(), {7, 8});
  });

  test('a plain entry read before the sweep is sealed on the way '
      '[SEC-073]', () async {
    final k = cacheKey('a');
    await FileCacheStore(dir.path).write(k, [3], 4);
    expect((await sealed().read(k))!.json, [3]);
    expect(File('${dir.path}/$k').readAsBytesSync()[8], recordVersion);
  });

  test('under the setting the data isolate seals plain entries too, with a '
      'key only the hardware opens [SEC-073]', () async {
    final platform = MemorySecretStore();
    final secrets = HardwareWrappedSecretStore(platform, FakeHardware());
    final k = cacheKey('old');
    await FileCacheStore('${dir.path}/plain').write(k, {'v': 'plain-value'}, 1);

    final worker = LazyDataWorker(
      () => DataWorker.start(
        httpClient: http.Client.new,
        cacheDirectory: dir.path,
        keys: SecretCacheKeys(secrets, 'data-cache.a'),
        sealed: true,
      ),
    );
    addTearDown(worker.close);

    final plain = worker.store(secure: false);
    final fresh = cacheKey('new');
    await plain.write(fresh, {'v': 'fresh-value'}, 2);
    expect((await plain.read(fresh))!.json, {'v': 'fresh-value'});
    expect((await plain.read(k))!.json, {'v': 'plain-value'});
    await worker.store(secure: true).write(fresh, [1], 3);

    for (final f in Directory(dir.path).listSync(recursive: true)) {
      if (f is! File) continue;
      expect(latin1.decode(f.readAsBytesSync()), isNot(contains('-value')));
      expect(f.readAsBytesSync()[8], recordVersion);
    }
    expect(platform.values['data-cache.a'], startsWith('hw1.'));
  });
}
