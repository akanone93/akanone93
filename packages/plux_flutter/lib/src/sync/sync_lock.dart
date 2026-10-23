// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The process-wide sync lock (SYN-014): background sync runs in the app's
/// own process (WorkManager on Android, `BGAppRefreshTask` on iOS), in a
/// second Flutter engine beside the foreground runtime, and both write the
/// same release store. Isolates and engines share no memory and a `dart:io`
/// file lock is per process, so the lock is held natively and reached over
/// the runtime channel from both engines.
library;

import 'dart:async';

import 'package:flutter/services.dart';
import 'package:plux_flutter/src/platform/runtime_channel.dart';

/// A lock that at most one sync run in the process holds.
abstract interface class SyncLock {
  /// Takes the lock if it is free; false when another run holds it.
  Future<bool> tryAcquire();

  /// Releases the lock this caller took. Releasing a lock that is not held
  /// does nothing.
  Future<void> release();
}

/// The native lock behind the `dev.plux/runtime` channel.
///
/// A platform without the plugin has no second engine to contend with, so
/// there the lock is always free.
final class ChannelSyncLock implements SyncLock {
  /// Creates the lock over the runtime's channel.
  const ChannelSyncLock([this._channel = const RuntimeChannel()]);

  final RuntimeChannel _channel;

  @override
  Future<bool> tryAcquire() async {
    try {
      return await _channel.invokeMethod<bool>('syncLock.tryAcquire') ?? false;
    } on MissingPluginException {
      return true;
    }
  }

  @override
  Future<void> release() async {
    try {
      await _channel.invokeMethod<void>('syncLock.release');
    } on MissingPluginException {
      // Nothing was taken.
    }
  }
}

/// A lock in memory, for one isolate: the tests', and the stand-in where no
/// platform holds one.
final class InMemorySyncLock implements SyncLock {
  /// Creates a free lock.
  InMemorySyncLock();

  bool _held = false;

  /// Whether the lock is held.
  bool get held => _held;

  @override
  Future<bool> tryAcquire() async {
    if (_held) return false;
    return _held = true;
  }

  @override
  Future<void> release() async => _held = false;
}

/// How the foreground waits for the lock (SYN-014): a background run is
/// short, so it retries every [retryInterval] for up to [timeout], and then
/// goes on without the lock. The foreground is never blocked indefinitely.
final class ForegroundSyncLock {
  /// Creates the foreground side of [lock]. [onWarning] hears that the lock
  /// could not be taken in time, or could not be asked.
  ForegroundSyncLock(
    this._lock, {
    this.retryInterval = const Duration(milliseconds: 200),
    this.timeout = const Duration(seconds: 10),
    void Function(String message)? onWarning,
  }) : _onWarning = onWarning ?? ((_) {});

  final SyncLock _lock;

  /// The wait between two attempts.
  final Duration retryInterval;

  /// How long to try before going on without the lock.
  final Duration timeout;
  final void Function(String message) _onWarning;

  int _users = 0;
  Future<bool>? _acquiring;

  /// Runs [body] holding the lock, and releases it when [body] ends, however
  /// it ends. Runs that overlap in this process share one hold, so a second
  /// foreground sync never waits for the first.
  Future<T> guard<T>(Future<T> Function() body) async {
    _users++;
    final acquiring = _acquiring ??= _acquire();
    try {
      await acquiring;
      return await body();
    } finally {
      if (--_users == 0) {
        _acquiring = null;
        if (await acquiring) {
          try {
            await _lock.release();
          } on Object {
            // The platform drops a hold with its engine; nothing else to do.
          }
        }
      }
    }
  }

  Future<bool> _acquire() async {
    final deadline = DateTime.now().add(timeout);
    try {
      while (true) {
        if (await _lock.tryAcquire()) return true;
        if (!DateTime.now().add(retryInterval).isBefore(deadline)) break;
        await Future<void>.delayed(retryInterval);
      }
      _onWarning(
        'the sync lock is still held by a background sync after '
        '${timeout.inSeconds}s; syncing without it',
      );
    } on Object catch (e) {
      _onWarning(
        'the sync lock cannot be asked (${e.runtimeType}); '
        'syncing without it',
      );
    }
    return false;
  }
}
