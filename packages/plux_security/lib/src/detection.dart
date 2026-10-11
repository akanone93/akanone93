// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What can be detected (SEC-070): the kinds of hostile condition, how
/// serious each is, and one finding.
library;

/// How serious a kind of detection is; reported with every detection
/// (ADR-0057).
enum PluxSeverity {
  /// The condition is a risk to the user's screen or data only.
  low,

  /// The environment lets another party watch or steer the app.
  medium,

  /// The app or the platform is modified.
  high,
}

/// The kinds of hostile condition `plux_security` detects (SEC-070).
enum PluxDetectionKind {
  /// A rooted Android device or a jailbroken iPhone.
  root('root', PluxSeverity.high, sticky: true),

  /// A hooking framework: Frida, Xposed, LSPosed, Substrate and the like.
  hooking('hooking', PluxSeverity.high, sticky: true),

  /// A debugger is attached.
  debugger('debugger', PluxSeverity.medium, sticky: true),

  /// An emulator or a simulator.
  emulator('emulator', PluxSeverity.medium, sticky: true),

  /// The app's signature or identity is not the one expected: it was
  /// repackaged.
  repackaging('repackaging', PluxSeverity.high, sticky: true),

  /// The app was not installed by one of the trusted stores.
  untrustedInstaller('untrusted_installer', PluxSeverity.low, sticky: true),

  /// The screen is being mirrored or recorded.
  screenCapture('screen_capture', PluxSeverity.low, sticky: false),

  /// Another window covered the app while the user touched it (tapjacking).
  overlay('overlay', PluxSeverity.medium, sticky: false),

  /// An accessibility service that can read the screen and perform
  /// gestures is enabled, as banking malware does.
  accessibility('accessibility', PluxSeverity.medium, sticky: false);

  const PluxDetectionKind(this.wireName, this.severity, {required this.sticky});

  /// The name used in telemetry and on the platform channel.
  final String wireName;

  /// How serious the kind is.
  final PluxSeverity severity;

  /// Whether a detection stays in force until the app restarts. A hostile
  /// environment seen once is not trusted again in this process, because
  /// what it did cannot be undone by its going away. Screen capture,
  /// overlays and accessibility services follow the current state instead.
  final bool sticky;

  /// The kind named [wire]; null for a name this version does not know.
  static PluxDetectionKind? fromWire(Object? wire) {
    for (final k in values) {
      if (k.wireName == wire) return k;
    }
    return null;
  }
}

/// One finding of a scan: a kind and the checks that tripped.
final class PluxDetection {
  /// Creates a finding. Each of [indicators] is a short code naming a
  /// check, such as `su_binary`; never a path, an identifier or a value
  /// read from the device (SEC-092).
  PluxDetection(this.kind, [Iterable<String> indicators = const []])
    : indicators = List.unmodifiable(indicators);

  /// What was detected.
  final PluxDetectionKind kind;

  /// The checks that tripped.
  final List<String> indicators;

  @override
  String toString() =>
      'PluxDetection(${kind.wireName}: ${indicators.join(',')})';
}
