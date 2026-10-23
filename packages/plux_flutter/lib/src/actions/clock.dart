// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Time as the action engine sees it: the monotonic clock of the timed
/// policies, the timers of triggers, the waits of `delay` and retries, and
/// the jitter of backoffs (ACT-003, ACT-006). Tests substitute a fake, so
/// they drive time and jitter deterministically.
library;

import 'dart:async';
import 'dart:developer' as developer;
import 'dart:math' as math;

/// The engine's clock.
class ActionClock {
  /// Creates the real clock.
  const ActionClock();

  /// The time on a monotonic clock.
  Duration get now => Duration(microseconds: developer.Timeline.now);

  /// Calls [f] once after [d].
  Timer timer(Duration d, void Function() f) => Timer(d, f);

  /// Calls [f] every [d].
  Timer periodic(Duration d, void Function() f) =>
      Timer.periodic(d, (_) => f());

  /// Completes after [d].
  Future<void> sleep(Duration d) => Future<void>.delayed(d);

  /// A value in [0, 1) that scales a backoff's jitter.
  double random() => math.Random().nextDouble();

  /// A random UUID (version 4) from the secure generator: the idempotency
  /// key of an `invokeFunction` step (FN-020).
  String uuid() {
    final r = math.Random.secure();
    final b = List<int>.generate(16, (_) => r.nextInt(256));
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    final h = b.map((x) => x.toRadixString(16).padLeft(2, '0')).join();
    return '${h.substring(0, 8)}-${h.substring(8, 12)}-${h.substring(12, 16)}-'
        '${h.substring(16, 20)}-${h.substring(20)}';
  }
}

/// A clock that a development session can fix at an instant, advance and
/// hand back to the real time (`DEV-031`). Until it is set it behaves as
/// [ActionClock]; while it is set, time passes only through [advance], which
/// runs the timers and waits that fall due, in order. Only builds with
/// `kPluxDevtools` use it.
class FakeableActionClock extends ActionClock {
  /// Creates a clock that follows real time.
  FakeableActionClock();

  Duration? _fixed;
  int _next = 0;
  final List<_FakeTimer> _timers = [];

  /// Whether the clock is fixed.
  bool get isFaked => _fixed != null;

  @override
  Duration get now => _fixed ?? super.now;

  /// Fixes the clock at [instant]. Timers made while it follows real time
  /// keep running on real time.
  void set(Duration instant) => _fixed = instant;

  /// Moves the fixed clock forward by [d], running what falls due on the
  /// way; throws [StateError] when the clock is not fixed or [d] is
  /// negative.
  void advance(Duration d) {
    final from = _fixed;
    if (from == null) throw StateError('the clock is not fixed');
    if (d.isNegative) throw StateError('time cannot go back');
    final target = from + d;
    while (true) {
      final due = _timers.where((t) => t.due <= target).toList()
        ..sort((a, b) {
          final c = a.due.compareTo(b.due);
          return c != 0 ? c : a.order.compareTo(b.order);
        });
      if (due.isEmpty) break;
      final t = due.first;
      _fixed = t.due > _fixed! ? t.due : _fixed;
      t.fire();
    }
    _fixed = target;
  }

  /// Returns to real time; the timers still waiting on the fixed clock are
  /// cancelled.
  void reset() {
    _fixed = null;
    for (final t in _timers.toList()) {
      t.cancel();
    }
  }

  @override
  Timer timer(Duration d, void Function() f) =>
      isFaked ? _add(d, f, periodic: false) : super.timer(d, f);

  @override
  Timer periodic(Duration d, void Function() f) =>
      isFaked ? _add(d, f, periodic: true) : super.periodic(d, f);

  @override
  Future<void> sleep(Duration d) {
    if (!isFaked) return super.sleep(d);
    final done = Completer<void>();
    _add(d, done.complete, periodic: false);
    return done.future;
  }

  _FakeTimer _add(Duration d, void Function() f, {required bool periodic}) {
    final t = _FakeTimer(this, now + d, periodic ? d : null, f, _next++);
    _timers.add(t);
    return t;
  }
}

final class _FakeTimer implements Timer {
  _FakeTimer(this._clock, this.due, this._every, this._f, this.order);

  final FakeableActionClock _clock;
  final Duration? _every;
  final void Function() _f;
  final int order;
  Duration due;
  bool _active = true;
  int _ticks = 0;

  void fire() {
    _ticks++;
    final every = _every;
    if (every == null) {
      _active = false;
      _clock._timers.remove(this);
    } else {
      due += every <= Duration.zero ? const Duration(microseconds: 1) : every;
    }
    _f();
  }

  @override
  void cancel() {
    _active = false;
    _clock._timers.remove(this);
  }

  @override
  bool get isActive => _active;

  @override
  int get tick => _ticks;
}
