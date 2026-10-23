// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/devtools_api/dev_events.dart';
import 'package:plux_flutter/src/functions/invoke_function.dart';
import 'package:plux_flutter/src/render/renderer.dart';
import 'package:plux_flutter/src/state/scope_state.dart';

import '../support/harness.dart';

final class _Sink implements DevEventSink {
  final List<Map<String, Object?>> events = [];

  @override
  void emit(Map<String, Object?> payload, {String? traceId}) =>
      events.add(payload);

  Set<String> get kinds => {for (final e in events) ...e.keys};
}

PluxActionTrace _trace(int id) => PluxActionTrace(
  runId: id,
  graphId: 'graph-1',
  trigger: 'event',
  route: '/home',
  plugin: '',
  startedAt: DateTime.fromMicrosecondsSinceEpoch(1000),
  duration: const Duration(microseconds: 500),
  status: PluxTraceStatus.ok,
  steps: const [],
  stepsExecuted: 0,
);

ScopeModel _model() => ScopeModel(
  kind: StateScopeKind.page,
  owner: '',
  decls: [
    const StateDecl(
      name: 'count',
      id: 'b',
      typeExpr: 'int',
      type: null,
      defaultValue: 0,
    ),
  ],
  evaluate: (_, _) => null,
  types: const {},
);

void main() {
  final g = Harness.goldens;
  late Harness h;

  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<void> start(WidgetTester tester) async {
    await tester.runAsync(
      () => h.startFrom(g.bundles['state/state.pxb']!, {
        'notes': g.bundles['state/notes.pxb']!,
      }),
    );
    await tester.pumpWidget(
      const MaterialApp(home: PluxScope(child: SizedBox())),
    );
  }

  PluxRenderer renderer() => h.runtime.renderer! as PluxRenderer;
  FunctionRuntime functions() =>
      renderer().actions!.services[FunctionRuntime]! as FunctionRuntime;

  /// Drives one event of every source the runtime wires.
  Future<void> drive() async {
    renderer().data!.client.devTap!(
      DevNetworkCall(
        method: 'GET',
        urlTemplate: 'https://api.example/items/{id}',
        status: 200,
        startedAt: DateTime.fromMicrosecondsSinceEpoch(1),
        total: const Duration(milliseconds: 3),
      ),
    );
    functions().devTap!(
      DevFunctionCall(
        function: 'shop.price',
        version: 1,
        placement: 'device',
        startedAt: DateTime.fromMicrosecondsSinceEpoch(1),
        duration: const Duration(milliseconds: 2),
      ),
    );
    renderer().devStateTap!(_model(), {'count': 0}, {'count': 1});
    h.runtime.traces.add(_trace(1));
    await Future<void>.delayed(Duration.zero);
  }

  testWidgets('DEV-020 ACT-030 FN-036: with no pairing the taps are null', (
    tester,
  ) async {
    await start(tester);
    expect(h.runtime.devBridge, isNull);
    expect(renderer().data!.client.devTap, isNull);
    expect(functions().devTap, isNull);
  });

  testWidgets(
    'DEV-020 ACT-030 FN-036: every source streams once attached and none '
    'after detach',
    (tester) async {
      await start(tester);
      final sink = _Sink();
      h.runtime.streamDevEvents(sink);
      expect(h.runtime.devBridge!.attached, isTrue);

      await tester.runAsync(drive);
      expect(
        sink.kinds,
        containsAll([
          'networkCall',
          'functionCall',
          'stateDiff',
          'actionTrace',
        ]),
      );

      h.runtime.reportProblem(
        const PluxException(PluxErrorCode.stateWriteTypeMismatch, 'bad'),
      );
      expect(sink.kinds, contains('log'));

      h.runtime.stopDevEvents();
      expect(h.runtime.devBridge, isNull);
      expect(renderer().data!.client.devTap, isNull);
      expect(functions().devTap, isNull);
      final before = sink.events.length;
      await tester.runAsync(() async {
        renderer().devStateTap!(_model(), {'count': 1}, {'count': 2});
        h.runtime.traces.add(_trace(2));
        h.runtime.reportProblem(
          const PluxException(PluxErrorCode.stateWriteTypeMismatch, 'again'),
        );
        await Future<void>.delayed(Duration.zero);
      });
      expect(sink.events, hasLength(before));
    },
  );
}
