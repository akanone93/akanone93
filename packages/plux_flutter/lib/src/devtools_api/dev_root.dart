// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The organisation's development root on a Dev app phone (ADR-0109): the
/// root's public key, which the phone reads off the sign-in QR code in
/// Studio and pins, and the certificates by which it trusts each app's
/// development key. A key is trusted only for the app and the environment
/// its certificate names, only typed development (SEC-056), and only when
/// the root's signature of the exact statement verifies (SEC-052).
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart'
    show KeyPairType, Signature, SimplePublicKey;
import 'package:cryptography/dart.dart';
import 'package:plux_flutter/src/core/config.dart' show PluxPublicKey;

/// The type a development key certificate's statement names.
const String devKeyCertificateType = 'plux.dev-key-certificate';

/// The size of an Ed25519 public key, the root's and an app's.
const int _ed25519KeyBytes = 32;

/// Reads the development root a sign-in link carries: an Ed25519 public
/// key, unpadded base64url. Throws [FormatException] for anything else.
Uint8List parseDevelopmentRoot(String text) {
  final Uint8List root;
  try {
    root = base64Url.decode(base64Url.normalize(text.trim()));
  } on FormatException {
    throw const FormatException('the development root is not base64url');
  }
  if (root.length != _ed25519KeyBytes) {
    throw const FormatException('the development root is not an Ed25519 key');
  }
  return root;
}

/// Why a certified key was refused.
final class DevKeyRefused implements Exception {
  /// Creates the refusal.
  const DevKeyRefused(this.message);

  /// What was wrong, for the developer.
  final String message;

  @override
  String toString() => 'DevKeyRefused: $message';
}

/// The development key [certificate] certifies for [appId] and
/// [environment], once its signature verifies under [root]; throws
/// [DevKeyRefused] otherwise. [certificate] is the `developmentKey` of a
/// join's answer, `{statement, signature}` in base64.
Future<PluxPublicKey> certifiedDevelopmentKey(
  Uint8List root,
  Object? certificate, {
  required String appId,
  required String environment,
}) async {
  if (certificate is! Map<String, Object?>) {
    throw const DevKeyRefused('the server sent no certified key');
  }
  final Uint8List statement;
  final Uint8List signature;
  try {
    statement = base64.decode(certificate['statement']! as String);
    signature = base64.decode(certificate['signature']! as String);
  } on Object {
    throw const DevKeyRefused('the certificate cannot be read');
  }
  final ok = await DartEd25519(sha512: const DartSha512()).verify(
    statement,
    signature: Signature(
      signature,
      publicKey: SimplePublicKey(root, type: KeyPairType.ed25519),
    ),
  );
  if (!ok) {
    throw const DevKeyRefused(
      "the app's key is not certified by your organisation's development root",
    );
  }
  return _keyOf(statement, appId: appId, environment: environment);
}

/// The key a verified [statement] names, when it is the development key of
/// [appId] and [environment].
PluxPublicKey _keyOf(
  Uint8List statement, {
  required String appId,
  required String environment,
}) {
  final Map<String, Object?> s;
  try {
    s = jsonDecode(utf8.decode(statement)) as Map<String, Object?>;
  } on Object {
    throw const DevKeyRefused('the certificate cannot be read');
  }
  if (s['type'] != devKeyCertificateType || s['version'] != 1) {
    throw const DevKeyRefused('the certificate is not a development key');
  }
  // Development only: a production key is never trusted here (SEC-056).
  if (s['environmentType'] != 'development' ||
      s['role'] != 'targets' ||
      s['algorithm'] != 'ed25519') {
    throw const DevKeyRefused('the certificate is not a development key');
  }
  if (s['appId'] != appId || s['environment'] != environment) {
    throw const DevKeyRefused('the certificate is for another app');
  }
  final keyId = s['keyId'];
  final Uint8List publicKey;
  try {
    publicKey = base64.decode(s['publicKey']! as String);
  } on Object {
    throw const DevKeyRefused('the certificate names no key');
  }
  if (keyId is! String ||
      keyId.isEmpty ||
      publicKey.length != _ed25519KeyBytes) {
    throw const DevKeyRefused('the certificate names no key');
  }
  return PluxPublicKey(
    keyId: keyId,
    algorithm: 'ed25519',
    role: 'targets',
    publicKey: publicKey,
  );
}
