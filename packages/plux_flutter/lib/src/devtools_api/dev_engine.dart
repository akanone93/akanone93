// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The part of the dev channel that owns the socket (DEV-021, L-6): it
/// connects, speaks the frames in JSON, buffers the device's events until
/// the server acknowledges them and reconnects with backoff, resuming where
/// it stopped. It runs on a background isolate in apps and in-process in
/// tests; what needs the UI isolate (the DPoP key, the applier, the command
/// handler) is reached through a [DevEngineHost].
library;

import 'dart:async';
import 'dart:collection';
import 'dart:convert';
import 'dart:math';

import 'package:plux_flutter/src/data/stream_transport.dart';

/// What the engine needs to open a socket: the URL, the single-use ticket
/// and the DPoP proof for the upgrade.
final class DevSocketAuth {
  /// Creates the credentials of one connection attempt.
  const DevSocketAuth({
    required this.url,
    required this.ticket,
    required this.proof,
  });

  /// The `ws`/`wss` URL of the dev socket.
  final Uri url;

  /// The ticket of `DevSessionService.Pair`, sent as `Plux-Dev-Ticket`.
  final String ticket;

  /// The DPoP proof, sent as `DPoP`.
  final String proof;
}

/// What the engine asks of the UI isolate.
abstract interface class DevEngineHost {
  /// The credentials for the next connection attempt; [first] is true for
  /// the first one, whose ticket Pair just returned. Null means the session
  /// can no longer be reached and the channel gives up.
  Future<DevSocketAuth?> auth({required bool first});

  /// Carries out a bundle or command frame (the proto3 JSON of a
  /// `DevServerFrame`) and returns the event payloads to report about it.
  Future<List<Map<String, Object?>>> frame(Map<String, Object?> frame);

  /// The channel closed for good: [endedCode] is the session's error code
  /// when the server ended the session, null when the channel gave up.
  Future<void> closed({String? endedCode});
}

/// The limits and timings of an engine, plain numbers so they cross to the
/// background isolate.
final class DevEngineConfig {
  /// Creates a configuration.
  const DevEngineConfig({
    required this.eventBytes,
    required this.bufferEvents,
    required this.bufferBytes,
    required this.maxFrameBytes,
    required this.connectTimeoutMs,
    required this.backoffMinMs,
    required this.backoffMaxMs,
  });

  /// `devtools.eventBytes`: the largest event; a larger one is dropped.
  final int eventBytes;

  /// `devtools.streamBufferEvents`: events kept until acknowledged.
  final int bufferEvents;

  /// `devtools.streamBufferBytes`: bytes kept until acknowledged.
  final int bufferBytes;

  /// The largest frame the server may send.
  final int maxFrameBytes;

  /// How long connecting may take.
  final int connectTimeoutMs;

  /// The first wait before a reconnect (`data.streamBackoffMin`).
  final int backoffMinMs;

  /// The longest wait between reconnects (`data.streamBackoffMax`).
  final int backoffMaxMs;
}

/// What the UI isolate holds of a running engine.
abstract interface class DevEngineHandle {
  /// Queues an event payload; never blocks.
  void emit(Map<String, Object?> payload, {String? traceId});

  /// Closes the socket and stops reconnecting.
  Future<void> close();
}

final class _Buffered {
  _Buffered(this.sequence, this.event, this.bytes);
  final int sequence;
  final Map<String, Object?> event;
  final int bytes;
}

/// The dev channel's socket, buffer and reconnect loop.
final class DevEngine implements DevEngineHandle {
  /// Creates an engine; call [start] to connect. [clock], [delay] and
  /// [random] are replaced in tests.
  DevEngine({
    required this.config,
    required this.host,
    required this.transport,
    DateTime Function()? clock,
    Future<void> Function(Duration)? delay,
    double Function()? random,
  }) : _clock = clock ?? DateTime.now,
       _delay = delay ?? Future<void>.delayed,
       _random = random ?? Random().nextDouble;

  /// Limits and timings.
  final DevEngineConfig config;

  /// The UI isolate's side.
  final DevEngineHost host;

  /// Opens the WebSocket.
  final StreamTransport transport;

  final DateTime Function() _clock;
  final Future<void> Function(Duration) _delay;
  final double Function() _random;

  final Queue<_Buffered> _buffer = Queue();
  final Completer<void> _closing = Completer();
  Future<void>? _loop;
  StreamConnection? _conn;
  var _bytes = 0;
  var _nextSequence = 1;
  var _lastAcknowledged = 0;
  var _lastServerSequence = 0;
  var _tooLarge = 0;
  var _bufferFull = 0;
  var _closed = false;

  /// The device sequence of the last event acknowledged.
  int get lastAcknowledgedEvent => _lastAcknowledged;

  /// Events held until the server acknowledges them.
  int get buffered => _buffer.length;

  /// Starts connecting; returns when the loop has ended.
  Future<void> start() => _loop ??= _run();

  @override
  void emit(Map<String, Object?> payload, {String? traceId}) {
    if (_closed) return;
    _add(payload, traceId);
    _reportDrops();
  }

