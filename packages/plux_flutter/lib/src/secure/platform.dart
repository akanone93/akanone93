// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The platform calls of the secure UI (SEC-091, SEC-093, SEC-094, ADR-0059)
/// through the runtime's own platform channel: the biometric prompt and the
/// protection of the screen while a secure page shows. Each is an interface
/// so tests run the secure UI on fakes.
library;

import 'package:flutter/services.dart';
import 'package:plux_flutter/src/device/types.dart';

const _channel = MethodChannel('dev.plux/runtime');

/// How a request for the user's confirmation ended.
enum BiometricOutcome {
  /// The user confirmed with a biometric or the device credential.
  success,

  /// The user dismissed the prompt.
  cancelled,

  /// The confirmation failed: the biometric was not recognised, or the
  /// platform locked the biometric out after too many attempts.
  failed,

  /// The device cannot ask: it has no biometric hardware, nothing is
  /// enrolled, or it has no screen lock.
  unavailable;

  /// The outcome the platform names with [code], or [failed] for a code it
  /// does not know: an unknown answer is never a confirmation.
  static BiometricOutcome fromCode(Object? code) => switch (code) {
    'success' => success,
    'cancelled' => cancelled,
    'unavailable' => unavailable,
    _ => failed,
  };
}

/// The platform's biometric prompt (`biometricAuth`, `BiometricButton`, the
/// inactivity lock).
abstract interface class BiometricPlatform {
  /// Asks the user to confirm, showing [reason] in the system prompt, with
  /// a biometric or, when [allowDeviceCredential] is set, the device PIN,
  /// pattern or password. Fails with a [PluxDeviceException] when the call
  /// itself fails; the answer never holds biometric data.
  Future<BiometricOutcome> authenticate({
    required String reason,
    required bool allowDeviceCredential,
  });
}

/// The runtime's platform channel, `dev.plux/runtime`, implemented by
/// `PluxFlutterPlugin` with `BiometricPrompt` on Android (androidx.biometric)
/// and `LAContext.evaluatePolicy` on iOS.
final class ChannelBiometricPlatform implements BiometricPlatform {
  /// Creates the platform.
  const ChannelBiometricPlatform();

  @override
  Future<BiometricOutcome> authenticate({
    required String reason,
    required bool allowDeviceCredential,
  }) async {
    try {
      return BiometricOutcome.fromCode(
        await _channel.invokeMethod<String>('biometricAuth', {
          'reason': reason,
          'allowDeviceCredential': allowDeviceCredential,
        }),
      );
    } on PlatformException catch (e) {
      throw PluxDeviceException.unavailable(
        'the biometric prompt failed: ${e.code}',
      );
    } on MissingPluginException {
      throw const PluxDeviceException.unavailable(
        'this platform has no biometric prompt',
      );
    }
  }
}

/// The platform's one-time-code delivery by SMS (`OtpInput`, SEC-090): the
/// SMS User Consent API on Android, which asks the user to share one
/// message and needs no SMS permission.
abstract interface class SmsConsentPlatform {
  /// Waits for an SMS the user agrees to share and completes with its text,
  /// or null when the user declines, nothing arrives in time, or [cancel]
  /// ends the wait. Fails with a [PluxDeviceException] when the call
  /// itself fails. The text holds a secret and is never logged (SEC-092).
  Future<String?> awaitSmsConsent();

  /// Ends a wait that [awaitSmsConsent] started; harmless when none runs.
  Future<void> cancel();
}

/// The runtime's platform channel, `dev.plux/runtime`, implemented by
/// `PluxSmsConsent` on Android; iOS fills codes through the keyboard.
final class ChannelSmsConsentPlatform implements SmsConsentPlatform {
  /// Creates the platform.
  const ChannelSmsConsentPlatform();

  @override
  Future<String?> awaitSmsConsent() async {
    try {
      return await _channel.invokeMethod<String>('smsConsent');
    } on PlatformException catch (e) {
      throw PluxDeviceException.unavailable(
        'the SMS consent failed: ${e.code}',
      );
    } on MissingPluginException {
      throw const PluxDeviceException.unavailable(
        'this platform has no SMS consent',
      );
    }
  }

  @override
  Future<void> cancel() async {
    try {
      await _channel.invokeMethod<void>('smsConsentCancel');
    } on PlatformException {
      // Nothing to end.
    } on MissingPluginException {
      // Nothing to end.
    }
  }
}

/// The platform's protection of the screen while a secure page shows.
abstract interface class SecureScreenPlatform {
  /// Turns the protection on or off. On Android it sets `FLAG_SECURE`, which
  /// blocks screenshots and screen recording and blurs the app switcher
  /// (SEC-091), and makes the window drop touches while another window
  /// covers it (SEC-093). On iOS it covers the window while the screen is
  /// captured and in the app switcher (SEC-091). Fails with a
  /// [PluxDeviceException] when the platform cannot.
  Future<void> setProtected({required bool enabled});
}

/// The runtime's platform channel, `dev.plux/runtime`.
final class ChannelSecureScreenPlatform implements SecureScreenPlatform {
  /// Creates the platform.
  const ChannelSecureScreenPlatform();

  @override
  Future<void> setProtected({required bool enabled}) async {
    try {
      await _channel.invokeMethod<void>('secureScreen', {'enabled': enabled});
    } on PlatformException catch (e) {
      throw PluxDeviceException.unavailable(
        'the screen protection failed: ${e.code}',
      );
    } on MissingPluginException {
      throw const PluxDeviceException.unavailable(
        'this platform cannot protect the screen',
      );
    }
  }
}
