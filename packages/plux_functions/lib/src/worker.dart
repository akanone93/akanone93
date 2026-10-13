// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The worker isolate: the only place the interpreter runs (FN-053). It owns
/// every loaded module, runs one invocation at a time, and answers the guest's
/// host calls from the invocation's [DeviceFunctionHost].
library;

import 'dart:convert';
import 'dart:ffi';
import 'dart:isolate';
import 'dart:typed_data';

import 'package:plux_functions/src/host.dart';
import 'package:plux_functions/src/native/plux_functions_native.dart';

/// Asks the worker to load a module.
final class LoadRequest {
  /// Creates the request.
  const LoadRequest(this.id, this.moduleId, this.bytes);

  /// The request's correlation id.
  final int id;

  /// The id the worker files the module under.
  final int moduleId;

  /// The WebAssembly binary.
  final Uint8List bytes;
}

/// Asks the worker to release a module.
final class UnloadRequest {
  /// Creates the request.
  const UnloadRequest(this.moduleId);

  /// The module to release.
  final int moduleId;
}

/// Asks the worker to run one function.
final class InvokeRequest {
  /// Creates the request.
  const InvokeRequest({
    required this.id,
    required this.moduleId,
    required this.index,
    required this.input,
    required this.timeoutMs,
    required this.memoryBytes,
    required this.outputBytes,
    required this.logBytes,
    required this.host,
    required this.handle,
  });

  /// The request's correlation id.
  final int id;

  /// The module to run.
  final int moduleId;

  /// The function's index in the module's dispatch table.
  final int index;

  /// The input JSON.
  final Uint8List input;

  /// `FunctionLimits.timeout` in milliseconds.
  final int timeoutMs;

  /// `FunctionLimits.memoryBytes`.
  final int memoryBytes;

  /// `FunctionLimits.outputBytes`.
  final int outputBytes;

  /// `FunctionLimits.logBytes`.
  final int logBytes;

  /// The host the guest's imports reach.
  final DeviceFunctionHost host;

  /// The address of the invocation's cancellation handle.
  final int handle;
}

/// Asks the worker to stop.
final class StopRequest {
  /// Creates the request.
  const StopRequest();
}

/// The worker's answer to a [LoadRequest] or an [InvokeRequest].
final class WorkerReply {
  /// Creates the reply.
  const WorkerReply(this.id, this.status, this.body);

  /// The request's correlation id.
  final int id;

  /// Zero (output), one (business error) or a PLX code.
  final int status;

  /// The output JSON, the business error JSON, or a diagnostic.
  final Uint8List body;
}

/// The entry point of the worker isolate; [toEngine] receives the worker's
/// command port first.
void functionWorkerMain(SendPort toEngine) {
  final commands = ReceivePort();
  final worker = _Worker(toEngine);
  commands.listen((Object? message) {
    switch (message) {
      case LoadRequest():
        worker.load(message);
      case UnloadRequest():
        worker.unload(message);
      case InvokeRequest():
        worker.invoke(message);
      case StopRequest():
        worker.close();
        commands.close();
    }
  });
  toEngine.send(commands.sendPort);
}

final class _Worker {
  _Worker(this._toEngine) {
    _ready = pfInit() == 0;
    _host = _allocate(sizeOf<PfHost>()).cast<PfHost>();
    _callables = [
      _log = NativeCallable<PfLogNative>.isolateLocal(_onLog),
      _clock = NativeCallable<PfClockNative>.isolateLocal(
        _onClock,
        exceptionalReturn: 0,
      ),
      _context = NativeCallable<PfContextNative>.isolateLocal(
        _onContext,
        exceptionalReturn: -7,
      ),
      _envGet = NativeCallable<PfReadNative>.isolateLocal(
        _onEnvGet,
        exceptionalReturn: -7,
      ),
      _kvGet = NativeCallable<PfReadNative>.isolateLocal(
        _onKvGet,
        exceptionalReturn: -7,
      ),
      _kvPut = NativeCallable<PfWriteNative>.isolateLocal(
        _onKvPut,
        exceptionalReturn: -7,
      ),
      _kvDelete = NativeCallable<PfDeleteNative>.isolateLocal(
        _onKvDelete,
        exceptionalReturn: -7,
      ),
    ];
    _host.ref
      ..log = _log.nativeFunction
      ..clockNow = _clock.nativeFunction
      ..context = _context.nativeFunction
      ..envGet = _envGet.nativeFunction
      ..kvGet = _kvGet.nativeFunction
      ..kvPut = _kvPut.nativeFunction
      ..kvDelete = _kvDelete.nativeFunction;
  }

  final SendPort _toEngine;
  final Map<int, Pointer<PfModule>> _modules = {};
  late final bool _ready;
  late final Pointer<PfHost> _host;
  late final List<NativeCallable<Function>> _callables;
  late final NativeCallable<PfLogNative> _log;
  late final NativeCallable<PfClockNative> _clock;
  late final NativeCallable<PfContextNative> _context;
  late final NativeCallable<PfReadNative> _envGet;
  late final NativeCallable<PfReadNative> _kvGet;
  late final NativeCallable<PfWriteNative> _kvPut;
  late final NativeCallable<PfDeleteNative> _kvDelete;

  /// The host of the invocation in progress; the worker runs one at a time.
  DeviceFunctionHost? _current;

