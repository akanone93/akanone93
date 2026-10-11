// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The confidential bundles of the active release, opened for this launch
/// (SEC-053, ADR-0055).
///
/// Bundles stay encrypted on disk. The first time a release is used in a
/// launch, its content key is unwrapped with the device's key-agreement key
/// and each confidential bundle is decrypted into memory on a background
/// isolate — after its hash matched the signed one — and read like any
/// bundle. Decrypted bytes live only in this object's memory, for this
/// launch, and are never written to disk; once a release is replaced they
/// are dropped.
library;

import 'dart:typed_data';

import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/aead.dart';
import 'package:plux_flutter/src/security/confidential.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/store/release_record.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

/// Opens the confidential bundles of releases, once per launch.
final class ConfidentialBundles {
  /// Creates the opener for one app and environment.
  ConfidentialBundles({
    required this.keys,
    required this.appId,
    required this.environment,
    this.aead = const DartAead(),
  });

  /// The device keys; the key-agreement key unwraps the content key.
  final DeviceKeys keys;

  /// The app.
  final String appId;

  /// The environment key.
  final String environment;

  /// The cipher that opens sections.
  final Aead aead;

  final Map<String, Future<Uint8List>> _plain = {};

  /// How many bundles have been decrypted in this launch.
  int get decrypted => _decrypted;
  int _decrypted = 0;

  /// The plain containers of the confidential bundles of [record], by
  /// plugin key (empty for the app bundle); empty when it has none. A bundle
  /// already opened in this launch is not opened again. Throws a
  /// [PluxException]: `PLX-6050` when the record has no content key or it
  /// does not open, and the bundle errors of the verifier otherwise.
  Future<Map<String, Uint8List>> open(
    ReleaseRecord record,
    ReleaseStore store,
  ) async {
    final sealed = record.bundles.where((b) => b.confidential).toList();
    if (sealed.isEmpty) return const {};
    Future<Uint8List>? key;
    Future<Uint8List> contentKey() => key ??= _unwrap(record);
    final out = <String, Uint8List>{};
    for (final b in sealed) {
      final pending = _plain[b.hash] ??= _decrypt(b, store, contentKey);
      try {
        out[b.key] = await pending;
      } on Object {
        // A failure is not remembered: the next use tries again.
        _plain.removeWhere((hash, _) => hash == b.hash);
        rethrow;
      }
    }
    return out;
  }

  Future<Uint8List> _unwrap(ReleaseRecord record) async {
    final envelope = record.contentKey;
    if (envelope == null) {
      throw PluxException(
        PluxErrorCode.confidentialKeyUnavailable,
        'release ${record.sequence} is confidential and has no content key',
      );
    }
    return await unwrapContentKey(
      envelope,
      keys: keys,
      alias: agreementKeyAlias(appId, environment),
      sequence: record.sequence,
    );
  }

  Future<Uint8List> _decrypt(
    RecordBundle b,
    ReleaseStore store,
    Future<Uint8List> Function() contentKey,
  ) async {
    final key = await contentKey();
    final plain = await decryptBundleFile(
      store.objectPath(ObjectKind.bundles, b.hash),
      hexDecode(b.hash),
      key,
      aead: aead,
    );
    _decrypted++;
    return plain;
  }

  /// Forgets every opened bundle except those of [hashes]: the release in
  /// use keeps its plain bundles, a replaced one gives its memory back.
  void retain(Set<String> hashes) {
    _plain.removeWhere((hash, _) => !hashes.contains(hash));
  }
}
