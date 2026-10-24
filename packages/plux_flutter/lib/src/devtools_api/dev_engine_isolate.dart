// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Runs the [DevEngine] on a background isolate (L-6): the socket, the JSON
/// and the event buffer never touch the UI isolate, which only posts
/// messages. What needs the UI isolate comes back as a request the isolate
/// answers through the [DevEngineHost].
library;

import 'dart:async';
import 'dart:isolate';

import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/data/stream_transport.dart';
import 'package:plux_flutter/src/devtools_api/dev_engine.dart';

/// Creates the transport on the background isolate; a replacement must be a
/// top-level or static function, because it crosses to that isolate.
StreamTransport defaultDevTransport() => SocketStreamTransport(http.Client());

/// Starts an engine for [host] with the given [config] and [transport].
typedef DevEngineStarter = Future<DevEngineHandle> Function(
  DevEngineConfig config,
  DevEngineHost host,
);

/// Starts the engine on a background isolate.
Future<DevEngineHandle> startDevEngineIsolate(
  DevEngineConfig config,
  DevEngineHost host, {
  StreamTransport Function() transport = defaultDevTransport,
}) async {
  final requests = ReceivePort();
  final ready = Completer<SendPort>();
  final exited = ReceivePort();
  // The engine handle cancels it when it closes.
  // ignore: cancel_subscriptions
  final sub = requests.listen((message) {
    switch (message) {
      case SendPort():
        ready.complete(message);
      case (final String kind, final Object? arg, final SendPort reply):
        unawaited(_answer(host, kind, arg, reply));
      case ('closed', final String? code):
        unawaited(host.closed(endedCode: code));
    }
  });
  final isolate = await Isolate.spawn(
    _main,
    (config, transport, requests.sendPort),
    debugName: 'plux-dev-channel',
    onExit: exited.sendPort,
  );
  final commands = await ready.future;
  return _IsolateEngine(isolate, commands, exited, requests, sub);
}

Future<void> _answer(
  DevEngineHost host,
  String kind,
  Object? arg,
  SendPort reply,
) async {
  try {
    switch (kind) {
      case 'auth':
        final auth = await host.auth(first: arg == true);
        reply.send(
          auth == null
              ? ('ok', null)
              : ('ok', [auth.url.toString(), auth.ticket, auth.proof]),
        );
      case 'frame':
        reply.send(('ok', await host.frame(arg! as Map<String, Object?>)));
      default:
        reply.send(('err', 'unknown request $kind'));
    }
  } on Object catch (e) {
    reply.send(('err', '$e'));
  }
}

final class _IsolateEngine implements DevEngineHandle {
  _IsolateEngine(
    this._isolate,
    this._commands,
    this._exited,
    this._requests,
    this._sub,
  );

  final Isolate _isolate;
  final SendPort _commands;
  final ReceivePort _exited;
  final ReceivePort _requests;
  final StreamSubscription<dynamic> _sub;
  var _closed = false;

  @override
  void emit(Map<String, Object?> payload, {String? traceId}) {
    if (_closed) return;
    _commands.send(('emit', payload, traceId));
  }

  @override
  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    _commands.send(('close',));
    try {
      await _exited.first.timeout(const Duration(seconds: 5));
    } on TimeoutException {
      _isolate.kill(priority: Isolate.immediate);
    }
    await _sub.cancel();
    _requests.close();
    _exited.close();
  }
}

final class _RemoteHost implements DevEngineHost {
  _RemoteHost(this._requests);

  final SendPort _requests;

  Future<Object?> _ask(String kind, Object? arg) async {
    final reply = ReceivePort();
    _requests.send((kind, arg, reply.sendPort));
    final answer = await reply.first as (String, Object?);
    reply.close();
    if (answer.$1 != 'ok') throw StateError('${answer.$2}');
    return answer.$2;
  }

  @override
  Future<DevSocketAuth?> auth({required bool first}) async {
    final r = await _ask('auth', first) as List<Object?>?;
    if (r == null) return null;
    return DevSocketAuth(
      url: Uri.parse(r[0]! as String),
      ticket: r[1]! as String,
      proof: r[2]! as String,
    );
  }

  @override
  Future<List<Map<String, Object?>>> frame(Map<String, Object?> frame) async {
    final r = await _ask('frame', frame) as List<Object?>;
    return [for (final p in r) (p! as Map).cast<String, Object?>()];
  }

  @override
  Future<void> closed({String? endedCode}) async {
    _requests.send(('closed', endedCode));
  }
}

Future<void> _main(
  (DevEngineConfig, StreamTransport Function(), SendPort) args,
) async {
  final (config, transport, requests) = args;
  final inbox = ReceivePort();
  final engine = DevEngine(
    config: config,
    host: _RemoteHost(requests),
    transport: transport(),
  );
  final loop = engine.start();
  requests.send(inbox.sendPort);
  await for (final m in inbox) {
    switch (m) {
      case ('emit', final Map<Object?, Object?> payload, final String? trace):
        engine.emit(payload.cast<String, Object?>(), traceId: trace);
      case ('close',):
        await engine.close();
        await loop;
        inbox.close();
    }
  }
  Isolate.exit();
}
