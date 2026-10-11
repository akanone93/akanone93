// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The response cache of data sources (DAT-010, LIM-004): entries keyed by
/// a hash of the request, each with the time it was stored, behind a
/// [CacheStore] — in memory, or in files, optionally encrypted with
/// AES-GCM under a key from the key provider (D6). The cache keeps within
/// `data.cacheBytes` and `data.cacheEntries` by evicting the least
/// recently used entries; a store that fails degrades to no cache.
library;

import 'dart:collection';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:crypto/crypto.dart' as crypto;
import 'package:cryptography/cryptography.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/record_cipher.dart';
import 'package:plux_flutter/src/store/directory_sync.dart';

/// A cached response.
final class CachedEntry {
  /// Creates an entry.
  const CachedEntry(this.json, this.storedAtMs, this.bytes);

  /// The response's JSON.
  final Object? json;

  /// When it was stored, in milliseconds since the epoch.
  final int storedAtMs;

  /// Its size in the store.
  final int bytes;
}

/// What the cache knows of a stored entry without reading it.
typedef CacheIndexEntry = ({String key, int bytes, int storedAtMs});

/// Where cached responses live.
abstract interface class CacheStore {
  /// The entry at [key], or null.
  Future<CachedEntry?> read(String key);

  /// Stores [json] at [key]; completes with the bytes it takes.
  Future<int> write(String key, Object? json, int storedAtMs);

  /// Removes the entry at [key].
  Future<void> remove(String key);

  /// Removes every entry.
  Future<void> clear();

  /// The stored entries.
  Future<List<CacheIndexEntry>> entries();
}

/// Entries in memory, for runtimes without storage and for tests.
final class MemoryCacheStore implements CacheStore {
  final Map<String, CachedEntry> _entries = {};

  @override
  Future<CachedEntry?> read(String key) async => _entries[key];

  @override
  Future<int> write(String key, Object? json, int storedAtMs) async {
    final bytes = utf8.encode(jsonEncode(json)).length;
    _entries[key] = CachedEntry(json, storedAtMs, bytes);
    return bytes;
  }

  @override
  Future<void> remove(String key) async => _entries.remove(key);

  @override
  Future<void> clear() async => _entries.clear();

  @override
  Future<List<CacheIndexEntry>> entries() async => [
    for (final e in _entries.entries)
      (key: e.key, bytes: e.value.bytes, storedAtMs: e.value.storedAtMs),
  ];
}

/// Entries in files of one directory, one file per key: the time stored,
/// then the JSON — or, with a [key], the JSON sealed with AES-256-GCM, its
/// file name as associated data so an entry cannot be moved to another
/// key — or, with a [key] and a [cipher], the JSON as a record of ADR-0058
/// (SEC-073) bound to [storeName] and the entry's key. It does file I/O, so
/// apps run it on the data isolate (L-6).
final class FileCacheStore implements CacheStore {
  /// Creates the store in [directory]; [key] (32 bytes) encrypts it, with
  /// the record format of [cipher] when one is given.
  FileCacheStore(
    this.directory, {
    List<int>? key,
    RecordCipher? cipher,
    this.storeName = 'data-cache',
  }) : _key = key == null ? null : SecretKey(List.of(key)),
       _raw = key == null ? null : Uint8List.fromList(key),
       _cipher = key == null ? null : cipher {
    if (key != null && key.length != 32) {
      throw ArgumentError.value(key.length, 'key', 'must be 32 bytes');
    }
  }

  /// The directory.
  final String directory;

  /// The store's name in the additional data of its records.
  final String storeName;

  final SecretKey? _key;
  final Uint8List? _raw;
  final RecordCipher? _cipher;
  static final _aes = AesGcm.with256bits();

  File _file(String key) {
    if (!RegExp(r'^[0-9a-f]{64}$').hasMatch(key)) {
      throw ArgumentError.value(key, 'key', 'is not a SHA-256 hex digest');
    }
    return File('$directory/$key');
  }

