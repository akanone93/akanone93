// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:isolate';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_functions/plux_functions.dart';

import 'support/support.dart';
import 'support/test_host.dart';

void main() {
  late FunctionEngine engine;
  late FunctionModule probe;
  late Directory scratch;
  late ReceivePort port;
  late List<List<Object?>> events;

  setUpAll(() async {
    engine = await FunctionEngine.start();
    probe = await engine.load(key: 'probe', bytes: fixture('probe.wasm'));
    scratch = Directory.systemTemp.createTempSync('plux_functions_host');
    port = ReceivePort();
    events = [];
    port.listen((Object? message) => events.add((message! as List).cast()));
  });

  tearDownAll(() async {
    await engine.close();
    port.close();
    scratch.deleteSync(recursive: true);
  });

  TestHost host({Map<String, String> environment = const {}}) => TestHost(
    directory: scratch.path,
    events: port.sendPort,
    environment: environment,
  );

  Future<String> run(
    String command, {
    FunctionLimits limits = roomy,
    TestHost? with_,
  }) async => outputText(
    await engine.invoke(
      module: probe,
      index: 0,
      input: utf8Bytes(command),
      limits: limits,
      host: with_ ?? host(),
    ),
  );

  Future<void> settle() =>
      Future<void>.delayed(const Duration(milliseconds: 50));

  test(
    '[FN-052] the key-value namespace round trips through the Dart host',
    () async {
      expect(await run('kv_get greeting'), '-1');
      expect(await run('kv_put greeting hello world'), '0');
      expect(await run('kv_get greeting'), '"hello world"');
      expect(await run('kv_delete greeting'), '0');
      expect(await run('kv_get greeting'), '-1');
      expect(await run('kv_delete greeting'), '-1');
    },
  );

  test('[FN-052] an empty value is stored and read back', () async {
    expect(await run('kv_put empty '), '0');
    expect(await run('kv_get empty'), '""');
  });

  test(
    '[FN-053] the host runs on the worker isolate, not the caller\'s',
    () async {
      events.clear();
      final context = jsonDecode(await run('context')) as Map<String, Object?>;
      expect(context['placement'], 'device');
      await settle();
      final seen = events.where((e) => e[0] == 'isolate').single[1];
      expect(seen, 'plux_functions.worker');
      expect(seen, isNot(Isolate.current.debugName));
    },
  );

  test('[FN-052] env_get answers from the host and -1 when absent', () async {
    expect(
      await run('env region', with_: host(environment: {'region': 'eu'})),
      '"eu"',
    );
    expect(await run('env missing'), '-1');
  });

  test('[FN-052] clock_now answers the host clock', () async {
    expect(await run('clock'), '1700000000000000000');
  });

  test('[FN-052] log, stdout and stderr reach the host as log lines', () async {
    events.clear();
    await run('log from the import');
    await run('stdout from stdout');
    await run('stderr from stderr');
    await settle();
    final logs = events.where((e) => e[0] == 'log').toList();
    String at(FunctionLogLevel level) => logs
        .where((e) => e[1] == level.index)
        .map((e) => e[2]! as String)
        .join('|');
    // The standard library writes stderr unbuffered, in pieces.
    expect(at(FunctionLogLevel.info), 'from the import|from stdout\n');
    expect(at(FunctionLogLevel.error).replaceAll('|', ''), 'from stderr\n');
  });

  test('[FN-053] log output beyond the limit is dropped', () async {
    events.clear();
    const limits = FunctionLimits(
      timeout: Duration(seconds: 5),
      memoryBytes: 32 * 1024 * 1024,
      inputBytes: 1024,
      outputBytes: 1024,
      logBytes: 10,
    );
    expect(await run('flood 500', limits: limits), '0');
    await settle();
    final logged = events
        .where((e) => e[0] == 'log')
        .map((e) => e[2]! as String);
    expect(logged.join().length, 10);
  });

  test('[FN-052] the WASI shim serves random_get from the host', () async {
    expect(await run('random'), '4');
  });

  test(
    '[FN-052] an unavailable host call answers -7 instead of failing the call',
    () async {
      expect(await run('kv_get missing-key'), '-1');
    },
  );
}
