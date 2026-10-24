// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/testing.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_host_kit/plux_host_kit.dart' hide RuntimeStarter;

import 'sign_in_screen_test.dart' show fakeServer;

const String _keys =
    '[{"keyId":"d1","algorithm":"ed25519","role":"root","publicKey":'
    '"0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}]';

/// The Dev app's flow [DEV-001].
void main() {
  Widget app({
    required MockClient client,
    required RuntimeStarter start,
    bool roots = true,
  }) => DevApp(
    rootKeys: roots ? parseDevelopmentRootKeys(_keys) : const [],
    startRuntime: start,
    scope: (child) => child,
    apiFactory: (server, {token}) => ViewerApi(
      server,
      token: token,
      client: client,
      clientName: devClientName,
      scopes: devScopes,
    ),
  );

  Future<void> signIn(WidgetTester tester) async {
    await tester.enterText(
      find.byKey(const Key('server-field')),
      'https://plux.example.org',
    );
    await tester.tap(find.byKey(const Key('sign-in')));
    await tester.pump();
    await tester.pump();
    await tester.pump(const Duration(seconds: 1));
    await tester.pump();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    await tester.pump();
  }

  testWidgets('[DEV-001] without development roots it pairs with nothing', (
    tester,
  ) async {
    await tester.pumpWidget(
      app(client: fakeServer(), start: (_) async => null, roots: false),
    );
    expect(find.textContaining('No development roots configured'), findsOne);
    expect(find.byKey(const Key('sign-in')), findsNothing);
  });

  testWidgets('[DEV-001] sign in, choose the app, start the runtime', (
    tester,
  ) async {
    PluxConfig? started;
    await tester.pumpWidget(
      app(
        client: fakeServer(),
        start: (config) async {
          started = config;
          return null;
        },
      ),
    );
    await signIn(tester);
    expect(find.text('Choose the app to preview'), findsOneWidget);
    expect(find.text('Shop'), findsOneWidget);
    expect(find.text('app_1'), findsOneWidget);

    await tester.tap(find.text('Shop'));
    await tester.pumpAndSettle();
    expect(started, isNotNull);
    expect(started!.appId, 'app_1');
    expect(started!.environment, devEnvironment);
    expect(find.text('Not paired'), findsOneWidget);

    await tester.pumpWidget(const SizedBox());
    await tester.pump();
  });

  testWidgets('[DEV-001] a runtime that does not start is reported', (
    tester,
  ) async {
    await tester.pumpWidget(
      app(
        client: fakeServer(),
        start: (_) async => throw StateError('no keys'),
      ),
    );
    await signIn(tester);
    await tester.tap(find.text('Shop'));
    await tester.pumpAndSettle();
    expect(find.textContaining('The app could not start:'), findsOneWidget);
    expect(find.text('Choose the app to preview'), findsOneWidget);
  });

  testWidgets('[DEV-001] an organisation without apps says so', (tester) async {
    await tester.pumpWidget(
      app(
        client: fakeServer(apps: const []),
        start: (_) async => null,
      ),
    );
    await signIn(tester);
    expect(find.text('This organisation has no apps yet.'), findsOneWidget);
  });

  testWidgets('[DEV-001] a refused token returns to the sign-in', (
    tester,
  ) async {
    await tester.pumpWidget(
      app(client: fakeServer(appsStatus: 401), start: (_) async => null),
    );
    await signIn(tester);
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('sign-in')), findsOneWidget);
  });
}
