// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_functions/plux_functions.dart';

import 'support/support.dart';
import 'support/test_host.dart';

void main() {
  late FunctionEngine engine;
  late FunctionModule loan;
  late Directory scratch;

  setUpAll(() async {
    engine = await FunctionEngine.start();
    loan = await engine.load(key: 'loan', bytes: fixture('loan_module.wasm'));
    scratch = Directory.systemTemp.createTempSync('plux_functions_loan');
  });

  tearDownAll(() async {
    await engine.close();
    scratch.deleteSync(recursive: true);
  });

  final stored =
      (jsonDecode(
            File('../../sdk/fn/examples/loan/function.json').readAsStringSync(),
          ) as Map<String, Object?>)['tests']!
          as List<Object?>;

  test('[FN-030] the stored cases are the ones the example ships', () {
    expect(stored, hasLength(6));
  });

  for (final raw in stored) {
    final testCase = raw! as Map<String, Object?>;
    test(
      '[FN-030] the device interpreter passes "${testCase['name']}"',
      () async {
        final outcome = await engine.invoke(
          module: loan,
          index: 0,
          input: utf8Bytes(jsonEncode(testCase['input'])),
          limits: roomy,
          host: TestHost(directory: scratch.path),
        );
        if (testCase.containsKey('error')) {
          final expected = testCase['error']! as Map<String, Object?>;
          expect(outcome, isA<FunctionBusinessError>());
          expect((outcome as FunctionBusinessError).code, expected['code']);
        } else {
          expect(outcome, isA<FunctionOutput>());
          expect((outcome as FunctionOutput).decode(), testCase['output']);
        }
      },
    );
  }

  test('[FN-050] a module loads once per key', () async {
    final again = await engine.load(
      key: 'loan',
      bytes: fixture('loan_module.wasm'),
    );
    expect(identical(again, loan), isTrue);
  });
}
