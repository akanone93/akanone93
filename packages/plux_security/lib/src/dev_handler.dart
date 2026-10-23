// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The `simulateSecurityDetection` dev command (DEV-001, ADR-0057 Revision
/// 2026-10-22): a developer injects a detection of a chosen kind to see how
/// the app reacts to it.
library;

import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_security/src/dev_mode.dart';
import 'package:plux_security/src/security.dart';

const _ok = 'DEV_COMMAND_STATUS_OK';
const _failed = 'DEV_COMMAND_STATUS_FAILED';
const _refused = 'DEV_COMMAND_STATUS_REFUSED';

/// The handler of `SimulateSecurityDetectionCommand` for [security].
///
/// The Dev app registers it under the key `simulateSecurityDetection` in
/// `RuntimeDevCommands.extensions`. Only builds with [kPluxDevtools] should
/// create it; in a module that does not run the development mode every
/// command is refused.
DevCommandHandler pluxSecuritySimulateDetectionHandler(PluxSecurity security) =>
    _SimulateDetectionHandler(security);

final class _SimulateDetectionHandler implements DevCommandHandler {
  _SimulateDetectionHandler(this._security);

  final PluxSecurity _security;

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    final body = command['simulateSecurityDetection'];
    if (body is! Map<String, Object?>) {
      return _result(_failed, 'simulateSecurityDetection is not an object');
    }
    final kind = kindFromDevValue(body['kind']);
    if (kind == null) {
      return _result(_failed, 'kind is not a known security detection kind');
    }
    if (!_security.simulateDetection(kind)) {
      return _result(
        _refused,
        'the security module does not run the development mode',
      );
    }
    return {'status': _ok};
  }

  Map<String, Object?> _result(String status, String message) => {
    'status': status,
    'error': {'message': message},
  };
}
