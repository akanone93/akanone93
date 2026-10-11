// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/core/active_release.dart';
import 'package:plux_flutter/src/core/confidential_bundles.dart';
import 'package:plux_flutter/src/core/features.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/content_key.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/software_keys.dart';
import 'package:plux_flutter/src/store/release_record.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

// The server's vectors (backend/internal/bundle): a real compiled bundle
// sealed under a known key, and that key wrapped for a known device key in
// app-1 / production at release 7.
final _vectors = jsonDecode(
  File('../../schema/testdata/confidential/vectors.json').readAsStringSync(),
) as Map<String, Object?>;
final _wrap = _vectors['wrap']! as Map<String, Object?>;
Uint8List _b64(Object? v) => base64.decode(v! as String);

final _sealed = _b64(_vectors['sealed']);
final _plain = _b64(_vectors['plain']);
final _hash = hexEncode(
  bundleHash(_sealed, BundleContainer.parse(_sealed).sections.length),
);
const _limits = VerifierLimits(maxDepth: 64, maxVisits: 1 << 20);

WrappedContentKey _envelope() => WrappedContentKey(
  ephemeralPublicKey: _b64(_wrap['ephemeralPublicKey']),
  wrappedKey: _b64(_wrap['wrapped']),
  deviceId: _wrap['deviceId']! as String,
  app: _wrap['app']! as String,
  environment: _wrap['environment']! as String,
);

ReleaseRecord _record({bool withKey = true}) => ReleaseRecord(
  sequence: _wrap['sequence']! as int,
  source: ReleaseSource.sync,
  bundles: [
    RecordBundle(
      key: '',
      version: 1,
      hash: _hash,
      features: const [confidentialFeature],
    ),
  ],
  contentKey: withKey ? _envelope() : null,
);

Matcher _throwsCode(PluxErrorCode code) =>
    throwsA(isA<PluxException>().having((e) => e.code, 'code', code));

void main() {
  late Directory dir;
  late ReleaseStore store;
  late ConfidentialBundles bundles;

  setUp(() {
    dir = Directory.systemTemp.createTempSync('plux-conf-store');
    store = ReleaseStore.open(dir.path);
    store.writeObject(ObjectKind.bundles, _hash, _sealed);
    bundles = ConfidentialBundles(
      keys: SoftwareDeviceKeys.withScalar(
        agreementKeyAlias(
          _wrap['app']! as String,
          _wrap['environment']! as String,
        ),
        BigInt.parse(_wrap['devicePrivateKey']! as String, radix: 16),
      ),
      appId: _wrap['app']! as String,
      environment: _wrap['environment']! as String,
    );
  });
  tearDown(() => dir.deleteSync(recursive: true));

  test(
    'SEC-053 the stored bundle is ciphertext, the opened one is plain',
    () async {
      expect(
        File(store.objectPath(ObjectKind.bundles, _hash)).readAsBytesSync(),
        _sealed,
      );
      final opened = await bundles.open(_record(), store);
      expect(opened.keys, {''});
      expect(opened['']!, _plain);
      // Nothing plain was written back to disk.
      expect(
        File(store.objectPath(ObjectKind.bundles, _hash)).readAsBytesSync(),
        _sealed,
      );
    },
  );

  test('SEC-053 a bundle is decrypted once per launch', () async {
    await bundles.open(_record(), store);
    expect(bundles.decrypted, 1);
    await bundles.open(_record(), store);
    await bundles.open(_record(), store);
    expect(bundles.decrypted, 1, reason: 'the bundle was opened again');
    // A release that replaced it lets go of the memory; using it again
    // decrypts it again.
    bundles.retain({});
    await bundles.open(_record(), store);
    expect(bundles.decrypted, 2);
  });

  test('SEC-053 a release without a content key is PLX-6050', () async {
    await expectLater(
      bundles.open(_record(withKey: false), store),
      _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
    );
  });

  test(
    'SEC-053 a device that lost its key-agreement key is PLX-6050',
    () async {
      final lost = ConfidentialBundles(
        keys: SoftwareDeviceKeys(),
        appId: _wrap['app']! as String,
        environment: _wrap['environment']! as String,
      );
      await expectLater(
        lost.open(_record(), store),
        _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
      );
    },
  );

  test(
    'SEC-052 a tampered stored bundle is refused before any decryption',
    () async {
      final damaged = Uint8List.fromList(_sealed)..[_sealed.length - 20] ^= 1;
      File(store.objectPath(ObjectKind.bundles, _hash))
          .writeAsBytesSync(damaged);
      await expectLater(
        bundles.open(_record(), store),
        _throwsCode(PluxErrorCode.sectionHashMismatch),
      );
      // The failure is not remembered: restoring the file lets it open.
      File(store.objectPath(ObjectKind.bundles, _hash))
          .writeAsBytesSync(_sealed);
      expect((await bundles.open(_record(), store))[''], _plain);
    },
  );

  test('SEC-053 the record keeps the wrapped key across a restart', () {
    final back = ReleaseRecord.decode(_record().encode());
    expect(back.contentKey!.wrappedKey, _envelope().wrappedKey);
    expect(back.bundles.single.confidential, isTrue);
    final plain = ReleaseRecord.decode(
      const ReleaseRecord(
        sequence: 1,
        source: ReleaseSource.sync,
        bundles: [RecordBundle(key: '', version: 1, hash: 'ab')],
      ).encode(),
    );
    expect(plain.contentKey, isNull);
    expect(plain.bundles.single.confidential, isFalse);
  });

  test(
    'SEC-053 the active release reads the decrypted buffer, not the mapping',
    () async {
      final decrypted = await bundles.open(_record(), store);
      final release = ActiveRelease.open(
        store,
        _record(),
        _limits,
        decrypted: decrypted,
      );
      addTearDown(release.retire);
      // The meta section of the compiled bundle reads through the gate.
      expect(release.meta('').key, isNotNull);
    },
  );

  test(
    'SEC-053 an active release with an unopened bundle reports PLX-6050',
    () {
      final errors = <PluxException>[];
      expect(
        () => ActiveRelease.open(store, _record(), _limits, report: errors.add),
        _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
      );
      expect(errors.single.code, PluxErrorCode.confidentialKeyUnavailable);
    },
  );

  test('SEC-053 the runtime supports the confidential feature', () {
    expect(RuntimeFeatures().supports(confidentialFeature), isTrue);
  });
}
