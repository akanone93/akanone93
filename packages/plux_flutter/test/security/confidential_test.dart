// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/confidential.dart';
import 'package:plux_flutter/src/security/software_keys.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';

// The vectors the server's tests pin (backend/internal/bundle): a real
// compiled bundle sealed under a known key, and a content key wrapped for a
// known device key.
final _vectors = jsonDecode(
  File('../../schema/testdata/confidential/vectors.json').readAsStringSync(),
) as Map<String, Object?>;

Uint8List _b64(Object? v) => base64.decode(v! as String);

Uint8List _hex(Object? v) {
  final s = v! as String;
  return Uint8List.fromList([
    for (var i = 0; i < s.length; i += 2)
      int.parse(s.substring(i, i + 2), radix: 16),
  ]);
}

final _key = _hex(_vectors['contentKey']);
final _plain = _b64(_vectors['plain']);
final _sealed = _b64(_vectors['sealed']);
final _sealedHash = bundleHash(
  _sealed,
  BundleContainer.parse(_sealed).sections.length,
);

/// An [Aead] that counts its calls, to show when decryption happens.
final class _Spy implements Aead {
  int opened = 0;

  @override
  Uint8List? open(
    Uint8List key,
    Uint8List nonce,
    Uint8List sealed,
    Uint8List aad,
  ) {
    opened++;
    return const DartAead().open(key, nonce, sealed, aad);
  }

  @override
  Uint8List seal(
    Uint8List key,
    Uint8List nonce,
    Uint8List plaintext,
    Uint8List aad,
  ) => const DartAead().seal(key, nonce, plaintext, aad);
}

Matcher _throwsCode(PluxErrorCode code) =>
    throwsA(isA<PluxException>().having((e) => e.code, 'code', code));

