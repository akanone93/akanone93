// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What the device keeps of the control document (SYN-060, ADR-0077): the
/// content of the newest document it verified, its ETag, and whether its
/// forced rollback has run. It lives beside the release store's pointer
/// and is only ever replaced by an atomic rename. The kill switches in it
/// stay in force after the document expires, until a fresh one replaces
/// them (fail closed for switches only).
library;

import 'dart:convert';
import 'dart:io';

import 'package:plux_flutter/src/control/control_snapshot.dart';

/// The stored control state.
final class StoredControl {
  /// Creates the state.
  const StoredControl({
    required this.etag,
    required this.snapshot,
    this.rollbackAppliedVersion = 0,
  });

  /// Reads a stored state; null when it cannot be read.
  static StoredControl? decode(List<int> bytes) {
    try {
      final j = jsonDecode(utf8.decode(bytes)) as Map<String, Object?>;
      final snapshot = ControlSnapshot.fromJson(j['snapshot']);
      if (snapshot == null) return null;
      return StoredControl(
        etag: j['etag']! as String,
        snapshot: snapshot,
        rollbackAppliedVersion: j['rollbackAppliedVersion']! as int,
      );
    } on Object {
      return null;
    }
  }

  /// The ETag of the document, for `If-None-Match`.
  final String etag;

  /// The document's content.
  final ControlSnapshot snapshot;

  /// The version of the document whose forced rollback has been carried
  /// out; a rollback is acted on once, not at every launch.
  final int rollbackAppliedVersion;

  /// A copy with the given fields replaced.
  StoredControl copyWith({int? rollbackAppliedVersion}) => StoredControl(
    etag: etag,
    snapshot: snapshot,
    rollbackAppliedVersion:
        rollbackAppliedVersion ?? this.rollbackAppliedVersion,
  );

  /// The state as it is stored.
  List<int> encode() => utf8.encode(
    jsonEncode({
      'etag': etag,
      'snapshot': snapshot.toJson(),
      'rollbackAppliedVersion': rollbackAppliedVersion,
    }),
  );
}

/// The file the state is kept in, in the release store's directory.
final class ControlDocumentStore {
  /// Keeps the state in [directory] (the release store's root).
  ControlDocumentStore(String directory)
    : _path = '$directory/control-document';

  final String _path;

  /// The stored state, or null.
  StoredControl? read() {
    final f = File(_path);
    return f.existsSync() ? StoredControl.decode(f.readAsBytesSync()) : null;
  }

  /// Replaces the state atomically: write `.tmp`, `fsync`, rename.
  void write(StoredControl state) {
    final tmp = File('$_path.tmp');
    final f = tmp.openSync(mode: FileMode.write);
    try {
      f
        ..writeFromSync(state.encode())
        ..flushSync();
    } finally {
      f.closeSync();
    }
    tmp.renameSync(_path);
  }
}
