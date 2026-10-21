// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_viewer/viewer.dart';

/// The auditor's sign-in [SEC-142].
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

  group('the sign-in screen [SEC-142]', () {
    testWidgets('shows the code to approve and signs in once approved', (
      tester,
    ) async {
      final controller = ViewerController();
      // A server that approves on the second poll.
      final api = serverAnswering(['pending', 'approved']);
      await tester.pumpWidget(
        MaterialApp(
          home: SignInScreen(
            controller: controller,
            server: server,
            apiFactory: (_) => api,
          ),
        ),
      );
      expect(find.text('Sign in with a device code'), findsOneWidget);
      await tester.tap(find.text('Sign in with a device code'));
      await tester.pump();
      await tester.pump();
      expect(find.text('ABCD-EFGH'), findsOneWidget);
      expect(find.text('https://plux.example.org/device'), findsOneWidget);
      expect(find.text('Waiting for approval'), findsOneWidget);
      // The interval is the server's two seconds.
      await tester.pump(const Duration(seconds: 2));
      await tester.pump(const Duration(seconds: 2));
      await tester.pump();
      expect(controller.session?.server, server);
      expect(controller.session?.token, 'plux_secret');
    });

    testWidgets('refuses a server that is not https before asking anything', (
      tester,
    ) async {
      final controller = ViewerController();
      var created = 0;
      await tester.pumpWidget(
        MaterialApp(
          home: SignInScreen(
            controller: controller,
            apiFactory: (s) {
              created++;
              return ViewerApi(s);
            },
          ),
        ),
      );
      await tester.enterText(find.byType(TextField), 'http://plux.example.org');
      await tester.tap(find.text('Sign in with a device code'));
      await tester.pump();
      expect(find.text('the server must be an https URL'), findsOneWidget);
      expect(created, 0);
      expect(controller.session, isNull);
    });

    testWidgets('signs in with a pasted token', (tester) async {
      final controller = ViewerController();
      await tester.pumpWidget(
        MaterialApp(
          home: SignInScreen(controller: controller, server: server),
        ),
      );
      await tester.tap(find.text('Paste an access token instead'));
      await tester.pump();
      await tester.tap(find.text('Sign in with the token'));
      await tester.pump();
      expect(find.text('Paste an access token.'), findsOneWidget);
      final field = tester.widget<TextField>(
        find.widgetWithText(TextField, 'Access token'),
      );
      expect(field.obscureText, isTrue);
      await tester.enterText(
        find.widgetWithText(TextField, 'Access token'),
        ' tok ',
      );
      await tester.tap(find.text('Sign in with the token'));
      await tester.pump();
      expect(controller.session?.token, 'tok');
    });

    testWidgets('shows why it asks again', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: SignInScreen(
            controller: ViewerController(),
            notice: 'This link opens a reconstruction on plux.example.org.',
          ),
        ),
      );
      expect(
        find.text('This link opens a reconstruction on plux.example.org.'),
        findsOneWidget,
      );
    });
  });

  test('a session never prints its token [SEC-142]', () {
    final s = ViewerSession(server, 'plux_secret');
    expect('$s', isNot(contains('plux_secret')));
  });
}
