// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_host_kit/plux_host_kit.dart';

/// The auditor's sign-in by device grant [SEC-142].
void main() {
  final server = Uri.parse('https://plux.example.org/');

  /// A server whose polls answer with [statuses] in turn.
  ViewerApi serverAnswering(
    List<String> statuses, {
    List<String>? seen,
    String expires = '2099-01-01T00:00:00Z',
  }) {
    var polls = 0;
    return ViewerApi(
      server,
      client: MockClient((request) async {
        final body = jsonDecode(request.body) as Map<String, Object?>;
        if (request.url.path.endsWith('StartDeviceAuthorization')) {
          return http.Response(
            jsonEncode({
              'deviceCode': 'dc',
              'userCode': 'ABCD-EFGH',
              'verificationUri': 'https://plux.example.org/device',
              'verificationUriComplete':
                  'https://plux.example.org/device?code=ABCD-EFGH',
              'intervalSeconds': 2,
              'expiresAt': expires,
            }),
            200,
          );
        }
        seen?.add(body['deviceCode']! as String);
        final status = statuses[polls < statuses.length ? polls++ : polls - 1];
        return http.Response(
          jsonEncode({
            'status': status,
            if (status == 'approved') 'secret': 'plux_secret',
          }),
          200,
        );
      }),
    );
  }

  group('the device grant [SEC-142]', () {
    late List<Duration> waits;
    late DateTime clock;

    DeviceSignIn flow(ViewerApi api) => DeviceSignIn(
      api,
      wait: (d) async {
        waits.add(d);
        clock = clock.add(d);
      },
      now: () => clock,
    );

    setUp(() {
      waits = [];
      clock = DateTime.utc(2026, 10, 21, 10);
    });

    test('polls at the server\'s interval until it is approved', () async {
      final seen = <String>[];
      final f = flow(
        serverAnswering(['pending', 'pending', 'approved'], seen: seen),
      );
      final grant = await f.start();
      expect(grant.userCode, 'ABCD-EFGH');
      expect(await f.complete(grant), 'plux_secret');
      expect(waits, const [
        Duration(seconds: 2),
        Duration(seconds: 2),
        Duration(seconds: 2),
      ]);
      expect(seen, ['dc', 'dc', 'dc']);
    });

    test('slows down when the server says so', () async {
      final f = flow(serverAnswering(['slow_down', 'slow_down', 'approved']));
      expect(await f.complete(await f.start()), 'plux_secret');
      expect(waits, const [
        Duration(seconds: 2),
        Duration(seconds: 7),
        Duration(seconds: 12),
      ]);
    });

    test('stops when the approving person denies it', () async {
      final f = flow(serverAnswering(['pending', 'denied']));
      await expectLater(
        f.complete(await f.start()),
        throwsA(
          isA<SignInException>().having(
            (e) => e.failure,
            'failure',
            SignInFailure.denied,
          ),
        ),
      );
    });

    test('stops when the code lapses, without polling past it', () async {
      final seen = <String>[];
      final f = flow(
        serverAnswering(
          ['pending'],
          seen: seen,
          expires: '2026-10-21T10:15:00Z',
        ),
      );
      final grant = await f.start();
      clock = DateTime.utc(2026, 10, 21, 10, 14, 59);
      await expectLater(
        f.complete(grant),
        throwsA(
          isA<SignInException>().having(
            (e) => e.failure,
            'failure',
            SignInFailure.expired,
          ),
        ),
      );
      expect(seen, isEmpty);
    });

    test('stops when the auditor cancels', () async {
      final seen = <String>[];
      final f = flow(serverAnswering(['pending'], seen: seen));
      final grant = await f.start();
      final done = f.complete(grant);
      f.cancel();
      await expectLater(
        done,
        throwsA(
          isA<SignInException>().having(
            (e) => e.failure,
            'failure',
            SignInFailure.cancelled,
          ),
        ),
      );
      expect(seen, isEmpty);
    });
  });

  test('a session never prints its token [SEC-142]', () {
    final s = ViewerSession(server, 'plux_secret');
    expect('$s', isNot(contains('plux_secret')));
  });
}
