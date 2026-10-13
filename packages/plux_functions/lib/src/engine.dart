// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The device function engine (FN-050): WAMR's interpreter behind a worker
/// isolate.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:ffi';
import 'dart:isolate';
import 'dart:typed_data';

import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_functions/src/cancellation.dart';
import 'package:plux_functions/src/host.dart';
import 'package:plux_functions/src/limits.dart';
import 'package:plux_functions/src/native/plux_functions_native.dart';
import 'package:plux_functions/src/outcome.dart';
import 'package:plux_functions/src/worker.dart';

/// A module loaded into the engine, ready to be invoked. It stays valid until
/// [FunctionEngine.unload] or [FunctionEngine.close].
final class FunctionModule {
  FunctionModule._(this.key, this._id);

  /// The key it was loaded under: the SHA-256 of the plugin bundle's module,
  /// so a module is loaded once per bundle hash.
  final String key;
  final int _id;
}

/// Runs device-placed Plux Functions (FN-050).
///
/// Every call to the interpreter happens on one long-lived worker isolate and
/// never on the caller's, so a slow function cannot block the UI (FN-053).
/// The worker runs one invocation at a time; later ones queue. Each
/// invocation gets a fresh instance of the module, so nothing survives it
/// (FN-010).
final class FunctionEngine {
  FunctionEngine._() {
    _replies.listen(_onMessage);
    _exit.listen((_) {
      _fail('the worker isolate ended');
      if (!_ended.isCompleted) _ended.complete();
    });
  }

  /// Starts the worker isolate.
  static Future<FunctionEngine> start() async {
    final engine = FunctionEngine._();
    await Isolate.spawn<SendPort>(
      functionWorkerMain,
      engine._replies.sendPort,
      debugName: 'plux_functions.worker',
      onExit: engine._exit.sendPort,
    );
    await engine._ready.future;
    return engine;
  }

  final ReceivePort _replies = ReceivePort('plux_functions.replies');
  final ReceivePort _exit = ReceivePort('plux_functions.exit');
  final Completer<SendPort> _ready = Completer<SendPort>();
  final Completer<void> _ended = Completer<void>();
  late final SendPort _commands;
  final Map<String, Future<FunctionModule>> _modules = {};
  final Map<int, Completer<WorkerReply>> _pending = {};
  int _nextId = 0;
  bool _closed = false;

  void _onMessage(Object? message) {
    switch (message) {
      case SendPort():
        _commands = message;
        _ready.complete(message);
      case WorkerReply():
        _pending.remove(message.id)?.complete(message);
    }
  }

  void _fail(String reason) {
    _closed = true;
    final pending = _pending.values.toList();
    _pending.clear();
    for (final completer in pending) {
      completer.complete(WorkerReply(-1, 7007, _bytes(reason)));
    }
  }

  Future<WorkerReply> _request(Object Function(int id) build) {
    final id = _nextId++;
    final completer = Completer<WorkerReply>();
    if (_closed) {
      completer.complete(WorkerReply(id, 7007, _bytes('the engine is closed')));
    } else {
      _pending[id] = completer;
      _commands.send(build(id));
    }
    return completer.future;
  }

  /// Loads [bytes] once per [key] and checks it: imports, exports and ABI
  /// version. A second call with the same key returns the loaded module.
  ///
  /// Throws [PluxException] with `PLX-7018` (invalid module: an import or
  /// export that is unknown or has the wrong signature, or one that imports a
  /// server-only function) or `PLX-7017` (unsupported ABI version).
  Future<FunctionModule> load({
    required String key,
    required Uint8List bytes,
  }) async {
    final cached = _modules[key];
    if (cached != null) return cached;
    final loading = _load(key, bytes);
    _modules[key] = loading;
    try {
      return await loading;
    } on Object {
      if (identical(_modules[key], loading)) _modules.remove(key)?.ignore();
      rethrow;
    }
  }

  Future<FunctionModule> _load(String key, Uint8List bytes) async {
    final moduleId = _nextId++;
    final reply = await _request((id) => LoadRequest(id, moduleId, bytes));
    if (reply.status != 0) {
      throw PluxException(
        _codeOf(reply.status),
        utf8.decode(reply.body, allowMalformed: true),
      );
    }
    return FunctionModule._(key, moduleId);
  }

