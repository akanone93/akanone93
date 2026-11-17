// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The handlers of `apiCall`, `refreshData`, `subscribe`, `unsubscribe`
/// (DAT-001, DAT-011, DAT-012) and `readConfig` (DAT-094, DAT-097), run on
/// the page's [DataActions] through `StepContext.data`. A failure is an
/// [ActionError] with its kind, so `onError` handles it — and an
/// engine-level optimistic hook (ACT-007) can roll back on it: the handler
/// ignores `optimistic`, which the engine applies around the step.
library;

import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/live_config.dart';
import 'package:plux_flutter/src/data/services.dart';
import 'package:plux_flutter/src/data/transfers.dart' show CancelToken;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/pxl/decimal.dart';

DataActions _data(StepContext c, String action) =>
    c.data ??
    (throw DataFailure.unavailable('$action: this page has no data layer')
        .toActionError());

/// The data actions' handlers, by action name; `handlerFor` adds them to
/// the engine's table.
final Map<String, ActionHandler> dataHandlers = {
  'apiCall': FunctionHandler((c, i) async {
    final op = i['operation'];
    if (op is! String) {
      throw const ActionError.validation('apiCall: operation is not a string');
    }
    final input = i['input'];
    // A transfer stops with its run (DAT-031).
    final cancel = CancelToken();
    c.run?.onEnd((succeeded) {
      if (!succeeded) cancel.cancel();
    });
    return StepDone(
      await _data(c, 'apiCall').callOperation(
        op,
        input is Map<String, Object?> ? input : const {},
        cancel: cancel,
      ),
    );
  }),
  'readConfig': FunctionHandler((c, i) async {
    final key = i['key'];
    if (key is! String || key.isEmpty) {
      throw const ActionError.validation('readConfig: key is not a string');
    }
    final config =
        c.service<ConfigSource>() ??
        (throw DataFailure.unavailable(
          'readConfig: this runtime has no live configuration',
        ).toActionError());
    final want = i['type'];
    final type = want == null ? null : _configType(want);
    final LiveConfigValue value;
    try {
      value = await config.read(key);
    } on DataFailure catch (f) {
      throw f.toActionError();
    }
    return StepDone(type == null ? value.value : _typed(value, type));
  }),
  'refreshData': FunctionHandler((c, i) async {
    final source = i['source'];
    if (source is! String) {
      throw const ActionError.validation('refreshData: source is not a string');
    }
    await _data(c, 'refreshData').refresh(source, more: i['more'] == true);
    return const StepDone();
  }),
  'subscribe': FunctionHandler((c, i) async {
    final stream = i['stream'];
    if (stream is! String) {
      throw const ActionError.validation('subscribe: stream is not a string');
    }
    final params = i['params'];
    await _data(
      c,
      'subscribe',
    ).subscribe(stream, params is Map<String, Object?> ? params : const {});
    return const StepDone();
  }),
  'unsubscribe': FunctionHandler((c, i) async {
    final stream = i['stream'];
    if (stream is! String) {
      throw const ActionError.validation('unsubscribe: stream is not a string');
    }
    await _data(c, 'unsubscribe').unsubscribe(stream);
    return const StepDone();
  }),
};

/// The live configuration type a readConfig step's `type` input names; an
/// unknown name fails the step rather than leaving the value unchecked.
LiveConfigType _configType(Object want) =>
    switch (enumMember('ConfigValueType', want)) {
      'string' => LiveConfigType.string,
      'int' => LiveConfigType.integer,
      'decimal' => LiveConfigType.decimal,
      'bool' => LiveConfigType.boolean,
      _ => throw const ActionError.validation(
        'readConfig: type is not a ConfigValueType',
      ),
    };

/// The value as the step's declared [type]: a value the server holds with
/// another type fails closed (PLX-6074), and a decimal, which the server
/// carries as a string, becomes an exact [Decimal].
Object _typed(LiveConfigValue v, LiveConfigType type) {
  if (v.type != type) {
    throw ActionError(
      ActionErrorKind.validation,
      PluxErrorCode.liveConfigTypeMismatch,
      'readConfig: ${v.key} is a ${v.type.name}, not a ${type.name}',
    );
  }
  if (type != LiveConfigType.decimal) return v.value;
  return Decimal.tryParse(v.value as String) ??
      (throw ActionError(
        ActionErrorKind.validation,
        PluxErrorCode.liveConfigTypeMismatch,
        'readConfig: ${v.key} does not read as a decimal',
      ));
}
