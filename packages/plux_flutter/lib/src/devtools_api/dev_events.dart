// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The device-side sources of the dev stream (DEV-020, ACT-030, FN-036).
///
/// A [DevEventBridge] turns what the runtime does — action runs, problems,
/// state changes, network calls, function calls, sync stages and frame
/// timings — into `DevEvent` payloads (proto3 JSON of `devevents.proto`,
/// keyed by oneof name) and hands them to a [DevEventSink]. Detached it
/// holds no listener and does no work; every source redacts at the source:
/// a sensitive state value, a header, a body or a query value never reaches
/// the stream (SCH-012, SEC-092).
library;

import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/scheduler.dart';
import 'package:plux_flutter/src/actions/trace.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/devtools_api/diagnostics.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/pxl/types.dart' show toJson;
import 'package:plux_flutter/src/state/scope_state.dart';
import 'package:plux_flutter/src/sync/sync_event.dart';

/// A log level of the dev stream (`DevLogLevel`).
enum DevLogLevel {
  /// Finest detail.
  trace('DEV_LOG_LEVEL_TRACE'),

  /// Detail for developers.
  debug('DEV_LOG_LEVEL_DEBUG'),

  /// Normal operation.
  info('DEV_LOG_LEVEL_INFO'),

  /// Something unexpected that the runtime handled.
  warning('DEV_LOG_LEVEL_WARNING'),

  /// A failure.
  error('DEV_LOG_LEVEL_ERROR');

  const DevLogLevel(this.protoName);

  /// The enum's full name in proto3 JSON.
  final String protoName;
}

/// How the runtime writes a log line to the dev stream; a line goes nowhere
/// while no session listens.
final class DevLogger {
  DevLogger._(this._bridge);

  final DevEventBridge _bridge;

  /// Logs [message], already free of sensitive values, from [source]:
  /// `runtime`, `action`, `pxl`, `host` or a plugin key.
  void log(
    DevLogLevel level,
    String source,
    String message, {
    String nodePath = '',
  }) {
    if (!kPluxDevtools || !_bridge.attached) return;
    _bridge._emit({
      'log': {
        'level': level.protoName,
        'source': source,
        'message': message,
        if (nodePath.isNotEmpty) 'nodePath': nodePath,
      },
    });
  }
}

/// One HTTP call of a data source, as the client reports it. It carries no
/// header, body or query value (SCH-012).
@immutable
final class DevNetworkCall {
  /// Creates the report.
  const DevNetworkCall({
    required this.method,
    required this.urlTemplate,
    required this.status,
    required this.startedAt,
    required this.total,
    this.requestBytes = 0,
    this.responseBytes = 0,
    this.error,
  });

  /// The HTTP method.
  final String method;

  /// The URL with path and query values as placeholders.
  final String urlTemplate;

  /// The status; 0 when no response arrived.
  final int status;

  /// When the call started.
  final DateTime startedAt;

  /// How long it took.
  final Duration total;

  /// Bytes of the request body.
  final int requestBytes;

  /// Bytes of the response body.
  final int responseBytes;

  /// The failure of a call that got no usable response.
  final PluxException? error;
}

/// One invocation of a Plux Function on the device or through the server.
@immutable
final class DevFunctionCall {
  /// Creates the report.
  const DevFunctionCall({
    required this.function,
    required this.version,
    required this.placement,
    required this.startedAt,
    required this.duration,
    this.runId = 0,
    this.businessErrorCode,
    this.error,
    this.replayed = false,
    this.logs = const [],
  });

  /// The dotted function name.
  final String function;

  /// The pinned version; 0 when unknown.
  final int version;

  /// `device` or `server`.
  final String placement;

  /// When the call started.
  final DateTime startedAt;

  /// How long it took.
  final Duration duration;

  /// The action run that made the call (its trace ID); 0 when none did.
  final int runId;

  /// The function's own error code (FN-022).
  final String? businessErrorCode;

  /// A system error of the call.
  final PluxException? error;

