// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The `live` configuration values a device holds (DAT-094 to DAT-097), on
/// the sync isolate next to the gateway agent that makes the calls which
/// name their version.
///
/// The values live in this object's memory and nowhere else: they are never
/// written to disk, logged or traced. A newer epoch seen on a gateway
/// response refreshes them in the background; a `strict` value is read
/// from the server each time, and a read that cannot reach it fails closed.
library;

import 'dart:async';

import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/live_config.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';

/// Holds the `live` values of one app and environment.
final class LiveConfig {
  /// Creates the holder; [_fetch] asks the server for the current snapshot
  /// and fails with a [DataFailure].
  LiveConfig(this._fetch);

  final Future<LiveConfigSnapshot> Function() _fetch;
  LiveConfigSnapshot? _snapshot;
  Future<void>? _fetching;
  int _seenEpoch = 0;
  int _triedEpoch = 0;

  /// The version of the values held, or 0 when none were fetched: what
  /// `Plux-Config-Version` carries (DAT-096).
  int get version => _snapshot?.version ?? 0;

  /// Notes the headers of a gateway response: a `Plux-Config-Epoch` newer
  /// than the values held starts a refresh in the background (DAT-095).
  /// Nothing is fetched while no value has ever been read.
  void noteHeaders(Map<String, String> headers) {
    final epoch = int.tryParse(
      headers[pluxConfigEpochHeader.toLowerCase()] ?? '',
    );
    if (epoch == null) return;
    if (epoch > _seenEpoch) _seenEpoch = epoch;
    final held = _snapshot;
    if (held == null || _seenEpoch <= held.epoch || _seenEpoch <= _triedEpoch) {
      return;
    }
    _triedEpoch = _seenEpoch;
    // A failed background refresh is not an error: the next call that
    // names a stale version refreshes again, and a strict read fails closed.
    unawaited(refresh().then<void>((_) {}, onError: (Object _) {}));
  }

  /// Fetches the current values; one fetch at a time is shared, and a
  /// caller that needs values newer than a fetch already in flight asks
  /// again with [afresh]. Fails with a [DataFailure] when the server
  /// cannot be reached or refuses.
  Future<void> refresh({bool afresh = false}) async {
    final running = _fetching;
    if (running != null) {
      if (!afresh) return running;
      try {
        await running;
      } on Object {
        // The fetch below reports its own failure.
      }
      final again = _fetching;
      if (again != null) return again;
    }
    final fetch = _fetch().then((s) {
      _snapshot = s;
      if (s.epoch > _seenEpoch) _seenEpoch = s.epoch;
    });
    final tracked = fetch.whenComplete(() => _fetching = null);
    _fetching = tracked;
    return tracked;
  }

  /// The value of [key] (DAT-097); see [ConfigSource.read].
  Future<LiveConfigValue> read(String key) async {
    var fetched = false;
    if (_snapshot == null || _snapshot!.values[key] == null) {
      await refresh();
      fetched = true;
    }
    var value = _snapshot?.values[key];
    if (value != null && value.strict && !fetched) {
      try {
        await refresh();
      } on DataFailure catch (f) {
        throw _closed(key, f);
      }
      value = _snapshot?.values[key];
    }
    if (value == null) {
      throw DataFailure(
        ActionErrorKind.custom,
        PluxErrorCode.liveConfigUnavailable,
        'there is no live configuration value $key',
      );
    }
    return value;
  }

  /// Forgets every value, at logout.
  void clear() {
    _snapshot = null;
    _seenEpoch = 0;
    _triedEpoch = 0;
  }
}

DataFailure _closed(String key, DataFailure cause) => DataFailure(
  cause.kind,
  cause.code,
  'the strict live configuration value $key cannot be read now: '
  '${cause.code.id}',
  status: cause.status,
);
