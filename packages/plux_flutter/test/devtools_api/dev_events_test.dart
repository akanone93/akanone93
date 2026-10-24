// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/actions/trace.dart';
import 'package:plux_flutter/src/devtools_api/dev_events.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/devtools_api/diagnostics.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/state/scope_state.dart';
import 'package:plux_flutter/src/sync/sync_event.dart';

final class _Sink implements DevEventSink {
  final List<(Map<String, Object?>, String?)> events = [];

  @override
  void emit(Map<String, Object?> payload, {String? traceId}) =>
      events.add((payload, traceId));

  Map<String, Object?> only(String kind) {
    final of = [
      for (final (p, _) in events)
        if (p.containsKey(kind)) p[kind]! as Map<String, Object?>,
    ];
    expect(of, hasLength(1), reason: 'events: $events');
    return of.single;
  }
}

final class _Diagnostics implements PluxDiagnostics {
  @override
  final ValueNotifier<List<PluxDiagnostic>> log = ValueNotifier(const []);
  @override
  final ValueNotifier<PluxReleaseInfo?> release = ValueNotifier(null);
  @override
  final ValueNotifier<SyncEvent?> syncStatus = ValueNotifier(null);
  @override
  final ValueNotifier<List<PluxActionTrace>> actionTraces = ValueNotifier(
    const [],
  );
}

PluxActionTrace _trace(int id) => PluxActionTrace(
  runId: id,
  graphId: 'graph-1',
  trigger: 'event',
  route: '/home',
  plugin: '',
  startedAt: DateTime.fromMicrosecondsSinceEpoch(1000),
  duration: const Duration(microseconds: 500),
  status: PluxTraceStatus.failed,
  steps: const [
    PluxStepTrace(
      id: 's1',
      action: 'setState',
      start: Duration(microseconds: 10),
      duration: Duration(microseconds: 20),
      status: PluxTraceStatus.failed,
      inputs: {'value': PluxStepTrace.redacted},
      errorCode: 'PLX-5301',
      errorKind: 'validation',
    ),
  ],
  stepsExecuted: 1,
  failingStep: 's1',
  errorKind: 'validation',
  errorCode: 'PLX-5301',
);

