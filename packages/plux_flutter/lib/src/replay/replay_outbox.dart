// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The sync isolate's store of unsent session replays and their upload
/// (ANL-050, ADR-0101): one JSON file per replay in `<store>/replay/`,
/// bounded to `replay.perDevicePerDay` files with the oldest dropped first,
/// each sent with the device's token and DPoP proof like telemetry. Runs on
/// the sync isolate only (L-6): the encoding happens here, not on the UI
/// isolate.
library;

import 'dart:convert';
import 'dart:io';

import 'package:plux_flutter/src/sync/api_client.dart';

/// What one flush did.
final class ReplayFlush {
  /// Creates a report.
  const ReplayFlush({
    required this.sent,
    required this.dropped,
    required this.left,
  });

  /// Replays the server took.
  final int sent;

  /// Replays dropped because the server refused them for good.
  final int dropped;

  /// Replays still stored.
  final int left;
}

/// The store.
final class ReplayOutbox {
  /// Opens the store of the store root at [storeRoot].
  ReplayOutbox(String storeRoot) : _dir = Directory('$storeRoot/replay');

  final Directory _dir;

  static final RegExp _name = RegExp(r'^[0-9a-f-]{36}\.json$');

  List<File> _files() {
    if (!_dir.existsSync()) return [];
    return [
      for (final e in _dir.listSync())
        if (e is File && _name.hasMatch(e.uri.pathSegments.last)) e,
    ]..sort((a, b) => a.statSync().modified.compareTo(b.statSync().modified));
  }

  /// Replays stored.
  int get length => _files().length;

  /// Stores [replay], encoded here, and drops the oldest replays so that at
  /// most [maxFiles] stay.
  void append(Map<String, Object?> replay, int maxFiles) {
    final id = replay['id'];
    if (id is! String || !_name.hasMatch('$id.json')) return;
    _dir.createSync(recursive: true);
    final tmp = File('${_dir.path}/$id.tmp')
      ..writeAsStringSync(jsonEncode(replay), flush: true);
    tmp.renameSync('${_dir.path}/$id.json');
    final files = _files();
    for (var i = 0; i < files.length - maxFiles; i++) {
      files[i].deleteSync();
    }
  }

  /// Deletes every stored replay, whose consent was withdrawn (SEC-161).
  void purge() {
    for (final f in _files()) {
      f.deleteSync();
    }
  }

  /// Sends the stored replays, oldest first. A replay the server answers is
  /// removed; one it refused for good (a client error other than
  /// authentication) is dropped, so it is never retried for ever; a
  /// retryable failure or a lost token stops the flush.
  Future<ReplayFlush> flush({
    required PluxApiClient api,
    required Future<DeviceToken> Function() token,
    required String appId,
    required String environment,
  }) async {
    var sent = 0;
    var dropped = 0;
    for (final f in _files()) {
      try {
        final body = jsonDecode(f.readAsStringSync()) as Map<String, Object?>;
        await api.ingestReplay(await token(), {
          'appId': appId,
          'environment': environment,
          ...body,
        });
        sent++;
      } on ApiError catch (e) {
        if (e.retryable || e.code == 'unauthenticated') {
          return ReplayFlush(sent: sent, dropped: dropped, left: length);
        }
        dropped++;
      } on FormatException {
        dropped++;
      }
      f.deleteSync();
    }
    return ReplayFlush(sent: sent, dropped: dropped, left: length);
  }
}
