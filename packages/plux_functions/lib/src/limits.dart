// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The limits of one device invocation (FN-053, LIM-001).
library;

/// Limits of one invocation. The caller takes every value from the limits
/// registry; the engine has no defaults of its own (LIM-001).
final class FunctionLimits {
  /// Creates the limits.
  const FunctionLimits({
    required this.timeout,
    required this.memoryBytes,
    required this.inputBytes,
    required this.outputBytes,
    required this.logBytes,
  });

  /// Wall-clock time the call may take, from the moment the worker starts it;
  /// exceeding it ends the call with `PLX-7001`.
  final Duration timeout;

  /// Bytes the engine may allocate for the call: the guest's linear memory,
  /// its operand stack and the instance. Exceeding it is `PLX-7002`.
  final int memoryBytes;

  /// Largest input in bytes; a larger one is `PLX-7022`.
  final int inputBytes;

  /// Largest output in bytes; a larger one is `PLX-7004`.
  final int outputBytes;

  /// Bytes of log output forwarded to the host; the rest is dropped.
  final int logBytes;
}