  /// Releases [module]; invocations already queued for it fail.
  void unload(FunctionModule module) {
    _modules.remove(module.key)?.ignore();
    if (!_closed) _commands.send(UnloadRequest(module._id));
  }

  /// Runs function [index] of [module] on [input] (UTF-8 JSON).
  ///
  /// Completes with the function's [FunctionOutput] or the
  /// [FunctionBusinessError] it returned. Throws [PluxException] for a
  /// failure of the platform: `PLX-7001` timeout, `PLX-7002` memory,
  /// `PLX-7004` output too large, `PLX-7007` trap or a result that cannot be read, `PLX-7008` the guest
  /// refused its input, `PLX-7009` its output is not valid, `PLX-7021`
  /// cancelled, `PLX-7022` input too large.
  ///
  /// [host] runs on the worker isolate; see [DeviceFunctionHost] for what it
  /// must be. [cancel], when given, ends the invocation as soon as it fires.
  Future<FunctionOutcome> invoke({
    required FunctionModule module,
    required int index,
    required Uint8List input,
    required FunctionLimits limits,
    required DeviceFunctionHost host,
    FunctionCancellation? cancel,
  }) async {
    if (cancel != null && cancel.isCancelled) {
      throw const PluxException(
        PluxErrorCode.functionCancelled,
        'the invocation was cancelled',
      );
    }
    if (input.length > limits.inputBytes) {
      throw PluxException(
        PluxErrorCode.functionInputTooLarge,
        'the input is ${input.length} bytes; the limit is ${limits.inputBytes}',
      );
    }
    final handle = pfInvocationNew();
    if (handle == nullptr) {
      throw const PluxException(PluxErrorCode.functionTrapped, 'out of memory');
    }
    if (cancel != null) attachCancellation(cancel, handle);
    final WorkerReply reply;
    try {
      reply = await _request(
        (id) => InvokeRequest(
          id: id,
          moduleId: module._id,
          index: index,
          input: input,
          timeoutMs: limits.timeout.inMilliseconds,
          memoryBytes: limits.memoryBytes,
          outputBytes: limits.outputBytes,
          logBytes: limits.logBytes,
          host: host,
          handle: handle.address,
        ),
      );
    } finally {
      if (cancel != null) detachCancellation(cancel, handle);
      pfInvocationFree(handle);
    }
    return _outcome(reply);
  }

  FunctionOutcome _outcome(WorkerReply reply) {
    switch (reply.status) {
      case 0:
        return FunctionOutput(reply.body);
      case 1:
        return _businessError(reply.body);
      default:
        throw PluxException(_codeOf(reply.status), _message(reply));
    }
  }

  /// Stops the worker and releases every module. Pending invocations fail.
  Future<void> close() async {
    if (_closed) return;
    _commands.send(const StopRequest());
    _closed = true;
    _modules.clear();
    await _ended.future;
    _replies.close();
    _exit.close();
  }
}

FunctionBusinessError _businessError(Uint8List body) {
  try {
    final decoded = jsonDecode(utf8.decode(body));
    if (decoded is Map<String, Object?>) {
      final code = decoded['code'];
      final key = decoded['messageKey'];
      final params = decoded['params'];
      if (code is String && key is String) {
        return FunctionBusinessError(
          code: code,
          messageKey: key,
          params: params is Map<String, Object?> ? params : const {},
        );
      }
    }
  } on FormatException {
    // Falls through to the output-invalid failure below.
  }
  throw const PluxException(
    PluxErrorCode.functionOutputInvalid,
    'the business error is not {code, messageKey, params}',
  );
}

/// The text of a failed reply: the guest's `{message}` for its own refusals,
/// the engine's diagnostic otherwise.
String _message(WorkerReply reply) {
  final text = utf8.decode(reply.body, allowMalformed: true);
  if (reply.status == 7008 || reply.status == 7009) {
    try {
      final decoded = jsonDecode(text);
      if (decoded is Map<String, Object?> && decoded['message'] is String) {
        return decoded['message']! as String;
      }
    } on FormatException {
      return text;
    }
  }
  return text;
}

PluxErrorCode _codeOf(int status) {
  for (final code in PluxErrorCode.values) {
    if (code.code == status) return code;
  }
  return PluxErrorCode.functionTrapped;
}

Uint8List _bytes(String text) => Uint8List.fromList(utf8.encode(text));
