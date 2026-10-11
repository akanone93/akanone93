// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The handler of `signTransaction` (SEC-027, SEC-028, ADR-0056): asks the
/// server for a challenge, shows the amount, the currency and the payee,
/// asks for the user's biometric or device credential and signs. The
/// confirmation is held for the `apiCall` that sends the same transaction;
/// a request that differs from it is refused before it leaves the device.
library;

import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/pxl/values.dart';
import 'package:plux_flutter/src/security/sca.dart';

/// The handlers of the security actions this runtime runs, by action name.
final Map<String, ActionHandler> securityHandlers = {
  'signTransaction': actionHandler(_signTransaction, waitsForUser: true),
};

/// `signTransaction`: `transaction` names the operation to confirm,
/// `<source>.<operation>` as `apiCall` does; `amount` and `payee` are what
/// the user is shown. The output is the challenge's ID; the `declined`
/// branch is taken when the user declines, or the step fails when the step
/// wires none.
Future<StepResult> _signTransaction(
  StepContext c,
  Map<String, Object?> i,
) async {
  final sca = c.service<ScaService>();
  if (sca == null) {
    throw const ActionError(
      ActionErrorKind.custom,
      PluxErrorCode.actionsNotAvailable,
      'signTransaction: this runtime cannot sign transactions',
    );
  }
  final ref = i['transaction'];
  final parts = ref is String ? ref.split('.') : const <String>[];
  if (parts.length != 2 || parts.any((p) => p.isEmpty)) {
    throw const ActionError.validation(
      'signTransaction: transaction is not "<source>.<operation>"',
    );
  }
  final payee = i['payee'];
  if (payee is! String || payee.isEmpty) {
    throw const ActionError.validation(
      'signTransaction: payee is not a non-empty string',
    );
  }
  final (amount, currency) = switch (i['amount']) {
    final Money m => (m.amount.toString(), m.currency),
    final Map<String, Object?> m
        when m['amount'] is String && m['currency'] is String =>
      (m['amount']! as String, m['currency']! as String),
    _ => throw const ActionError.validation(
      'signTransaction: amount is not money',
    ),
  };
  try {
    final confirmation = await sca.confirmAhead(
      TransactionRequest(
        dataSource: parts[0],
        operation: parts[1],
        summary: TransactionSummary(
          amount: amount,
          currency: currency,
          payee: payee,
        ),
      ),
      on: c.device?.context?.call(),
    );
    return StepDone(confirmation.challengeId);
  } on ScaDeclined {
    if (c.wiredBranches.contains('declined')) {
      return const StepDone(null, 'declined');
    }
    throw const ActionError(
      ActionErrorKind.cancelled,
      PluxErrorCode.actionCancelled,
      'signTransaction: the user declined the transaction',
    );
  } on PluxException catch (e) {
    throw ActionError(
      e.code == PluxErrorCode.deviceKeyUnavailable
          ? ActionErrorKind.custom
          : ActionErrorKind.permission,
      e.code,
      'signTransaction: ${e.message}',
    );
  }
}
