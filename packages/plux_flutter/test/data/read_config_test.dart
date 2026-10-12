// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/live_config.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';

import 'io_support.dart';

/// Answers reads from [values]; a key it does not hold fails as the sync
/// isolate does when the server cannot be reached.
final class _Source implements ConfigSource {
  _Source(this.values);

  final Map<String, LiveConfigValue> values;
  final List<String> reads = [];

  @override
  Future<LiveConfigValue> read(String key) async {
    reads.add(key);
    return values[key] ??
        (throw const DataFailure(
          ActionErrorKind.network,
          PluxErrorCode.liveConfigUnavailable,
          'offline',
        ));
  }
}

void main() {
  final handler = builtInHandlers['readConfig']!;

  StepContext context(ConfigSource? source) => StepContext(
    navigator: NoNavigator(),
    emit: (_, _) {},
    nativeActions: const NoNativeActions(),
    services: {ConfigSource: ?source},
  );

  test(
    'outputs the live value the sync isolate holds, typed [DAT-094]',
    () async {
      final source = _Source({
        'limit': const LiveConfigValue(
          key: 'limit',
          type: LiveConfigType.integer,
          value: 500,
          strict: true,
        ),
      });
      final r =
          await handler.run(context(source), {'key': 'limit'}) as StepDone;
      expect(r.output, 500);
      expect(source.reads, ['limit']);
    },
  );

  test('a value that cannot be had fails closed with its code, for onError [DAT-097]', () async {
    await expectLater(
      handler.run(context(_Source({})), {'key': 'limit'}),
      throwsA(
        isA<ActionError>()
            .having((e) => e.code, 'code', PluxErrorCode.liveConfigUnavailable)
            .having((e) => e.kind, 'kind', ActionErrorKind.network),
      ),
    );
  });

  test(
    'without a live configuration, or without a key, nothing is read [DAT-094]',
    () async {
      await expectLater(
        handler.run(context(null), {'key': 'limit'}),
        throwsA(isA<ActionError>()),
      );
      final source = _Source({});
      await expectLater(
        handler.run(context(source), {'key': ''}),
        throwsA(
          isA<ActionError>().having(
            (e) => e.kind,
            'kind',
            ActionErrorKind.validation,
          ),
        ),
      );
      expect(source.reads, isEmpty);
    },
  );
}
