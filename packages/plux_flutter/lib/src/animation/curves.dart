// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The curves a bundle names by permanent ID (ANI-001, ANI-002). The
/// compiler's table lists the same curves in the same order.
library;

import 'package:flutter/animation.dart';

/// The curves by permanent ID minus one: linear, easeIn, easeOut,
/// easeInOut, fastOutSlowIn, decelerate, bounceIn, bounceOut, elasticOut
/// and overshoot.
const List<Curve> animationCurves = [
  Curves.linear,
  Curves.easeIn,
  Curves.easeOut,
  Curves.easeInOut,
  Curves.fastOutSlowIn,
  Curves.decelerate,
  Curves.bounceIn,
  Curves.bounceOut,
  Curves.elasticOut,
  Curves.easeOutBack,
];

/// The curve with permanent [id]; linear for 0 (none named) and for an ID
/// this runtime does not know.
Curve curveById(int id) => id >= 1 && id <= animationCurves.length
    ? animationCurves[id - 1]
    : Curves.linear;

/// The curve a bundle gives by permanent [id], or, when [cubic] holds four
/// control points x1, y1, x2, y2, the cubic Bézier through them (build log
/// row 48), as CSS's `cubic-bezier()`.
Curve curveOf(int id, List<double>? cubic) => cubic != null && cubic.length == 4
    ? Cubic(cubic[0], cubic[1], cubic[2], cubic[3])
    : curveById(id);

/// A cubic Bézier from a value of its four control points x1, y1, x2, y2,
/// such as a `Curve` prop's literal or a `cubicBezier` token; null for any
/// other value.
Cubic? cubicOf(Object? v) => switch (v) {
  [final num a, final num b, final num c, final num d] => Cubic(
    a.toDouble(),
    b.toDouble(),
    c.toDouble(),
    d.toDouble(),
  ),
  _ => null,
};
