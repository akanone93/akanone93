// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/engine.dart';
import 'package:plux_flutter/src/actions/graph.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/actions/run.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/functions/invoke_function.dart';

import '../actions/engine_test.dart' show FakeNavigator, id, input, lit;

Uint8List _bytes(String s) => Uint8List.fromList(utf8.encode(s));

final class _FakeEngine implements PluxFunctionEngine {
  int loads = 0;
  final List<String> loadedKeys = [];
  final List<PluxFunctionCancel?> cancels = [];
  Completer<PluxFunctionResult>? hold;
  Future<PluxFunctionResult> Function(PluxFunctionKv kv, Uint8List input)? on;
  PluxFunctionLimits? seenLimits;
  Uint8List? seenInput;

  @override
  String get name => 'fake';

  @override
  Future<Object> load({required String key, required Uint8List bytes}) async {
    loads++;
    loadedKeys.add(key);
    return key;
  }

  @override
  Future<PluxFunctionResult> invoke({
    required Object module,
    required int index,
    required Uint8List input,
    required PluxFunctionLimits limits,
    required PluxFunctionContext context,
    required PluxFunctionKv kv,
    PluxFunctionCancel? cancel,
  }) {
    seenLimits = limits;
    seenInput = input;
    cancels.add(cancel);
    final h = hold;
    if (h != null) {
      cancel?.onCancel(
        () => h.completeError(
          const PluxException(PluxErrorCode.functionCancelled, 'cancelled'),
        ),
      );
      return h.future;
    }
    return on?.call(kv, input) ??
        Future.value(PluxFunctionOutput(_bytes('{"ok":true}')));
  }
}

final class _FakeServer implements FunctionServer {
  final List<({String function, int version, String key, String input})> calls =
      [];
  final List<Object> script = [];

  @override
  Future<FunctionServerReply> invoke({
    required String function,
    required int version,
    required Uint8List input,
    required String idempotencyKey,
    bool attest = false,
    String challengeId = '',
    Uint8List? signature,
  }) async {
    calls.add((
      function: function,
      version: version,
      key: idempotencyKey,
      input: utf8.decode(input),
    ));
    final next = script.isEmpty
        ? FunctionServerOutput(_bytes('{"server":true}'))
        : script.removeAt(0);
    if (next is Exception) throw next;
    return next as FunctionServerReply;
  }
}