  /// Whether the idempotency record answered (FN-021).
  final bool replayed;

  /// The lines a device function logged.
  final List<({DateTime at, DevLogLevel level, String message})> logs;
}

/// Turns runtime activity into dev stream events while attached to a
/// [DevEventSink].
final class DevEventBridge {
  /// Creates a bridge that follows [diagnostics]. State diffs are bounded by
  /// [stateDiffBytes] (`devtools.stateDiffBytes`); frames are batched per
  /// [frameWindow], tagged with the route [route] returns.
  DevEventBridge({
    required this.diagnostics,
    this.stateDiffBytes = 16384,
    this.frameWindow = const Duration(seconds: 1),
    String Function()? route,
  }) : _route = route ?? (() => '') {
    logger = DevLogger._(this);
  }

  /// What the bridge follows: problems, sync stages and action traces.
  final PluxDiagnostics diagnostics;

  /// The most bytes of values one state event carries.
  final int stateDiffBytes;

  /// The length of a frame timing window.
  final Duration frameWindow;

  final String Function() _route;

  /// Where the runtime writes log lines.
  late final DevLogger logger;

  DevEventSink? _sink;
  int _lastRun = 0;
  PluxDiagnostic? _lastProblem;
  int _calls = 0;
  Stopwatch? _syncWatch;
  final List<Map<String, Object?>> _frames = [];
  Timer? _frameTimer;

  /// Whether a sink receives the events.
  bool get attached => _sink != null;

  /// Starts sending events to [sink]; replaces an earlier sink. Only what
  /// happens from now on is sent. Does nothing without [kPluxDevtools].
  void attach(DevEventSink sink) {
    if (!kPluxDevtools) return;
    if (_sink != null) detach();
    _sink = sink;
    final traces = diagnostics.actionTraces.value;
    _lastRun = traces.isEmpty ? 0 : traces.last.runId;
    final log = diagnostics.log.value;
    _lastProblem = log.isEmpty ? null : log.last;
    diagnostics.actionTraces.addListener(_onTraces);
    diagnostics.log.addListener(_onProblems);
    diagnostics.syncStatus.addListener(_onSync);
    SchedulerBinding.instance.addTimingsCallback(_onTimings);
    _frameTimer = Timer.periodic(frameWindow, (_) => flushFrames());
  }

  /// Stops sending events and drops what was batched.
  void detach() {
    if (_sink == null) return;
    diagnostics.actionTraces.removeListener(_onTraces);
    diagnostics.log.removeListener(_onProblems);
    diagnostics.syncStatus.removeListener(_onSync);
    SchedulerBinding.instance.removeTimingsCallback(_onTimings);
    _frameTimer?.cancel();
    _frameTimer = null;
    _frames.clear();
    _syncWatch = null;
    _sink = null;
  }

  void _emit(Map<String, Object?> payload, {String? traceId}) =>
      _sink?.emit(payload, traceId: traceId);

  // --- action traces (ACT-030) ---

  void _onTraces() {
    for (final run in diagnostics.actionTraces.value) {
      if (run.runId <= _lastRun) continue;
      _lastRun = run.runId;
      _emit({'actionTrace': _trace(run)}, traceId: '${run.runId}');
    }
  }

  Map<String, Object?> _trace(PluxActionTrace run) {
    final start = run.startedAt.microsecondsSinceEpoch;
    return {
      'runId': '${run.runId}',
      'graph': run.graphId,
      'trigger': run.trigger,
      'status': _runStatus(run.status),
      'startUnixMicros': '$start',
      'endUnixMicros': '${start + run.duration.inMicroseconds}',
      'steps': [
        for (final s in run.steps)
          {
            'stepId': s.id,
            'action': s.action,
            'branch': 'main',
            'status': _runStatus(s.status),
            'startUnixMicros': '${start + s.start.inMicroseconds}',
            'durationMicros': '${s.duration.inMicroseconds}',
            if (s.inputs.isNotEmpty) 'inputs': _json(s.inputs),
            if (s.output != null) 'outputs': _json(s.output),
            if (s.errorCode != null)
              'error': {'code': s.errorCode, 'message': ?s.errorKind},
          },
      ],
      if (run.errorCode != null)
        'error': {'code': run.errorCode, 'message': run.errorKind ?? ''},
    };
  }

