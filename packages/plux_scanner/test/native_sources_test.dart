// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Verifies: RT-060.
// The vendored ZXing tree compiles where `char` is unsigned (Arm: Android,
// iOS and Linux on Arm), not only on the x86 hosts the other tests run on:
// every source is compiled as the build hook compiles it, a C source as C and
// a C++ source as C++20, with -funsigned-char. A C source compiled as C++
// failed there with narrowing errors (libzueci's `char` tables of -1). It
// needs clang, which the other tests do not.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

const _core = 'third_party/zxing-cpp/core/src';

List<String> _sources(String extension) => [
  for (final e in Directory(_core).listSync(recursive: true))
    if (e is File && e.path.endsWith(extension)) e.path,
]..sort();

Future<String?> _syntaxErrors(
  String clang,
  String file,
  List<String> language,
) async {
  final r = await Process.run(clang, [
    ...language,
    '-funsigned-char',
    '-fsyntax-only',
    '-Inative',
    '-I$_core',
    file,
  ]);
  return r.exitCode == 0 ? null : '$file\n${r.stderr}';
}

Future<List<String>> _check(
  String clang,
  List<String> files,
  List<String> language,
) async {
  final failures = <String>[];
  const batch = 8;
  for (var i = 0; i < files.length; i += batch) {
    final results = await Future.wait([
      for (final f in files.skip(i).take(batch))
        _syntaxErrors(clang, f, language),
    ]);
    failures.addAll(results.whereType<String>());
  }
  return failures;
}

void main() {
  final clang = Platform.isWindows
      ? ''
      : Process.runSync('which', ['clang']).stdout.toString().trim();
  final skip = clang.isEmpty ? 'needs clang on a POSIX host' : null;

  test(
    'every vendored source compiles in its own language with an unsigned char',
    () async {
      final c = _sources('.c');
      final cpp = ['native/plux_zxing.cpp', ..._sources('.cpp')];
      expect(c, isNotEmpty, reason: 'libzueci is C');
      final failures = [
        ...await _check(clang, c, ['-x', 'c']),
        ...await _check(clang, cpp, ['-x', 'c++', '-std=c++20']),
      ];
      expect(failures, isEmpty, reason: failures.take(3).join('\n'));
    },
    skip: skip,
    timeout: const Timeout(Duration(minutes: 5)),
  );

  test('compiled as C++, libzueci fails with an unsigned char', () async {
    // The guard above would pass for the wrong reason if -funsigned-char
    // changed nothing: the old way of building must still fail here.
    final failures = await _check(clang, _sources('.c'), [
      '-x',
      'c++',
      '-std=c++20',
    ]);
    expect(failures, isNotEmpty);
  }, skip: skip);
}
