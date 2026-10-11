// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The inactivity lock of secure pages (SEC-094, ADR-0059): after the idle
/// time the security settings give, secure content stays hidden until the
/// user confirms with a biometric or the device credential.
library;

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:plux_flutter/src/actions/clock.dart';
import 'package:plux_flutter/src/device/types.dart';
import 'package:plux_flutter/src/secure/platform.dart';
import 'package:plux_flutter/src/security/security_config.dart';

/// Locks secure pages after a time without activity (SEC-094).
///
/// Secure pages [attach] while they are on screen; the lock counts the
/// user's pointer and key events, and the edits the secure widgets report
/// with [activity], and engages when none arrives within
/// `inactivityLockTimeout`. It also engages when the app comes back to the
/// foreground after the timeout has passed. A locked page stays hidden,
/// whatever touches arrive, until [unlock] succeeds. With the setting
/// `inactivityLock` off nothing engages. Time comes from [clock], so tests
/// drive it.
final class InactivityLock extends ChangeNotifier {
  /// Creates the lock reading [settings].
  InactivityLock({
    required this.settings,
    this.clock = const ActionClock(),
    BiometricPlatform? biometrics,
  }) : _biometrics = biometrics ?? const ChannelBiometricPlatform() {
    settings.addListener(_arm);
  }

  /// The security settings in force (SEC-182).
  final ValueListenable<SecuritySettings> settings;

  /// The monotonic clock and timers.
  final ActionClock clock;

  final BiometricPlatform _biometrics;

  int _attached = 0;
  bool _locked = false;
  bool _asking = false;
  Duration _last = Duration.zero;
  Timer? _timer;

  /// Whether the setting is on, so that the lock can engage.
  bool get enabled => settings.value.flag(SecuritySetting.inactivityLock);

  /// The idle time before the lock engages.
  Duration get timeout => Duration(
    seconds: settings.value.number(SecuritySetting.inactivityLockTimeout),
  );

  /// Whether secure content is locked: a secure page shows nothing until
  /// [unlock] succeeds.
  bool get locked => _locked && enabled;

  /// Whether a confirmation prompt is open.
  bool get asking => _asking;

  /// A secure page appeared. The first one starts the idle time.
  void attach() {
    if (_attached++ > 0) return;
    _last = clock.now;
    GestureBinding.instance.pointerRouter.addGlobalRoute(_onPointer);
    HardwareKeyboard.instance.addHandler(_onKey);
    _arm();
  }

  /// A secure page went. The last one stops the timer and clears a lock:
  /// with no secure content on screen there is nothing to protect, and the
  /// next secure page starts again.
  void detach() {
    if (_attached == 0 || --_attached > 0) return;
    GestureBinding.instance.pointerRouter.removeGlobalRoute(_onPointer);
    HardwareKeyboard.instance.removeHandler(_onKey);
    _timer?.cancel();
    _timer = null;
    if (_locked) {
      _locked = false;
      notifyListeners();
    }
  }

  void _onPointer(PointerEvent event) => activity();

  bool _onKey(KeyEvent event) {
    activity();
    return false;
  }

  /// The user did something: the idle time starts again. Ignored while
  /// locked, so that touches on the lock screen do not unlock it.
  void activity() {
    if (_attached == 0 || _locked) return;
    _last = clock.now;
    _arm();
  }

  /// The app returned to the foreground: engages the lock when the idle
  /// time passed while it was away (the timer does not run then).
  void resumed() {
    if (_attached == 0 || _locked || !enabled) return;
    if (clock.now - _last >= timeout) _engage();
  }

  void _arm() {
    _timer?.cancel();
    _timer = null;
    if (_attached == 0) return;
    if (!enabled) {
      if (_locked) {
        _locked = false;
        notifyListeners();
      }
      return;
    }
    if (_locked) return;
    final left = timeout - (clock.now - _last);
    _timer = clock.timer(left.isNegative ? Duration.zero : left, _engage);
  }

  void _engage() {
    _timer = null;
    if (_locked || !enabled || _attached == 0) return;
    _locked = true;
    notifyListeners();
  }

  /// Asks the user to confirm, with a biometric or the device credential,
  /// and unlocks on success; the outcome tells the lock screen what to say.
  /// A confirmation already open is joined, not asked twice. A failure of
  /// the platform call leaves the lock engaged.
  Future<BiometricOutcome> unlock({required String reason}) async {
    if (_asking) return BiometricOutcome.cancelled;
    _asking = true;
    notifyListeners();
    var outcome = BiometricOutcome.failed;
    try {
      outcome = await _biometrics.authenticate(
        reason: reason,
        allowDeviceCredential: true,
      );
    } on PluxDeviceException {
      outcome = BiometricOutcome.unavailable;
    } finally {
      _asking = false;
    }
    if (outcome == BiometricOutcome.success) {
      _locked = false;
      _last = clock.now;
      _arm();
    }
    notifyListeners();
    return outcome;
  }

  @override
  void dispose() {
    settings.removeListener(_arm);
    _timer?.cancel();
    if (_attached > 0) {
      GestureBinding.instance.pointerRouter.removeGlobalRoute(_onPointer);
      HardwareKeyboard.instance.removeHandler(_onKey);
    }
    _attached = 0;
    super.dispose();
  }
}
