// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Cancelling a device invocation (FN-053).
library;

import 'dart:ffi';

import 'package:plux_functions/src/native/plux_functions_native.dart';

/// A token that cancels invocations it is passed to. One token may serve
/// several invocations; cancelling it stops all of them, running or queued,
/// and any started later.
final class FunctionCancellation {
  /// Creates a token that has not been cancelled.
  FunctionCancellation();

  bool _cancelled = false;
  final Set<Pointer<PfInvocation>> _running = {};

  /// Whether [cancel] was called.
  bool get isCancelled => _cancelled;

  /// Ends every invocation using this token with `PLX-7021`. The call is
  /// made on the calling isolate: the worker is busy inside the interpreter,
  /// so the signal goes to the engine directly.
  void cancel() {
    _cancelled = true;
    for (final handle in _running) {
      pfCancel(handle);
    }
  }
}

/// Registers [handle] with [token] for the duration of one invocation; the
/// engine only (not exported).
void attachCancellation(
  FunctionCancellation token,
  Pointer<PfInvocation> handle,
) {
  token._running.add(handle);
  if (token._cancelled) pfCancel(handle);
}

/// Removes [handle] once its invocation has answered; the engine only.
void detachCancellation(
  FunctionCancellation token,
  Pointer<PfInvocation> handle,
) => token._running.remove(handle);
