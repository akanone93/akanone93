// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/animation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/animation/curves.dart';

void main() {
  test('a curve is named by its permanent ID, or a cubic Bézier by its '
      'control points (build log row 48) [ANI-001] [ANI-002]', () {
    expect(curveOf(4, null), Curves.easeInOut);
    expect(curveOf(0, null), Curves.linear);
    final cubic = curveOf(0, const [0.2, 0, 0, 1]) as Cubic;
    expect([cubic.a, cubic.b, cubic.c, cubic.d], [0.2, 0, 0, 1]);
    // Material 3's emphasized decelerate: past the middle by half time.
    expect(
      curveOf(0, const [0.05, 0.7, 0.1, 1]).transform(0.5),
      greaterThan(0.9),
    );
    expect(curveOf(3, const [0.2, 0]), Curves.easeOut);
  });

  test('a Curve prop or token value of four numbers is a cubic Bézier '
      '[ANI-001]', () {
    final c = cubicOf([0.4, 0, 0.2, 1])!;
    expect([c.a, c.b, c.c, c.d], [0.4, 0, 0.2, 1]);
    expect(cubicOf('easeIn'), isNull);
    expect(cubicOf([1, 2, 3]), isNull);
  });
}