  void load(LoadRequest request) {
    if (!_ready) {
      _toEngine.send(
        WorkerReply(request.id, 7018, _utf8('the runtime did not start')),
      );
      return;
    }
    final bytes = _copyIn(request.bytes);
    final status = _allocate(4).cast<Int32>();
    const capacity = 256;
    final error = _allocate(capacity);
    try {
      final module = pfModuleLoad(
        bytes,
        request.bytes.length,
        status,
        error,
        capacity,
      );
      if (module == nullptr) {
        _toEngine.send(
          WorkerReply(
            request.id,
            status.value,
            Uint8List.fromList(error.asTypedList(_cLength(error, capacity))),
          ),
        );
        return;
      }
      _modules[request.moduleId] = module;
      _toEngine.send(WorkerReply(request.id, 0, Uint8List(0)));
    } finally {
      pfFree(bytes.cast());
      pfFree(status.cast());
      pfFree(error.cast());
    }
  }

  void unload(UnloadRequest request) {
    final module = _modules.remove(request.moduleId);
    if (module != null) pfModuleFree(module);
  }

  void invoke(InvokeRequest request) {
    final module = _modules[request.moduleId];
    if (module == null) {
      _toEngine.send(
        WorkerReply(request.id, 7018, _utf8('the module is not loaded')),
      );
      return;
    }
    final input = _copyIn(request.input);
    final limits = _allocate(sizeOf<PfLimits>()).cast<PfLimits>();
    limits.ref
      ..timeoutMs = request.timeoutMs
      ..memoryBytes = request.memoryBytes
      ..outputBytes = request.outputBytes
      ..logBytes = request.logBytes;
    final out = _allocate(sizeOf<IntPtr>()).cast<Pointer<Uint8>>();
    final outLength = _allocate(4).cast<Uint32>();
    out.value = nullptr;
    outLength.value = 0;
    _current = request.host;
    try {
      final status = pfInvoke(
        module,
        request.index,
        input,
        request.input.length,
        limits,
        _host,
        Pointer<PfInvocation>.fromAddress(request.handle),
        out,
        outLength,
      );
      final body = out.value == nullptr
          ? Uint8List(0)
          : Uint8List.fromList(out.value.asTypedList(outLength.value));
      _toEngine.send(WorkerReply(request.id, status, body));
    } finally {
      _current = null;
      if (out.value != nullptr) pfFree(out.value.cast());
      pfFree(input.cast());
      pfFree(limits.cast());
      pfFree(out.cast());
      pfFree(outLength.cast());
    }
  }

  void close() {
    _modules.values.forEach(pfModuleFree);
    _modules.clear();
    for (final callable in _callables) {
      callable.close();
    }
    pfFree(_host.cast());
  }

  // The host calls below run inside pf_invoke, on this isolate.

  void _onLog(int level, Pointer<Uint8> message, int length) {
    final host = _current;
    if (host == null) return;
    try {
      host.log(
        FunctionLogLevel.values[level.clamp(0, 3)],
        Uint8List.fromList(message.asTypedList(length)),
      );
    } on Object {
      // A logging failure never reaches the guest.
    }
  }

  int _onClock() {
    try {
      return _current?.clockNowNanos() ??
          DateTime.now().microsecondsSinceEpoch * 1000;
    } on Object {
      return DateTime.now().microsecondsSinceEpoch * 1000;
    }
  }

  int _onContext(Pointer<Pointer<Uint8>> out, Pointer<Uint32> length) =>
      _answer(() => _current!.context(), out, length);

  int _onEnvGet(
    Pointer<Uint8> key,
    int keyLength,
    Pointer<Pointer<Uint8>> out,
    Pointer<Uint32> length,
  ) => _answer(
    () => _current!.envGet(Uint8List.fromList(key.asTypedList(keyLength))),
    out,
    length,
  );

  int _onKvGet(
    Pointer<Uint8> key,
    int keyLength,
    Pointer<Pointer<Uint8>> out,
    Pointer<Uint32> length,
  ) => _answer(
    () => _current!.kvGet(Uint8List.fromList(key.asTypedList(keyLength))),
    out,
    length,
  );

  int _onKvPut(
    Pointer<Uint8> key,
    int keyLength,
    Pointer<Uint8> value,
    int valueLength,
  ) => _status(
    () => _current!.kvPut(
      Uint8List.fromList(key.asTypedList(keyLength)),
      Uint8List.fromList(value.asTypedList(valueLength)),
    ),
  );

  int _onKvDelete(Pointer<Uint8> key, int keyLength) => _status(
    () => _current!.kvDelete(Uint8List.fromList(key.asTypedList(keyLength))),
  );

  /// Runs [read] and hands its bytes to the engine in memory the engine
  /// frees; a [HostStatus] or any failure becomes a negative status.
  int _answer(
    Uint8List Function() read,
    Pointer<Pointer<Uint8>> out,
    Pointer<Uint32> length,
  ) {
    try {
      final bytes = read();
      final copy = _allocate(bytes.isEmpty ? 1 : bytes.length)
        ..asTypedList(bytes.length).setAll(0, bytes);
      out.value = copy;
      length.value = bytes.length;
      return 0;
    } on HostStatus catch (status) {
      return _negative(status.value);
    } on Object {
      return -7;
    }
  }

  int _status(void Function() action) {
    try {
      action();
      return 0;
    } on HostStatus catch (status) {
      return _negative(status.value);
    } on Object {
      return -7;
    }
  }

  int _negative(int value) => value < 0 ? value : -7;
}

Pointer<Uint8> _allocate(int size) => pfAlloc(size);

Pointer<Uint8> _copyIn(Uint8List bytes) {
  final pointer = _allocate(bytes.isEmpty ? 1 : bytes.length);
  pointer.asTypedList(bytes.length).setAll(0, bytes);
  return pointer;
}

int _cLength(Pointer<Uint8> text, int capacity) {
  var length = 0;
  while (length < capacity && text[length] != 0) {
    length++;
  }
  return length;
}

Uint8List _utf8(String text) => Uint8List.fromList(utf8.encode(text));
