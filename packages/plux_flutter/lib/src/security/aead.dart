// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// AES-256-GCM, the one authenticated cipher of the runtime's encrypted
/// stores and confidential bundles (SEC-053, SEC-073, ADR-0055, ADR-0058).
///
/// Calls are synchronous: bulk work runs off the UI isolate (L-7), and a
/// platform implementation is one FFI call per record or section.
library;

import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:cryptography/dart.dart';

/// The key length in bytes: AES-256.
const int aeadKeyLength = 32;

/// The nonce length in bytes.
const int aeadNonceLength = 12;

/// The authentication tag's length in bytes, appended to the ciphertext.
const int aeadTagLength = 16;

/// AES-256-GCM with a 12-byte nonce and a 16-byte tag.
abstract interface class Aead {
  /// Encrypts [plaintext] under [key] and [nonce], authenticating [aad];
  /// the ciphertext followed by the tag.
  Uint8List seal(
    Uint8List key,
    Uint8List nonce,
    Uint8List plaintext,
    Uint8List aad,
  );

  /// Decrypts [sealed] (ciphertext then tag); null when the tag does not
  /// verify, whatever the reason: a wrong key, nonce or [aad], or altered
  /// bytes.
  Uint8List? open(
    Uint8List key,
    Uint8List nonce,
    Uint8List sealed,
    Uint8List aad,
  );
}

/// AES-256-GCM in pure Dart: tests, desktop development, and platforms
/// with no native implementation (ADR-0058).
final class DartAead implements Aead {
  /// Creates the cipher.
  const DartAead();

  @override
  Uint8List seal(
    Uint8List key,
    Uint8List nonce,
    Uint8List plaintext,
    Uint8List aad,
  ) {
    _check(key, nonce);
    final box = DartAesGcm.with256bits().encryptSync(
      plaintext,
      secretKeyData: SecretKeyData(key),
      nonce: nonce,
      aad: aad,
    );
    return Uint8List(box.cipherText.length + aeadTagLength)
      ..setAll(0, box.cipherText)
      ..setAll(box.cipherText.length, box.mac.bytes);
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
    final split = sealed.length - aeadTagLength;
    try {
      return Uint8List.fromList(
        DartAesGcm.with256bits().decryptSync(
          SecretBox(
            sealed.sublist(0, split),
            nonce: nonce,
            mac: Mac(sealed.sublist(split)),
          ),
          secretKeyData: SecretKeyData(key),
          aad: aad,
        ),
      );
    } on SecretBoxAuthenticationError {
      return null;
    }
  }
}

void _check(Uint8List key, Uint8List nonce) {
  if (key.length != aeadKeyLength || nonce.length != aeadNonceLength) {
    throw ArgumentError('AES-256-GCM takes a 32-byte key and a 12-byte nonce');
  }
}
