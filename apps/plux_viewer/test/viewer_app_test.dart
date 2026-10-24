// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/plux_flutter.dart' show PluxPublicKey;
import 'package:plux_host_kit/plux_host_kit.dart';
import 'package:plux_viewer/viewer.dart';

/// The viewer opens a link once the auditor is signed in to its server
/// [SEC-142].
void main() {
  final server = Uri.parse('https://plux.example.org/');
  late ViewerController controller;
  late List<String> opened;

  ViewerApi apiFor(Uri s, {String? token}) => ViewerApi(
    s,
    token: token,
    client: MockClient((request) async {
      final path = request.url.path;
      if (path.endsWith('ListApps')) {
        return http.Response(jsonEncode({'apps': <Object?>[]}), 200);
      }
      opened.add(path);
      return http.Response(
        jsonEncode({'code': 'not_found', 'message': 'no such release'}),
        404,
      );
    }),
  );

  final pinned = [
    PluxPublicKey(
      keyId: 'root-1',
      algorithm: 'ed25519',
      role: 'root',
      publicKey: Uint8List(32),
    ),
  ];

  Future<void> pumpApp(WidgetTester tester) async {
    await tester.pumpWidget(
      ViewerApp(
        controller: controller,
        rootKeys: pinned,
        openerFor: (api) => ReconstructionOpener(
          apiFor(api.server, token: 'x'),
          rootKeys: pinned,
          hostBuild: 'plux-viewer',
          start: (_) async => null,
          stop: () async {},
        ),
        apiFactory: apiFor,
      ),
    );
    await tester.pumpAndSettle();
  }

  setUp(() {
    controller = ViewerController();
    opened = [];
  });

  ViewerLink link([Uri? s]) => ViewerLink(id: 'rec_1', server: s ?? server);

  testWidgets('starts at the sign-in screen', (tester) async {
    await pumpApp(tester);
    expect(find.text('Sign in with a device code'), findsOneWidget);
  });

  testWidgets('keeps a link until the auditor has signed in to its server', (
    tester,
  ) async {
    await pumpApp(tester);
    controller.offerLink(link());
    await tester.pumpAndSettle();
    expect(
      find.textContaining('opens a reconstruction on plux.example.org'),
      findsOneWidget,
    );
    expect(find.byType(ReconstructionScreen), findsNothing);
    expect(
      tester
          .widget<TextField>(find.widgetWithText(TextField, 'Plux server'))
          .controller!
          .text,
      'https://plux.example.org/',
    );
    controller.signedIn(ViewerSession(server, 't'));
    await tester.pumpAndSettle();
    expect(find.byType(ReconstructionScreen), findsOneWidget);
    expect(controller.pendingLink, isNull);
    expect(opened, ['/plux.v1.AuditService/GetReconstruction']);
  });

  testWidgets('opens a link at once when already signed in to its server', (
    tester,
  ) async {
    controller.signedIn(ViewerSession(server, 't'));
    await pumpApp(tester);
    expect(find.text('Reconstructions'), findsOneWidget);
    controller.offerLink(link());
    await tester.pumpAndSettle();
    expect(find.byType(ReconstructionScreen), findsOneWidget);
  });

  testWidgets('never sends the token to another server than its own', (
    tester,
  ) async {
    controller.signedIn(ViewerSession(server, 't'));
    await pumpApp(tester);
    controller.offerLink(link(Uri.parse('https://other.example.org/')));
    await tester.pumpAndSettle();
    expect(controller.session, isNull);
    expect(find.byType(ReconstructionScreen), findsNothing);
    expect(find.text('Sign in with a device code'), findsOneWidget);
    expect(opened, isEmpty);
    expect(
      find.textContaining('opens a reconstruction on other.example.org'),
      findsOneWidget,
    );
  });

  testWidgets('shows why a link was refused', (tester) async {
    await pumpApp(tester);
    final links = ViewerLinks(controller);
    expect(
      links.offer(Uri.parse('plux-viewer://reconstruction/a?server=http://x')),
      isTrue,
    );
    await tester.pump();
    expect(
      find.textContaining('the server must be an https URL'),
      findsOneWidget,
    );
    expect(controller.pendingLink, isNull);
  });

  testWidgets('returns to the sign-in screen on sign-out', (tester) async {
    controller.signedIn(ViewerSession(server, 't'));
    await pumpApp(tester);
    await tester.tap(find.byTooltip('Sign out'));
    await tester.pumpAndSettle();
    expect(controller.session, isNull);
    expect(find.text('Sign in with a device code'), findsOneWidget);
  });
}
