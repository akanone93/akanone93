// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a device function answers when it runs to completion (FN-022).
library;

import 'dart:convert';
import 'dart:typed_data';

/// The answer of a function that ran: its output, or a business error it
/// chose to return. Failures of the platform — a trap, a limit, a refused
/// module — are not outcomes; they throw `PluxException`.
sealed class FunctionOutcome {
  const FunctionOutcome();
}

/// The function's output.
final class FunctionOutput extends FunctionOutcome {
  /// Wraps the UTF-8 [json] the function returned.
  const FunctionOutput(this.json);

  /// The output as UTF-8 JSON, exactly as the function produced it.
  final Uint8List json;

  /// The output decoded; decode on a background isolate when it is large.
  Object? decode() => jsonDecode(utf8.decode(json));
}

/// A business error the function returned (`PLX-7100` when it reaches an
/// action); the function defines its code.
final class FunctionBusinessError extends FunctionOutcome {
  /// Creates the error.
  const FunctionBusinessError({
    required this.code,
    required this.messageKey,
    required this.params,
  });

  /// The function-defined error code.
  final String code;

  /// The localisation key of its message.
  final String messageKey;

  /// Values the message interpolates.
  final Map<String, Object?> params;
}
