// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/engine.dart';
import 'package:plux_flutter/src/actions/graph.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/actions/run.dart';
import 'package:plux_flutter/src/reconstruction/gate.dart';
import 'package:plux_flutter/src/reconstruction/server_only_client.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';

int _id(String action) =>
    actionDescriptors.firstWhere((d) => d.name == action).id;

int _input(String action, String name) =>
    actionDescriptors.firstWhere((d) => d.name == action).inputs[name]!;

InputReader _lit(Object? v) =>
    (_) => v;

final class _Navigator implements RunNavigator {
  final List<String> calls = [];

  @override
  Future<void> navigate(
    String route,
    Map<String, Object?> params,
    String mode,
    String? until,
  ) async => calls.add('$mode $route');

  @override
  Future<Object?> present(
    String route,
    Map<String, Object?> params, {
    required bool sheet,
    required bool dismissible,
  }) async => null;

  @override
  void pop(Object? result) => calls.add('pop');

  @override
  void switchTab(String tab) => calls.add('tab $tab');
}

/// The actions of a reconstruction [SEC-142].
void main() {
  group('the gate decides every action on purpose [SEC-142]', () {
    test('each action of the registry is admitted or refused, not both', () {
      final all = {for (final d in actionDescriptors) d.name};
      expect(
        ReconstructionGate.admitted.intersection(ReconstructionGate.refused),
        isEmpty,
      );
      expect(
        ReconstructionGate.admitted.union(ReconstructionGate.refused),
        all,
        reason: 'a new action must be put on one of the two lists',
      );
    });

    test('refuses what reaches beyond the page with PLX-4010', () {
      const gate = ReconstructionGate();
      for (final action in ReconstructionGate.refused) {
        expect(
          () => gate.admit(action),
          throwsA(
            isA<ActionError>()
                .having(
                  (e) => e.code,
                  'code',
                  PluxErrorCode.actionsNotAvailable,
                )
                .having((e) => e.message, 'message', contains(action)),
          ),
          reason: action,
        );
      }
    });

    test('refuses an action it does not know', () {
      expect(
        () => const ReconstructionGate().admit('somethingNew'),
        throwsA(isA<ActionError>()),
      );
    });

    test('admits what only changes the page', () {
      for (final action in ReconstructionGate.admitted) {
        const ReconstructionGate().admit(action);
      }
    });

    test(
      'data sources, functions, host and device actions are all refused',
      () {
        for (final action in [
          'apiCall',
          'refreshData',
          'subscribe',
          'submitForm',
          'invokeFunction',
          'callNative',
          'emitHostEvent',
          'logout',
          'openUrl',
          'share',
          'copyToClipboard',
          'capturePhoto',
          'getLocation',
          'signTransaction',
          'dbInsert',
          'kvSet',
          'trackEvent',
          'sync',
        ]) {
          expect(ReconstructionGate.refused, contains(action));
        }
      },
    );
  });

  group('the engine runs only the admitted steps [SEC-142]', () {
    late _Navigator nav;
    late List<(String, Map<String, Object?>)> events;
    late List<PluxException> reports;

    ActionHost host({ReconstructionGate? gate}) => ActionHost(
      context: StepContext(
        navigator: nav,
        emit: (n, p) => events.add((n, p)),
        nativeActions: const NoNativeActions(),
        reconstruction: gate,
      ),
      limits: const ActionLimits(
        stepsPerRun: 100,
        stepTimeout: Duration(seconds: 5),
        runTimeout: Duration(seconds: 10),
      ),
      report: reports.add,
      record: (name, {fields = const {}, route = '', pluginKey = ''}) {},
      route: 'home',
      pluginKey: 'p',
    );

    Future<RunResult> run(ActionHost h, List<GraphStep> steps) async =>
        (await h.start(
          ActionGraph(id: 'g', steps: steps),
          roots: () => const {},
          key: 'k',
        ))!;

    final emit = GraphStep(
      id: 'emit',
      action: _id('emitHostEvent'),
      inputs: {_input('emitHostEvent', 'event'): _lit('sent')},
    );

    setUp(() {
      nav = _Navigator();
      events = [];
      reports = [];
    });

    test('a host action runs outside reconstruction mode', () async {
      final r = await run(host(), [emit]);
      expect(r.outcome, RunOutcome.ok);
      expect(events.map((e) => e.$1), ['sent']);
    });

    test('a host action fails with PLX-4010 and has no effect', () async {
      final r = await run(host(gate: const ReconstructionGate()), [emit]);
      expect(r.outcome, RunOutcome.failed);
      expect(r.error!.code, PluxErrorCode.actionsNotAvailable);
      expect(r.failingStep, 'emit');
      expect(events, isEmpty);
    });

    test('a step that follows a refused one does not run', () async {
      final r = await run(host(gate: const ReconstructionGate()), [
        GraphStep(
          id: 'link',
          next: 1,
          action: _id('openUrl'),
          inputs: {_input('openUrl', 'url'): _lit('https://example.org')},
        ),
        emit,
      ]);
      expect(r.outcome, RunOutcome.failed);
      expect(events, isEmpty);
    });

    test('an onError step can still handle the refusal', () async {
      final r = await run(host(gate: const ReconstructionGate()), [
        GraphStep(
          id: 'call',
          action: _id('apiCall'),
          inputs: {_input('apiCall', 'operation'): _lit('o')},
          onError: 1,
        ),
        GraphStep(
          id: 'handled',
          action: _id('condition'),
          inputs: {_input('condition', 'when'): _lit(true)},
        ),
      ]);
      expect(r.outcome, RunOutcome.ok);
    });

    test('navigation inside the app and local state still run', () async {
      final r = await run(host(gate: const ReconstructionGate()), [
        GraphStep(
          id: 'go',
          next: 1,
          action: _id('navigate'),
          inputs: {_input('navigate', 'route'): _lit('detail')},
        ),
        GraphStep(id: 'back', action: _id('pop')),
      ]);
      expect(r.outcome, RunOutcome.ok);
      expect(nav.calls, ['push detail', 'pop']);
    });
  });

  group('the client reaches the Plux server and nothing else [SEC-142]', () {
    final server = Uri.parse('https://plux.example.org/');
    late List<Uri> sent;
    late ServerOnlyClient client;

    setUp(() {
      sent = [];
      client = ServerOnlyClient(
        MockClient((request) async {
          sent.add(request.url);
          return http.Response('ok', 200);
        }),
        server,
      );
    });

    test('sends a request to the server', () async {
      final r = await client.get(Uri.parse('https://plux.example.org/v1/x'));
      expect(r.statusCode, 200);
      expect(sent, hasLength(1));
    });

    for (final (url, host) in [
      ('https://api.customer.example/v1/accounts', 'api.customer.example'),
      (
        'https://plux.example.org.evil.example/',
        'plux.example.org.evil.example',
      ),
      ('http://plux.example.org/', 'plux.example.org'),
      ('https://plux.example.org:8443/', 'plux.example.org'),
      ('https://user@evil.example/', 'evil.example'),
    ]) {
      test('refuses $url before it leaves the device', () async {
        await expectLater(
          client.get(Uri.parse(url)),
          throwsA(
            isA<PluxException>()
                .having((e) => e.message, 'names the host', contains(host))
                .having(
                  (e) => e.message,
                  'keeps the path out',
                  isNot(contains('accounts')),
                ),
          ),
        );
        expect(sent, isEmpty);
      });
    }
  });
}
