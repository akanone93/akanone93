// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/stream_session.dart';
import 'package:plux_flutter/src/data/stream_transport.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/platform/pinned_http.dart';

import '../support/test_certificate.dart';

void main() {
  late TestCertificate cert;
  late HttpServer server;

  setUpAll(() async {
    cert = await TestCertificate.generate();
  });

  setUp(() async {
    server = await HttpServer.bindSecure('localhost', 0, cert.serverContext());
    server.listen((r) async {
      final ws = await WebSocketTransformer.upgrade(r);
      ws.listen((m) => ws.add('echo $m'), onDone: ws.close);
    });
  });
  tearDown(() => server.close(force: true));

  StreamRequest request() => StreamRequest(
    kind: StreamKind.webSocket,
    url: Uri.parse('wss://localhost:${server.port}/live'),
    maxMessageBytes: 1000,
    connectTimeout: const Duration(seconds: 5),
  );

  WebSocketClients sockets({String? host, List<String>? pins}) =>
      WebSocketClients(serverHost: host, serverPins: pins)
        ..trustContext = cert.clientContext();

  SocketStreamTransport transport(WebSocketClients sockets) {
    final client = http.Client();
    addTearDown(client.close);
    return SocketStreamTransport(client, pinned: sockets);
  }

  Matcher mismatch() => throwsA(
    isA<DataFailure>().having(
      (f) => f.code,
      'code',
      PluxErrorCode.certificatePinMismatch,
    ),
  );

  Future<void> echoes(SocketStreamTransport t) async {
    final c = await t.open(request());
    addTearDown(c.close);
    final first = c.frames.first;
    c.send('hello');
    expect((await first).data, isNotNull);
  }

  test('[SEC-041] the server host connects over its pinned client', () async {
    final s = sockets(host: 'localhost', pins: [unrelatedPin(1), cert.pin]);
    expect(s.clientFor(Uri.parse('wss://localhost/live')), isNotNull);
    await echoes(transport(s));
  });

  test('[SEC-041] a server certificate matching no pin fails with '
      'PLX-6020', () async {
    final s = sockets(
      host: 'localhost',
      pins: [unrelatedPin(1), unrelatedPin(2)],
    );
    await expectLater(transport(s).open(request()), mismatch());
  });

  test(
    '[SEC-041] a mismatch is not retried on an unpinned connection',
    () async {
      final s = sockets(
        host: 'localhost',
        pins: [unrelatedPin(1), unrelatedPin(2)],
      );
      try {
        await transport(s).open(request());
        fail('the stream opened');
      } on DataFailure catch (f) {
        expect(streamRetryable(f), isFalse);
      }
    },
  );

  test('[SEC-041] an unpinned host gets the default client', () {
    final s = sockets(host: 'plux.example', pins: [cert.pin, unrelatedPin(1)]);
    expect(s.clientFor(Uri.parse('wss://other.example/live')), isNull);
    expect(
      WebSocketClients().clientFor(Uri.parse('ws://plux.example')),
      isNull,
    );
  });

  test('[SEC-042] a pinned customer domain gets its pins, updated at run '
      'time', () async {
    final s = sockets();
    final t = transport(s);
    // Unpinned, the default client does not trust the test certificate.
    await expectLater(t.open(request()), throwsA(isA<DataFailure>()));

    s.pinDomains({
      'LOCALHOST': [unrelatedPin(1), cert.pin],
    });
    await echoes(t);

    s.pinDomains({
      'localhost': [unrelatedPin(1), unrelatedPin(2)],
    });
    await expectLater(t.open(request()), mismatch());

    s.pinDomains(const {});
    expect(s.clientFor(Uri.parse('wss://localhost/live')), isNull);
  });

  test('[SEC-042] invalid pins of a domain fail the stream closed', () async {
    final s = sockets()
      ..pinDomains({
        'localhost': [cert.pin],
      });
    expect(
      () => s.clientFor(Uri.parse('wss://localhost/live')),
      throwsA(isA<PluxException>()),
    );
    await expectLater(
      transport(s).open(request()),
      throwsA(isA<DataFailure>()),
    );
  });

  test('[SEC-041] a host-supplied client is used as it is', () async {
    var made = 0;
    final s = sockets(
      host: 'localhost',
      pins: [unrelatedPin(1), unrelatedPin(2)],
    );
    final client = http.Client();
    addTearDown(client.close);
    final t = SocketStreamTransport(
      client,
      pinned: s,
      webSocketClient: () {
        made++;
        return HttpClient(context: cert.clientContext());
      },
    );
    await echoes(t);
    expect(made, 1);
  });
}
