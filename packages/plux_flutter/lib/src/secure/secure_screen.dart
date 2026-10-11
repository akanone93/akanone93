// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The protection of the screen while secure pages show (SEC-091,
/// SEC-093, ADR-0059): one switch for the whole window, held by every
/// secure page on screen and released when the last one goes.
library;

import 'dart:async';

import 'package:plux_flutter/src/device/types.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/secure/platform.dart';

/// Counts the secure pages on screen and turns the platform's protection on
/// for the first and off after the last, so that a page that opens before
/// another closes never unprotects the window. Pages hold a
/// [SecureScreenLease] from [acquire] for as long as they show.
final class SecureScreen {
  /// Creates the switch over [platform]; [report] receives a platform
  /// failure.
  SecureScreen(this.platform, {required this.report});

  /// The platform's protection.
  final SecureScreenPlatform platform;

  /// Reports a problem.
  final void Function(PluxException error) report;

  int _holders = 0;
  bool _reported = false;

  /// How many leases are held.
  int get holders => _holders;

  /// Whether any lease is held, so the window is protected.
  bool get protected => _holders > 0;

  /// Holds the protection until the lease is released. The platform is
  /// asked at once when this is the first lease.
  SecureScreenLease acquire() {
    if (_holders++ == 0) _apply(enabled: true);
    return SecureScreenLease._(this);
  }

  void _release() {
    if (--_holders == 0) _apply(enabled: false);
  }

  void _apply({required bool enabled}) {
    unawaited(
      platform
          .setProtected(enabled: enabled)
          .then<void>(
            (_) {},
            onError: (Object e) {
              // One report per runtime: a platform without the protection
              // would otherwise report on every page.
              if (_reported) return;
              _reported = true;
              report(
                PluxException(
                  PluxErrorCode.deviceUnavailable,
                  'a secure page cannot be protected: '
                  '${e is PluxDeviceException ? e.message : 'platform error'}',
                ),
              );
            },
          ),
    );
  }
}

/// One page's hold on the protection of the screen.
final class SecureScreenLease {
  SecureScreenLease._(this._screen);

  final SecureScreen _screen;
  bool _released = false;

  /// Lets go of the hold; releasing twice does nothing.
  void release() {
    if (_released) return;
    _released = true;
    _screen._release();
  }
}
