// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/actions/engine.dart';
import 'package:plux_flutter/src/actions/graph.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/actions/run.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/live_control.dart';
import 'package:plux_flutter/src/data/client.dart' show DataCaller;
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/services.dart';
import 'package:plux_flutter/src/data/spec.dart';
import 'package:plux_flutter/src/data/transport.dart';
import 'package:plux_flutter/src/functions/invoke_function.dart';

import '../actions/engine_test.dart' show FakeNavigator, id, input, lit;

/// Kill switches by scope at the points where a graph runs, a data source
/// is asked and a function is called (REL-060): each fails with its scope's
/// code, PLX-3105, PLX-3106 or PLX-3107.
void main() {
  final switches = ControlSnapshot(
    version: 1,
    expiresAt: DateTime.utc(2100),
    switches: const [
      ControlSwitch(SwitchKind.actionGraph, 'g-off'),
      ControlSwitch(SwitchKind.dataSource, 'd-off'),
      ControlSwitch(SwitchKind.function, 'orders.total'),
    ],
  );
  final live = LiveControl(snapshot: switches);

  group('action graph', () {
    late List<PluxException> reports;
    late List<String> events;

    ActionHost host() => ActionHost(
      context: StepContext(
        navigator: FakeNavigator(),
        emit: (name, _) => events.add(name),
        nativeActions: const NoNativeActions(),
      ),
      limits: ActionLimits.of(const {}),
      report: reports.add,
      record: (name, {fields = const {}, route = '', pluginKey = ''}) {},
      route: 'home',
      pluginKey: 'shop',
      disabled: (graph) =>
          live.isOff(SwitchKind.actionGraph, graph, () => const {}),
    );

    Future<RunResult?> run(String graph) {
      final h = host();
      addTearDown(h.dispose);
      return h.start(
        ActionGraph(
          id: graph,
          steps: [
            GraphStep(
              id: 'emit',
              action: id('emitHostEvent'),
              inputs: {input('emitHostEvent', 'event'): lit('ran')},
            ),
          ],
        ),
        roots: () => const {},
        key: 'k',
      );
    }

    setUp(() {
      reports = [];
      events = [];
    });

    test('a switched-off graph does not run and is reported with PLX-3105 '
        '[REL-060]', () async {
      expect(await run('g-off'), isNull);
      expect(events, isEmpty);
      expect(reports.single.code, PluxErrorCode.actionGraphSwitchedOff);
      expect(reports.single.details['scope'], 'actionGraph');
      expect(reports.single.details['target'], 'g-off');
    });

    test('another graph still runs [REL-060]', () async {
      expect(await run('g-on'), isNotNull);
      expect(events, ['ran']);
      expect(reports, isEmpty);
    });
  });

  group('data source', () {
    late int requests;

    DataServices services() => DataServices(
      transport: ClientTransport(
        MockClient((_) async {
          requests++;
          return http.Response(jsonEncode({'items': <Object?>[]}), 200);
        }),
      ),
      authDelegate: () => null,
      environment: 'production',
      record: (name, {fields = const {}, route = '', pluginKey = ''}) {},
      report: (_) {},
      allowCleartext: true,
      disabled: (source) =>
          live.isOff(SwitchKind.dataSource, source, () => const {}),
    );

    DataSourceSpec source(String sourceId) => DataSourceSpec(
      route: DataRoute.direct,
      id: sourceId,
      name: 'tasks',
      kind: DataKind.rest,
      type: 'list<Task>',
      baseUrls: const {'production': 'http://127.0.0.1/v1'},
      path: '/tasks',
    );

    const caller = DataCaller(pluginKey: 'shop', domains: ['127.0.0.1']);

    setUp(() => requests = 0);

    test('a switched-off source fails with PLX-3106 before anything is sent '
        '[REL-060]', () async {
      await expectLater(
        services().client.load(source('d-off'), caller, const {}),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.dataSourceSwitchedOff,
          ),
        ),
      );
      expect(requests, 0);
    });

    test('another source is asked [REL-060]', () async {
      await services().client.load(source('d-on'), caller, const {});
      expect(requests, 1);
    });
  });

  group('function', () {
    test(
      'a switched-off function fails its step with PLX-3107 [REL-060]',
      () async {
        final reports = <PluxException>[];
        var resolved = 0;
        final rt = FunctionRuntime(
          resolve: (plugin, name) {
            resolved++;
            if (live.isOff(SwitchKind.function, name, () => const {})) {
              throw LiveControl.disabled(SwitchKind.function, name);
            }
            return null;
          },
          deviceEnabled: () => true,
          limits: () => const {},
          context: () => const PluxFunctionContext(
            app: 'a',
            environment: 'production',
            assuranceLevel: 'AL1',
          ),
          kvSnapshot: (_) async => {},
          kvApply: (_, _, _) async {},
        );
        final host = ActionHost(
          context: StepContext(
            navigator: FakeNavigator(),
            emit: (_, _) {},
            nativeActions: const NoNativeActions(),
            services: {FunctionRuntime: rt},
          ),
          limits: ActionLimits.of(const {}),
          report: reports.add,
          record: (name, {fields = const {}, route = '', pluginKey = ''}) {},
          route: 'r',
          pluginKey: 'shop',
        );
        addTearDown(host.dispose);
        final result = await host.start(
          ActionGraph(
            id: 'g',
            steps: [
              GraphStep(
                id: 'call',
                action: id('invokeFunction'),
                inputs: {
                  input('invokeFunction', 'function'): lit('orders.total'),
                  input('invokeFunction', 'input'): lit(<String, Object?>{}),
                },
              ),
            ],
          ),
          roots: () => const {},
          key: 'k',
        );
        expect(resolved, 1);
        expect(result!.outcome, RunOutcome.failed);
        expect(result.error!.code, PluxErrorCode.functionSwitchedOff);
        expect(reports.single.code, PluxErrorCode.functionSwitchedOff);
      },
    );
  });
}
