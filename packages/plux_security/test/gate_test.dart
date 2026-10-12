// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_security/plux_security.dart';
import 'package:plux_security/testing.dart';

import 'support/fake_host.dart';

/// Counts how often it is built and keeps a counter, to show that the
/// content survives a block.
final class _Counter extends StatefulWidget {
  const _Counter();

  @override
  State<_Counter> createState() => _CounterState();
}

final class _CounterState extends State<_Counter> {
  var taps = 0;

  @override
  Widget build(BuildContext context) => GestureDetector(
    onTap: () => setState(() => taps++),
    child: Text('taps $taps', textDirection: TextDirection.ltr),
  );
}

void main() {
  late FakeRaspDetector detector;
  late FakeHost host;

  Future<PluxSecurity> pump(
    WidgetTester tester, {
    required String rootResponse,
    PluxRaspPolicy policy = const PluxRaspPolicy(),
    PluxBlockedBuilder? blocked,
    PluxWarningBuilder? warning,
  }) async {
    host = FakeHost(settingsWithRaspResponse(rootResponse));
    final security = PluxSecurity(detector: detector, policy: policy);
    security.attach(host);
    await tester.pumpWidget(
      Directionality(
        textDirection: TextDirection.ltr,
        child: PluxSecurityGate(
          security: security,
          blockedBuilder: blocked,
          warningBuilder: warning,
          child: const _Counter(),
        ),
      ),
    );
    return security;
  }

  setUp(() => detector = FakeRaspDetector());

  testWidgets('shows the content while nothing is in force [SEC-071]', (
    tester,
  ) async {
    final security = await pump(tester, rootResponse: 'block');
    await tester.pump();
    expect(find.text('taps 0'), findsOneWidget);
    expect(find.textContaining('cannot run safely'), findsNothing);
    security.detach();
  });

  testWidgets('block replaces the content with the blocking screen, keeps its '
      'state [SEC-071]', (tester) async {
    detector.findings = [PluxDetection(PluxDetectionKind.root)];
    final security = await pump(tester, rootResponse: 'report');
    await tester.tap(find.text('taps 0'));
    await tester.pump();
    expect(find.text('taps 1'), findsOneWidget);

    host.settings.value = settingsWithRaspResponse('block');
    await tester.pump();
    expect(find.textContaining('cannot run safely'), findsOneWidget);
    expect(find.text('taps 1', skipOffstage: true), findsNothing);
    expect(
      find.text('taps 1', skipOffstage: false),
      findsOneWidget,
      reason: 'still mounted, so navigation state survives',
    );
    security.detach();
  });

  testWidgets('the host\'s own blocking screen is used [SEC-071]', (
    tester,
  ) async {
    detector.findings = [PluxDetection(PluxDetectionKind.hooking)];
    final security = await pump(
      tester,
      rootResponse: 'block',
      blocked: (context, detections) => Text(
        'blocked: ${detections.single.kind.wireName}',
        textDirection: TextDirection.ltr,
      ),
    );
    await tester.pump();
    expect(find.text('blocked: hooking'), findsOneWidget);
    security.detach();
  });

  testWidgets('warn shows a message over the content [SEC-071]', (
    tester,
  ) async {
    detector.findings = [PluxDetection(PluxDetectionKind.root)];
    final security = await pump(
      tester,
      rootResponse: 'warn',
      warning: (context, warnings) =>
          Text('warning ${warnings.length}', textDirection: TextDirection.ltr),
    );
    await tester.pump();
    expect(find.text('warning 1'), findsOneWidget);
    expect(find.text('taps 0'), findsOneWidget);
    security.detach();
  });

  testWidgets('report shows nothing, degrade shows the default message '
      '[SEC-071]', (tester) async {
    detector.findings = [PluxDetection(PluxDetectionKind.root)];
    final security = await pump(tester, rootResponse: 'report');
    await tester.pump();
    expect(find.textContaining('not secure'), findsNothing);
    host.settings.value = settingsWithRaspResponse('degrade');
    await tester.pump();
    expect(find.textContaining('not secure'), findsOneWidget);
    security.detach();
  });

  testWidgets('the message goes when its cause does [SEC-071]', (tester) async {
    detector.findings = [PluxDetection(PluxDetectionKind.screenCapture)];
    final security = await pump(
      tester,
      rootResponse: 'report',
      policy: const PluxRaspPolicy(
        responses: {PluxDetectionKind.screenCapture: PluxRaspResponse.warn},
      ),
    );
    await tester.pump();
    expect(find.textContaining('not secure'), findsOneWidget);
    detector.findings = [];
    await security.scanNow();
    await tester.pump();
    expect(find.textContaining('not secure'), findsNothing);
    security.detach();
  });
}
