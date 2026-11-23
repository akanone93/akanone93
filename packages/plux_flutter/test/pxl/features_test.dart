// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/core/features.dart';
import 'package:plux_flutter/src/pxl/builtins.dart';
import 'package:plux_flutter/src/pxl/tables.g.dart';

/// The core functions the compiler inlines as operators or constants
/// (`backend/internal/pxl/call.go`, `inline`): they have no builtin.
const _inlined = {'coalesce', 'ifNull', 'isNull', 'typeOf'};

void main() {
  // A runtime that evaluated a group's functions but did not declare its
  // feature refused every bundle using them (PLX-3010, build log #257); one
  // that declared a group it cannot evaluate would fail at render instead.
  test('the runtime declares exactly the PXL groups its evaluator implements '
      '[BND-008, PXL-006, I18N-001]', () {
    final features = RuntimeFeatures();
    for (final MapEntry(key: group, value: feature) in groupFeatures.entries) {
      final functions = [
        for (var i = 0; i < overloads.length; i++)
          if (overloads[i].group == group &&
              !_inlined.contains(overloads[i].name))
            i,
      ];
      final implemented =
          functions.isNotEmpty && functions.every((i) => builtins[i] != null);
      expect(
        features.supports(feature),
        implemented,
        reason:
            '$feature: ${functions.length} functions, implemented: $implemented',
      );
    }
  });
}
