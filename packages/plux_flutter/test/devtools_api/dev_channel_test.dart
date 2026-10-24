// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/src/data/stream_transport.dart';
import 'package:plux_flutter/src/devtools_api/dev_channel.dart';
import 'package:plux_flutter/src/devtools_api/dev_engine.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/dpop.dart';
import 'package:plux_flutter/src/sync/api_client.dart';

final class _Keys implements DeviceKeys {
  @override
  Future<Uint8List> sign(String alias, Uint8List data) async =>
      Uint8List.fromList([for (var i = 0; i < 64; i++) i]);

  @override
  Future<DeviceKey> create(
    String alias,
    KeyPurpose purpose, {
    Uint8List? challenge,
    bool strongBox = true,
  }) => throw UnimplementedError();

  @override
  Future<DeviceKey?> load(String alias, KeyPurpose purpose) =>
      throw UnimplementedError();

  @override
  Future<Uint8List> agree(String alias, EcPublicKey peer) =>
      throw UnimplementedError();

  @override
  Future<void> delete(String alias) => throw UnimplementedError();
}

final class _Connection implements StreamConnection {
  final StreamController<StreamFrame> _frames = StreamController();
  final List<Map<String, Object?>> sent = [];
  var closed = false;

  @override
  Stream<StreamFrame> get frames => _frames.stream;

  @override
  void send(Object? json) {
    // The wire carries JSON; a round trip proves the frame encodes.
    sent.add(jsonDecode(jsonEncode(json)) as Map<String, Object?>);
  }

  void receive(Map<String, Object?> frame) =>
      _frames.add(StreamFrame.message(frame));

  Future<void> drop() => _frames.close();

  @override
  Future<void> close() async {
    closed = true;
    if (!_frames.isClosed) await _frames.close();
  }

  List<Map<String, Object?>> get events => [
    for (final f in sent)
      if (f['event'] case final Map<String, Object?> e) e,
  ];
}

final class _Transport implements StreamTransport {
  final List<StreamRequest> requests = [];
  final List<Completer<StreamConnection>> _pending = [];
  final List<Completer<void>> _waiters = [];

  @override
  Future<StreamConnection> open(StreamRequest request) {
    requests.add(request);
    final c = Completer<StreamConnection>();
    _pending.add(c);
    for (final w in _waiters.toList()) {
      w.complete();
    }
    _waiters.clear();
    return c.future;
  }

  Future<_Connection> accept() async {
    while (_pending.isEmpty) {
      final w = Completer<void>();
      _waiters.add(w);
      await w.future;
    }
    final conn = _Connection();
    _pending.removeAt(0).complete(conn);
    await pumpEventQueue();
    return conn;
  }

  Future<void> refuse() async {
    while (_pending.isEmpty) {
      final w = Completer<void>();
      _waiters.add(w);
      await w.future;
    }
    _pending.removeAt(0).completeError(StateError('refused'));
    await pumpEventQueue();
  }
}

final class _Applier implements DevBundleApplier {
  final List<DevBundle> applied = [];
  var discarded = 0;
  DevApplyResult result = const DevApplyResult(
    pagesApplied: ['/home'],
    scopesReset: ['cart'],
  );

  @override
  Future<DevApplyResult> apply(DevBundle bundle) async {
    applied.add(bundle);
    return result;
  }

  @override
  Future<void> discard() async => discarded++;
}

final class _Commands implements DevCommandHandler {
  final List<Map<String, Object?>> handled = [];

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    handled.add(command);
    return {'status': 'DEV_COMMAND_STATUS_OK'};
  }
}

final _x = Uint8List.fromList([for (var i = 0; i < 32; i++) i]);
final _y = Uint8List.fromList([for (var i = 32; i < 64; i++) i]);

