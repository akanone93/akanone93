// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The `invokeFunction` action (FN-020, FN-050 to FN-055): a Plux Function
/// runs on the device, in the optional engine, or on the server through
/// `FunctionService.Invoke`, according to its placement. The placement is
/// the function's, never the step's (FN-008).
library;

import 'dart:async';
import 'dart:convert';
import 'dart:isolate';
import 'dart:typed_data';

import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/data/client.dart' show AuthSession;
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/functions/engine.dart';
import 'package:plux_flutter/src/pxl/types.dart' show toJson;
import 'package:plux_flutter/src/schema/limit_values.dart';
import 'package:plux_flutter/src/schema/limits.g.dart';
import 'package:plux_flutter/src/security/sca.dart';

/// A function a plugin may call, resolved against the plugin's grants and
/// the active release.
sealed class FunctionTarget {
  const FunctionTarget(this.name, this.version);

  /// The function's dotted name (FN-006).
  final String name;

  /// The version the release pinned; 0 where the device does not know it yet.
  final int version;
}

/// A function placed on the device (FN-051): the entry in the plugin's
/// device module.
final class DeviceFunctionTarget extends FunctionTarget {
  /// Creates the target.
  const DeviceFunctionTarget({
    required String name,
    required int version,
    required this.moduleHash,
    required this.module,
    required this.index,
    this.timeoutMs = 0,
    this.memoryBytes = 0,
  }) : super(name, version);

  /// The module's SHA-256 in hex: the key the engine loads it under.
  final String moduleHash;

  /// Reads the module's bytes from the verified bundle; called only when the
  /// engine has not loaded the module yet.
  final Uint8List Function() module;

  /// The function's entry in the module (`plux_invoke`'s index).
  final int index;

  /// The function's own time limit in milliseconds; 0 for the registry's.
  final int timeoutMs;

  /// The function's own memory limit in bytes; 0 for the registry's.
  final int memoryBytes;
}

/// A function placed on the server (FN-020).
final class ServerFunctionTarget extends FunctionTarget {
  /// Creates the target; [version] is 0 while the release does not carry the
  /// pinned version (FN-006).
  const ServerFunctionTarget({
    required String name,
    required int version,
    this.requiresAttestation = false,
    this.requiresSca = false,
  }) : super(name, version);

  /// Whether the function declares `attestation`: every call carries
  /// platform evidence bound to it (FN-020, SEC-003, SEC-004).
  final bool requiresAttestation;

  /// Whether the function declares `sca`: every call carries a transaction
  /// the user confirmed (FN-020, SEC-027).
  final bool requiresSca;
}

/// How the server answered an invocation.
sealed class FunctionServerReply {
  const FunctionServerReply();
}

/// The function's output, UTF-8 JSON.
final class FunctionServerOutput extends FunctionServerReply {
  /// Wraps [json].
  const FunctionServerOutput(this.json);

  /// The output.
  final Uint8List json;
}

/// A business error the function returned (FN-022).
final class FunctionServerBusinessError extends FunctionServerReply {
  /// Creates the error.
  const FunctionServerBusinessError({
    required this.code,
    required this.messageKey,
    required this.params,
  });

  /// The function-defined code.
  final String code;

  /// The message key.
  final String messageKey;

  /// The message values.
  final Map<String, Object?> params;
}

/// Calls `FunctionService.Invoke` over the device's DPoP-bound channel
/// (FN-020). Failures are [DataFailure]s, as the gateway's are.
abstract interface class FunctionServer {
  /// Invokes [function] at [version]; calls with the same [idempotencyKey]
  /// run once on the server within `function.idempotencyWindow`.
  ///
  /// With the host's end-user token [userToken] the call carries the Plux
  /// user token exchanged for the audience `function:<name>`, where the
  /// device holds the keys; a function that declares `claims` reads the
  /// verified user from it (SEC-026). Without it the call carries none.
  ///
  /// With [attest] the call carries platform evidence bound to the function,
  /// the version, the input and the request's DPoP proof; the evidence is
  /// made where the proof is, off the UI isolate (SEC-003, SEC-004).
  /// [challengeId] and [signature] are the confirmed transaction of a
  /// function that declares `sca` (SEC-027).
  Future<FunctionServerReply> invoke({
    required String function,
    required int version,
    required Uint8List input,
    required String idempotencyKey,
    String? userToken,
    bool attest = false,
    String challengeId = '',
    Uint8List? signature,
  });
}

/// What `invokeFunction` needs from the runtime; installed as a step
/// service. Everything that depends on the release, the settings or the
/// database is a function, so the handler holds no state of its own.
final class FunctionRuntime {
  /// Creates the runtime services.
  FunctionRuntime({
    required this.resolve,
    required this.deviceEnabled,
    required this.limits,
    required this.context,
    required this.kvSnapshot,
    required this.kvApply,
    this.engine,
    this.server,
    this.auth,
  });

  /// The function [function] that [plugin] calls, or null when the plugin
  /// holds no grant for it (deny by default, FN-012). Throws
  /// `PluxException` when the release cannot be read.
  final FunctionTarget? Function(String plugin, String function) resolve;

