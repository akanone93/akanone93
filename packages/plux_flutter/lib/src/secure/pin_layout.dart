// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The key layout of `SecurePinPad` (SEC-090): which digit sits where.
library;

import 'dart:math';

/// The digits of the pad's ten digit keys in reading order: three rows of
/// three, then the digit in the middle of the bottom row.
///
/// Without [random] the order is the familiar `1 2 3 / 4 5 6 / 7 8 9 / 0`.
/// With one it is a permutation of the ten digits drawn from [random], so
/// an observer who sees where a finger lands learns nothing about the
/// digit; the same [Random] seed gives the same order, which tests rely
/// on. The pad draws a fresh generator from the platform's secure source
/// each time it appears.
List<int> pinDigitOrder([Random? random]) {
  final digits = [1, 2, 3, 4, 5, 6, 7, 8, 9, 0];
  if (random != null) digits.shuffle(random);
  return digits;
}