class _Fixture {
  _Fixture({
    int eventBytes = 65536,
    int bufferEvents = 5000,
    int bufferBytes = 8388608,
  }) {
    final client = MockClient((request) async {
      pairRequests.add(request);
      return http.Response(
        jsonEncode({
          'sessionId': 'ses-1',
          'deviceId': 'dev-1',
          'socketTicket': 'ticket-${pairRequests.length}',
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    });
    api = PluxApiClient(client, Uri.parse('https://dev.example'));
    token = DeviceToken(
      'access',
      DateTime.utc(2030),
      DpopProofs(
        keys: _Keys(),
        alias: 'k',
        publicKey: EcPublicKey(_x, _y),
        now: () => DateTime.utc(2026, 7, 1),
      ),
    );
    config = (eventBytes, bufferEvents, bufferBytes);
  }

  final List<http.Request> pairRequests = [];
  final transport = _Transport();
  final applier = _Applier();
  final commands = _Commands();
  final delays = <Duration>[];
  late final PluxApiClient api;
  late final DeviceToken token;
  late final (int, int, int) config;
  var now = DateTime.utc(2026, 7, 1, 12);
  DevEngine? engine;

  Future<DevChannel> pair([
    String link =
        'plux-dev://pair?server=https%3A%2F%2Fdev.example&token=pair-me',
  ]) => pairWith(DevPairingLink.parse(link));

  Future<DevChannel> pairWith(DevPairingCredential pairing) => DevChannel.pair(
    pairing,
    api: api,
    token: token,
    applier: applier,
    commands: commands,
    deviceName: 'Pixel',
    platform: 'android',
    runtimeVersion: '0.6.0',
    eventBytes: config.$1,
    streamBufferEvents: config.$2,
    streamBufferBytes: config.$3,
    engine: (cfg, host) async {
      final e = DevEngine(
        config: cfg,
        host: host,
        transport: transport,
        clock: () => now,
        delay: (d) async => delays.add(d),
        random: () => 0,
      );
      unawaited(e.start());
      engine = e;
      return e;
    },
  );
}

Map<String, Object?> _bundleFrame(int sequence) => {
  'sequence': '$sequence',
  'bundle': {
    'bundleSequence': '7',
    'manifest': base64.encode([1, 2, 3]),
    'sections': {
      'page.home': base64.encode([4, 5]),
    },
  },
};

void main() {
  group('DEV-002 pairing link', () {
    test('DEV-002 parses an https server and a token', () {
      final link = DevPairingLink.parse(
        'plux-dev://pair?server=https%3A%2F%2Fdev.example%2Fplux&token=abc',
      );
      expect(link.server, Uri.parse('https://dev.example/plux'));
      expect(link.token, 'abc');
      expect(link.socketUrl, Uri.parse('wss://dev.example/plux/dev/socket'));
    });

    test('DEV-002 rejects other schemes, missing parts and http', () {
      for (final bad in [
        'https://pair?server=https%3A%2F%2Fa.example&token=t',
        'plux://pair?server=https%3A%2F%2Fa.example&token=t',
        'plux-dev://other?server=https%3A%2F%2Fa.example&token=t',
        'plux-dev://pair?server=https%3A%2F%2Fa.example',
        'plux-dev://pair?server=https%3A%2F%2Fa.example&token=',
        'plux-dev://pair?token=t',
        'plux-dev://pair?server=http%3A%2F%2Fa.example&token=t',
        'plux-dev://pair?server=ftp%3A%2F%2Fa.example&token=t',
      ]) {
        expect(
          () => DevPairingLink.parse(bad, allowInsecureLoopback: true),
          throwsFormatException,
          reason: bad,
        );
      }
    });

    test('DEV-002 accepts http for loopback only when allowed', () {
      const text =
          'plux-dev://pair?server=http%3A%2F%2F10.0.2.2%3A8080&token=t';
      final link = DevPairingLink.parse(text, allowInsecureLoopback: true);
      expect(link.socketUrl, Uri.parse('ws://10.0.2.2:8080/dev/socket'));
      expect(
        () => DevPairingLink.parse(text, allowInsecureLoopback: false),
        throwsFormatException,
      );
    });
  });

  group('DEV-002 pairing code', () {
    test('DEV-002 reads a code whatever its case, dashes and look-alikes', () {
      for (final typed in ['7KQ2-M9XD', '7kq2m9xd', ' 7kq2 - m9xd ']) {
        expect(DevPairingCode.parse(typed).value, '7KQ2M9XD', reason: typed);
      }
      expect(DevPairingCode.parse('o1il-0000').value, '01110000');
      expect(
        DevPairingCode.parse('7KQ2-M9XD'),
        DevPairingCode.parse('7kq2m9xd'),
      );
      expect(
        DevPairingCode.parse('7KQ2-M9XD').toString(),
        isNot(contains('7KQ2')),
      );
    });

    test('DEV-002 refuses what is not a code, links included', () {
      for (final bad in [
        '',
        '7KQ2-M9X',
        '7KQ2-M9XDA',
        '7KQ2-M9XU',
        '7KQ2_M9XD',
        'plux-dev://pair?server=https%3A%2F%2Fa.example&token=t',
      ]) {
        expect(DevPairingCode.tryParse(bad), isNull, reason: bad);
        expect(() => DevPairingCode.parse(bad), throwsFormatException);
      }
    });
  });

  group('DevChannel', () {
    test(
      'DEV-002 pair with a code sends pairingCode to the API client\'s server',
      () async {
        final f = _Fixture();
        final channel = await f.pairWith(DevPairingCode.parse('7kq2-m9xd'));
        final req = f.pairRequests.single;
        expect(
          req.url.toString(),
          'https://dev.example/plux.v1.DevSessionService/Pair',
        );
        expect(jsonDecode(req.body), {
          'pairingCode': '7KQ2M9XD',
          'name': 'Pixel',
          'platform': 'android',
          'runtimeVersion': '0.6.0',
        });
        await f.transport.accept();
        expect(
          f.transport.requests.single.url,
          Uri.parse('wss://dev.example/dev/socket'),
        );
        await channel.close();
      },
    );

    test(
      'DEV-002 pair calls Pair with the token, then opens the socket',
      () async {
        final f = _Fixture();
        final channel = await f.pair();
        expect(f.pairRequests, hasLength(1));
        final req = f.pairRequests.single;
        expect(
          req.url.toString(),
          'https://dev.example/plux.v1.DevSessionService/Pair',
        );
        expect(req.headers['Authorization'], 'DPoP access');
        expect(req.headers['DPoP'], isNotEmpty);
        expect(jsonDecode(req.body), {
          'pairingToken': 'pair-me',
          'name': 'Pixel',
          'platform': 'android',
          'runtimeVersion': '0.6.0',
        });
        expect(channel.sessionId, 'ses-1');
        expect(channel.deviceId, 'dev-1');
        await f.transport.accept();
        final open = f.transport.requests.single;
        expect(open.kind, StreamKind.webSocket);
        expect(open.url, Uri.parse('wss://dev.example/dev/socket'));
        expect(open.headers['Plux-Dev-Ticket'], 'ticket-1');
        final proof = open.headers['DPoP']!.split('.');
        expect(proof, hasLength(3));
        final payload = jsonDecode(
          utf8.decode(base64Url.decode(base64Url.normalize(proof[1]))),
        ) as Map<String, Object?>;
        expect(payload['htm'], 'GET');
        expect(payload['htu'], 'wss://dev.example/dev/socket');
        await channel.close();
      },
    );

    test(
      'DEV-002 pair refuses a server other than the API client\'s',
      () async {
        final f = _Fixture();
        await expectLater(
          f.pair('plux-dev://pair?server=https%3A%2F%2Fother.example&token=t'),
          throwsFormatException,
        );
        expect(f.pairRequests, isEmpty);
      },
    );

    test('DEV-021 the hello is the first frame on connect', () async {
      final f = _Fixture();
      final channel = await f.pair();
      final conn = await f.transport.accept();
      expect(conn.sent.first, {
        'hello': {'lastServerSequence': '0', 'lastAcknowledgedEvent': '0'},
      });
      await channel.close();
    });

    test(
      'DEV-010 a bundle frame is applied and a hotApply event follows',
      () async {
        final f = _Fixture();
        final channel = await f.pair();
        final conn = await f.transport.accept();
        conn.receive(_bundleFrame(1));
        await pumpEventQueue();
        final bundle = f.applier.applied.single;
        expect(bundle.sequence, 7);
        expect(bundle.manifest, [1, 2, 3]);
        expect(bundle.sections['page.home'], [4, 5]);
        final event = conn.events.single;
        expect(event['sequence'], '1');
        expect(
          event['timeUnixMicros'],
          '${DateTime.utc(2026, 7, 1, 12).microsecondsSinceEpoch}',
        );
        final hot = event['hotApply']! as Map<String, Object?>;
        expect(hot['pagesApplied'], ['/home']);
        expect(hot['scopesReset'], ['cart']);
        expect(hot['bundleSha256'], hasLength(64));
        expect(hot['applyMicros'], isA<String>());
        await channel.close();
      },
    );

    test('DEV-010 a refused bundle reports its error code', () async {
      final f = _Fixture();
      f.applier.result = DevApplyResult(
        error: PluxException(PluxErrorCode.devPatchRejected, 'bad signature'),
      );
      final channel = await f.pair();
      final conn = await f.transport.accept();
      conn.receive(_bundleFrame(1));
      await pumpEventQueue();
      expect(conn.events.map((e) => e.keys.last), ['hotApply', 'error']);
      final error = (conn.events.last['error']! as Map)['error'] as Map;
      expect(error['code'], 'PLX-9207');
      await channel.close();
    });

    test(
      'DEV-010 a command frame is handled and a commandResult follows',
      () async {
        final f = _Fixture();
        final channel = await f.pair();
        final conn = await f.transport.accept();
        conn.receive({
          'sequence': '1',
          'command': {
            'commandId': 'c-9',
            'command': {
              'openScreen': {'route': '/home'},
            },
          },
        });
        await pumpEventQueue();
        expect(f.commands.handled.single, {
          'openScreen': {'route': '/home'},
        });
        final result = conn.events.single['commandResult']! as Map;
        expect(result['commandId'], 'c-9');
        expect(result['status'], 'DEV_COMMAND_STATUS_OK');
        await channel.close();
      },
    );

    test('DEV-021 the buffer keeps its bound and reports the drops', () async {
      final f = _Fixture(bufferEvents: 3);
      final channel = await f.pair();
      // Not connected yet: nothing is sent, everything is buffered.
      for (var i = 0; i < 5; i++) {
        channel.emit({
          'log': {'message': 'line $i'},
        });
      }
      await pumpEventQueue();
      expect(f.engine!.buffered, lessThanOrEqualTo(3));
      final conn = await f.transport.accept();
      final events = conn.events;
      expect(events.length, lessThanOrEqualTo(3));
      final drops = [
        for (final e in events)
          if (e['drop'] case final Map<Object?, Object?> d) d,
      ];
      expect(drops, isNotEmpty);
      for (final d in drops) {
        expect(d['reason'], 'DEV_DROP_REASON_BUFFER_FULL');
        expect(int.parse(d['dropped']! as String), greaterThan(0));
      }
      // The newest log line survived; the oldest did not.
      final lines = [
        for (final e in events)
          if (e['log'] case final Map<Object?, Object?> l) l['message'],
      ];
      expect(lines, contains('line 4'));
      expect(lines, isNot(contains('line 0')));
      await channel.close();
    });

    test(
      'DEV-021 an event over devtools.eventBytes is dropped and counted',
      () async {
        final f = _Fixture(eventBytes: 300);
        final channel = await f.pair();
        final conn = await f.transport.accept();
        channel.emit({
          'log': {'message': 'x' * 1000},
        });
        channel.emit({
          'log': {'message': 'small'},
        });
        await pumpEventQueue();
        final events = conn.events;
        expect(events.map((e) => e.keys.last), ['drop', 'log']);
        final drop = events.first['drop']! as Map;
        expect(drop['dropped'], '1');
        expect(drop['reason'], 'DEV_DROP_REASON_EVENT_TOO_LARGE');
        await channel.close();
      },
    );

    test(
      'DEV-021 events carry the device sequence from 1 and the trace ID',
      () async {
        final f = _Fixture();
        final channel = await f.pair();
        final conn = await f.transport.accept();
        channel.emit({
          'log': {'message': 'a'},
        }, traceId: 't-1');
        channel.emit({
          'log': {'message': 'b'},
        });
        await pumpEventQueue();
        expect(conn.events.map((e) => e['sequence']), ['1', '2']);
        expect(conn.events.first['traceId'], 't-1');
        expect(conn.events.last.containsKey('traceId'), isFalse);
        await channel.close();
      },
    );

    test(
      'DEV-021 a reconnect resumes and resends unacknowledged events',
      () async {
        final f = _Fixture();
        final channel = await f.pair();
        final first = await f.transport.accept();
        for (final m in ['a', 'b', 'c']) {
          channel.emit({
            'log': {'message': m},
          });
        }
        first.receive({
          'sequence': '1',
          'ack': {'upTo': '1'},
        });
        first.receive(_bundleFrame(2));
        await pumpEventQueue();
        expect(f.engine!.lastAcknowledgedEvent, 1);
        await first.drop();
        await pumpEventQueue();
        expect(f.delays, [const Duration(seconds: 1)]);

        final second = await f.transport.accept();
        expect(
          f.transport.requests.last.headers['Plux-Dev-Ticket'],
          'ticket-2',
        );
        expect(second.sent.first, {
          'hello': {'lastServerSequence': '2', 'lastAcknowledgedEvent': '1'},
        });
        // b, c and the hotApply of the bundle; a was acknowledged.
        expect(second.events.map((e) => e['sequence']), ['2', '3', '4']);
        expect((second.events.first['log']! as Map)['message'], 'b');
        await channel.close();
      },
    );

    test(
      'DEV-021 a frame the server replays after a resume is not applied twice',
      () async {
        final f = _Fixture();
        final channel = await f.pair();
        final first = await f.transport.accept();
        first.receive(_bundleFrame(1));
        await pumpEventQueue();
        await first.drop();
        await pumpEventQueue();
        final second = await f.transport.accept();
        second.receive(_bundleFrame(1));
        second.receive(_bundleFrame(2));
        await pumpEventQueue();
        expect(f.applier.applied, hasLength(2));
        await channel.close();
      },
    );

    test('DEV-021 the reconnect wait doubles up to its cap', () async {
      final f = _Fixture();
      final channel = await f.pair();
      final first = await f.transport.accept();
      await first.drop();
      await pumpEventQueue();
      await f.transport.refuse();
      await f.transport.refuse();
      await f.transport.refuse();
      expect(f.delays, [
        const Duration(seconds: 1),
        const Duration(seconds: 2),
        const Duration(seconds: 4),
        const Duration(seconds: 8),
      ]);
      await channel.close();
    });

    test('DEV-021 an ended session discards the overlay and stops', () async {
      final f = _Fixture();
      final channel = await f.pair();
      final conn = await f.transport.accept();
      conn.receive({
        'sequence': '1',
        'ended': {'code': 'PLX-9200'},
      });
      await pumpEventQueue();
      expect(f.applier.discarded, 1);
      expect(conn.closed, isTrue);
      expect(f.transport.requests, hasLength(1));
      expect(f.delays, isEmpty);
      channel.emit({
        'log': {'message': 'after'},
      });
      await channel.close();
    });
  });
}