  static String _runStatus(PluxTraceStatus s) => switch (s) {
    PluxTraceStatus.ok => 'DEV_RUN_STATUS_SUCCEEDED',
    PluxTraceStatus.failed => 'DEV_RUN_STATUS_FAILED',
    PluxTraceStatus.cancelled => 'DEV_RUN_STATUS_CANCELLED',
  };

  /// The value as the base64 of its UTF-8 JSON (a `bytes` field).
  static String _json(Object? value) =>
      base64.encode(utf8.encode(jsonEncode(toJson(value))));

  // --- problems (error events) ---

  void _onProblems() {
    final all = diagnostics.log.value;
    final last = _lastProblem;
    final from = last == null ? -1 : all.lastIndexOf(last);
    for (var i = from + 1; i < all.length; i++) {
      final e = all[i].error;
      _emit({
        'error': {
          'error': _error(e),
          if ((e.details['node'] ?? '').isNotEmpty)
            'nodePath': e.details['node'],
          'severity': 'SEVERITY_ERROR',
        },
      });
    }
    _lastProblem = all.isEmpty ? null : all.last;
  }

  static Map<String, Object?> _error(PluxException e) => {
    'code': e.code.id,
    'reason': e.code.reason,
    'message': e.message,
  };

  // --- state (DEV-020, SCH-012) ---

  /// Reports a change of a scope's values; install as the renderer's
  /// `devStateTap`.
  void stateTap(
    ScopeModel model,
    Map<String, Object?> previous,
    Map<String, Object?> next,
  ) {
    if (_sink == null) return;
    final changes =
        <({String path, Object? old, Object? now, bool redacted})>[];
    void walk(String path, Object? a, Object? b, bool sensitive) {
      if (sameValue(a, b)) return;
      if (!sensitive &&
          a is Map<String, Object?> &&
          b is Map<String, Object?>) {
        for (final k in {...a.keys, ...b.keys}) {
          walk('$path.$k', a[k], b[k], false);
        }
        return;
      }
      changes.add((path: path, old: a, now: b, redacted: sensitive));
    }

    for (final k in {...previous.keys, ...next.keys}) {
      walk(k, previous[k], next[k], model.byName[k]?.sensitive ?? false);
    }
    if (changes.isEmpty) return;
    var size = 0;
    final values = <int, (String, String)>{};
    for (var i = 0; i < changes.length; i++) {
      final c = changes[i];
      if (c.redacted) continue;
      final pair = (_json(c.old), _json(c.now));
      values[i] = pair;
      size += pair.$1.length + pair.$2.length;
    }
    final truncated = size > stateDiffBytes;
    _emit({
      'stateDiff': {
        'scope': model.devScope.isEmpty ? model.root : model.devScope,
        'changes': [
          for (var i = 0; i < changes.length; i++)
            {
              'path': changes[i].path,
              if (changes[i].redacted) 'redacted': true,
              if (!truncated && values[i] != null) ...{
                'oldValue': values[i]!.$1,
                'newValue': values[i]!.$2,
              },
            },
        ],
        if (truncated) 'truncated': true,
      },
    });
  }

  // --- network (DEV-020) ---

  /// Reports a call of a data source; install as the data client's
  /// `devTap`. Headers and bodies are withheld (SCH-012).
  void networkTap(DevNetworkCall call) {
    if (_sink == null) return;
    final id = '${++_calls}';
    _emit({
      'networkCall': {
        'callId': id,
        'method': call.method,
        'urlTemplate': call.urlTemplate,
        'status': call.status,
        'startUnixMicros': '${call.startedAt.microsecondsSinceEpoch}',
        'totalMicros': '${call.total.inMicroseconds}',
        'requestBytes': '${call.requestBytes}',
        'responseBytes': '${call.responseBytes}',
        'headersRedacted': true,
        'requestBodyRedacted': true,
        'responseBodyRedacted': true,
        if (call.error != null) 'error': _error(call.error!),
      },
    });
  }

