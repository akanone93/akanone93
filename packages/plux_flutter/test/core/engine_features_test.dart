// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/core/features.dart';

/// Every engine feature the compiler can require, written by the compiler's
/// tests from its own feature tables (`EngineFeatures`, build log #271).
const _vector = '../../schema/testdata/features/engine.json';

/// Features the compiler can require that this runtime deliberately does not
/// support yet, each with the runtime version that brings it. Empty: a
/// feature appears here only by a decision, never to make this test pass.
const Map<String, String> _notYet = {};

void main() {
  final features =
      ((jsonDecode(File(_vector).readAsStringSync()) as Map)['features']
              as List)
          .cast<Map<String, Object?>>();

  // #257: the runtime evaluated the locale groups but did not declare them, so
  // it refused every bundle using them (PLX-3010). The PXL groups were then
  // guarded; this guards every family the compiler can require.
  test('the runtime declares every engine feature the compiler can require '
      '[BND-008]', () {
    expect(features.length, greaterThan(20), reason: 'the vector is read');
    final runtime = RuntimeFeatures(deviceFunctions: true);
    for (final f in features) {
      final name = f['name']! as String;
      if (_notYet.containsKey(name)) {
        expect(
          runtime.supports(name),
          isFalse,
          reason: '$name is listed as not yet supported',
        );
        continue;
      }
      expect(
        runtime.supports(name),
        isTrue,
        reason:
            '$name (runtime ${f['runtime'] ?? 'any'}) is required by the compiler '
            'but not declared in RuntimeFeatures (core/features.dart)',
      );
    }
  });

  test('device-placed functions are supported only with a function engine '
      '[BND-008, FN-051]', () {
    expect(RuntimeFeatures().supports('section.wasm.v1'), isFalse);
    expect(
      RuntimeFeatures(deviceFunctions: true).supports('section.wasm.v1'),
      isTrue,
    );
  });
}