void main() {
  group('AES-KW (RFC 3394)', () {
    test('SEC-053 unwraps the RFC 3394 known answers', () async {
      // §4.1: a 128-bit key under a 128-bit KEK.
      expect(
        await aesKeyUnwrap(
          _hex('000102030405060708090A0B0C0D0E0F'.toLowerCase()),
          _hex(
            '1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5'.toLowerCase(),
          ),
        ),
        _hex('00112233445566778899AABBCCDDEEFF'.toLowerCase()),
      );
      // §4.6: a 256-bit key under a 256-bit KEK.
      expect(
        await aesKeyUnwrap(
          _hex(
            '000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F'
                .toLowerCase(),
          ),
          _hex(
            '28C9F404C4B810F4CBCCB35CFB87F8263F5786E2D80ED326CBC7F0E71A99F43BFB988B9B7A02DD21'
                .toLowerCase(),
          ),
        ),
        _hex(
          '00112233445566778899AABBCCDDEEFF000102030405060708090A0B0C0D0E0F'
              .toLowerCase(),
        ),
      );
    });

    test('SEC-053 refuses an altered wrap', () async {
      final kek = _hex('000102030405060708090A0B0C0D0E0F'.toLowerCase());
      final wrapped = _hex(
        '1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5'.toLowerCase(),
      );
      wrapped[5] ^= 1;
      expect(await aesKeyUnwrap(kek, wrapped), isNull);
      expect(await aesKeyUnwrap(kek, Uint8List(16)), isNull);
    });
  });

  group('content key', () {
    final wrap = _vectors['wrap']! as Map<String, Object?>;
    late SoftwareDeviceKeys keys;
    setUp(() {
      keys = SoftwareDeviceKeys.withScalar(
        'agree',
        BigInt.parse(wrap['devicePrivateKey']! as String, radix: 16),
      );
    });

    WrappedContentKey envelope({String? deviceId, String? wrapped}) =>
        WrappedContentKey(
          ephemeralPublicKey: _b64(wrap['ephemeralPublicKey']),
          wrappedKey: _b64(wrapped ?? wrap['wrapped']),
          deviceId: deviceId ?? wrap['deviceId']! as String,
          app: wrap['app']! as String,
          environment: wrap['environment']! as String,
        );

    test(
      'SEC-053 unwraps the key the server wrapped for this device',
      () async {
        final key = await unwrapContentKey(
          envelope(),
          keys: keys,
          alias: 'agree',
          sequence: wrap['sequence']! as int,
        );
        expect(key, _key);
      },
    );

    test(
      'SEC-053 refuses another release, device or altered bytes [SEC-053]',
      () async {
        final sequence = wrap['sequence']! as int;
        for (final (name, e, seq) in [
          ('another release', envelope(), sequence + 1),
          ('another device ID', envelope(deviceId: 'device-2'), sequence),
          (
            'an altered wrapped key',
            envelope(
              wrapped:
                  ((wrap['wrong']! as List).last
                          as Map<String, Object?>)['wrapped']!
                      as String,
            ),
            sequence,
          ),
        ]) {
          await expectLater(
            unwrapContentKey(e, keys: keys, alias: 'agree', sequence: seq),
            _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
            reason: name,
          );
        }
      },
    );

    test('SEC-053 a device with no key-agreement key gets PLX-6050', () async {
      await expectLater(
        unwrapContentKey(
          envelope(),
          keys: SoftwareDeviceKeys(),
          alias: 'agree',
          sequence: wrap['sequence']! as int,
        ),
        _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
      );
    });

    test('SEC-053 a malformed envelope gets PLX-6050', () async {
      final bad = WrappedContentKey(
        ephemeralPublicKey: Uint8List(64),
        wrappedKey: Uint8List(40),
        deviceId: 'd',
        app: 'a',
        environment: 'e',
      );
      await expectLater(
        unwrapContentKey(bad, keys: keys, alias: 'agree', sequence: 1),
        _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
      );
    });

    test('the envelope survives JSON', () {
      final back = WrappedContentKey.fromJson(envelope().toJson());
      expect(back.wrappedKey, envelope().wrappedKey);
      expect(back.ephemeralPublicKey, envelope().ephemeralPublicKey);
      expect(back.deviceId, 'device-1');
    });

    test('the info matches the server', () {
      expect(
        utf8.decode(
          contentKeyInfo(
            'd',
            'a',
            'e',
            7,
          ).map((b) => b == 0 ? 0x7c : b).toList(),
        ),
        'plux.confidential.v1|d|a|e|7',
      );
    });
  });

  group('bundle', () {
    test(
      'SEC-053 decrypts the sealed bundle to the plain one the server made',
      () {
        expect(decryptBundle(_sealed, _sealedHash, _key), _plain);
      },
    );

    test('SEC-052 the plain bundle then passes the full verifier', () {
      final plain = decryptBundle(_sealed, _sealedHash, _key);
      final plainHash = bundleHash(
        plain,
        BundleContainer.parse(plain).sections.length,
      );
      verifyBundle(
        plain,
        plainHash,
        limits: const VerifierLimits(maxDepth: 64, maxVisits: 1 << 20),
        supportsFeature: (_) => true,
      );
    });

    test(
      'SEC-052 a ciphertext byte flipped fails its hash before any decryption',
      () {
        final spy = _Spy();
        final damaged = Uint8List.fromList(_sealed)..[_sealed.length - 20] ^= 1;
        expect(
          () => decryptBundle(damaged, _sealedHash, _key, aead: spy),
          _throwsCode(PluxErrorCode.sectionHashMismatch),
        );
        // The header hash still matches (the directory is untouched), so it is
        // the section hash that refuses.
        expect(spy.opened, 0);
      },
    );

    test('SEC-052 a bundle that is not the signed one is refused before decryption', () {
      final spy = _Spy();
      final other = Uint8List.fromList(_sealedHash)..[0] ^= 1;
      expect(
        () => decryptBundle(_sealed, other, _key, aead: spy),
        _throwsCode(PluxErrorCode.bundleMalformed),
      );
      expect(spy.opened, 0);
    });

    test('SEC-052 every refused vector fails the way the server says', () {
      for (final r
          in (_vectors['refused']! as List).cast<Map<String, Object?>>()) {
        final data = _b64(r['sealed']);
        final key = r['key'] == null ? _key : _hex(r['key']);
        final expected = switch (r['error']) {
          'PLX-3041' => PluxErrorCode.sectionHashMismatch,
          'PLX-6050' => PluxErrorCode.confidentialKeyUnavailable,
          _ => PluxErrorCode.bundleMalformed,
        };
        final spy = _Spy();
        Uint8List hash;
        try {
          hash = bundleHash(data, BundleContainer.parse(data).sections.length);
        } on PluxException {
          hash = Uint8List(32);
        }
        expect(
          () => decryptBundle(data, hash, key, aead: spy),
          _throwsCode(expected),
          reason: r['name']! as String,
        );
        if (expected != PluxErrorCode.confidentialKeyUnavailable) {
          expect(
            spy.opened,
            0,
            reason: 'decrypted before refusing: ${r['name']}',
          );
        }
      }
    });

    test('SEC-053 a plain bundle is not accepted as a confidential one', () {
      final hash = bundleHash(
        _plain,
        BundleContainer.parse(_plain).sections.length,
      );
      expect(
        () => decryptBundle(_plain, hash, _key),
        _throwsCode(PluxErrorCode.bundleMalformed),
      );
    });

    test('SEC-053 the old verifier still refuses an encrypted bundle', () {
      expect(
        () => verifyBundle(
          _sealed,
          _sealedHash,
          limits: const VerifierLimits(maxDepth: 64, maxVisits: 1 << 20),
          supportsFeature: (_) => true,
        ),
        _throwsCode(PluxErrorCode.bundleEncryptedUnsupported),
      );
    });

    test('SEC-053 decrypts a file off the calling isolate', () async {
      final dir = Directory.systemTemp.createTempSync('plux-conf');
      addTearDown(() => dir.deleteSync(recursive: true));
      final file = File('${dir.path}/b')..writeAsBytesSync(_sealed);
      expect(await decryptBundleFile(file.path, _sealedHash, _key), _plain);
    });
  });
}
