// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// AES-256-GCM by the platform (SEC-073, ADR-0058): `javax.crypto` on
/// Android, CryptoKit on iOS, reached by one synchronous FFI call per
/// record into `plux_native` (`native/plux_native.c`), so stores and
/// confidential bundles run it on any isolate without a platform channel,
/// which cannot answer synchronously.
///
/// Where the platform has none — desktop development, tests, a build
/// whose plugin is absent — [platformAead] answers [DartAead], and no
/// caller sees a difference but speed.
library;

import 'dart:typed_data';

import 'package:plux_flutter/src/native/plux_native.dart';
import 'package:plux_flutter/src/security/aead.dart';

/// The native call [PlatformAead] makes: [nativeAead]'s signature, so
/// tests can stand in for the platform.
typedef AeadCall = int Function({
  required bool open,
  required Uint8List key,
  required Uint8List nonce,
  required Uint8List aad,
  required Uint8List input,
  required Uint8List output,
});

/// The platform's AES-256-GCM. Build it with [PlatformAead.tryCreate],
/// which proves the platform answers correctly before it is trusted.
final class PlatformAead implements Aead {
  const PlatformAead._(this._call);

  /// The platform's implementation, or null when the platform has none or
  /// its answer to a known vector is wrong: a native path that fails the
  /// check is never used, so a bad platform build degrades to [DartAead]
  /// instead of storing records no other implementation can read.
  ///
  /// [call] replaces the native call in tests.
  static PlatformAead? tryCreate({AeadCall call = nativeAead}) {
    final aead = PlatformAead._(call);
    try {
      return _knownAnswer(aead) ? aead : null;
    } on Object {
      return null;
    }
  }

  final AeadCall _call;

  @override
  Uint8List seal(
    Uint8List key,
    Uint8List nonce,
    Uint8List plaintext,
    Uint8List aad,
  ) {
    _check(key, nonce);
    final out = Uint8List(plaintext.length + aeadTagLength);
    final n = _call(
      open: false,
      key: key,
      nonce: nonce,
      aad: aad,
      input: plaintext,
      output: out,
    );
    if (n != out.length) throw StateError('the platform AES-GCM failed ($n)');
    return out;
  }

  @override
  Uint8List? open(
    Uint8List key,
    Uint8List nonce,
    Uint8List sealed,
    Uint8List aad,
  ) {
    _check(key, nonce);
    if (sealed.length < aeadTagLength) return null;
    final length = sealed.length - aeadTagLength;
    // A zero-length buffer has no address to pass.
    final out = Uint8List(length == 0 ? 1 : length);
    final n = _call(
      open: true,
      key: key,
      nonce: nonce,
      aad: aad,
      input: sealed,
      output: out,
    );
    if (n == NativeAeadResult.authentication) return null;
    if (n != length) throw StateError('the platform AES-GCM failed ($n)');
    return length == 0 ? Uint8List(0) : out;
  }
}

/// Whether the platform seals a fixed input to the bytes [DartAead] does,
/// opens them again, and refuses an altered record: a build whose native
/// path disagrees is not used.
bool _knownAnswer(PlatformAead aead) {
  const reference = DartAead();
  final key = Uint8List.fromList([for (var i = 0; i < 32; i++) i]);
  final nonce = Uint8List.fromList([for (var i = 0; i < 12; i++) 100 + i]);
  final aad = Uint8List.fromList([1, 2, 3]);
  final clear = Uint8List.fromList([for (var i = 0; i < 40; i++) i * 3]);
  final expected = reference.seal(key, nonce, clear, aad);
  final sealed = aead.seal(key, nonce, clear, aad);
  if (!_same(sealed, expected)) return false;
  final opened = aead.open(key, nonce, sealed, aad);
  if (opened == null || !_same(opened, clear)) return false;
  final altered = Uint8List.fromList(sealed)..[0] ^= 1;
  return aead.open(key, nonce, altered, aad) == null;
}

bool _same(Uint8List a, Uint8List b) {
  if (a.length != b.length) return false;
  var diff = 0;
  for (var i = 0; i < a.length; i++) {
    diff |= a[i] ^ b[i];
  }
  return diff == 0;
}

void _check(Uint8List key, Uint8List nonce) {
  if (key.length != aeadKeyLength || nonce.length != aeadNonceLength) {
    throw ArgumentError('AES-256-GCM takes a 32-byte key and a 12-byte nonce');
  }
}

/// The best AES-256-GCM this isolate has: the platform's, proven by
/// [PlatformAead.tryCreate], else [DartAead]. Decided once per isolate.
Aead get platformAead => _best;

final Aead _best = PlatformAead.tryCreate() ?? const DartAead();