void main() {
  late _FakeEngine engine;
  late _FakeServer server;
  late bool deviceEnabled;
  late Map<String, String> kv;
  late Map<String, FunctionTarget> targets;
  late int moduleReads;
  late List<(String, Object?)> events;

  FunctionRuntime runtime({bool withEngine = true}) => FunctionRuntime(
    resolve: (plugin, name) => plugin == 'shop' ? targets[name] : null,
    deviceEnabled: () => deviceEnabled,
    limits: () => const {},
    context: () => const PluxFunctionContext(
      app: 'a',
      environment: 'production',
      assuranceLevel: 'AL1',
    ),
    kvSnapshot: (_) async => Map.of(kv),
    kvApply: (_, k, v) async {
      if (v == null) {
        kv.remove(k);
      } else {
        kv[k] = v;
      }
    },
    engine: withEngine ? engine : null,
    server: server,
  );

  ActionHost host(FunctionRuntime rt) {
    final h = ActionHost(
      context: StepContext(
        navigator: FakeNavigator(),
        emit: (n, p) => events.add((n, p)),
        nativeActions: const NoNativeActions(),
        services: {FunctionRuntime: rt},
      ),
      limits: ActionLimits.of(const {}),
      report: (_) {},
      record: (name, {fields = const {}, route = '', pluginKey = ''}) {},
      route: 'r',
      pluginKey: 'shop',
    );
    addTearDown(h.dispose);
    return h;
  }

  List<GraphStep> steps(
    String function, {
    Object? payload,
    RetryPolicy? retry,
  }) => [
    GraphStep(
      id: 'call',
      action: id('invokeFunction'),
      inputs: {
        input('invokeFunction', 'function'): lit(function),
        input('invokeFunction', 'input'): lit(payload ?? {'a': 1}),
      },
      onError: 1,
      next: 1,
      retry: retry,
    ),
    GraphStep(
      id: 'after',
      action: id('emitHostEvent'),
      inputs: {
        input('emitHostEvent', 'event'): lit('seen'),
        input('emitHostEvent', 'payload'): (roots) =>
            (roots['steps']! as Map)['call'],
      },
    ),
  ];

  Future<RunResult?> run(
    FunctionRuntime rt,
    String function, {
    RetryPolicy? retry,
    ActionHost? on,
  }) => (on ?? host(rt)).start(
    ActionGraph(
      id: 'g',
      steps: steps(function, retry: retry),
    ),
    roots: () => const {},
    key: 'k',
  );

  Map<String, Object?> seen() => events.single.$2! as Map<String, Object?>;

  setUp(() {
    engine = _FakeEngine();
    server = _FakeServer();
    deviceEnabled = true;
    kv = {};
    moduleReads = 0;
    events = [];
    targets = {
      'loan.quote': DeviceFunctionTarget(
        name: 'loan.quote',
        version: 3,
        moduleHash: 'abc',
        module: () {
          moduleReads++;
          return Uint8List(4);
        },
        index: 2,
      ),
      'loan.score': DeviceFunctionTarget(
        name: 'loan.score',
        version: 3,
        moduleHash: 'abc',
        module: () {
          moduleReads++;
          return Uint8List(4);
        },
        index: 5,
        timeoutMs: 100,
        memoryBytes: 1 << 20,
      ),
      'fee.compute': const ServerFunctionTarget(
        name: 'fee.compute',
        version: 7,
      ),
    };
  });

  test('[FN-050] a device function runs on the engine and its output is the step result', () async {
    final r = await run(runtime(), 'loan.quote');
    expect(r?.outcome, RunOutcome.ok);
    expect(seen()['output'], {'ok': true});
    expect(utf8.decode(engine.seenInput!), '{"a":1}');
    expect(
      server.calls,
      isEmpty,
      reason: 'FN-054: nothing is sent to the server',
    );
  });

  test('[FN-053] the device limits come from the registry and a function can only tighten them', () async {
    final rt = runtime();
    await run(rt, 'loan.quote');
    expect(engine.seenLimits!.timeout, const Duration(milliseconds: 500));
    expect(engine.seenLimits!.memoryBytes, 33554432);
    await run(rt, 'loan.score');
    expect(engine.seenLimits!.timeout, const Duration(milliseconds: 100));
    expect(engine.seenLimits!.memoryBytes, 1 << 20);
  });

  test(
    '[FN-051] the device module is handed to the engine once per bundle hash',
    () async {
      final rt = runtime();
      await run(rt, 'loan.quote');
      await run(rt, 'loan.score');
      await run(rt, 'loan.quote');
      expect(engine.loads, 1);
      expect(engine.loadedKeys, ['abc']);
      expect(moduleReads, 1);
    },
  );

  test('[FN-055] a device function fails with PLX-7014 while deviceFunctions is off', () async {
    deviceEnabled = false;
    final r = await run(runtime(), 'loan.quote');
    expect(r?.outcome, RunOutcome.ok);
    expect((seen()['error']! as Map)['message'], contains('deviceFunctions'));
    expect(engine.loads, 0);
    final h = host(runtime());
    final failed = await h.start(
      ActionGraph(
        id: 'g2',
        steps: [_withoutErrorBranch(steps('loan.quote').first)],
      ),
      roots: () => const {},
      key: 'k2',
    );
    expect(failed?.error?.code, PluxErrorCode.deviceFunctionsDisabled);
  });

  test('[FN-050] a device function fails with PLX-5401 when no engine is registered', () async {
    final h = host(runtime(withEngine: false));
    final r = await h.start(
      ActionGraph(
        id: 'g',
        steps: [_withoutErrorBranch(steps('loan.quote').first)],
      ),
      roots: () => const {},
      key: 'k',
    );
    expect(r?.outcome, RunOutcome.failed);
    expect(r?.error?.code, PluxErrorCode.devicePackageMissing);
  });

  test('[FN-053] cancelling the run cancels the invocation', () async {
    engine.hold = Completer<PluxFunctionResult>();
    final rt = runtime();
    final h = host(rt);
    final future = run(rt, 'loan.quote', on: h);
    await Future<void>.delayed(const Duration(milliseconds: 20));
    expect(engine.cancels.single!.isCancelled, isFalse);
    h.dispose();
    await future;
    expect(engine.cancels.single!.isCancelled, isTrue);
  });

  test('[FN-022] a business error reaches the error branch as a function error with code, key and params', () async {
    engine.on = (_, _) async => const PluxFunctionBusinessError(
      code: 'LOAN_TOO_LARGE',
      messageKey: 'loan.tooLarge',
      params: {'max': 5000},
    );
    final r = await run(runtime(), 'loan.quote');
    expect(r?.outcome, RunOutcome.ok);
    final error = seen()['error']! as Map<String, Object?>;
    expect(error['kind'], 'function');
    expect(error['code'], 'LOAN_TOO_LARGE');
    expect(error['messageKey'], 'loan.tooLarge');
    expect(error['params'], {'max': 5000});
  });

  test('[FN-022] a system error is generic and carries its PLX code', () async {
    engine.on = (_, _) async => throw const PluxException(
      PluxErrorCode.functionTrapped,
      'secret detail',
    );
    final r = await run(runtime(), 'loan.quote');
    expect(r?.outcome, RunOutcome.ok);
    final error = seen()['error']! as Map<String, Object?>;
    expect(error['kind'], 'function');
    expect(error['message'], contains('PLX-7007'));
    expect(error['message'], isNot(contains('secret')));
  });

  test('[FN-052] the key-value writes of a function reach the plugin namespace once it succeeds', () async {
    kv = {'old': 'x'};
    engine.on = (k, _) async {
      expect(k.entries, {'old': 'x'});
      k.journal
        ..send(<Object?>['new', 'y'])
        ..send(<Object?>['old', null]);
      return PluxFunctionOutput(_bytes('1'));
    };
    await run(runtime(), 'loan.quote');
    expect(kv, {'new': 'y'});
  });

  test('[FN-052] a failed invocation leaves the namespace as it was', () async {
    kv = {'old': 'x'};
    engine.on = (k, _) async {
      k.journal.send(<Object?>['new', 'y']);
      throw const PluxException(PluxErrorCode.functionTrapped, 'trap');
    };
    await run(runtime(), 'loan.quote');
    expect(kv, {'old': 'x'});
  });

  test('[FN-020] a server function goes through the server with its pinned version', () async {
    final r = await run(runtime(), 'fee.compute');
    expect(r?.outcome, RunOutcome.ok);
    expect(seen()['output'], {'server': true});
    expect(server.calls.single.function, 'fee.compute');
    expect(server.calls.single.version, 7);
    expect(server.calls.single.input, '{"a":1}');
    expect(engine.loads, 0);
  });

  test('[FN-020] a server function works without the device package or with device functions off', () async {
    deviceEnabled = false;
    final r = await run(runtime(withEngine: false), 'fee.compute');
    expect(r?.outcome, RunOutcome.ok);
    expect(server.calls, hasLength(1));
  });

  test('[FN-020] the idempotency key is stable across retries of a step and differs between runs', () async {
    const down = DataFailure(
      ActionErrorKind.network,
      PluxErrorCode.dataNetworkFailed,
      'down',
    );
    server.script.addAll([down, down]);
    final rt = runtime();
    const retry = RetryPolicy(count: 3, on: {ActionErrorKind.network});
    var h = host(rt);
    final r = await run(rt, 'fee.compute', retry: retry, on: h);
    expect(r?.outcome, RunOutcome.ok);
    expect(server.calls, hasLength(3));
    expect(server.calls.map((c) => c.key).toSet(), hasLength(1));
    final first = server.calls.first.key;
    expect(
      first,
      matches(
        RegExp(
          r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$',
        ),
      ),
    );
    events.clear();
    h = host(rt);
    await run(rt, 'fee.compute', on: h);
    expect(server.calls.last.key, isNot(first));
  });

  test(
    '[FN-022] a business error of a server function reaches the error branch',
    () async {
      server.script.add(
        const FunctionServerBusinessError(
          code: 'NO_CREDIT',
          messageKey: 'fee.noCredit',
          params: {'x': 1},
        ),
      );
      await run(runtime(), 'fee.compute');
      final error = seen()['error']! as Map<String, Object?>;
      expect(error['kind'], 'function');
      expect(error['code'], 'NO_CREDIT');
      expect(error['messageKey'], 'fee.noCredit');
    },
  );

  test(
    '[FN-012] a function the plugin holds no grant for is refused',
    () async {
      targets.remove('fee.compute');
      await run(runtime(), 'fee.compute');
      final error = seen()['error']! as Map<String, Object?>;
      expect(error['kind'], 'permission');
      expect(server.calls, isEmpty);
    },
  );
}

GraphStep _withoutErrorBranch(GraphStep s) =>
    GraphStep(id: s.id, action: s.action, inputs: s.inputs);
