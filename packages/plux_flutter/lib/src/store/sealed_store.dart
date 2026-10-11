// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The encrypted file store of the `strict` and `maximum` profiles
/// (SEC-073, ADR-0058): a [PluxKeyValueStore] whose file is one record of
/// the format of `RecordCipher` — version byte, nonce, ciphertext, tag, the
/// store's name and the record key as additional data — sealed under a data
/// key that secure hardware wraps (`HardwareWrappedSecretStore`).
///
/// The store replaces the stores a runtime ran before the profile tightened
/// (ADR-0049), in place. The first load under the new profile reads the
/// file as it was — the plain JSON of `PlainFileStore`, or the `PXK1` file
/// of `EncryptedFileStore` under its software-wrapped key — and writes it
/// back as a record: a new file beside it, flushed, renamed over it, the
/// directory synced. A crash leaves the old file or the new one, never
/// neither; the old key is removed only after the rename.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:isolate';
import 'dart:math';
import 'dart:typed_data';

import 'package:plux_flutter/src/security/record_cipher.dart';
import 'package:plux_flutter/src/store/directory_sync.dart';
import 'package:plux_flutter/src/store/kv_store.dart';

/// The record key of the one record a store file holds.
const String _recordKey = 'entries';

/// What the first bytes of a store file say it is.
enum _Format { record, legacyEncrypted, plain, unknown }

/// A [PluxKeyValueStore] in one file holding a sealed record.
final class SealedFileStore implements PluxKeyValueStore {
  /// Creates the store at [path], named [name] in the records' additional
  /// data, whose data key is secret [keyName] of [secrets] (a store that
  /// wraps with secure hardware). Entries larger than [maxBytes], encoded,
  /// are refused.
  ///
  /// The file may still be in a format an earlier profile wrote. With
  /// [plainMigration] a plain JSON file is read and sealed. With
  /// [legacyKeyName] and [legacyLabel] an `EncryptedFileStore` file is read
  /// with its key from [legacySecrets] and sealed, and the old key is then
  /// removed.
  SealedFileStore({
    required this.path,
    required this.name,
    required this.secrets,
    required this.keyName,
    required this.maxBytes,
    required this.cipher,
    this.plainMigration = false,
    this.legacySecrets,
    this.legacyKeyName,
    this.legacyLabel,
    OffIsolate? offIsolate,
    Random? random,
  }) : _off = offIsolate ?? _isolateRun,
       _random = random; // ignore: prefer_initializing_formals

  /// The file.
  final String path;

  /// The store's name, authenticated with every record.
  final String name;

  /// Where the data key is kept, wrapped by secure hardware.
  final SecretStore secrets;

  /// The data key's name in [secrets].
  final String keyName;

  /// The largest encoded size of the entries.
  int maxBytes;

  /// The cipher of the records.
  final RecordCipher cipher;

  /// Whether a plain JSON file is migrated.
  final bool plainMigration;

  /// Where the key of a legacy encrypted file is kept.
  final SecretStore? legacySecrets;

  /// The name of that key in [legacySecrets].
  final String? legacyKeyName;

  /// The legacy file's label, which its records authenticated.
  final String? legacyLabel;

  final OffIsolate _off;
  final Random? _random;
  Uint8List? _key;

  Future<Uint8List?> _keyFor({required bool create}) async => _key ??=
      await installationKey(secrets, keyName, create: create, random: _random);

  @override
  Future<Map<String, Object?>> load() async {
    final file = path;
    if (!File(file).existsSync()) return {};
    final _Format format;
    try {
      format = await _off(() => _formatOf(file));
    } on FileSystemException {
      throw const StoreException(
        StoreFailure.unavailable,
        'reading the store file',
      );
    }
    switch (format) {
      case _Format.record:
        return _loadRecord();
      case _Format.legacyEncrypted when _hasLegacy:
        return _migrateLegacy();
      case _Format.plain when plainMigration:
        return _migratePlain();
      case _Format.legacyEncrypted || _Format.plain || _Format.unknown:
        await _remove();
        throw const StoreException(
          StoreFailure.corrupt,
          'the store file is in a format this profile cannot read',
        );
    }
  }

  bool get _hasLegacy =>
      legacySecrets != null && legacyKeyName != null && legacyLabel != null;

  Future<Map<String, Object?>> _loadRecord() async {
    final key = await _keyFor(create: false);
    if (key == null) {
      // A file without its key cannot be read: it is another
      // installation's, or the hardware key was lost with a restore.
      await _remove();
      throw const StoreException(
        StoreFailure.corrupt,
        'the store has no key on this installation',
      );
    }
    final file = path;
    final c = cipher;
    final store = name;
    final Map<String, Object?>? result;
    try {
      result = await _off(() => _open(file, c, key, store));
    } on FileSystemException {
      throw const StoreException(
        StoreFailure.unavailable,
        'reading the store file',
      );
    }
    if (result == null) {
      await _remove();
      throw const StoreException(
        StoreFailure.corrupt,
        'the store did not authenticate',
      );
    }
    return result;
  }

