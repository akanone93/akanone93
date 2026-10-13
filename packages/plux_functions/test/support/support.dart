// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Shared helpers for the engine tests.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_functions/plux_functions.dart';

/// Limits generous enough for any test that is not about a limit.
const roomy = FunctionLimits(
  timeout: Duration(seconds: 5),
  memoryBytes: 32 * 1024 * 1024,
  inputBytes: 1024 * 1024,
  outputBytes: 1024 * 1024,
  logBytes: 4096,
);

/// A fixture module's bytes.
Uint8List fixture(String name) => File('test/fixtures/$name').readAsBytesSync();

/// UTF-8 bytes of [text].
Uint8List utf8Bytes(String text) => Uint8List.fromList(utf8.encode(text));

/// Matches a [PluxException] with [code].
Matcher plux(PluxErrorCode code) =>
    isA<PluxException>().having((e) => e.code, 'code', code);

/// The output of [outcome] as text; fails the test for a business error.
String outputText(FunctionOutcome outcome) {
  expect(outcome, isA<FunctionOutput>());
  return utf8.decode((outcome as FunctionOutput).json);
}
