// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// When the device asks for the control document (SYN-060, ADR-0077): on
/// every resume, every five minutes while the app is in the foreground and
/// never in the background, and at once when a silent push carries the Plux
/// marker. The check itself runs on the sync isolate; this only decides
/// when.
library;

import 'dart:async';

/// How often the foreground checks for a new control document.
const Duration controlCheckInterval = Duration(minutes: 5);

/// The value of the `plux` key a silent push carries (`push.Marker` on the
/// server).
const String controlPushMarker = 'control';

/// Makes a timer that calls [tick] every [period]; tests pass their own.
typedef PeriodicTimerFactory = Timer Function(
  Duration period,
  void Function() tick,
);

/// Whether [payload] is the silent push that asks for a control check: it
/// carries the Plux marker and, when it names an app, names [appId].
/// Anything else is none of Plux's business and is ignored.
bool isControlPush(Map<String, Object?> payload, String appId) {
  if (payload['plux'] != controlPushMarker) return false;
  final app = payload['app'];
  return app == null || app == appId;
}

/// Schedules the control checks.
final class ControlPoller {
  /// Creates the scheduler. [check] runs one check and never throws. The
  /// [timer] defaults to a timer of the root zone, which a widget test's
  /// fake clock does not hold.
  ControlPoller({
    required Future<void> Function() check,
    PeriodicTimerFactory? timer,
    this.interval = controlCheckInterval,
  }) : _check = check, // ignore: prefer_initializing_formals
       _timer = timer ?? _rootTimer;

  static Timer _rootTimer(Duration period, void Function() tick) =>
      Zone.root.createPeriodicTimer(period, (_) => tick());

  /// The time between checks in the foreground.
  final Duration interval;

  final Future<void> Function() _check;
  final PeriodicTimerFactory _timer;
  Timer? _ticking;
  Future<void>? _running;
  bool _closed = false;

  /// Whether the periodic check is running, that is the app is in the
  /// foreground.
  bool get ticking => _ticking != null;

  /// The app is in the foreground: checks now and then every [interval].
  void resume() {
    if (_closed) return;
    unawaited(checkNow());
    _ticking ??= _timer(interval, () => unawaited(checkNow()));
  }

  /// The app went to the background: no check is scheduled until [resume].
  void pause() {
    _ticking?.cancel();
    _ticking = null;
  }

  /// Checks now; a check already running is joined rather than repeated, so
  /// a burst of pushes costs one request.
  Future<void> checkNow() {
    if (_closed) return Future.value();
    return _running ??= _check().whenComplete(() => _running = null);
  }

  /// Stops scheduling.
  void close() {
    _closed = true;
    pause();
  }
}
