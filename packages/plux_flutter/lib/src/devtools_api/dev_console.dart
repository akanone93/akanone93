// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The PXL console of a dev session (`DEV-032`): `evaluatePxl` evaluates an
/// expression against the page on screen and answers with its value. The
/// console only reads: an expression that assigns is refused, and the
/// values it sees are the ones the host hands over, without the state
/// tagged `sensitive` (`SCH-012`). Only builds with `kPluxDevtools` create
/// it.
library;

import 'dart:async';
import 'dart:convert';

import 'package:plux_flutter/src/devtools_api/dev_nodes.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/pxl/program.dart';
import 'package:plux_flutter/src/pxl/vm.dart';

/// Compiles [expression] to a PXL program; a [PluxException] carries the
/// code of why it does not compile. [roots] are the values it is compiled
/// against, so a compiler can check what the expression reads.
typedef DevPxlCompiler = FutureOr<Program> Function(
  String expression,
  Map<String, Object?> roots,
);

/// Carries out `evaluatePxl`.
final class DevConsoleHandler implements DevCommandHandler {
  /// Creates the handler. [roots] gives the PXL roots of the page on screen
  /// (`app`, `page`, `plugin`, and so on) without sensitive values.
  DevConsoleHandler({
    required this.compile,
    required this.roots,
    PxlLimits? limits,
  }) : limits = limits ?? PxlLimits.defaults();

  /// Compiles the expressions.
  final DevPxlCompiler compile;

  /// The values of the page on screen.
  final Map<String, Object?> Function() roots;

  /// The limits of one evaluation.
  final PxlLimits limits;

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    final body = devCommandBody(command, 'evaluatePxl');
    final expression = body?['expression'];
    if (expression is! String || expression.trim().isEmpty) {
      return devFailure(devStatusFailed, 'evaluatePxl needs an expression');
    }
    if (_assigns(expression)) {
      return devFailure(
        devStatusRefused,
        'the console reads state and does not write it',
        PluxErrorCode.devCommandNotAllowed,
      );
    }
    final values = roots();
    final Program program;
    try {
      program = await compile(expression, values);
    } on PluxException catch (e) {
      return devFailure(devStatusFailed, e.message, e.code);
    } on Object catch (e) {
      return devFailure(
        devStatusFailed,
        'the expression does not compile: ${e.runtimeType}',
        PluxErrorCode.pxlSyntaxError,
      );
    }
    final result = evaluate(program, values, limits);
    return switch (result) {
      PxlValue(:final value) => {
        'status': devStatusOk,
        'result': base64.encode(
          utf8.encode(jsonEncode(value, toEncodable: (o) => o.toString())),
        ),
      },
      PxlFailure(:final error) => devFailure(
        devStatusFailed,
        'pxl ${error.kind.name}: ${error.message}',
      ),
    };
  }
}

final RegExp _literals = RegExp(
  r'"(?:\\.|[^"\\])*"|'
  r"'(?:\\.|[^'\\])*'",
);
final RegExp _assignment = RegExp(r'(?<![=!<>])=(?![=>])');

/// Whether [expression] writes: an assignment outside the string literals.
bool _assigns(String expression) =>
    _assignment.hasMatch(expression.replaceAll(_literals, '""'));
