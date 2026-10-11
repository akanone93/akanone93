// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Verifies: SEC-052, SEC-053, QA-008.

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/confidential.dart';

// Tamper and replay attacks against a confidential bundle, over the bundle
// the server sealed (schema/testdata/confidential/vectors.json).

final _vectors = jsonDecode(
  File('../../schema/testdata/confidential/vectors.json').readAsStringSync(),
) as Map<String, Object?>;

Uint8List _hex(String s) => Uint8List.fromList([
  for (var i = 0; i < s.length; i += 2)
    int.parse(s.substring(i, i + 2), radix: 16),
]);

final _key = _hex(_vectors['contentKey']! as String);
final _sealed = base64.decode(_vectors['sealed']! as String);

Uint8List _hashOf(Uint8List data) =>
    bundleHash(data, BundleContainer.parse(data).sections.length);

/// A sealed container for [sections] with every hash consistent, as an
/// attacker who edits the file and recomputes them would write it.
Uint8List _resealed(int kind, List<Section> sections) {
  final out = encodeBundle(kind, sections);
  out[8] |= 1;
  out.setAll(16, bundleHash(out, sections.length));
  return out;
}

Matcher _throwsCode(PluxErrorCode code) =>
    throwsA(isA<PluxException>().having((e) => e.code, 'code', code));

Section _with(Section s, {int? kind, Uint8List? id, Uint8List? data}) =>
    Section(kind ?? s.kind, id ?? s.id, Uint8List(0), data ?? s.data);

void main() {
  final parsed = BundleContainer.parse(_sealed);
  final sections = parsed.sections;

  Uint8List flippedAt(Section s, int offset) {
    final data = Uint8List.fromList(s.data)..[offset] ^= 1;
    return data;
  }

  test('a sealed vector with at least three sections is the base of the '
      'matrix [SEC-053]', () {
    expect(sections.length, greaterThanOrEqualTo(3));
    expect(_resealed(parsed.kind, sections), _sealed);
  });

  group('hashes recomputed, so only the authentication tag can refuse', () {
    final cases = <String, Uint8List Function()>{
      'a nonce byte altered': () => _resealed(parsed.kind, [
        _with(sections[0], data: flippedAt(sections[0], 0)),
        ...sections.skip(1),
      ]),
      'a ciphertext byte altered': () => _resealed(parsed.kind, [
        ...sections.take(1),
        _with(sections[1], data: flippedAt(sections[1], 12 + 1)),
        ...sections.skip(2),
      ]),
      'a tag byte altered': () => _resealed(parsed.kind, [
        ...sections.take(2),
        _with(
          sections[2],
          data: flippedAt(sections[2], sections[2].data.length - 1),
        ),
        ...sections.skip(3),
      ]),
      'two sections\' contents swapped': () => _resealed(parsed.kind, [
        _with(sections[0], data: sections[1].data),
        _with(sections[1], data: sections[0].data),
        ...sections.skip(2),
      ]),
      'two sections\' kinds swapped': () => _resealed(parsed.kind, [
        _with(sections[0], kind: sections[1].kind),
        _with(sections[1], kind: sections[0].kind),
        ...sections.skip(2),
      ]),
      'a section moved to another ID': () => _resealed(parsed.kind, [
        _with(sections[0], id: Uint8List(16)..[0] = 9),
        ...sections.skip(1),
      ]),
      'the container kind changed': () => _resealed(
        parsed.kind == BundleKinds.plugin
            ? BundleKinds.app
            : BundleKinds.plugin,
        sections,
      ),
    };
    cases.forEach((name, build) {
      test('$name [SEC-053] [QA-008]', () {
        final attack = build();
        expect(
          () => decryptBundle(attack, _hashOf(attack), _key),
          _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
        );
      });
    });
  });

  group('hashes left as they were, so the hash refuses before decryption', () {
    for (final entry in {'a nonce byte': 0, 'a ciphertext byte': 13}.entries) {
      test('${entry.key} altered in the file [SEC-052] [QA-008]', () {
        final attack = Uint8List.fromList(_sealed);
        final last = sections.last;
        final start = attack.length - last.data.length;
        attack[start + entry.value] ^= 1;
        expect(
          () => decryptBundle(attack, _hashOf(_sealed), _key),
          _throwsCode(PluxErrorCode.sectionHashMismatch),
        );
      });
    }

    test('a tag byte altered in the file [SEC-052] [QA-008]', () {
      final attack = Uint8List.fromList(_sealed)..[_sealed.length - 1] ^= 1;
      expect(
        () => decryptBundle(attack, _hashOf(_sealed), _key),
        _throwsCode(PluxErrorCode.sectionHashMismatch),
      );
    });

    test('a truncated file is refused [SEC-052] [QA-008]', () {
      for (final length in [0, 16, 47, 48, _sealed.length - 1]) {
        final attack = Uint8List.sublistView(_sealed, 0, length);
        expect(
          () => decryptBundle(attack, _hashOf(_sealed), _key),
          throwsA(isA<PluxException>()),
          reason: '$length bytes',
        );
      }
    });
  });

  group('keys and releases', () {
    test('a key of another release does not open this one [SEC-053] '
        '[QA-008]', () {
      final other = Uint8List.fromList(_key)..[0] ^= 1;
      expect(
        () => decryptBundle(_sealed, _hashOf(_sealed), other),
        _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
      );
    });

    test('an older release replayed after an update opens under its own key '
        'only, and is not the bundle the newer manifest signs [SEC-052] '
        '[SEC-053] [QA-008]', () {
      // The newer release: the same sections under another ID, so another
      // bundle hash, with its own key.
      final newer = _resealed(parsed.kind, [
        _with(sections[0], id: Uint8List(16)..[0] = 9),
        ...sections.skip(1),
      ]);
      final newerKey = Uint8List.fromList(_key)..[0] ^= 1;
      // The older bundle against the newer manifest's hash.
      final spy = _Spy();
      expect(
        () => decryptBundle(_sealed, _hashOf(newer), newerKey, aead: spy),
        _throwsCode(PluxErrorCode.bundleMalformed),
      );
      expect(spy.opened, 0, reason: 'nothing decrypted before the refusal');
      // The older bundle with its own hash, but under the newer key.
      expect(
        () => decryptBundle(_sealed, _hashOf(_sealed), newerKey),
        _throwsCode(PluxErrorCode.confidentialKeyUnavailable),
      );
    });

    test('a content key of the wrong length opens nothing [SEC-053] '
        '[QA-008]', () {
      for (final key in [Uint8List(0), Uint8List(16), Uint8List(33)]) {
        expect(
          () => decryptBundle(_sealed, _hashOf(_sealed), key),
          throwsA(anything),
          reason: '${key.length} bytes',
        );
      }
    });
  });
}

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
