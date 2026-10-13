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