  // --- functions (FN-036) ---

  /// Reports an invocation of a function; install as the function
  /// runtime's `devTap`. The event's trace ID is the action run's, so the
  /// server merges the logs of a server function by it.
  void functionTap(DevFunctionCall call) {
    if (_sink == null) return;
    _emit({
      'functionCall': {
        'callId': '${++_calls}',
        'function': call.function,
        'version': call.version,
        'placement': call.placement,
        'startUnixMicros': '${call.startedAt.microsecondsSinceEpoch}',
        'durationMicros': '${call.duration.inMicroseconds}',
        if (call.businessErrorCode != null)
          'businessErrorCode': call.businessErrorCode,
        if (call.error != null) 'error': _error(call.error!),
        if (call.replayed) 'replayed': true,
        if (call.logs.isNotEmpty)
          'logs': [
            for (final l in call.logs)
              {
                'timeUnixMicros': '${l.at.microsecondsSinceEpoch}',
                'level': l.level.protoName,
                'message': l.message,
              },
          ],
      },
    }, traceId: call.runId == 0 ? null : '${call.runId}');
  }

  // --- sync (SYN-010) ---

  void _onSync() {
    final e = diagnostics.syncStatus.value;
    if (e == null) return;
    if (e is SyncChecking) _syncWatch = (Stopwatch()..start());
    final elapsed = _syncWatch?.elapsedMicroseconds ?? 0;
    final ended = e is SyncUpToDate || e is SyncStaged || e is SyncFailed;
    Map<String, Object?> event(
      String phase, {
      int? sequence,
      int bytes = 0,
      PluxException? error,
    }) => {
      'phase': phase,
      if (sequence != null) 'releaseSequence': '$sequence',
      if (bytes != 0) 'bytesTransferred': '$bytes',
      if (ended || e is SyncActivated) 'durationMicros': '$elapsed',
      if (error != null) 'error': _error(error),
    };
    final payload = switch (e) {
      SyncChecking() => event('DEV_SYNC_PHASE_CHECK'),
      SyncUpToDate(:final sequence) => event(
        'DEV_SYNC_PHASE_COMPLETE',
        sequence: sequence,
      ),
      SyncDownloading(:final received) => event(
        'DEV_SYNC_PHASE_DOWNLOAD',
        bytes: received,
      ),
      SyncStaged(:final sequence) => event(
        'DEV_SYNC_PHASE_STAGED',
        sequence: sequence,
      ),
      SyncActivated(:final sequence) => event(
        'DEV_SYNC_PHASE_APPLY',
        sequence: sequence,
      ),
      SyncRolledBack(:final to) => event('DEV_SYNC_PHASE_APPLY', sequence: to),
      SyncFailed(:final error) => event('DEV_SYNC_PHASE_FAILED', error: error),
    };
    if (ended) _syncWatch = null;
    _emit({'sync': payload});
  }

  // --- frames (DEV-020) ---

  void _onTimings(List<FrameTiming> timings) {
    for (final t in timings) {
      final build = t.buildDuration.inMicroseconds;
      final raster = t.rasterDuration.inMicroseconds;
      _frames.add({
        'buildMicros': '$build',
        'rasterMicros': '$raster',
        if (build > 16667 || raster > 16667) 'janky': true,
      });
    }
  }

  /// Sends the frames batched since the last window, if any; the timer
  /// calls it once per [frameWindow].
  void flushFrames() {
    if (_sink == null || _frames.isEmpty) return;
    final frames = List<Map<String, Object?>>.of(_frames);
    _frames.clear();
    _emit({
      'frameTiming': {
        'windowMicros': '${frameWindow.inMicroseconds}',
        'route': _route(),
        'frames': frames,
      },
    });
  }
}