ScopeModel _model() => ScopeModel(
  kind: StateScopeKind.page,
  owner: '',
  decls: [
    StateDecl(
      name: 'token',
      id: 'a',
      typeExpr: 'string',
      type: null,
      defaultValue: '',
      sensitive: true,
    ),
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

String _decode(Object? b64) => utf8.decode(base64.decode(b64! as String));

void main() {
  late _Diagnostics diag;
  late _Sink sink;
  late DevEventBridge bridge;

  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    diag = _Diagnostics();
    sink = _Sink();
    bridge = DevEventBridge(diagnostics: diag, stateDiffBytes: 64)
      ..attach(sink);
  });
  tearDown(() => bridge.detach());

  test(
    'a new action run is sent once, keyed by its run id [DEV-020] [ACT-030]',
    () {
      diag.actionTraces.value = [_trace(1)];
      diag.actionTraces.value = [_trace(1), _trace(2)];
      expect(sink.events.map((e) => e.$2), ['1', '2']);
      final t = sink.events.last.$1['actionTrace']! as Map<String, Object?>;
      expect(t['runId'], '2');
      expect(t['status'], 'DEV_RUN_STATUS_FAILED');
      expect(t['startUnixMicros'], '1000');
      expect(t['endUnixMicros'], '1500');
      final step = (t['steps']! as List).single as Map<String, Object?>;
      expect(step['action'], 'setState');
      expect(_decode(step['inputs']), '{"value":"‹redacted›"}');
      expect((t['error']! as Map)['code'], 'PLX-5301');
    },
  );

  test('a reported problem becomes an error event with its node [DEV-020]', () {
    diag.log.value = [
      PluxDiagnostic(
        at: DateTime(2026),
        error: const PluxException(
          PluxErrorCode.stateWriteTypeMismatch,
          'bad value',
          details: {'node': '/pages/home/0/1'},
        ),
      ),
    ];
    final e = sink.only('error');
    expect((e['error']! as Map)['code'], 'PLX-5301');
    expect(e['nodePath'], '/pages/home/0/1');
    expect(e['severity'], 'SEVERITY_ERROR');
  });

  test('the logger sends a log line [DEV-020]', () {
    bridge.logger.log(DevLogLevel.info, 'runtime', 'ready', nodePath: '/a');
    expect(sink.only('log'), {
      'level': 'DEV_LOG_LEVEL_INFO',
      'source': 'runtime',
      'message': 'ready',
      'nodePath': '/a',
    });
  });

  test(
    'a sensitive state value is withheld and flagged [DEV-020] [SCH-012]',
    () {
      final model = _model();
      bridge.stateTap(
        model,
        {'token': 'a', 'count': 1},
        {'token': 'b', 'count': 2},
      );
      final d = sink.only('stateDiff');
      expect(d['scope'], 'page');
      final changes = (d['changes']! as List).cast<Map<String, Object?>>();
      final token = changes.firstWhere((c) => c['path'] == 'token');
      expect(token['redacted'], true);
      expect(token.containsKey('oldValue'), isFalse);
      expect(token.containsKey('newValue'), isFalse);
      final count = changes.firstWhere((c) => c['path'] == 'count');
      expect(_decode(count['oldValue']), '1');
      expect(_decode(count['newValue']), '2');
      expect(jsonEncode(d), isNot(contains(base64.encode(utf8.encode('"a"')))));
    },
  );

  test('a state diff over stateDiffBytes keeps paths only [DEV-020]', () {
    bridge.stateTap(_model(), {'count': 1}, {'count': 'x' * 200});
    final d = sink.only('stateDiff');
    expect(d['truncated'], true);
    final c = (d['changes']! as List).single as Map<String, Object?>;
    expect(c['path'], 'count');
    expect(c.containsKey('newValue'), isFalse);
  });

  test('a scope write reaches the tap through the model [DEV-020]', () {
    final seen = <String>[];
    final model = ScopeModel(
      kind: StateScopeKind.app,
      owner: '',
      decls: _model().decls,
      evaluate: (_, _) => null,
      types: const {},
      devTap: (m, p, n) {
        seen.add('${p['count']}->${n['count']}');
        bridge.stateTap(m, p, n);
      },
    );
    final provider = NotifierProvider<_Scope, Map<String, Object?>>(
      () => _Scope(model),
    );
    final c = ProviderContainer();
    addTearDown(c.dispose);
    c.read(provider);
    c.read(provider.notifier).writeEntry('count', 5);
    expect(seen, ['0->5']);
    expect(sink.only('stateDiff')['scope'], 'app');
  });

  test(
    'a network event has no headers, bodies or queries [DEV-020] [SCH-012]',
    () {
      bridge.networkTap(
        DevNetworkCall(
          method: 'GET',
          urlTemplate: '/orders/{id}?limit={limit}',
          status: 200,
          startedAt: DateTime.fromMicrosecondsSinceEpoch(5),
          total: const Duration(milliseconds: 12),
          requestBytes: 3,
          responseBytes: 40,
        ),
      );
      final n = sink.only('networkCall');
      expect(n['method'], 'GET');
      expect(n['urlTemplate'], '/orders/{id}?limit={limit}');
      expect(n['status'], 200);
      expect(n['totalMicros'], '12000');
      expect(n['headersRedacted'], true);
      expect(n['requestBodyRedacted'], true);
      expect(n['responseBodyRedacted'], true);
      for (final k in [
        'requestHeaders',
        'responseHeaders',
        'requestBody',
        'responseBody',
      ]) {
        expect(n.containsKey(k), isFalse, reason: k);
      }
    },
  );

  test('a function call carries the trace id of its run [FN-036]', () {
    bridge.functionTap(
      DevFunctionCall(
        function: 'orders.price',
        version: 3,
        placement: 'server',
        startedAt: DateTime.fromMicrosecondsSinceEpoch(7),
        duration: const Duration(microseconds: 900),
        runId: 42,
        businessErrorCode: 'OUT_OF_STOCK',
      ),
    );
    final (payload, trace) = sink.events.single;
    expect(trace, '42');
    final f = payload['functionCall']! as Map<String, Object?>;
    expect(f['function'], 'orders.price');
    expect(f['version'], 3);
    expect(f['placement'], 'server');
    expect(f['durationMicros'], '900');
    expect(f['businessErrorCode'], 'OUT_OF_STOCK');
  });

  test('sync stages map to sync events [DEV-020]', () {
    diag.syncStatus.value = const SyncChecking();
    diag.syncStatus.value = const SyncDownloading(10, 100);
    diag.syncStatus.value = const SyncStaged(7);
    final phases = [
      for (final (p, _) in sink.events)
        (p['sync']! as Map<String, Object?>)['phase'],
    ];
    expect(phases, [
      'DEV_SYNC_PHASE_CHECK',
      'DEV_SYNC_PHASE_DOWNLOAD',
      'DEV_SYNC_PHASE_STAGED',
    ]);
    final staged = sink.events.last.$1['sync']! as Map<String, Object?>;
    expect(staged['releaseSequence'], '7');
  });

  test('frame timings are batched per window [DEV-020]', () {
    int us(int v) => v;
    SchedulerBinding.instance.platformDispatcher.onReportTimings!([
      FrameTiming(
        vsyncStart: us(0),
        buildStart: us(0),
        buildFinish: us(4000),
        rasterStart: us(4000),
        rasterFinish: us(24000),
        rasterFinishWallTime: us(24000),
        frameNumber: 1,
      ),
    ]);
    expect(sink.events, isEmpty);
    bridge.flushFrames();
    final f = sink.only('frameTiming');
    final frame = (f['frames']! as List).single as Map<String, Object?>;
    expect(frame['buildMicros'], '4000');
    expect(frame['rasterMicros'], '20000');
    expect(frame['janky'], true);
  });

  test('detaching stops every source [DEV-020]', () {
    bridge.detach();
    diag.actionTraces.value = [_trace(1)];
    diag.syncStatus.value = const SyncChecking();
    bridge.logger.log(DevLogLevel.info, 'runtime', 'x');
    bridge.stateTap(_model(), {'count': 1}, {'count': 2});
    bridge.networkTap(
      DevNetworkCall(
        method: 'GET',
        urlTemplate: '/x',
        status: 200,
        startedAt: DateTime(2026),
        total: Duration.zero,
      ),
    );
    bridge.flushFrames();
    expect(bridge.attached, isFalse);
    expect(sink.events, isEmpty);
  });

  test('a bridge that was never attached emits nothing [DEV-020]', () {
    final other = DevEventBridge(diagnostics: diag);
    diag.actionTraces.value = [_trace(9)];
    other.stateTap(_model(), {'count': 1}, {'count': 2});
    expect(other.attached, isFalse);
    expect(sink.events.map((e) => e.$2), ['9']);
  });
}

final class _Scope extends Notifier<Map<String, Object?>> with ScopeValues {
  _Scope(this.m);
  final ScopeModel m;

  @override
  ScopeModel? get model => m;

  @override
  Map<String, Object?> build() => initialValues();
}
