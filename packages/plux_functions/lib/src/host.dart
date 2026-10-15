// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The host side of the `plux_v1` imports a device function may call
/// (ADR-0062, FN-052).
library;

import 'dart:typed_data';

/// The severity of a guest log line, as the ABI `log` import numbers it.
enum FunctionLogLevel {
  /// Debug.
  debug,

  /// Info; also what a guest writes to standard output.
  info,

  /// Warning.
  warn,

  /// Error; also what a guest writes to standard error.
  error,
}

/// A negative status a host call answers with (ADR-0062). The guest's SDK
/// maps it to its own error; `capabilityDenied` becomes `PLX-7003`.
final class HostStatus implements Exception {
  /// A status with ABI value [value], which is negative.
  const HostStatus(this.value);

  /// The key or name does not exist.
  static const notFound = HostStatus(-1);

  /// The function was not granted the capability.
  static const capabilityDenied = HostStatus(-2);

  /// A limit would be exceeded.
  static const limitExceeded = HostStatus(-3);

  /// An argument is not acceptable.
  static const invalidArgument = HostStatus(-6);

  /// The service cannot answer now.
  static const unavailable = HostStatus(-7);

  /// The ABI status.
  final int value;

  @override
  String toString() => 'HostStatus($value)';
}

/// What a device function may reach: `log`, `clock` and the plugin's
/// key-value namespace, `env_get` for the environment's public values, and
/// the invocation context (FN-052). Nothing else exists on the device.
///
/// **Isolates.** The engine calls the host on its worker isolate, in the
/// middle of the guest's call, so every method is synchronous and the
/// implementation must be sendable to that isolate: it carries data and
/// paths, not live handles. State it keeps must live where the worker can
/// reach it (a file, a database), not in the sending isolate's memory.
///
/// An implementation never logs, traces or persists inputs, outputs or
/// values it is given beyond what the plugin's own store holds (SEC-092).
abstract interface class DeviceFunctionHost {
  /// A line the guest logged, already bounded by the log limit. [message] is
  /// UTF-8 and may be cut inside a character.
  void log(FunctionLogLevel level, Uint8List message);

  /// The Unix time in nanoseconds.
  int clockNowNanos();

  /// The invocation context as JSON: `{"app","environment","placement",
  /// "assuranceLevel"}`.
  Uint8List context();

  /// The environment value named [name]; throws [HostStatus] when it cannot
  /// be read.
  Uint8List envGet(Uint8List name);

  /// The value of [key] in the plugin's namespace; throws
  /// [HostStatus.notFound] when absent.
  Uint8List kvGet(Uint8List key);

  /// Stores [value] under [key]; throws [HostStatus] when refused.
  void kvPut(Uint8List key, Uint8List value);

  /// Deletes [key]; throws [HostStatus.notFound] when absent.
  void kvDelete(Uint8List key);
}
