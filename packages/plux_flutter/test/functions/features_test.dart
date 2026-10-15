// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/core/features.dart';

void main() {
  test(
    '[FN-051] the wasm section is supported only with a function engine',
    () {
      expect(RuntimeFeatures().supports('section.wasm.v1'), isFalse);
      expect(
        RuntimeFeatures(deviceFunctions: true).supports('section.wasm.v1'),
        isTrue,
      );
      expect(
        RuntimeFeatures(deviceFunctions: true).supports('section.wasm.v2'),
        isFalse,
      );
    },
  );
}
