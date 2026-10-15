// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The package's registration with the runtime (RT-060, E9): the host adds
/// `PluxFunctions()` to `PluxConfig.functionEngine`.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:isolate';
import 'dart:typed_data';

import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_functions/src/cancellation.dart';
import 'package:plux_functions/src/engine.dart';
import 'package:plux_functions/src/host.dart';
import 'package:plux_functions/src/limits.dart';
import 'package:plux_functions/src/outcome.dart';

/// Runs device-placed functions for the runtime (FN-050). The worker isolate
/// starts on first use, so an app that never calls a device function never
/// starts it.
final class PluxFunctions implements PluxFunctionEngine {
  /// Creates the engine; nothing starts until a module is loaded.
  PluxFunctions();

  Future<FunctionEngine>? _engine;

  Future<FunctionEngine> get _started => _engine ??= FunctionEngine.start();

  @override
  String get name => 'plux_functions';

  @override
  Future<Object> load({required String key, required Uint8List bytes}) async {
    return (await _started).load(key: key, bytes: bytes);
  }

  @override
  Future<PluxFunctionResult> invoke({
    required Object module,
    required int index,
    required Uint8List input,
    required PluxFunctionLimits limits,
    required PluxFunctionContext context,
    required PluxFunctionKv kv,
    PluxFunctionCancel? cancel,
  }) async {
    final engine = await _started;
    final token = FunctionCancellation();
    cancel?.onCancel(token.cancel);
    final outcome = await engine.invoke(
      module: module as FunctionModule,
      index: index,
      input: input,
      limits: FunctionLimits(
        timeout: limits.timeout,
        memoryBytes: limits.memoryBytes,
        inputBytes: limits.inputBytes,
        outputBytes: limits.outputBytes,
        logBytes: limits.logBytes,
      ),
      host: RuntimeFunctionHost(
        info: context,
        entries: Map.of(kv.entries),
        journal: kv.journal,
      ),
      cancel: token,
    );
    return switch (outcome) {
      FunctionOutput(:final json) => PluxFunctionOutput(json),
      FunctionBusinessError() => PluxFunctionBusinessError(
        code: outcome.code,
        messageKey: outcome.messageKey,
        params: outcome.params,
      ),
    };
  }

  /// Stops the worker and releases every module.
  Future<void> close() async {
    final started = _engine;
    _engine = null;
    if (started != null) await (await started).close();
  }
}

/// The host of an invocation the runtime starts (FN-052): `log` is dropped
/// (SEC-092), `clock` reads the device clock, `env_get` has no values on the
/// device, and the key-value namespace is the snapshot the runtime passed,
/// with every write reported on the journal port for the runtime to apply.
/// It holds data and a port only, so it crosses to the worker isolate.
final class RuntimeFunctionHost implements DeviceFunctionHost {
  /// Creates the host.
  RuntimeFunctionHost({
    required this.info,
    required this.entries,
    required this.journal,
  });

  /// What `context()` reports.
  final PluxFunctionContext info;

  /// The namespace; this copy lives on the worker isolate.
  final Map<String, String> entries;

  /// Receives `[key, value]` for a write and `[key, null]` for a delete.
  final SendPort journal;

  @override
  void log(FunctionLogLevel level, Uint8List message) {}

  @override
  int clockNowNanos() => DateTime.now().microsecondsSinceEpoch * 1000;

  @override
  Uint8List context() => Uint8List.fromList(
    utf8.encode(
      jsonEncode({
        'app': info.app,
        'environment': info.environment,
        'placement': 'device',
        'assuranceLevel': info.assuranceLevel,
      }),
    ),
  );

  @override
  Uint8List envGet(Uint8List name) => throw HostStatus.notFound;

  @override
  Uint8List kvGet(Uint8List key) {
    final value = entries[utf8.decode(key, allowMalformed: true)];
    if (value == null) throw HostStatus.notFound;
    return Uint8List.fromList(utf8.encode(value));
  }

  @override
  void kvPut(Uint8List key, Uint8List value) {
    final String text;
    try {
      text = utf8.decode(value);
    } on FormatException {
      throw HostStatus.invalidArgument;
    }
    final name = utf8.decode(key, allowMalformed: true);
    entries[name] = text;
    journal.send(<Object?>[name, text]);
  }

  @override
  void kvDelete(Uint8List key) {
    final name = utf8.decode(key, allowMalformed: true);
    if (entries.remove(name) == null) throw HostStatus.notFound;
    journal.send(<Object?>[name, null]);
  }
}
