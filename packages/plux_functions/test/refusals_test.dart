// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_functions/plux_functions.dart';

import 'support/support.dart';
import 'support/test_host.dart';

/// Hand-assembled modules from test/fixtures/src/refusals.py, each exporting
/// memory, plux_abi_version, plux_alloc and plux_invoke unless noted.
const _prefix =
    '0061736d010000000117046000017f60017f017f60037f7f7f017e60027f7f017e';
const _valid =
    '${_prefix}0304030001020503010001073804066d656d6f7279020010706c75785f6162695f76657273696f6e00000a706c75785f616c6c6f6300010b706c75785f696e766f6b6500020a1003040041010b040041000b040042000b';
const _abiTwo =
    '${_prefix}0304030001020503010001073804066d656d6f7279020010706c75785f6162695f76657273696f6e00000a706c75785f616c6c6f6300010b706c75785f696e766f6b6500020a1003040041020b040041000b040042000b';
const _importsHttp =
    '${_prefix}02180107706c75785f76310c687474705f7265717565737400030304030001020503010001073804066d656d6f7279020010706c75785f6162695f76657273696f6e00010a706c75785f616c6c6f6300020b706c75785f696e766f6b6500030a1003040041010b040041000b040042000b';
const _importsUnknown =
    '${_prefix}020f0103656e76076d79737465727900030304030001020503010001073804066d656d6f7279020010706c75785f6162695f76657273696f6e00010a706c75785f616c6c6f6300020b706c75785f696e766f6b6500030a1003040041010b040041000b040042000b';
const _noAlloc =
    '${_prefix}0304030001020503010001072b03066d656d6f7279020010706c75785f6162695f76657273696f6e00000b706c75785f696e766f6b6500020a1003040041010b040041000b040042000b';

Uint8List _hex(String hex) => Uint8List.fromList([
  for (var i = 0; i < hex.length; i += 2)
    int.parse(hex.substring(i, i + 2), radix: 16),
]);

void main() {
  late FunctionEngine engine;

  setUpAll(() async => engine = await FunctionEngine.start());
  tearDownAll(() => engine.close());

  test(
    '[FN-052] a module that imports claims is refused with PLX-7018',
    () async {
      await expectLater(
        engine.load(key: 'fee', bytes: fixture('fee_module.wasm')),
        throwsA(
          isA<PluxException>()
              .having(
                (e) => e.code,
                'code',
                PluxErrorCode.functionModuleInvalid,
              )
              .having((e) => e.message, 'message', contains('claims')),
        ),
      );
    },
  );

  test(
    '[FN-052] a module that imports http_request is refused with PLX-7018',
    () async {
      await expectLater(
        engine.load(key: 'http', bytes: _hex(_importsHttp)),
        throwsA(plux(PluxErrorCode.functionModuleInvalid)),
      );
    },
  );

  test('[FN-052] an unknown import is refused with PLX-7018', () async {
    await expectLater(
      engine.load(key: 'unknown', bytes: _hex(_importsUnknown)),
      throwsA(plux(PluxErrorCode.functionModuleInvalid)),
    );
  });

  test(
    '[FN-050] plux_abi_version other than 1 is refused with PLX-7017',
    () async {
      await expectLater(
        engine.load(key: 'abi2', bytes: _hex(_abiTwo)),
        throwsA(plux(PluxErrorCode.functionAbiUnsupported)),
      );
    },
  );

  test(
    '[FN-050] a module without plux_alloc is refused with PLX-7018',
    () async {
      await expectLater(
        engine.load(key: 'noalloc', bytes: _hex(_noAlloc)),
        throwsA(plux(PluxErrorCode.functionModuleInvalid)),
      );
    },
  );

  test(
    '[FN-050] bytes that are not WebAssembly are refused with PLX-7018',
    () async {
      await expectLater(
        engine.load(key: 'junk', bytes: utf8Bytes('not a module')),
        throwsA(plux(PluxErrorCode.functionModuleInvalid)),
      );
    },
  );

  test('[FN-050] a refused key can be loaded again with good bytes', () async {
    await expectLater(
      engine.load(key: 'retry', bytes: utf8Bytes('junk')),
      throwsA(isA<PluxException>()),
    );
    final module = await engine.load(key: 'retry', bytes: _hex(_valid));
    expect(module.key, 'retry');
  });

  test(
    '[FN-050] a module that returns an empty result fails with PLX-7007',
    () async {
      final module = await engine.load(key: 'valid', bytes: _hex(_valid));
      final scratch = Directory.systemTemp.createTempSync(
        'plux_functions_refusal',
      );
      addTearDown(() => scratch.deleteSync(recursive: true));
      await expectLater(
        engine.invoke(
          module: module,
          index: 0,
          input: utf8Bytes('{}'),
          limits: roomy,
          host: TestHost(directory: scratch.path),
        ),
        throwsA(plux(PluxErrorCode.functionTrapped)),
      );
    },
  );
}
