// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// A detector for tests (SEC-070): host apps use it to see how their app
/// reacts to a rooted or hooked device without one.
library;

import 'package:plux_security/plux_security.dart';

/// A [PluxRaspDetector] that returns what a test sets, and records what it
/// was asked.
final class FakeRaspDetector implements PluxRaspDetector {
  /// Creates a detector that finds [findings] until told otherwise.
  FakeRaspDetector([Iterable<PluxDetection> findings = const []])
    : findings = [...findings];

  /// What the next scan finds.
  List<PluxDetection> findings;

  /// When set, the next scan throws it instead, once.
  Exception? failure;

  /// The expectations of each scan, in order.
  final List<PluxRaspExpectations> scans = [];

  @override
  Future<List<PluxDetection>> scan(PluxRaspExpectations expectations) async {
    scans.add(expectations);
    final f = failure;
    if (f != null) {
      failure = null;
      throw f;
    }
    return [...findings];
  }
}
