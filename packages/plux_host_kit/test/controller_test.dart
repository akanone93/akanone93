// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_host_kit/plux_host_kit.dart';

/// What a host remembers while it runs [SEC-142].
void main() {
  final server = Uri.parse('https://plux.example.org/');
  final other = Uri.parse('https://other.example.org/');
  late ViewerController controller;

  setUp(() => controller = ViewerController());

  ViewerLink linkTo(Uri s) => ViewerLink(id: 'rec_1', server: s);

  test('holds a link until a session on its server can open it', () {
    controller.offerLink(linkTo(server));
    expect(controller.pendingLink, linkTo(server));
    expect(controller.takeLink(), isNull);
    controller.signedIn(ViewerSession(server, 't'));
    expect(controller.takeLink(), linkTo(server));
    expect(controller.pendingLink, isNull);
    expect(controller.takeLink(), isNull);
  });

  test('never keeps a token for another server than its own [SEC-142]', () {
    controller.signedIn(ViewerSession(server, 't'));
    controller.offerLink(linkTo(other));
    expect(controller.session, isNull);
    expect(controller.pendingLink, linkTo(other));
  });

  test('keeps a pending link when the auditor signs out', () {
    controller.signedIn(ViewerSession(server, 't'));
    controller.offerLink(linkTo(server));
    controller.signOut();
    expect(controller.session, isNull);
    expect(controller.pendingLink, linkTo(server));
  });

  test('shows a refusal once', () {
    controller.rejectLink('no');
    expect(controller.hasRejection, isTrue);
    expect(controller.takeRejection(), 'no');
    expect(controller.takeRejection(), isNull);
  });

  test('tells its listeners about every change', () {
    var notified = 0;
    controller.addListener(() => notified++);
    controller.signedIn(ViewerSession(server, 't'));
    controller.offerLink(linkTo(server));
    controller.rejectLink('no');
    controller.signOut();
    expect(notified, 4);
  });
}