  /// Whether the `deviceFunctions` setting allows device placement (FN-055).
  final bool Function() deviceEnabled;

  /// The delivered limits (LIM-001).
  final Map<String, int> Function() limits;

  /// The context a device function reads.
  final PluxFunctionContext Function() context;

  /// The entries of [plugin]'s key-value namespace that are text.
  final Future<Map<String, String>> Function(String plugin) kvSnapshot;

  /// Applies one write of a function to [plugin]'s namespace; a null value
  /// deletes the key.
  final Future<void> Function(String plugin, String key, String? value) kvApply;

  /// The device engine, or null when the app registered none (E9).
  final PluxFunctionEngine? engine;

  /// The server channel, or null where no runtime provides one.
  final FunctionServer? server;

  /// The host's auth delegate, which supplies the end-user token for server
  /// functions (SEC-026, HST-010); null where the runtime has none.
  final AuthSession? auth;

  final Map<String, Future<Object>> _modules = {};

  /// The engine's handle for [target]'s module, loaded once per bundle hash
  /// (FN-051).
  Future<Object> module(PluxFunctionEngine engine, DeviceFunctionTarget t) {
    final have = _modules[t.moduleHash];
    if (have != null) return have;
    final loading = engine.load(key: t.moduleHash, bytes: t.module());
    _modules[t.moduleHash] = loading;
    unawaited(
      loading.then<void>(
        (_) {},
        onError: (Object _) {
          if (identical(_modules[t.moduleHash], loading)) {
            _modules.remove(t.moduleHash)?.ignore();
          }
        },
      ),
    );
    return loading;
  }
}

/// The handlers of the function actions this runtime runs, by action name.
final Map<String, ActionHandler> functionHandlers = {
  'invokeFunction': actionHandler(_invokeFunction),
};

Future<StepResult> _invokeFunction(
  StepContext c,
  Map<String, Object?> i,
) async {
  final rt = c.service<FunctionRuntime>();
  if (rt == null) {
    throw const ActionError(
      ActionErrorKind.custom,
      PluxErrorCode.actionsNotAvailable,
      'invokeFunction: this runtime does not run functions',
    );
  }
  final name = i['function'];
  if (name is! String || name.isEmpty) {
    throw const ActionError.validation(
      'invokeFunction: function is not a non-empty string',
    );
  }
  final FunctionTarget? target;
  try {
    target = rt.resolve(c.pluginKey, name);
  } on PluxException catch (e) {
    throw ActionError(ActionErrorKind.custom, e.code, 'invokeFunction: $name');
  }
  if (target == null) {
    // FN-012: a plugin calls only the functions it holds a grant for.
    throw const ActionError(
      ActionErrorKind.permission,
      PluxErrorCode.functionCapabilityDenied,
      'invokeFunction: the plugin holds no grant for this function',
    );
  }
  final input = Uint8List.fromList(utf8.encode(jsonEncode(toJson(i['input']))));
  return switch (target) {
    DeviceFunctionTarget() => _onDevice(c, rt, target, input),
    ServerFunctionTarget() => _onServer(c, rt, target, input),
  };
}

Future<StepResult> _onDevice(
  StepContext c,
  FunctionRuntime rt,
  DeviceFunctionTarget target,
  Uint8List input,
) async {
  // FN-055: the setting is checked before anything else is touched.
  if (!rt.deviceEnabled()) {
    throw const ActionError(
      ActionErrorKind.custom,
      PluxErrorCode.deviceFunctionsDisabled,
      'invokeFunction: device functions are disabled by the security '
      'profile (deviceFunctions)',
    );
  }
  final engine = rt.engine;
  if (engine == null) {
    throw const ActionError(
      ActionErrorKind.custom,
      PluxErrorCode.devicePackageMissing,
      'invokeFunction: a device function needs plux_functions: add it to '
      'PluxConfig.functionEngine',
    );
  }
  final limits = rt.limits();
  int of(PluxLimit l) => limits.valueOf(l);
  int tighter(int own, int registry) =>
      own > 0 && own < registry ? own : registry;
  final cancel = PluxFunctionCancel();
  final withdraw = c.run?.onAbandon(cancel.cancel);
  final journal = ReceivePort();
  final writes = <List<Object?>>[];
  journal.listen((m) => writes.add((m as List).cast<Object?>()));
  try {
    final module = await rt.module(engine, target);
    final entries = await rt.kvSnapshot(c.pluginKey);
    final PluxFunctionResult result;
    try {
      result = await engine.invoke(
        module: module,
        index: target.index,
        input: input,
        limits: PluxFunctionLimits(
          timeout: Duration(
            milliseconds: tighter(
              target.timeoutMs,
              of(PluxLimit.functionDeviceTimeout),
            ),
          ),
          memoryBytes: tighter(
            target.memoryBytes,
            of(PluxLimit.functionDeviceMemory),
          ),
          inputBytes: of(PluxLimit.functionInputSize),
          outputBytes: of(PluxLimit.functionOutputSize),
          logBytes: of(PluxLimit.functionLogBytes),
        ),
        context: rt.context(),
        kv: PluxFunctionKv(entries: entries, journal: journal.sendPort),
        cancel: cancel,
      );
    } on PluxException catch (e) {
      throw _systemError(e);
    }
    // The host's sends precede the engine's reply on the same isolate pair;
    // one turn of the event loop delivers them.
    await Future<void>.delayed(Duration.zero);
    for (final w in writes) {
      await rt.kvApply(c.pluginKey, w[0]! as String, w[1] as String?);
    }
    // FN-054: the result stays on the device; nothing here sends it anywhere.
    return switch (result) {
      PluxFunctionOutput(:final json) => StepDone(_decode(json)),
      PluxFunctionBusinessError() => throw _business(
        result.code,
        result.messageKey,
        result.params,
      ),
    };
  } finally {
    withdraw?.call();
    journal.close();
  }
}