  @override
  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    if (!_closing.isCompleted) _closing.complete();
    final conn = _conn;
    _conn = null;
    if (conn != null) {
      try {
        await conn.close();
      } on Object {
        // Already broken: nothing is left to close.
      }
    }
  }

  void _add(Map<String, Object?> payload, String? traceId) {
    final event = <String, Object?>{
      'sequence': '$_nextSequence',
      'timeUnixMicros': '${_clock().microsecondsSinceEpoch}',
      'traceId': ?traceId,
      ...payload,
    };
    final bytes = utf8.encode(jsonEncode(event)).length;
    if (bytes > config.eventBytes) {
      _tooLarge++;
      return;
    }
    _nextSequence++;
    _buffer.add(_Buffered(_nextSequence - 1, event, bytes));
    _bytes += bytes;
    _send({'event': event});
    _trim(keep: 1);
  }

  /// Drops the oldest events until the buffer fits its bounds with room for
  /// [reserveEvents] more events of [reserveBytes]; the newest [keep]
  /// events stay.
  void _trim({int keep = 0, int reserveEvents = 0, int reserveBytes = 0}) {
    while (_buffer.length > keep &&
        (_buffer.length + reserveEvents > config.bufferEvents ||
            _bytes + reserveBytes > config.bufferBytes)) {
      _bytes -= _buffer.removeFirst().bytes;
      _bufferFull++;
    }
  }

  /// Reports what was dropped as one `drop` event per reason, once.
  void _reportDrops() {
    if (_tooLarge > 0) {
      final n = _tooLarge;
      _tooLarge = 0;
      _addDrop(n, 'DEV_DROP_REASON_EVENT_TOO_LARGE');
    }
    if (_bufferFull > 0) {
      _trim(reserveEvents: 1, reserveBytes: _dropEventBytes);
      final n = _bufferFull;
      _bufferFull = 0;
      _addDrop(n, 'DEV_DROP_REASON_BUFFER_FULL');
    }
  }

  // The size reserved for a drop event: its JSON is well below this.
  static const int _dropEventBytes = 256;

  void _addDrop(int count, String reason) {
    _trim(reserveEvents: 1, reserveBytes: _dropEventBytes);
    _add({
      'drop': {'dropped': '$count', 'reason': reason},
    }, null);
  }

  void _send(Map<String, Object?> frame) {
    try {
      _conn?.send(frame);
    } on Object {
      // A socket that broke is noticed by its frame stream, which ends.
    }
  }

  Future<void> _run() async {
    var attempt = 0;
    var first = true;
    while (!_closed) {
      try {
        final auth = await host.auth(first: first);
        first = false;
        if (_closed) break;
        if (auth == null) {
          await host.closed();
          break;
        }
        final conn = await transport.open(
          StreamRequest(
            kind: StreamKind.webSocket,
            url: auth.url,
            headers: {'Plux-Dev-Ticket': auth.ticket, 'DPoP': auth.proof},
            maxMessageBytes: config.maxFrameBytes,
            connectTimeout: Duration(milliseconds: config.connectTimeoutMs),
          ),
        );
        if (_closed) {
          await conn.close();
          break;
        }
        _conn = conn;
        attempt = 0;
        await _serve(conn);
      } on Object {
        // The attempt failed: wait, then try again.
      }
      final conn = _conn;
      _conn = null;
      if (conn != null) {
        try {
          await conn.close();
        } on Object {
          // Already broken.
        }
      }
      if (_closed) break;
      await Future.any([_delay(_backoff(attempt++)), _closing.future]);
    }
  }

  Duration _backoff(int attempt) {
    final grown = config.backoffMinMs * (1 << min(attempt, 20));
    final ms = min(grown, config.backoffMaxMs);
    return Duration(milliseconds: (ms * (1 - 0.5 * _random())).round());
  }

  Future<void> _serve(StreamConnection conn) async {
    conn.send({
      'hello': {
        'lastServerSequence': '$_lastServerSequence',
        'lastAcknowledgedEvent': '$_lastAcknowledged',
      },
    });
    for (final b in _buffer.toList()) {
      conn.send({'event': b.event});
    }
    await for (final f in conn.frames) {
      if (f.kind == StreamFrameKind.close) break;
      if (f.kind != StreamFrameKind.message) continue;
      final data = f.data;
      if (data is! Map<String, Object?>) continue;
      if (await _handle(data)) break;
    }
  }

  /// Handles one server frame; true when the session ended.
  Future<bool> _handle(Map<String, Object?> frame) async {
    final sequence = _u64(frame['sequence']);
    if (sequence != null && sequence != 0 && sequence <= _lastServerSequence) {
      return false;
    }
    if (frame['ack'] case final Map<String, Object?> ack) {
      final upTo = _u64(ack['upTo']) ?? 0;
      if (upTo > _lastAcknowledged) _lastAcknowledged = upTo;
      while (_buffer.isNotEmpty && _buffer.first.sequence <= upTo) {
        _bytes -= _buffer.removeFirst().bytes;
      }
    } else if (frame['ended'] case final Map<String, Object?> ended) {
      _closed = true;
      if (!_closing.isCompleted) _closing.complete();
      await host.closed(endedCode: ended['code'] as String? ?? '');
      return true;
    } else if (frame.containsKey('bundle') || frame.containsKey('command')) {
      final List<Map<String, Object?>> payloads;
      try {
        payloads = await host.frame(frame);
      } on Object {
        return false;
      }
      for (final p in payloads) {
        emit(p);
      }
    }
    if (sequence != null && sequence > _lastServerSequence) {
      _lastServerSequence = sequence;
    }
    return false;
  }
}

int? _u64(Object? v) => switch (v) {
  int() => v,
  String() => int.tryParse(v),
  _ => null,
};
