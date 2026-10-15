// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What the optional package `plux_functions` provides to the runtime
/// (FN-050, RT-060): the interface the core defines, so that an app pays
/// for the WebAssembly interpreter only when it registers the package in
/// `PluxConfig.functionEngine`.
library;

import 'dart:isolate';
import 'dart:typed_data';

/// Runs device-placed Plux Functions in an interpreter, off the UI isolate
/// (FN-050, FN-053). `plux_functions` implements it.
abstract interface class PluxFunctionEngine {
  /// The package's name, `plux_functions`.
  String get name;

  /// Loads and checks [bytes], the device module of a plugin bundle, once
  /// per [key] (the module's SHA-256, FN-051); a second call with the same
  /// key returns the loaded module. The handle is opaque to the runtime.
  ///
  /// Throws `PluxException` with `PLX-7018` or `PLX-7017` for a module the
  /// engine refuses.
  Future<Object> load({required String key, required Uint8List bytes});

  /// Runs function [index] of [module] on [input] (UTF-8 JSON).
  ///
  /// Completes with the function's result. Throws `PluxException` for a
  /// failure of the platform: a timeout, a limit, a trap, a cancellation.
  /// When [cancel] fires the invocation ends with `PLX-7021`.
  ///
  /// [kv] is the plugin's key-value namespace (FN-052); see [PluxFunctionKv].
  Future<PluxFunctionResult> invoke({
    required Object module,
    required int index,
    required Uint8List input,
    required PluxFunctionLimits limits,
    required PluxFunctionContext context,
    required PluxFunctionKv kv,
    PluxFunctionCancel? cancel,
  });
}

/// The limits of one device invocation, from the limits registry
/// (`function.deviceTimeout`, `function.deviceMemory`, `function.inputSize`,
/// `function.outputSize`, `function.logBytes`; LIM-001).
final class PluxFunctionLimits {
  /// Creates the limits.
  const PluxFunctionLimits({
    required this.timeout,
    required this.memoryBytes,
    required this.inputBytes,
    required this.outputBytes,
    required this.logBytes,
  });

  /// Wall-clock time the call may take.
  final Duration timeout;

  /// Memory the call may use.
  final int memoryBytes;

  /// Largest input.
  final int inputBytes;

  /// Largest output.
  final int outputBytes;

  /// Log bytes forwarded to the host.
  final int logBytes;
}

/// What a function sees of its invocation through `context()`. It holds
/// data only, so that it crosses to the engine's worker isolate.
final class PluxFunctionContext {
  /// Creates the context.
  const PluxFunctionContext({
    required this.app,
    required this.environment,
    required this.assuranceLevel,
  });

  /// The app's ID.
  final String app;

  /// The environment key.
  final String environment;

  /// The device's assurance level, `AL0` to `AL3`.
  final String assuranceLevel;
}

/// The plugin's key-value namespace as one invocation sees it (FN-052).
///
/// The engine's host runs on a worker isolate and cannot reach the
/// runtime's database, so an invocation gets a snapshot of the namespace
/// and reports what the function wrote. The host serves reads from
/// [entries] and the function's own writes. Every `kv_put` and `kv_delete`
/// is sent to [journal] as a list `[key, value]`, `value` being null for a
/// delete; the runtime applies the journal to the namespace once the
/// invocation has completed, and discards it when the invocation fails, so
/// a failed call leaves no partial write. Values are text: the host
/// answers `kv_put` of bytes that are not UTF-8 with the ABI status
/// "invalid argument". An implementation never logs keys or values
/// (SEC-092).
final class PluxFunctionKv {
  /// Creates the namespace view.
  const PluxFunctionKv({required this.entries, required this.journal});

  /// The namespace at the start of the invocation.
  final Map<String, String> entries;

  /// Where the host reports writes.
  final SendPort journal;
}

/// A token that ends the invocations it is passed to.
final class PluxFunctionCancel {
  /// Creates a token that has not fired.
  PluxFunctionCancel();

  bool _cancelled = false;
  final List<void Function()> _listeners = [];

  /// Whether [cancel] was called.
  bool get isCancelled => _cancelled;

  /// Calls [f] when the token fires, at once if it already has.
  void onCancel(void Function() f) {
    if (_cancelled) {
      f();
    } else {
      _listeners.add(f);
    }
  }

  /// Fires the token.
  void cancel() {
    if (_cancelled) return;
    _cancelled = true;
    final listeners = [..._listeners];
    _listeners.clear();
    for (final f in listeners) {
      f();
    }
  }
}

/// How a function that ran ended.
sealed class PluxFunctionResult {
  const PluxFunctionResult();
}

/// The function's output, as UTF-8 JSON.
final class PluxFunctionOutput extends PluxFunctionResult {
  /// Wraps [json].
  const PluxFunctionOutput(this.json);

  /// The output.
  final Uint8List json;
}

/// A business error the function returned (FN-022).
final class PluxFunctionBusinessError extends PluxFunctionResult {
  /// Creates the error.
  const PluxFunctionBusinessError({
    required this.code,
    required this.messageKey,
    required this.params,
  });

  /// The function-defined code.
  final String code;

  /// The localisation key of the message.
  final String messageKey;

  /// The values the message interpolates.
  final Map<String, Object?> params;
}