  @override
  Future<CachedEntry?> read(String key) async {
    final f = _file(key);
    if (!f.existsSync()) return null;
    final data = await f.readAsBytes();
    if (data.length < 8) return null;
    final at = ByteData.sublistView(data, 0, 8).getInt64(0);
    var payload = Uint8List.sublistView(data, 8);
    if (_cipher case final c?) {
      if (!RecordCipher.isRecord(payload)) {
        // Written before the profile encrypted the cache: sealed now.
        return _migrateEntry(f, key, at, payload);
      }
      final clear = c.open(_raw!, storeName, key, payload);
      if (clear == null) {
        // Another key or tampered: never readable, so forget it.
        await f.delete();
        return null;
      }
      payload = clear;
    } else if (_key case final k?) {
      try {
        final box = SecretBox.fromConcatenation(
          payload,
          nonceLength: 12,
          macLength: 16,
        );
        payload = Uint8List.fromList(
          await _aes.decrypt(box, secretKey: k, aad: utf8.encode(key)),
        );
      } on SecretBoxAuthenticationError {
        // Another key or tampered: never readable, so forget it.
        await f.delete();
        return null;
      }
    }
    try {
      return CachedEntry(jsonDecode(utf8.decode(payload)), at, data.length);
    } on FormatException {
      await f.delete();
      return null;
    }
  }

  /// Seals the plain entry [plain] of [f] in place and returns it; an entry
  /// that is not JSON (an encrypted one of the earlier format, whose key is
  /// not this one) is removed.
  Future<CachedEntry?> _migrateEntry(
    File f,
    String key,
    int at,
    Uint8List plain,
  ) async {
    final Object? json;
    try {
      json = jsonDecode(utf8.decode(plain));
    } on FormatException {
      await f.delete();
      return null;
    }
    final bytes = await write(key, json, at);
    return CachedEntry(json, at, bytes);
  }

  /// Seals every entry written before the profile encrypted the cache, in
  /// place and atomically; entries that cannot be read are removed. A
  /// no-op for a store that does not use the record format.
  Future<void> migrate() async {
    if (_cipher == null) return;
    final d = Directory(directory);
    if (!d.existsSync()) return;
    await for (final f in d.list()) {
      final name = f.uri.pathSegments.last;
      if (f is File && name.startsWith('.') && name.endsWith('.tmp')) {
        // A plain entry a crashed write left behind.
        await f.delete();
        continue;
      }
      if (f is! File || !RegExp(r'^[0-9a-f]{64}$').hasMatch(name)) continue;
      final head = await f.openRead(0, 9).expand((b) => b).toList();
      if (head.length < 9 || head[8] == recordVersion) continue;
      await read(name);
    }
  }

  @override
  Future<int> write(String key, Object? json, int storedAtMs) async {
    var payload = Uint8List.fromList(utf8.encode(jsonEncode(json)));
    if (_cipher case final c?) {
      payload = c.seal(_raw!, storeName, key, payload);
    } else if (_key case final k?) {
      final box = await _aes.encrypt(
        payload,
        secretKey: k,
        nonce: _aes.newNonce(),
        aad: utf8.encode(key),
      );
      payload = Uint8List.fromList(box.concatenation());
    }
    final out = BytesBuilder(copy: false)
      ..add((ByteData(8)..setInt64(0, storedAtMs)).buffer.asUint8List())
      ..add(payload);
    final bytes = out.takeBytes();
    Directory(directory).createSync(recursive: true);
    // Written aside, then renamed: a reader never sees half an entry.
    final tmp = File('$directory/.$key.tmp');
    await tmp.writeAsBytes(bytes, flush: true);
    await tmp.rename(_file(key).path);
    if (_cipher != null) syncDirectory(directory);
    return bytes.length;
  }

