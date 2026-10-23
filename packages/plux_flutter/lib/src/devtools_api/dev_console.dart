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
import 'dart:typed_data';

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
///
/// The server compiles the expression and sends the program with the
/// command; the handler decodes and verifies it before it runs. A
/// [compile] function is the fallback for a command that carries no
/// program; without one, such a command is unsupported (`PLX-9232`).
final class DevConsoleHandler implements DevCommandHandler {
  /// Creates the handler. [roots] gives the PXL roots of the page on screen
  /// (`app`, `page`, `plugin`, and so on) without sensitive values.
  DevConsoleHandler({required this.roots, this.compile, PxlLimits? limits})
    : limits = limits ?? PxlLimits.defaults();

  /// Compiles the expressions of commands without a program.
  final DevPxlCompiler? compile;

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
      final bytes = _programBytes(body?['program']);
      if (bytes != null) {
        program = Program.decode(bytes);
      } else {
        final compiler = compile;
        if (compiler == null) {
          return devFailure(
            devStatusFailed,
            'the command carries no compiled program',
            PluxErrorCode.devCommandUnsupported,
          );
        }
        program = await compiler(expression, values);
      }
    } on InvalidProgram catch (e) {
      return devFailure(
        devStatusFailed,
        'the program is invalid: ${e.message}',
        PluxErrorCode.pxlSyntaxError,
      );
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

/// The bytes of the `program` field, which the JSON form of the command
/// carries as base64; null when it is absent or empty.
Uint8List? _programBytes(Object? field) {
  if (field is Uint8List) return field.isEmpty ? null : field;
  if (field is! String || field.isEmpty) return null;
  return base64.decode(field);
}

final RegExp _literals = RegExp(
  r'"(?:\\.|[^"\\])*"|'
  r"'(?:\\.|[^'\\])*'",
);
final RegExp _assignment = RegExp(r'(?<![=!<>])=(?![=>])');

/// Whether [expression] writes: an assignment outside the string literals.
bool _assigns(String expression) =>
    _assignment.hasMatch(expression.replaceAll(_literals, '""'));
