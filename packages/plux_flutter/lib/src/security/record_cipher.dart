// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The record format of the encrypted local stores (SEC-073, ADR-0058):
/// a version byte, a random 12-byte nonce, the ciphertext and the 16-byte
/// tag, with the store's name and the record's key as additional
/// authenticated data, so a record copied to another store or another key
/// fails to open.
///
/// A key seals at most a few thousand records of one store, far below the
/// 2^32 messages after which random 96-bit nonces risk a repeat.
library;

import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:plux_flutter/src/security/aead.dart';

/// The version byte of the record format. A byte that is neither this nor
/// a byte an older store wrote is a record this runtime cannot read.
const int recordVersion = 1;

/// The bytes a record adds to its plaintext: the version byte, the nonce
/// and the tag.
const int recordOverhead = 1 + aeadNonceLength + aeadTagLength;

/// Seals and opens the records of one [Aead] under data keys.
final class RecordCipher {
  /// Creates the cipher over [aead].
  const RecordCipher(this.aead);

  /// The AES-256-GCM implementation.
  final Aead aead;

  /// A fresh random nonce; [random] is for tests.
  static Uint8List newNonce([Random? random]) {
    final r = random ?? Random.secure();
    return Uint8List.fromList([
      for (var i = 0; i < aeadNonceLength; i++) r.nextInt(256),
    ]);
  }

  /// The additional data of a record: the store's name, length-prefixed so
  /// no store and key pair reads as another, then the record key.
  static Uint8List aadOf(String store, String recordKey) {
    final s = utf8.encode(store);
    final k = utf8.encode(recordKey);
    if (s.length > 0xffff) throw ArgumentError.value(store.length, 'store');
    return Uint8List(2 + s.length + k.length)
      ..[0] = s.length >> 8
      ..[1] = s.length & 0xff
      ..setAll(2, s)
      ..setAll(2 + s.length, k);
  }

  /// The record holding [plaintext] under [key], bound to [store] and
  /// [recordKey]. [nonce] is for tests; a fresh one is made when null.
  Uint8List seal(
    Uint8List key,
    String store,
    String recordKey,
    Uint8List plaintext, {
    Uint8List? nonce,
  }) {
    final n = nonce ?? newNonce();
    final sealed = aead.seal(key, n, plaintext, aadOf(store, recordKey));
    return Uint8List(1 + aeadNonceLength + sealed.length)
      ..[0] = recordVersion
      ..setAll(1, n)
      ..setAll(1 + aeadNonceLength, sealed);
  }

  /// The plaintext of [record], or null when it is not a record of this
  /// format, or does not authenticate under [key] for [store] and
  /// [recordKey].
  Uint8List? open(
    Uint8List key,
    String store,
    String recordKey,
    Uint8List record,
  ) {
    if (!isRecord(record)) return null;
    return aead.open(
      key,
      Uint8List.sublistView(record, 1, 1 + aeadNonceLength),
      Uint8List.sublistView(record, 1 + aeadNonceLength),
      aadOf(store, recordKey),
    );
  }

  /// Whether [bytes] start like a record of this format: the version byte
  /// and room for a nonce and a tag. It says nothing of authenticity.
  static bool isRecord(Uint8List bytes) =>
      bytes.length >= recordOverhead && bytes[0] == recordVersion;
}