  Future<Map<String, Object?>> _migrateLegacy() async {
    final old = EncryptedFileStore(
      path: path,
      secrets: legacySecrets!,
      keyName: legacyKeyName!,
      label: legacyLabel!,
      maxBytes: maxBytes,
      offIsolate: _off,
    );
    // Removes the file and reports it when it does not authenticate.
    final entries = await old.load();
    await save(entries);
    try {
      await legacySecrets!.delete(legacyKeyName!);
    } on Object {
      throw const StoreException(
        StoreFailure.unavailable,
        'removing the earlier store key from secure storage',
      );
    }
    return entries;
  }

  Future<Map<String, Object?>> _migratePlain() async {
    final file = path;
    final Map<String, Object?>? entries;
    try {
      entries = await _off(() => _readPlain(file));
    } on FileSystemException {
      throw const StoreException(
        StoreFailure.unavailable,
        'reading the store file',
      );
    }
    if (entries == null) {
      await _remove();
      throw const StoreException(
        StoreFailure.corrupt,
        'the store file is not a JSON object',
      );
    }
    await save(entries);
    return entries;
  }

  @override
  Future<void> save(Map<String, Object?> entries) async {
    final key = (await _keyFor(create: true))!;
    final nonce = RecordCipher.newNonce(_random);
    final file = path;
    final c = cipher;
    final store = name;
    final limit = maxBytes;
    final bool ok;
    try {
      ok = await _off(() => _seal(file, c, key, store, nonce, entries, limit));
    } on FileSystemException {
      throw const StoreException(
        StoreFailure.unavailable,
        'writing the store file',
      );
    }
    if (!ok) {
      throw StoreException(
        StoreFailure.tooLarge,
        'the entries exceed $limit bytes',
      );
    }
  }

  @override
  Future<void> wipe() async {
    await _remove();
    _key = null;
    try {
      await secrets.delete(keyName);
      if (_hasLegacy) await legacySecrets!.delete(legacyKeyName!);
    } on Object {
      throw const StoreException(
        StoreFailure.unavailable,
        'removing the store key from secure storage',
      );
    }
  }

  Future<void> _remove() async {
    final f = File(path);
    if (f.existsSync()) await f.delete();
  }
}

Future<R> _isolateRun<R>(FutureOr<R> Function() work) => Isolate.run(work);

/// What [file] starts with, read off the UI isolate.
_Format _formatOf(String file) {
  final raf = File(file).openSync();
  try {
    final head = raf.readSync(4);
    if (head.isEmpty) return _Format.unknown;
    if (head.length == 4 &&
        head[0] == 0x50 &&
        head[1] == 0x58 &&
        head[2] == 0x4b &&
        head[3] == 0x31) {
      return _Format.legacyEncrypted;
    }
    if (head[0] == recordVersion) return _Format.record;
    if (head[0] == 0x7b) return _Format.plain; // "{"
    return _Format.unknown;
  } finally {
    raf.closeSync();
  }
}

/// Opens [file] off the UI isolate: the entries, or null when it does not
/// authenticate for [store] under [key].
Map<String, Object?>? _open(
  String file,
  RecordCipher cipher,
  Uint8List key,
  String store,
) {
  final clear = cipher.open(
    key,
    store,
    _recordKey,
    File(file).readAsBytesSync(),
  );
  if (clear == null) return null;
  try {
    final decoded = jsonDecode(utf8.decode(clear));
    return decoded is Map<String, Object?> ? decoded : null;
  } on FormatException {
    return null;
  }
}

/// Reads a plain JSON [file]: the entries, or null when it is not a JSON
/// object.
Map<String, Object?>? _readPlain(String file) {
  try {
    final decoded = jsonDecode(utf8.decode(File(file).readAsBytesSync()));
    return decoded is Map<String, Object?> ? decoded : null;
  } on FormatException {
    return null;
  }
}

/// Seals [entries] into [file] off the UI isolate, replacing it
/// atomically: a new file beside it, flushed, renamed, the directory
/// synced. False when they exceed [limit] bytes.
bool _seal(
  String file,
  RecordCipher cipher,
  Uint8List key,
  String store,
  Uint8List nonce,
  Map<String, Object?> entries,
  int limit,
) {
  final clear = Uint8List.fromList(utf8.encode(jsonEncode(entries)));
  if (clear.length > limit) return false;
  final record = cipher.seal(key, store, _recordKey, clear, nonce: nonce);
  final parent = File(file).parent..createSync(recursive: true);
  File('$file.tmp')
    ..writeAsBytesSync(record, flush: true)
    ..renameSync(file);
  syncDirectory(parent.path);
  return true;
}
