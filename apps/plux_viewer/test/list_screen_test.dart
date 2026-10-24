// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_host_kit/plux_host_kit.dart';
import 'package:plux_viewer/viewer.dart';

/// The list of the auditor's reconstructions [SEC-142].
void main() {
  final server = Uri.parse('https://plux.example.org/');

  Map<String, Object?> item(
    String id,
    String app, {
    String state = 'signed',
    String created = '2026-10-21T10:00:00Z',
    String moment = '2026-10-20T09:30:15Z',
    int release = 7,
  }) => {
    'id': id,
    'appId': app,
    'deviceId': 'dev_$id',
    'moment': moment,
    'releaseSequence': '$release',
    'state': state,
    'createdAt': created,
  };

  /// A server with two apps and the given reconstructions per app.
  ViewerApi serverWith(
    Map<String, List<Map<String, Object?>>> byApp, {
    int? failWith,
    List<String>? calls,
  }) => ViewerApi(
    server,
    token: 't',
    client: MockClient((request) async {
      calls?.add(request.url.path);
      if (failWith != null) {
        return http.Response(
          jsonEncode({
            'code': failWith == 401 ? 'unauthenticated' : 'unavailable',
            'message': 'it failed',
          }),
          failWith,
        );
      }
      final body = jsonDecode(request.body) as Map<String, Object?>;
      if (request.url.path.endsWith('ListApps')) {
        return http.Response(
          jsonEncode({
            'apps': [
              for (final id in byApp.keys) {'id': id, 'name': 'App $id'},
            ],
          }),
          200,
        );
      }
      return http.Response(
        jsonEncode({'reconstructions': byApp[body['appId']] ?? const []}),
        200,
      );
    }),
  );

  Future<void> show(
    WidgetTester tester,
    ViewerApi api, {
    void Function(ReconstructionSummary)? onOpen,
    VoidCallback? onSignOut,
  }) async {
    await tester.pumpWidget(
      MaterialApp(
        home: ListScreen(
          api: api,
          onOpen: onOpen ?? (_) {},
          onSignOut: onSignOut ?? () {},
        ),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets('lists the reconstructions of every app, newest first', (
    tester,
  ) async {
    await show(
      tester,
      serverWith({
        'a1': [
          item('older', 'a1', created: '2026-10-19T10:00:00Z'),
          item('newest', 'a1', created: '2026-10-21T12:00:00Z', release: 9),
        ],
        'a2': [item('middle', 'a2', created: '2026-10-20T10:00:00Z')],
      }),
    );
    final tiles = tester.widgetList<ListTile>(find.byType(ListTile)).toList();
    expect(tiles, hasLength(3));
    String subtitle(ListTile t) => (t.subtitle! as Text).data!;
    expect(subtitle(tiles[0]), 'App a1 · release 9 · device dev_newest');
    expect(subtitle(tiles[1]), 'App a2 · release 7 · device dev_middle');
    expect(subtitle(tiles[2]), 'App a1 · release 7 · device dev_older');
    expect(find.text('2026-10-20 09:30 UTC'), findsNWidgets(3));
  });

  testWidgets('opens the one that is tapped [SEC-142]', (tester) async {
    final opened = <String>[];
    await show(
      tester,
      serverWith({
        'a1': [item('rec_1', 'a1')],
      }),
      onOpen: (r) => opened.add(r.id),
    );
    await tester.tap(find.byType(ListTile));
    expect(opened, ['rec_1']);
  });

  testWidgets('does not open one the server has not signed yet', (
    tester,
  ) async {
    final opened = <String>[];
    await show(
      tester,
      serverWith({
        'a1': [item('rec_1', 'a1', state: 'pending')],
      }),
      onOpen: (r) => opened.add(r.id),
    );
    expect(find.textContaining('waiting to be signed'), findsOneWidget);
    await tester.tap(find.byType(ListTile));
    expect(opened, isEmpty);
    expect(tester.widget<ListTile>(find.byType(ListTile)).enabled, isFalse);
  });

  testWidgets('says so when there are none', (tester) async {
    await show(tester, serverWith({'a1': []}));
    expect(find.textContaining('no reconstructions yet'), findsOneWidget);
    expect(find.byType(ListTile), findsNothing);
  });

  testWidgets('shows a failure and loads again on request', (tester) async {
    final calls = <String>[];
    await show(tester, serverWith({}, failWith: 503, calls: calls));
    expect(find.text('it failed'), findsOneWidget);
    expect(calls, hasLength(1));
    await tester.tap(find.text('Try again'));
    await tester.pumpAndSettle();
    expect(calls, hasLength(2));
  });

  testWidgets('signs out when the server refuses the token', (tester) async {
    var signedOut = 0;
    await show(
      tester,
      serverWith({}, failWith: 401),
      onSignOut: () => signedOut++,
    );
    expect(signedOut, 1);
  });

  testWidgets('signs out on request', (tester) async {
    var signedOut = 0;
    await show(tester, serverWith({'a1': []}), onSignOut: () => signedOut++);
    await tester.tap(find.byTooltip('Sign out'));
    expect(signedOut, 1);
  });
}
