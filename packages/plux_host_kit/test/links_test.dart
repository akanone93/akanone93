// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_host_kit/plux_host_kit.dart';

/// The links the platform delivers reach the controller [SEC-142].
void main() {
  final server = Uri.parse('https://plux.example.org/');
  late ViewerController controller;

  setUp(() => controller = ViewerController());

  ViewerLink link() => ViewerLink(id: 'rec_1', server: server);

  group('ViewerLinks [SEC-142]', () {
    test('leaves other routes to the framework', () {
      final links = ViewerLinks(controller);
      expect(links.offer(Uri.parse('/')), isFalse);
      expect(links.offer(Uri.parse('https://plux.example.org/')), isFalse);
      expect(controller.pendingLink, isNull);
      expect(controller.hasRejection, isFalse);
    });

    test('claims a valid link and queues it', () {
      final links = ViewerLinks(controller);
      expect(
        links.offer(
          Uri.parse(
            'plux-viewer://reconstruction/rec_1?server=https://plux.example.org',
          ),
        ),
        isTrue,
      );
      expect(controller.pendingLink, link());
    });

    test('claims, and rejects, a link it cannot follow', () {
      final links = ViewerLinks(controller);
      expect(links.offer(Uri.parse('plux-viewer://elsewhere/rec_1')), isTrue);
      expect(controller.pendingLink, isNull);
      expect(controller.takeRejection(), contains('cannot be opened'));
      expect(controller.hasRejection, isFalse);
    });

    testWidgets('receives the route information the platform pushes', (
      tester,
    ) async {
      final links = ViewerLinks(controller)..attach('/');
      addTearDown(links.detach);
      // As the engine delivers a link opened while the app runs.
      await tester.binding.defaultBinaryMessenger.handlePlatformMessage(
        'flutter/navigation',
        const JSONMethodCodec().encodeMethodCall(
          const MethodCall('pushRouteInformation', {
            'location': 'plux-viewer://reconstruction/rec_1?server=https://plux.example.org',
          }),
        ),
        (_) {},
      );
      await tester.pump();
      expect(controller.pendingLink, link());
    });

    testWidgets('offers the link the app was launched with', (tester) async {
      final links = ViewerLinks(controller)
        ..attach(
          'plux-viewer://reconstruction/rec_1?server=https://plux.example.org',
        );
      addTearDown(links.detach);
      expect(controller.pendingLink, link());
    });
  });
}
