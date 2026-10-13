// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:isolate';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_functions/plux_functions.dart';

import 'support/support.dart';

/// The core's view of the engine (`PluxFunctionEngine`), on the shared probe.
void main() {
  late PluxFunctions engine;
  late Object module;
  final probe = File('../../schema/testdata/functions/probe-device.wasm')
      .readAsBytesSync();
  const limits = PluxFunctionLimits(
    timeout: Duration(seconds: 5),
    memoryBytes: 32 * 1024 * 1024,
    inputBytes: 1024 * 1024,
    outputBytes: 1024 * 1024,
    logBytes: 4096,
  );
  const context = PluxFunctionContext(
    app: 'app-1',
    environment: 'prod',
    assuranceLevel: 'AL2',
  );

  setUpAll(() async {
    engine = PluxFunctions();
    module = await engine.load(key: 'probe', bytes: probe);
  });
  tearDownAll(() => engine.close());

  test(
    '[FN-050] the engine registers as the runtime\'s PluxFunctionEngine',
    () {
      expect(engine, isA<PluxFunctionEngine>());
      expect(engine.name, 'plux_functions');
    },
  );

  test(
    '[FN-052] key-value reads come from the snapshot and writes are journalled',
    () async {
      final port = ReceivePort();
      addTearDown(port.close);
      final writes = <List<Object?>>[];
      port.listen((m) => writes.add((m as List).cast()));
      final result = await engine.invoke(
        module: module,
        index: 1,
        input: utf8Bytes('k\nv'),
        limits: limits,
        context: context,
        kv: PluxFunctionKv(entries: {'seed': 's'}, journal: port.sendPort),
      );
      expect(result, isA<PluxFunctionOutput>());
      expect(jsonDecode(utf8.decode((result as PluxFunctionOutput).json)), {
        'put': 0,
        'get': 'v',
        'delete': 0,
        'after': -1,
      });
      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(writes, [
        ['k', 'v'],
        ['k', null],
      ]);
    },
  );

  test(
    '[FN-053] a cancelled token ends the invocation with PLX-7021',
    () async {
      final port = ReceivePort();
      addTearDown(port.close);
      final cancel = PluxFunctionCancel()..cancel();
      await expectLater(
        engine.invoke(
          module: module,
          index: 0,
          input: utf8Bytes('{}'),
          limits: limits,
          context: context,
          kv: PluxFunctionKv(entries: const {}, journal: port.sendPort),
          cancel: cancel,
        ),
        throwsA(plux(PluxErrorCode.functionCancelled)),
      );
    },
  );
}