Future<StepResult> _onServer(
  StepContext c,
  FunctionRuntime rt,
  ServerFunctionTarget target,
  Uint8List input,
) async {
  final server = rt.server;
  if (server == null) {
    throw const ActionError(
      ActionErrorKind.custom,
      PluxErrorCode.actionsNotAvailable,
      'invokeFunction: this runtime cannot reach the server',
    );
  }
  // FN-020: one key per execution of the step, reused by its retries.
  final key = c.execution?.key ?? c.clock.uuid();
  final confirmation = target.requiresSca
      ? await _confirm(c, target, input)
      : null;
  // SEC-026: the end user's token goes with the call when a user is signed
  // in; the device exchanges it for the audience of this function.
  var user = await rt.auth?.token();
  var refreshed = false;
  FunctionServerReply reply;
  while (true) {
    try {
      reply = await server.invoke(
        function: target.name,
        version: target.version,
        input: input,
        idempotencyKey: key,
        userToken: user,
        attest: target.requiresAttestation,
        challengeId: confirmation?.challengeId ?? '',
        signature: confirmation?.signature,
      );
      break;
    } on DataFailure catch (f) {
      // The identity provider rejected the host's token (PLX-6016): like a
      // 401 it is refreshed once and the call repeated with the same key,
      // so the server runs it once. A confirmed transaction is single use
      // and is never sent twice (as for data sources).
      if (user != null &&
          !refreshed &&
          confirmation == null &&
          f.code == PluxErrorCode.userTokenInvalid) {
        refreshed = true;
        user = await rt.auth?.refresh();
        if (user != null) continue;
      }
      throw f.toActionError();
    }
  }
  return switch (reply) {
    FunctionServerOutput(:final json) => StepDone(_decode(json)),
    FunctionServerBusinessError() => throw _business(
      reply.code,
      reply.messageKey,
      reply.params,
    ),
  };
}

/// FN-020, SEC-027: a function that declares `sca` is called only with a
/// transaction the user confirmed, which binds the function, the version and
/// the input. A device without an SCA key, or a user who declines, fails the
/// step.
Future<Confirmation> _confirm(
  StepContext c,
  ServerFunctionTarget target,
  Uint8List input,
) async {
  final sca = c.service<ScaService>();
  if (sca == null) {
    throw const ActionError(
      ActionErrorKind.custom,
      PluxErrorCode.actionsNotAvailable,
      'invokeFunction: this runtime cannot confirm a transaction',
    );
  }
  try {
    return await sca.confirmInvocation(
      InvocationRequest(
        function: target.name,
        version: target.version,
        input: input,
      ),
      on: c.device?.context?.call(),
    );
  } on ScaDeclined {
    throw const ActionError(
      ActionErrorKind.cancelled,
      PluxErrorCode.actionCancelled,
      'invokeFunction: the user declined the transaction',
    );
  } on PluxException catch (e) {
    throw ActionError(
      e.code == PluxErrorCode.deviceKeyUnavailable
          ? ActionErrorKind.custom
          : ActionErrorKind.permission,
      e.code,
      'invokeFunction: ${e.message}',
    );
  }
}

Object? _decode(Uint8List json) {
  try {
    return jsonDecode(utf8.decode(json));
  } on FormatException {
    throw const ActionError(
      ActionErrorKind.function,
      PluxErrorCode.functionOutputInvalid,
      'invokeFunction: the output is not JSON',
    );
  }
}

/// FN-022: a business error reaches the graph's error branch as a `function`
/// error with the function's own code, message key and parameters.
ActionError _business(
  String code,
  String messageKey,
  Map<String, Object?> params,
) => ActionError(
  ActionErrorKind.function,
  PluxErrorCode.functionBusinessError,
  'the function returned a business error',
  errorCode: code,
  messageKey: messageKey,
  params: params,
);

/// FN-022: a system error is reported with its registered code and a
/// generic message; the engine's text is not passed on (SEC-092).
ActionError _systemError(PluxException e) => switch (e.code) {
  PluxErrorCode.functionCancelled => const ActionError.cancelled(),
  PluxErrorCode.functionTimeout => ActionError(
    ActionErrorKind.timeout,
    e.code,
    'the function took longer than its time limit',
  ),
  _ => ActionError(
    ActionErrorKind.function,
    e.code,
    'the function failed: ${e.code.id}',
  ),
};
