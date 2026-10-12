// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/device/guard.dart';
import 'package:plux_flutter/src/device/platform.dart';
import 'package:plux_flutter/src/render/builders/secure.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';
import 'package:plux_flutter/src/secure/kyc.dart';
import 'package:plux_flutter/src/secure/secure_ui.dart';

import 'secure_pages_test.dart' show FakeBiometrics, FakeClock, FakeScreen;
import 'secure_runtime_test.dart' show FakeNode;

Widget _app(Widget child) => MaterialApp(
  home: Scaffold(body: SingleChildScrollView(child: child)),
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    final view =
        TestWidgetsFlutterBinding.instance.platformDispatcher.views.first;
    view
      ..physicalSize = const Size(800, 2400)
      ..devicePixelRatio = 1;
    addTearDown(view.resetPhysicalSize);
    addTearDown(view.resetDevicePixelRatio);
  });

  group('KycCameraCaptureView', () {
    Future<List<String>> show(
      WidgetTester tester,
      List<KycShot?> shots, {
      String mode = 'document',
    }) async {
      final accepted = <String>[];
      await tester.pumpWidget(
        _app(
          KycCameraCaptureView(
            mode: mode,
            capture: () async => shots.removeAt(0),
            onCaptured: accepted.add,
          ),
        ),
      );
      return accepted;
    }

    testWidgets(
      'shows the framing guide for a document and for a selfie [SEC-090]',
      (tester) async {
        final handle = tester.ensureSemantics();
        await show(tester, []);
        expect(
          find.text('Fit the whole document inside the frame.'),
          findsWidgets,
        );
        expect(find.byType(CustomPaint), findsWidgets);
        expect(
          tester.getSemantics(
            find
                .bySemanticsLabel('Fit the whole document inside the frame.')
                .first,
          ),
          isNotNull,
        );
        expect(find.text('Use photo'), findsNothing);
        await show(tester, [], mode: 'selfie');
        expect(find.text('Center your face in the oval.'), findsWidgets);
        handle.dispose();
      },
    );

    testWidgets(
      'a photo that passed can be used, and its reference is the event [SEC-090]',
      (tester) async {
        final accepted = await show(tester, [
          const KycShot(path: '/private/a.jpg', reference: 'pf-1'),
        ]);
        await tester.tap(find.text('Take photo'));
        await tester.pump();
        expect(find.text('The photo looks good.'), findsOneWidget);
        await tester.tap(find.text('Use photo'));
        expect(accepted, ['pf-1']);
      },
    );

    testWidgets(
      'a photo that failed a check says why and cannot be used until a retake passes [SEC-090]',
      (tester) async {
        final accepted = await show(tester, [
          const KycShot(
            path: '/private/a.jpg',
            reference: 'pf-1',
            problems: ['blurry', 'dark'],
          ),
          const KycShot(path: '/private/b.jpg', reference: 'pf-2'),
        ]);
        await tester.tap(find.text('Take photo'));
        await tester.pump();
        expect(find.textContaining('blurry'), findsOneWidget);
        expect(find.textContaining('too dark'), findsOneWidget);
        expect(
          tester.widget<FilledButton>(find.bySubtype<FilledButton>()).onPressed,
          isNull,
        );
        await tester.tap(find.text('Retake'));
        await tester.pump();
        expect(find.textContaining('blurry'), findsNothing);
        await tester.tap(find.text('Use photo'));
        expect(accepted, ['pf-2']);
      },
    );

    testWidgets(
      'cancelling leaves the step as it was; no capture service disables it [SEC-090]',
      (tester) async {
        await show(tester, [null]);
        await tester.tap(find.text('Take photo'));
        await tester.pump();
        expect(find.text('Take photo'), findsOneWidget);
        expect(find.text('Use photo'), findsNothing);
        await tester.pumpWidget(
          _app(
            const KycCameraCaptureView(
              mode: 'document',
              capture: null,
              onCaptured: null,
            ),
          ),
        );
        expect(
          tester
              .widget<OutlinedButton>(find.bySubtype<OutlinedButton>())
              .onPressed,
          isNull,
        );
      },
    );
  });

  group('the KycCameraCapture builder', () {
    final d = widgetDescriptors.firstWhere((w) => w.type == 'KycCameraCapture');

    late List<PluxException> reports;
    late FakeKyc kyc;

    setUp(() {
      reports = [];
      kyc = FakeKyc();
    });

    Future<FakeNode> build(
      WidgetTester tester, {
      List<String> declared = const ['camera'],
      bool package = true,
    }) async {
      final ui = SecureUi(
        settings: ValueNotifier(SecuritySettings.builtIn),
        report: reports.add,
        platforms: SecurePlatforms(
          screen: FakeScreen(),
          biometrics: FakeBiometrics(),
        ),
        clock: FakeClock(),
      );
      addTearDown(ui.dispose);
      late FakeNode node;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: SecureUiScope(
              ui: ui,
              plugin: 'bank',
              guard: DeviceGuard(
                capabilities: (_) =>
                    (deviceApis: declared, networkDomains: const []),
                deepLinks: () => null,
                report: reports.add,
              ),
              services: {
                PickedFiles: PickedFiles(),
                if (package) PluxKycCapture: kyc,
              },
              child: Builder(
                builder: (context) {
                  node = FakeNode(
                    context,
                    {d.props['mode']!: 'selfie'},
                    handled: {d.events['onCaptured']!},
                  );
                  return SingleChildScrollView(
                    child: buildKycCameraCapture(node),
                  );
                },
              ),
            ),
          ),
        ),
      );
      return node;
    }

    testWidgets(
      'captures through the registered service, keeps the photo behind a handle and fires onCaptured [SEC-090]',
      (tester) async {
        final node = await build(tester);
        await tester.tap(find.text('Take photo'));
        await tester.pump();
        expect(kyc.modes, ['selfie']);
        await tester.tap(find.text('Use photo'));
        expect(node.fired, [d.events['onCaptured']]);
        final handle = node.payloads.single! as String;
        expect(handle, startsWith('pf-'));
        expect(
          handle,
          isNot(contains('private')),
          reason: 'no path in the event',
        );
        expect(reports, isEmpty);
      },
    );

    testWidgets('quality problems reach the review [SEC-090]', (tester) async {
      kyc.problems = ['bright'];
      await build(tester);
      await tester.tap(find.text('Take photo'));
      await tester.pump();
      expect(find.textContaining('too bright'), findsOneWidget);
    });

    testWidgets(
      'without plux_media the problem is reported, not thrown [RT-060]',
      (tester) async {
        await build(tester, package: false);
        await tester.tap(find.text('Take photo'));
        await tester.pump();
        expect(reports.single.code, PluxErrorCode.devicePackageMissing);
        expect(find.text('Use photo'), findsNothing);
      },
    );

    testWidgets('without the camera capability nothing is captured [SEC-080]', (
      tester,
    ) async {
      await build(tester, declared: ['haptics']);
      await tester.tap(find.text('Take photo'));
      await tester.pump();
      expect(kyc.modes, isEmpty);
      expect(reports.single.code, PluxErrorCode.deviceCapabilityBlocked);
    });

    testWidgets(
      'a refused permission is reported with its own code [SEC-080]',
      (tester) async {
        kyc.failure = const PluxDeviceException.denied(
          'the permission was denied',
        );
        await build(tester);
        await tester.tap(find.text('Take photo'));
        await tester.pump();
        expect(reports.single.code, PluxErrorCode.devicePermissionDenied);
        expect(find.text('Take photo'), findsOneWidget);
      },
    );
  });
}

final class FakeKyc implements PluxKycCapture {
  final List<String> modes = [];
  List<String> problems = const [];
  PluxDeviceException? failure;

  @override
  Future<PluxKycPhoto?> capture(String mode) async {
    modes.add(mode);
    if (failure case final f?) throw f;
    return PluxKycPhoto(
      file: const PluxPickedFile(
        path: '/private/kyc.jpg',
        name: 'kyc.jpg',
        mimeType: 'image/jpeg',
        sizeBytes: 9,
      ),
      problems: problems,
    );
  }
}
