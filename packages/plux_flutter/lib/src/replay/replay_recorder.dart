// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Records a structural session replay on the UI isolate (ANL-050,
/// ADR-0101): the screens opened, the layout of their nodes, the nodes
/// touched and the non-sensitive state entries that changed. Never a
/// pixel, a text or a sensitive value (SCH-012). Off unless the host
/// consented to `replay` (SEC-161) and the security profile allows it.
///
/// Recording is a bounded list of small maps and a port message when the
/// replay closes: no I/O, encoding or hashing here (L-6). The sync
/// isolate encodes, stores and uploads it.
library;

import 'dart:math' as math;

import 'package:plux_flutter/src/core/config.dart';
import 'package:plux_flutter/src/telemetry/events.dart';

/// Words in a widget's type name that mark it as taking or showing what a
/// user entered, or a secret: only its rectangle is recorded.
const List<String> _maskedWords = [
  'input',
  'field',
  'password',
  'passcode',
  'pincode',
  'secure',
  'secret',
  'otp',
  'editable',
  'textarea',
];

/// Whether the widget type [widget] is recorded as a rectangle only.
bool isMaskedWidget(String widget) {
  final lower = widget.toLowerCase();
  return _maskedWords.any(lower.contains);
}

final RegExp _stateName = RegExp(r'^[a-z][A-Za-z0-9_]{0,63}$');

/// The longest integer whose value is recorded: a longer one may be a
/// card, phone or account number.
const int maxReplayDigits = 9;

/// The sampling rate of replays when neither the host nor the app sets one.
const double defaultReplaySampling = 0.1;

/// A node of a layout snapshot.
final class ReplayNode {
  /// Creates a node.
  const ReplayNode({
    required this.path,
    required this.widget,
    required this.x,
    required this.y,
    required this.width,
    required this.height,
  });

  /// The node's path in the page, as errors report it.
  final String path;

  /// The widget type name.
  final String widget;

  /// The rectangle in logical pixels.
  final double x, y, width, height;
}

/// What one replay may hold, from the limits registry (LIM-001).
final class ReplayBounds {
  /// Creates the bounds.
  const ReplayBounds({
    required this.events,
    required this.bytes,
    required this.perDay,
  });

  /// `replay.eventsPerReplay`.
  final int events;

  /// `replay.bytesPerReplay`.
  final int bytes;

  /// `replay.perDevicePerDay`.
  final int perDay;
}

/// The recorder.
final class ReplayRecorder {
  /// Creates a recorder that hands each closed replay to [post] and calls
  /// [withdraw] when the replay consent is withdrawn.
  ReplayRecorder({
    required this._post,
    required this._withdraw,
    required this._enabled,
    required this._bounds,
    required this._sampling,
    required this._consent,
    math.Random? random,
    DateTime Function()? clock,
  }) : _random = random ?? math.Random.secure(),
       _clock = clock ?? DateTime.now;

  final void Function(Map<String, Object?> replay) _post;
  final void Function() _withdraw;
  final bool Function() _enabled;
  final ReplayBounds Function() _bounds;
  final double? Function() _sampling;
  final math.Random _random;
  final DateTime Function() _clock;
  PluxConsent _consent;

  final List<Map<String, Object?>> _events = [];
  final List<DateTime> _starts = [];
  bool _recording = false;
  bool _truncated = false;
  int _bytes = 0;
  DateTime _startedAt = DateTime.fromMillisecondsSinceEpoch(0);
  List<String> _masked = const [];

  /// The active release, which a replay is attributed to.
  int releaseSequence = 0;

  /// Whether a replay is being recorded now.
  bool get recording => _recording;

  /// What the user consented to. Withdrawing `replay` discards the replay
  /// being recorded and every unsent one (SEC-161).
  PluxConsent get consent => _consent;
  set consent(PluxConsent c) {
    final withdrawn = _consent.replay && !c.replay;
    _consent = c;
    if (withdrawn) {
      _discard();
      _withdraw();
    }
  }

  /// Starts a replay of the foreground stretch that begins now, when the
  /// consent, the security profile, the sampling rate and the daily bound
  /// allow it.
  void start() {
    if (_recording || !_consent.replay || !_enabled()) return;
    final now = _clock();
    _starts.removeWhere((t) => now.difference(t) >= const Duration(days: 1));
    if (_starts.length >= _bounds().perDay) return;
    final rate = _sampling() ?? defaultReplaySampling;
    if (rate <= 0 || (rate < 1 && _random.nextDouble() >= rate)) return;
    _starts.add(now);
    _recording = true;
    _startedAt = now;
  }

