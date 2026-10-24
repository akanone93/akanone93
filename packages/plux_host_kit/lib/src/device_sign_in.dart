// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The auditor's sign-in by the device authorization grant (RFC 8628), the
/// flow `plux login` uses (CLI-002): the viewer shows a code, the auditor
/// approves it in a browser on any device, and the viewer receives a scoped,
/// expiring access token. The auditor's password never reaches the viewer.
library;

import 'dart:async';

import 'package:plux_host_kit/src/api.dart';

/// Why a sign-in did not complete.
enum SignInFailure {
  /// The approving person refused it.
  denied,

  /// The code lapsed before it was approved.
  expired,

  /// The auditor stopped waiting.
  cancelled,
}

/// A sign-in that ended without a token.
final class SignInException implements Exception {
  /// Creates the exception.
  const SignInException(this.failure);

  /// What happened.
  final SignInFailure failure;

  @override
  String toString() => switch (failure) {
    SignInFailure.denied => 'The sign-in was denied.',
    SignInFailure.expired => 'The code expired before it was approved.',
    SignInFailure.cancelled => 'The sign-in was cancelled.',
  };
}

/// Runs one device authorization against [api].
final class DeviceSignIn {
  /// Creates a sign-in. [wait] and [now] are the seams tests replace.
  DeviceSignIn(this.api, {this._wait = _sleep, this._now = DateTime.now});

  /// The unauthenticated client of the server being signed in to.
  final ViewerApi api;

  final Future<void> Function(Duration) _wait;
  final DateTime Function() _now;
  bool _cancelled = false;

  /// Stops [complete] at its next poll.
  void cancel() => _cancelled = true;

  /// Asks the server for a code to show.
  Future<DeviceGrant> start() => api.startDeviceAuthorization();

  /// Polls until the grant is approved, and returns the access token. Throws
  /// [SignInException] when it is denied, lapses or is cancelled. The poll
  /// interval is the server's, and grows when the server says to slow down.
  Future<String> complete(DeviceGrant grant) async {
    var interval = grant.interval;
    while (true) {
      await _wait(interval);
      if (_cancelled) throw const SignInException(SignInFailure.cancelled);
      if (!_now().isBefore(grant.expiresAt)) {
        throw const SignInException(SignInFailure.expired);
      }
      final poll = await api.pollDeviceAuthorization(grant.deviceCode);
      switch (poll.status) {
        case PollStatus.approved:
          final token = poll.token;
          if (token != null) return token;
        case PollStatus.denied:
          throw const SignInException(SignInFailure.denied);
        case PollStatus.slowDown:
          interval += const Duration(seconds: 5);
        case PollStatus.pending:
      }
    }
  }
}

Future<void> _sleep(Duration d) => Future<void>.delayed(d);
