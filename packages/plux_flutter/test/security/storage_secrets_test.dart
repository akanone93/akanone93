// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart' show Plux, PluxErrorCode;
import 'package:plux_flutter/src/security/key_wrap.dart';
import 'package:plux_flutter/src/state/persistence.dart';
import 'package:plux_flutter/src/store/kv_store.dart';

import '../support/fake_hardware.dart';
import '../support/harness.dart';

/// Whether [needle] occurs in [haystack], byte for byte.
bool _contains(Uint8List haystack, List<int> needle) {
  if (needle.isEmpty || needle.length > haystack.length) return false;
  final first = needle.first;
  for (var i = 0; i <= haystack.length - needle.length; i++) {
    if (haystack[i] != first) continue;
    var j = 1;
    while (j < needle.length && haystack[i + j] == needle[j]) {
      j++;
    }
    if (j == needle.length) return true;
  }
  return false;
}

/// Every file under [dir] that holds any of [needles], with the needle's
/// name. A file that a running write renames away meanwhile is skipped:
/// its replacement is found by the same walk or the next.
List<String> _leaks(Directory dir, Map<String, List<int>> needles) {
  final found = <String>[];
  for (final f in dir.listSync(recursive: true).whereType<File>()) {
    final Uint8List bytes;
    try {
      bytes = f.readAsBytesSync();
    } on FileSystemException {
      continue;
    }
    for (final MapEntry(:key, :value) in needles.entries) {
      if (_contains(bytes, value)) {
        found.add('$key in ${f.path.substring(dir.path.length)}');
      }
    }
  }
  return found;
}

/// A session against the fake server and fake platform: the device
/// registers, gets a token and proves possession with DPoP, the app keeps
/// `persisted`, `secure` and key-value data, and its stores are encrypted.
/// Then every file the runtime wrote is searched for each token, proof,
/// key and secret value the session used.
///
/// Verifies: SEC-072, SEC-073.
void main() {
  late Harness h;
  final g = Harness.goldens;
  const persisted = 'persisted-value-4f7a91c3';
  const secure = 'secure-value-b83d02e6';
  const kv = 'kv-value-19ce57a0';

  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<Map<String, List<int>>> session(
    WidgetTester tester, {
    required bool encrypt,
  }) async {
    final hardware = FakeHardware();
    final needles = <String, List<int>>{};
    await tester.runAsync(() async {
      await h.startFrom(
        g.bundles['triggers/triggers.pxb']!,
        {'lab': g.bundles['triggers/lab.pxb']!},
        encryptLocalStores: encrypt,
        keyWrapper: () => hardware,
      );
      final rt = h.runtime;
      await rt.sync();
      rt.statePersistence
        ..write(Persistence.persisted, 'profile', 'fp', persisted)
        ..write(Persistence.secure, 'pin', 'fp', secure);
      await rt.statePersistence.flush();
      await rt.database.kvSet('lab', 'note', kv);

      // What the session sent: the access token and every DPoP proof.
      expect(h.server.credentialsSeen, isNotEmpty);
      for (final c in h.server.credentialsSeen) {
        needles['credential $c'] = utf8.encode(c.split(' ').last);
      }
      needles['persisted value'] = utf8.encode(persisted);
      needles['secure value'] = utf8.encode(secure);
      needles['kv value'] = utf8.encode(kv);

      // The data keys, as the hardware opens them.
      if (encrypt) {
        final keys = HardwareWrappedSecretStore(h.secrets, hardware);
        var found = 0;
        for (final name in h.secrets.values.keys.where(
          (n) => n.startsWith('dk-'),
        )) {
          final key = await installationKey(keys, name, create: false);
          needles['key $name'] = key!;
          needles['key $name (base64)'] = utf8.encode(base64Encode(key));
          needles['key $name (hex)'] = utf8.encode(
            key.map((b) => b.toRadixString(16).padLeft(2, '0')).join(),
          );
          found++;
        }
        expect(found, greaterThanOrEqualTo(3), reason: 'one key per store');
      }
    });
    return needles;
  }

  testWidgets('with the stores encrypted, no file under the storage '
      'directory holds a token, proof, key or secret value of the session '
      '[SEC-072] [SEC-073]', (tester) async {
    final needles = await session(tester, encrypt: true);
    expect(needles.keys.where((k) => k.startsWith('credential')), isNotEmpty);
    expect(_leaks(Directory(h.dir), needles), isEmpty);
    // The stores exist: the search had files to look in.
    final names = Directory(h.dir)
        .listSync(recursive: true)
        .whereType<File>()
        .map((f) => f.uri.pathSegments.last);
    expect(names, containsAll(['persisted.json', 'secure.pxk', 'kv.pxk']));
  });

  testWidgets('the search finds a value that is stored in the clear, so an '
      'empty result means something [SEC-072]', (tester) async {
    final needles = await session(tester, encrypt: false);
    final leaks = _leaks(Directory(h.dir), needles);
    expect(leaks, hasLength(1));
    expect(leaks.single, startsWith('persisted value in '));
    expect(leaks.single, endsWith('/plux-state/persisted.json'));
  });

  testWidgets('a launch under a stricter profile migrates the stores of the '
      'standard profile in place, keeping their data [SEC-073]', (
    tester,
  ) async {
    final hardware = FakeHardware();
    Future<void> launch({required bool encrypt}) async {
      await h.startFrom(
        g.bundles['triggers/triggers.pxb']!,
        {'lab': g.bundles['triggers/lab.pxb']!},
        encryptLocalStores: encrypt,
        keyWrapper: () => hardware,
      );
      await h.runtime.statePersistence.load();
    }

    await tester.runAsync(() async {
      await launch(encrypt: false);
      final first = h.runtime;
      first.statePersistence
        ..write(Persistence.persisted, 'profile', 'fp', persisted)
        ..write(Persistence.secure, 'pin', 'fp', secure);
      await first.statePersistence.flush();
      await first.database.kvSet('lab', 'note', kv);
      expect(
        _leaks(Directory(h.dir), {'p': utf8.encode(persisted)}),
        hasLength(1),
      );
      await Plux.dispose();

      await launch(encrypt: true);
      final next = h.runtime;
      expect(
        next.statePersistence.read(Persistence.persisted, 'profile')?.value,
        persisted,
      );
      expect(
        next.statePersistence.read(Persistence.secure, 'pin')?.value,
        secure,
      );
      expect(await next.database.kvGet('lab', 'note'), kv);
      // The fake server has no release to offer; the stores report nothing.
      expect(
        h.errors.where((e) => e.code != PluxErrorCode.syncFailed),
        isEmpty,
      );
      expect(
        _leaks(Directory(h.dir), {
          'persisted': utf8.encode(persisted),
          'secure': utf8.encode(secure),
          'kv': utf8.encode(kv),
        }),
        isEmpty,
      );
      expect(
        h.secrets.values.keys.where((n) => n.startsWith('state-secure.')),
        isEmpty,
        reason: 'the earlier software-wrapped key is removed',
      );
    });
  });
}