  /// Closes the replay and hands it on, unless it holds nothing.
  void finish() {
    if (!_recording) return;
    final events = List<Map<String, Object?>>.of(_events);
    final truncated = _truncated;
    final started = _startedAt;
    _discard();
    if (events.isEmpty) return;
    _post({
      'id': _uuid(),
      'startedAt': started.toUtc().toIso8601String(),
      'durationMs': _clock().difference(started).inMilliseconds,
      if (releaseSequence > 0) 'releaseSequence': '$releaseSequence',
      'truncated': truncated,
      'events': events,
      'consent': {'replay': true},
    });
  }

  void _discard() {
    _recording = false;
    _truncated = false;
    _bytes = 0;
    _masked = const [];
    _events.clear();
  }

  void _add(Map<String, Object?> event, int cost) {
    if (!_recording) return;
    final bounds = _bounds();
    if (_events.length >= bounds.events || _bytes + cost > bounds.bytes) {
      _truncated = true;
      finish();
      return;
    }
    _bytes += cost;
    _events.add({
      'atMs': _clock().difference(_startedAt).inMilliseconds,
      ...event,
    });
  }

  /// A screen was opened.
  void screen(String route, String pluginKey) {
    if (!_recording) return;
    final r = clipText(route);
    _add({
      'kind': 'screen',
      'route': r,
      if (pluginKey.isNotEmpty) 'pluginKey': pluginKey,
    }, 48 + r.length + pluginKey.length);
  }

  /// The nodes of the shown page were laid out. A text input, a secure
  /// widget and every node inside one is recorded as a rectangle only.
  void layout(List<ReplayNode> nodes) {
    if (!_recording || nodes.isEmpty) return;
    final masked = <String>[];
    final out = <Map<String, Object?>>[];
    var cost = 48;
    for (final n in nodes) {
      final hidden =
          isMaskedWidget(n.widget) ||
          masked.any((p) => n.path.startsWith('$p/'));
      if (hidden) masked.add(n.path);
      cost += 120 + n.path.length + n.widget.length;
      out.add({
        'path': n.path,
        'widget': n.widget,
        'x': _round(n.x),
        'y': _round(n.y),
        'width': _round(n.width),
        'height': _round(n.height),
        if (hidden) 'masked': true,
      });
    }
    _masked = masked;
    _add({'kind': 'layout', 'nodes': out}, cost);
  }

  /// The user touched the node at [path], [fx] and [fy] (0 to 1) inside it.
  /// A touch on a masked node is also an input, without what was typed.
  void tap(String path, double fx, double fy) {
    if (!_recording) return;
    _add({
      'kind': 'tap',
      'nodePath': path,
      'fx': _round(fx.clamp(0, 1).toDouble()),
      'fy': _round(fy.clamp(0, 1).toDouble()),
    }, 72 + path.length);
    if (_masked.contains(path)) {
      _add({'kind': 'input', 'nodePath': path}, 48 + path.length);
    }
  }

  /// A state entry changed to [value]. Sensitive entries (SCH-012) and
  /// entries whose name marks sensitive data are not recorded at all; of the
  /// others only the type is recorded, and the value of a bool, a short
  /// integer or a double.
  void state(String name, Object? value, {bool sensitive = false}) {
    if (!_recording || sensitive) return;
    if (!_stateName.hasMatch(name) || isSensitiveField(name)) return;
    final (type, text) = switch (value) {
      bool() => ('bool', '$value'),
      int() when '${value.abs()}'.length <= maxReplayDigits => (
        'int',
        '$value',
      ),
      int() => ('int', ''),
      double() when value.isFinite => ('double', '$value'),
      double() => ('double', ''),
      String() => ('string', ''),
      List() => ('list', ''),
      Map() => ('map', ''),
      null => ('null', ''),
      _ => ('', ''),
    };
    if (type.isEmpty) return;
    _add({
      'kind': 'state',
      'name': name,
      'valueType': type,
      if (text.isNotEmpty) 'value': text,
    }, 72 + name.length + text.length);
  }

  static double _round(double v) =>
      v.isFinite ? (v * 10).roundToDouble() / 10 : 0;

  String _uuid() {
    final r = math.Random.secure();
    final b = List<int>.generate(16, (_) => r.nextInt(256));
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    final h = b.map((x) => x.toRadixString(16).padLeft(2, '0')).join();
    return '${h.substring(0, 8)}-${h.substring(8, 12)}-${h.substring(12, 16)}-'
        '${h.substring(16, 20)}-${h.substring(20)}';
  }
}
