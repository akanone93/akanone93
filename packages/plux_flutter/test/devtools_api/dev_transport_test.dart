// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:isolate';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/data/stream_transport.dart';
import 'package:plux_flutter/src/devtools_api/dev_engine_isolate.dart';

import '../support/test_certificate.dart';

/// A client that trusts only [pem]: what a host that trusts its own
/// certificate authority gives `PluxConfig.webSocketClient`. Built at the top
/// level, so that it captures [pem] alone and can cross isolates.
HttpClient Function() _trusting(String pem) =>
    () => HttpClient(
      context: SecurityContext(withTrustedRoots: false)
        ..setTrustedCertificatesBytes(utf8.encode(pem)),
    );

/// Opens [url] with a transport [make] builds on the isolate it runs on,
/// sends a frame and returns the echo; the dev channel's engine runs on its
/// own isolate in the same way.
Future<String?> _echoOnAnotherIsolate(
  StreamTransport Function() make,
  Uri url,
) => Isolate.run(() async {
  final c = await make().open(
    StreamRequest(
      kind: StreamKind.webSocket,
      url: url,
      maxMessageBytes: 1000,
      connectTimeout: const Duration(seconds: 30),
    ),
  );
  try {
    final first = c.frames.first;
    c.send('hello');
    return (await first).data?.toString();
  } finally {
    await c.close();
  }
});

void main() {
  late TestCertificate cert;
  late HttpServer server;

  setUpAll(() async => cert = await TestCertificate.generate());
  setUp(() async {
    server = await HttpServer.bindSecure('localhost', 0, cert.serverContext());
    server.listen((r) async {
      final ws = await WebSocketTransformer.upgrade(r);
      ws.listen((m) => ws.add('echo $m'), onDone: ws.close);
    });
  });
  tearDown(() => server.close(force: true));

  Uri url() => Uri.parse('wss://localhost:${server.port}/dev/socket');

  test('DEV-002 the dev channel connects with the host\'s WebSocket client '
      'from its own isolate', () async {
    final echo = await _echoOnAnotherIsolate(
      devTransportWith(_trusting(cert.pem)),
      url(),
    );
    expect(echo, contains('hello'));
  });

  test('DEV-002 without the host\'s client the channel trusts the system\'s '
      'authorities only', () async {
    await expectLater(
      _echoOnAnotherIsolate(defaultDevTransport, url()),
      throwsA(anything),
    );
  });
}
