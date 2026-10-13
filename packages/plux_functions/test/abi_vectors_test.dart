// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The shared ABI conformance vectors (FN-015) on the device engine.
///
/// Every case of `schema/testdata/functions/abi-v1.json` runs. A case on a
/// module that imports a server-only function must be refused by the device
/// engine at load time with `PLX-7018`; every other case runs and is compared
/// with its `expect` block.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:isolate';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_functions/plux_functions.dart';

const _vectors = '../../schema/testdata/functions';

/// The module that carries the server-only imports.
const _serverOnlyModule = 'probe';

/// A fake host built from a case's `host` and `grants` blocks. It is sent to
/// the worker isolate, so it holds data and a port, not live state.
final class _VectorHost implements DeviceFunctionHost {
  _VectorHost({
    required this.contextJson,
    required this.env,
    required this.kv,
    required this.kvGranted,
    required this.fail,
    required this.events,
  });

  final Map<String, Object?> contextJson;
  final Map<String, String> env;
  final Map<String, String> kv;
  final bool kvGranted;
  final Map<String, String> fail;
  final SendPort events;

  static const _statuses = {
    'notFound': HostStatus.notFound,
    'limit': HostStatus.limitExceeded,
    'unavailable': HostStatus.unavailable,
  };

  void _check(String capability) {
    if (!kvGranted) throw HostStatus.capabilityDenied;
    final failure = fail[capability];
    if (failure != null) throw _statuses[failure] ?? HostStatus.unavailable;
  }

  @override
  void log(FunctionLogLevel level, Uint8List message) =>
      events.send([level.index, utf8.decode(message, allowMalformed: true)]);

  @override
  int clockNowNanos() => 1700000000000000000;

  @override
  Uint8List context() => _bytes(jsonEncode(contextJson));

  @override
  Uint8List envGet(Uint8List name) {
    final value = env[utf8.decode(name)];
    if (value == null) throw HostStatus.notFound;
    return _bytes(value);
  }

  @override
  Uint8List kvGet(Uint8List key) {
    _check('kvGet');
    final value = kv[utf8.decode(key)];
    if (value == null) throw HostStatus.notFound;
    return _bytes(value);
  }

  @override
  void kvPut(Uint8List key, Uint8List value) {
    _check('kvPut');
    kv[utf8.decode(key)] = utf8.decode(value);
  }

  @override
  void kvDelete(Uint8List key) {
    _check('kvDelete');
    if (kv.remove(utf8.decode(key)) == null) throw HostStatus.notFound;
  }
}

Uint8List _bytes(String text) => Uint8List.fromList(utf8.encode(text));

Map<String, Object?> _merge(
  Map<String, Object?> base,
  Map<String, Object?> over,
) {
  final merged = {...base};
  for (final entry in over.entries) {
    final current = merged[entry.key];
    final value = entry.value;
    merged[entry.key] =
        current is Map<String, Object?> && value is Map<String, Object?>
        ? _merge(current, value)
        : value;
  }
  return merged;
}

Uint8List _module(Map<String, Object?> spec) {
  final file = spec['file'];
  if (file is String) return File('$_vectors/$file').readAsBytesSync();
  final hex = spec['hex']! as String;
  return Uint8List.fromList([
    for (var i = 0; i < hex.length; i += 2)
      int.parse(hex.substring(i, i + 2), radix: 16),
  ]);
}

Map<String, Object?> _map(Object? value) => (value! as Map).cast();

void main() {
  final document = _map(
    jsonDecode(File('$_vectors/abi-v1.json').readAsStringSync()),
  );
  final defaults = _map(document['defaults']);
  final modules = _map(document['modules']);
  final cases = (document['cases']! as List).map(_map).toList();
  late FunctionEngine engine;

  setUpAll(() async => engine = await FunctionEngine.start());
  tearDownAll(() => engine.close());

  test('[FN-015] the vectors file has the 99 cases of version 1', () {
    expect(document['version'], 1);
    expect(cases, hasLength(99));
  });

  for (final raw in cases) {
    final spec = _merge(defaults, raw);
    final name = spec['name']! as String;
    final moduleName = spec['module']! as String;
    final expect_ = _map(spec['expect']);

    test('[FN-015] $name', () async {
      final bytes = _module(_map(modules[moduleName]));
      final key = '$moduleName-$name';
      if (moduleName == _serverOnlyModule) {
        await expectLater(
          engine.load(key: key, bytes: bytes),
          throwsA(_code(7018)),
          reason: 'the device engine refuses a server-only import',
        );
        return;
      }
      if (expect_.containsKey('compileError')) {
        await expectLater(
          engine.load(key: key, bytes: bytes),
          throwsA(_code(expect_['compileError']! as int)),
        );
        return;
      }
      final module = await engine.load(key: key, bytes: bytes);
      final limits = _map(spec['limits']);
      final grants = _map(spec['grants']);
      final context = _map(spec['context']);
      final logs = ReceivePort();
      addTearDown(logs.close);
      final seen = <List<Object?>>[];
      logs.listen((message) => seen.add((message as List).cast()));
      final host = _VectorHost(
        contextJson: {
          'app': context['app'],
          'environment': context['environment'],
          'placement': 'device',
          'assuranceLevel': context['assuranceLevel'],
        },
        env: _map(spec['env']).cast<String, String>(),
        kv: _map(_map(spec['host'])['kv']).cast<String, String>(),
        kvGranted: grants['kv'] == true,
        fail: _map(_map(spec['host'])['fail']).cast<String, String>(),
        events: logs.sendPort,
      );
      final run = engine.invoke(
        module: module,
        index: spec['index']! as int,
        input: _bytes(spec['input']! as String),
        limits: FunctionLimits(
          timeout: Duration(milliseconds: limits['timeoutMs']! as int),
          memoryBytes: limits['memoryBytes']! as int,
          inputBytes: limits['inputBytes']! as int,
          outputBytes: limits['outputBytes']! as int,
          logBytes: limits['logBytes']! as int,
        ),
        host: host,
      );
      if (expect_.containsKey('errorCode')) {
        await expectLater(run, throwsA(_code(expect_['errorCode']! as int)));
        return;
      }
      final outcome = await run;
      if (expect_['tag'] == 1) {
        expect(outcome, isA<FunctionBusinessError>());
        final business = _map(expect_['business']);
        final error = outcome as FunctionBusinessError;
        expect(error.code, business['code']);
        expect(error.messageKey, business['messageKey']);
        expect(error.params, business['params']);
        return;
      }
      expect(outcome, isA<FunctionOutput>());
      final json = (outcome as FunctionOutput).json;
      if (expect_.containsKey('outputText')) {
        expect(utf8.decode(json), expect_['outputText']);
      }
      if (expect_.containsKey('output')) {
        expect(outcome.decode(), expect_['output']);
      }
      if (expect_.containsKey('outputLength')) {
        expect(json.length, expect_['outputLength']);
      }
      if (expect_.containsKey('logs')) {
        await Future<void>.delayed(Duration.zero);
        expect(seen, [
          for (final log in expect_['logs']! as List)
            [_map(log)['level'], _map(log)['message']],
        ]);
      }
    });
  }
}

Matcher _code(int number) =>
    isA<PluxException>().having((e) => e.code.code, 'code', number);
