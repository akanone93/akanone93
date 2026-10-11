// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/native/plux_native.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/platform_aead.dart';

/// A stand-in for the platform that is correct: [DartAead] behind the
/// native call's signature, with the native results for a failed open.
int _correct({
  required bool open,
  required Uint8List key,
  required Uint8List nonce,
  required Uint8List aad,
  required Uint8List input,
  required Uint8List output,
}) {
  const aead = DartAead();
  if (!open) {
    final sealed = aead.seal(key, nonce, input, aad);
    output.setAll(0, sealed);
    return sealed.length;
  }
  final clear = aead.open(key, nonce, input, aad);
  if (clear == null) return NativeAeadResult.authentication;
  output.setAll(0, clear);
  return clear.length;
}

void main() {
  final key = Uint8List.fromList(List.generate(32, (i) => i));
  final nonce = Uint8List.fromList(List.generate(12, (i) => 100 + i));
  final aad = Uint8List.fromList([1, 2, 3]);

  test('the platform answer is used when it agrees with the reference '
      '[SEC-073]', () {
    final aead = PlatformAead.tryCreate(call: _correct);
    expect(aead, isNotNull);
    for (final n in [0, 1, 15, 64, 1024, 65536]) {
      final clear = Uint8List.fromList(List.generate(n, (i) => i % 251));
      final sealed = aead!.seal(key, nonce, clear, aad);
      expect(sealed, const DartAead().seal(key, nonce, clear, aad));
      expect(aead.open(key, nonce, sealed, aad), clear);
    }
  });

  test('a tag that does not verify opens to null; short input too '
      '[SEC-073]', () {
    final aead = PlatformAead.tryCreate(call: _correct)!;
    final sealed = aead.seal(key, nonce, Uint8List.fromList([7, 8, 9]), aad);
    final altered = Uint8List.fromList(sealed)..[sealed.length - 1] ^= 1;
    expect(aead.open(key, nonce, altered, aad), isNull);
    expect(aead.open(key, nonce, sealed, Uint8List(0)), isNull);
    expect(aead.open(key, nonce, Uint8List(3), aad), isNull);
  });

  test('a key or nonce of the wrong length is refused [SEC-073]', () {
    final aead = PlatformAead.tryCreate(call: _correct)!;
    final empty = Uint8List(0);
    expect(
      () => aead.seal(Uint8List(31), nonce, empty, aad),
      throwsArgumentError,
    );
    expect(
      () => aead.open(key, Uint8List(11), Uint8List(16), aad),
      throwsArgumentError,
    );
  });

  test('a platform that is missing, wrong or failing is not used '
      '[SEC-073]', () {
    int unsupported({
      required bool open,
      required Uint8List key,
      required Uint8List nonce,
      required Uint8List aad,
      required Uint8List input,
      required Uint8List output,
    }) => NativeAeadResult.unsupported;

    // Seals to other bytes than the reference does.
    int wrongCipher({
      required bool open,
      required Uint8List key,
      required Uint8List nonce,
      required Uint8List aad,
      required Uint8List input,
      required Uint8List output,
    }) {
      final n = _correct(
        open: open,
        key: key,
        nonce: nonce,
        aad: aad,
        input: input,
        output: output,
      );
      if (n > 0) output[0] ^= 1;
      return n;
    }

    // Opens whatever it is given.
    int noAuthentication({
      required bool open,
      required Uint8List key,
      required Uint8List nonce,
      required Uint8List aad,
      required Uint8List input,
      required Uint8List output,
    }) {
      if (!open) {
        return _correct(
          open: open,
          key: key,
          nonce: nonce,
          aad: aad,
          input: input,
          output: output,
        );
      }
      final n = input.length - aeadTagLength;
      output.setAll(0, input.sublist(0, n));
      return n;
    }

    expect(PlatformAead.tryCreate(call: unsupported), isNull);
    expect(PlatformAead.tryCreate(call: wrongCipher), isNull);
    expect(PlatformAead.tryCreate(call: noAuthentication), isNull);
    expect(
      PlatformAead.tryCreate(
        call: ({
          required open,
          required key,
          required nonce,
          required aad,
          required input,
          required output,
        }) => throw StateError('boom'),
      ),
      isNull,
    );
  });

  test('the native entry point answers on this host, and platformAead '
      'agrees with the reference [SEC-073]', () {
    // On a host with no platform implementation the entry point says so;
    // where there is one, the platform is used and must agree.
    final r = nativeAead(
      open: false,
      key: key,
      nonce: nonce,
      aad: aad,
      input: Uint8List(4),
      output: Uint8List(20),
    );
    expect(
      r == NativeAeadResult.unsupported || r == 20,
      isTrue,
      reason: 'got $r',
    );
    final clear = Uint8List.fromList([1, 2, 3, 4, 5]);
    final sealed = platformAead.seal(key, nonce, clear, aad);
    expect(sealed, const DartAead().seal(key, nonce, clear, aad));
    expect(platformAead.open(key, nonce, sealed, aad), clear);
  });

  test('the entry point refuses bad arguments instead of crashing '
      '[SEC-073]', () {
    int call({
      required bool open,
      Uint8List? k,
      Uint8List? n,
      required Uint8List input,
      required int outLength,
    }) => nativeAead(
      open: open,
      key: k ?? key,
      nonce: n ?? nonce,
      aad: aad,
      input: input,
      output: Uint8List(outLength),
    );
    // A buffer too small, and an open of fewer bytes than a tag, never
    // reach the platform.
    expect(
      call(open: false, input: Uint8List(8), outLength: 8),
      NativeAeadResult.capacity,
    );
    expect(
      call(open: true, input: Uint8List(3), outLength: 8),
      NativeAeadResult.authentication,
    );
  });
}
