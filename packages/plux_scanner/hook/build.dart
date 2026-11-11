// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Builds `plux_zxing`, the scanner's offline engine, from source for every
/// target the Flutter tool asks for: ZXing-C++ 2.3.0, reader sources only
/// (third_party/zxing-cpp/VERSION), behind `native/plux_zxing.cpp`
/// (decision B21, ADR-0051).
library;

import 'dart:io';

import 'package:code_assets/code_assets.dart';
import 'package:hooks/hooks.dart';
import 'package:native_toolchain_c/native_toolchain_c.dart';

const _core = 'third_party/zxing-cpp/core/src';

/// The vendored tree's sources with [extension] (`.cpp` or `.c`), found on
/// disk so the tree is the only list of them.
List<String> _sources(Uri root, String extension) {
  final base = Directory.fromUri(root.resolve('$_core/'));
  final prefix = base.uri.path;
  final out = <String>[
    for (final e in base.listSync(recursive: true))
      if (e is File && e.path.endsWith(extension))
        '$_core/${e.uri.path.substring(prefix.length)}',
  ]..sort();
  return out;
}

/// The static library of the vendored tree's C sources (libzueci), linked
/// into `plux_zxing`. A C source is compiled as C, as upstream's CMake does
/// (by its extension): compiled as C++ it is held to C++'s rules, and
/// libzueci's `char` tables of -1 are a narrowing error wherever `char` is
/// unsigned (Arm: Android, Linux), while x86 accepts them.
const _cLibrary = 'plux_zueci';

void main(List<String> args) async {
  await build(args, (input, output) async {
    if (!input.config.buildCodeAssets) return;
    final os = input.config.code.targetOS;
    final apple = os == OS.iOS || os == OS.macOS;
    final sections = [
      '-fvisibility=hidden',
      '-ffunction-sections',
      '-fdata-sections',
    ];
    await CBuilder.library(
      name: _cLibrary,
      sources: _sources(input.packageRoot, '.c'),
      includes: [_core],
      language: Language.c,
      flags: sections,
      optimizationLevel: OptimizationLevel.oS,
    ).run(
      input: input,
      output: output,
      linkModePreference: LinkModePreference.static,
    );
    await CBuilder.library(
      name: 'plux_zxing',
      assetName: 'src/zxing_ffi.dart',
      sources: [
        'native/plux_zxing.cpp',
        ..._sources(input.packageRoot, '.cpp'),
      ],
      includes: ['native', _core],
      language: Language.cpp,
      std: 'c++20',
      libraries: [_cLibrary],
      // Android: the NDK's libc++ is linked statically, so the package ships
      // one library per ABI and no libc++_shared.so.
      cppLinkStdLib: os == OS.android ? 'c++_static' : null,
      flags: [
        ...sections,
        if (apple) '-Wl,-dead_strip' else '-Wl,--gc-sections',
        // Google Play requires 16 KiB page alignment for Android 15+.
        if (os == OS.android) '-Wl,-z,max-page-size=16384',
      ],
      optimizationLevel: OptimizationLevel.oS,
    ).run(input: input, output: output);
  });
}
