// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_devtools/plux_devtools.dart';

const String _keys =
    '[{"keyId":"d1","algorithm":"ed25519","role":"root","publicKey":'
    '"0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}]';

const String _link =
    'plux-dev://pair?server=https%3A%2F%2Fplux.example.org'
    '&token=abc';

/// The home screen of the Dev app [DEV-001] [DEV-040].
void main() {
  late List<String> pairedWith;

  PluxDevSession sessionWith(Future<PluxDevJoined> Function()? join) =>
      PluxDevSession(
        devRootKeys: parseDevelopmentRootKeys(_keys),
        pairer: (link, {required devRootKeys, required extensions}) {
          pairedWith.add(link);
          return join!();
        },
      );

  Future<void> pump(
    WidgetTester tester,
    PluxDevSession session, {
    bool hasRoots = true,
    Future<String?> Function(BuildContext)? scan,
  }) => tester.pumpWidget(
    MaterialApp(
      home: DevHomeScreen(
        session: session,
        hasRoots: hasRoots,
        scan: scan ?? (_) async => null,
        openPage: (_, _) async {},
      ),
    ),
  );

  setUp(() => pairedWith = []);

  testWidgets('idle: shows it is not paired and offers both ways to pair '
      '[DEV-001]', (tester) async {
    await pump(tester, sessionWith(null));
    expect(find.text('Not paired'), findsOneWidget);
    expect(find.text('Scan QR'), findsOneWidget);
    expect(find.text('Paste link'), findsOneWidget);
    expect(
      tester.widget<FilledButton>(find.byKey(const Key('scan-qr'))).onPressed,
      isNotNull,
    );
  });

  testWidgets('paired: a pasted link pairs and the status names the session '
      '[DEV-001]', (tester) async {
    final session = sessionWith(
      () async => (sessionId: 'ds_1', deviceId: 'dev_9', close: () async {}),
    );
    await pump(tester, session);
    await tester.tap(find.byKey(const Key('paste-link')));
    await tester.pump();
    await tester.enterText(find.byKey(const Key('link-field')), _link);
    await tester.tap(find.byKey(const Key('pair-link')));
    await tester.pumpAndSettle();
    expect(pairedWith, [_link]);
    expect(find.text('Paired with session ds_1 as device dev_9'), findsOne);
    await tester.tap(find.byKey(const Key('leave-session')));
    await tester.pumpAndSettle();
    expect(find.text('Not paired'), findsOneWidget);
  });

  testWidgets('paired: a scanned code pairs [DEV-001]', (tester) async {
    final session = sessionWith(
      () async => (sessionId: 'ds_2', deviceId: 'dev_1', close: () async {}),
    );
    await pump(tester, session, scan: (_) async => _link);
    await tester.tap(find.byKey(const Key('scan-qr')));
    await tester.pumpAndSettle();
    expect(pairedWith, [_link]);
    expect(find.textContaining('ds_2'), findsOneWidget);
  });

  testWidgets('error: a link that is not a pairing link is reported '
      '[DEV-001]', (tester) async {
    await pump(tester, sessionWith(null));
    await tester.tap(find.byKey(const Key('paste-link')));
    await tester.pump();
    await tester.enterText(find.byKey(const Key('link-field')), 'hello');
    await tester.tap(find.byKey(const Key('pair-link')));
    await tester.pumpAndSettle();
    expect(pairedWith, isEmpty);
    expect(find.text('Pairing failed (invalid_link)'), findsOneWidget);
  });

  testWidgets('error: a failed pairing shows its code [DEV-001]', (
    tester,
  ) async {
    final session = sessionWith(() async => throw StateError('no channel'));
    await pump(tester, session, scan: (_) async => _link);
    await tester.tap(find.byKey(const Key('scan-qr')));
    await tester.pumpAndSettle();
    expect(find.text('Pairing failed (unavailable)'), findsOneWidget);
  });

  testWidgets('no roots: says so and refuses to pair [DEV-001]', (
    tester,
  ) async {
    await pump(tester, sessionWith(null), hasRoots: false);
    expect(find.textContaining('No development roots configured'), findsOne);
    expect(
      tester.widget<FilledButton>(find.byKey(const Key('scan-qr'))).onPressed,
      isNull,
    );
    expect(
      tester
          .widget<OutlinedButton>(find.byKey(const Key('paste-link')))
          .onPressed,
      isNull,
    );
    expect(pairedWith, isEmpty);
  });
}