  @override
  Future<void> remove(String key) async {
    final f = _file(key);
    if (f.existsSync()) await f.delete();
  }

  @override
  Future<void> clear() async {
    final d = Directory(directory);
    if (d.existsSync()) await d.delete(recursive: true);
  }

  @override
  Future<List<CacheIndexEntry>> entries() async {
    final d = Directory(directory);
    if (!d.existsSync()) return const [];
    final out = <CacheIndexEntry>[];
    await for (final f in d.list()) {
      final name = f.uri.pathSegments.last;
      if (f is! File || !RegExp(r'^[0-9a-f]{64}$').hasMatch(name)) continue;
      final raf = await f.open();
      try {
        final head = await raf.read(8);
        if (head.length < 8) continue;
        out.add((
          key: name,
          bytes: await raf.length(),
          storedAtMs: ByteData.sublistView(head).getInt64(0),
        ));
      } finally {
        await raf.close();
      }
    }
    return out;
  }
}

/// The cache key of a request: a SHA-256 of what identifies it, so neither
/// file names nor the index reveal URLs or bodies.
String cacheKey(String identity) =>
    crypto.sha256.convert(utf8.encode(identity)).toString();

/// The cache over a store, within its limits.
final class ResponseCache {
  /// Creates the cache; [now] gives the time in milliseconds.
  ResponseCache(
    this.store, {
    required this.maxBytes,
    required this.maxEntries,
    required this.now,
  });

  /// The store.
  final CacheStore store;

  /// `data.cacheBytes`.
  final int maxBytes;

  /// `data.cacheEntries`.
  final int maxEntries;

  /// The clock.
  final int Function() now;

  // Least recently used first.
  final LinkedHashMap<String, int> _sizes = LinkedHashMap();
  int _bytes = 0;
  Future<void>? _loaded;

  Future<void> _load() => _loaded ??= () async {
    final all = await store.entries()
      ..sort((a, b) => a.storedAtMs.compareTo(b.storedAtMs));
    for (final e in all) {
      _sizes[e.key] = e.bytes;
      _bytes += e.bytes;
    }
    await _evict();
  }();

  /// The entry at [key] stored less than [ttl] ago, or null; a stale entry
  /// is kept for [read] with a longer TTL. Fails with a [DataFailure] when
  /// the store does.
  Future<CachedEntry?> read(String key, Duration ttl) => _guard(() async {
    await _load();
    if (!_sizes.containsKey(key)) return null;
    final e = await store.read(key);
    if (e == null) {
      _forget(key);
      return null;
    }
    _sizes[key] = _sizes.remove(key)!; // most recently used
    return now() - e.storedAtMs < ttl.inMilliseconds ? e : null;
  });

  /// Stores [json] at [key], evicting beyond the limits.
  Future<void> write(String key, Object? json) => _guard(() async {
    await _load();
    final bytes = await store.write(key, json, now());
    _forget(key);
    _sizes[key] = bytes;
    _bytes += bytes;
    await _evict();
  });

  /// Removes every entry, as on logout or `Plux.wipeData`.
  Future<void> clear() => _guard(() async {
    await store.clear();
    _sizes.clear();
    _bytes = 0;
  });

  void _forget(String key) {
    final old = _sizes.remove(key);
    if (old != null) _bytes -= old;
  }

  Future<void> _evict() async {
    while (_sizes.isNotEmpty &&
        (_bytes > maxBytes || _sizes.length > maxEntries)) {
      final oldest = _sizes.keys.first;
      _forget(oldest);
      await store.remove(oldest);
    }
  }

  static Future<T> _guard<T>(Future<T> Function() f) async {
    try {
      return await f();
    } on DataFailure {
      rethrow;
    } on Object catch (e) {
      throw DataFailure(
        ActionErrorKind.custom,
        PluxErrorCode.dataCacheUnavailable,
        'the response cache failed: ${e.runtimeType}',
      );
    }
  }
}
