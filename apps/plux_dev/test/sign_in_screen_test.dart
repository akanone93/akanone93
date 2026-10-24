// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_host_kit/plux_host_kit.dart';

/// A server that approves after [pending] polls, or answers [pollStatus].
MockClient fakeServer({
  int pending = 0,
  String pollStatus = 'approved',
  int startStatus = 200,
  List<String>? calls,
  List<Map<String, String>> apps = const [
    {'id': 'app_1', 'name': 'Shop'},
  ],
  int appsStatus = 200,
}) {
  var polls = 0;
  return MockClient((request) async {
    final name = request.url.pathSegments.last;
    calls?.add(name);
    switch (name) {
      case 'StartDeviceAuthorization':
        if (startStatus != 200) {
          return http.Response(
            jsonEncode({'code': 'unavailable', 'message': 'down'}),
            startStatus,
          );
        }
        return http.Response(
          jsonEncode({
            'deviceCode': 'dc',
            'userCode': 'ABCD-1234',
            'verificationUri': 'https://plux.example.org/device',
            'verificationUriComplete': 'https://plux.example.org/device?c=1',
            'intervalSeconds': 1,
            'expiresAt': '2099-01-01T00:00:00Z',
          }),
          200,
        );
      case 'PollDeviceAuthorization':
        polls++;
        final status = polls <= pending ? 'pending' : pollStatus;
        return http.Response(
          jsonEncode({'status': status, 'secret': 'tok'}),
          200,
        );
      case 'ListApps':
        // A real server takes a frame to answer; the screens show it.
        await Future<void>.delayed(const Duration(milliseconds: 50));
        return http.Response(
          jsonEncode({'apps': apps, 'code': 'unauthenticated'}),
          appsStatus,
        );
    }
    return http.Response('{}', 404);
  });
}

/// The Dev app's sign-in [DEV-001].
void main() {
  Future<void> pump(
    WidgetTester tester,
    MockClient client,
    List<ViewerSession> signedIn,
  ) => tester.pumpWidget(
    MaterialApp(
      home: DevSignInScreen(
        onSignedIn: signedIn.add,
        apiFactory: (server, {token}) => ViewerApi(
          server,
          token: token,
          client: client,
          clientName: devClientName,
          scopes: devScopes,
        ),
      ),
    ),
  );

  Future<void> start(WidgetTester tester, String server) async {
    await tester.enterText(find.byKey(const Key('server-field')), server);
    await tester.tap(find.byKey(const Key('sign-in')));
    await tester.pump();
    await tester.pump();
  }

  testWidgets('[DEV-001] shows the code, then signs in once approved', (
    tester,
  ) async {
    final signedIn = <ViewerSession>[];
    await pump(tester, fakeServer(pending: 1), signedIn);
    await start(tester, 'https://plux.example.org');
    expect(find.text('ABCD-1234'), findsOneWidget);
    expect(find.text('https://plux.example.org/device'), findsOneWidget);
    expect(signedIn, isEmpty);
    await tester.pump(const Duration(seconds: 1));
    await tester.pump(const Duration(seconds: 1));
    expect(signedIn, hasLength(1));
    expect(signedIn.single.token, 'tok');
    expect(signedIn.single.server.host, 'plux.example.org');
  });

  testWidgets('[DEV-001] copies the code to the clipboard', (tester) async {
    String? copied;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          copied = (call.arguments as Map)['text'] as String?;
        }
        return null;
      },
    );
    addTearDown(
      () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      ),
    );
    await pump(tester, fakeServer(pending: 5), []);
    await start(tester, 'https://plux.example.org');
    await tester.tap(find.byTooltip('Copy the code'));
    await tester.pump();
    expect(copied, 'ABCD-1234');
    await tester.tap(find.text('Cancel'));
    await tester.pump();
    await tester.pump(const Duration(seconds: 2));
  });

  testWidgets('[DEV-001] a server that is not a URL is reported', (
    tester,
  ) async {
    final calls = <String>[];
    await pump(tester, fakeServer(calls: calls), []);
    await start(tester, '');
    expect(find.byType(Semantics), findsWidgets);
    expect(find.byKey(const Key('sign-in')), findsOneWidget);
    expect(calls, isEmpty);
  });

  testWidgets('[DEV-001] a denied sign-in is reported and can be retried', (
    tester,
  ) async {
    final signedIn = <ViewerSession>[];
    await pump(tester, fakeServer(pollStatus: 'denied'), signedIn);
    await start(tester, 'https://plux.example.org');
    await tester.pump(const Duration(seconds: 1));
    await tester.pump();
    expect(find.text('The sign-in was denied.'), findsOneWidget);
    expect(find.byKey(const Key('sign-in')), findsOneWidget);
    expect(signedIn, isEmpty);
  });

  testWidgets('[DEV-001] an unreachable server is reported', (tester) async {
    await pump(tester, fakeServer(startStatus: 503), []);
    await start(tester, 'https://plux.example.org');
    expect(find.text('down'), findsOneWidget);
  });

  testWidgets('[DEV-001] cancelling stops the sign-in', (tester) async {
    final signedIn = <ViewerSession>[];
    await pump(tester, fakeServer(pending: 99), signedIn);
    await start(tester, 'https://plux.example.org');
    await tester.tap(find.text('Cancel'));
    await tester.pump();
    expect(find.byKey(const Key('sign-in')), findsOneWidget);
    await tester.pump(const Duration(seconds: 2));
    expect(signedIn, isEmpty);
    expect(find.text('ABCD-1234'), findsNothing);
  });

  testWidgets('[DEV-001] the default client names the Dev app', (tester) async {
    final api = defaultDevApi(Uri.parse('https://plux.example.org/'));
    expect(api.clientName, devClientName);
    expect(api.scopes, devScopes);
    expect(api.token, isNull);
    final authed = defaultDevApi(Uri.parse('https://p.example/'), token: 't');
    expect(authed.token, 't');
    api.close();
    authed.close();
  });
}
