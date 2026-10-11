// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_security/plux_security.dart';
import 'package:plux_security/testing.dart';

import 'support/fake_host.dart';

PluxDetection _root() =>
    PluxDetection(PluxDetectionKind.root, ['su_binary', 'test_keys']);

void main() {
  late FakeRaspDetector detector;
  late FakeHost host;

  PluxSecurity module({
    PluxRaspPolicy policy = const PluxRaspPolicy(),
    String rootResponse = 'report',
  }) {
    host = FakeHost(settingsWithRaspResponse(rootResponse));
    return PluxSecurity(detector: detector, policy: policy);
  }

  setUp(() => detector = FakeRaspDetector());

  group('report', () {
    test('reports the detection and takes no other step [SEC-071]', () async {
      detector.findings = [_root()];
      final security = module();
      security.attach(host);
      await security.scanNow();
      final r = host.reported.single;
      expect((r.kind, r.severity, r.response), ('root', 'high', 'report'));
      expect(r.indicators, ['su_binary', 'test_keys']);
      expect(host.ceilings, isEmpty);
      expect(security.status.value.blocked, isFalse);
      expect(security.status.value.warnings, isEmpty);
      security.detach();
    });

    test('a device with nothing found reports nothing [SEC-070]', () async {
      final security = module();
      security.attach(host);
      await security.scanNow();
      expect(host.log, isEmpty);
      expect(security.status.value, PluxSecurityStatus.clear);
      security.detach();
    });
  });

  group('warn', () {
    test(
      'lists the detection as a warning and caps nothing [SEC-071]',
      () async {
        detector.findings = [
          PluxDetection(PluxDetectionKind.emulator, ['qemu']),
        ];
        final security = module(
          policy: const PluxRaspPolicy(
            responses: {PluxDetectionKind.emulator: PluxRaspResponse.warn},
          ),
        );
        security.attach(host);
        await security.scanNow();
        expect(host.reported.single.response, 'warn');
        expect(
          security.status.value.warnings.single.kind,
          PluxDetectionKind.emulator,
        );
        expect(host.ceilings, isEmpty);
        security.detach();
      },
    );
  });

  group('degrade', () {
    test(
      'caps assurance at AL2, which refuses AL3 pages [SEC-071] [SEC-007]',
      () async {
        detector.findings = [_root()];
        final security = module(rootResponse: 'degrade');
        security.attach(host);
        await security.scanNow();
        expect(host.reported.single.response, 'degrade');
        expect(host.ceilings, [2]);
        expect(host.assurance.value, 2);
        expect(security.status.value.assuranceCeiling, 2);
        expect(security.status.value.blocked, isFalse);
        security.detach();
      },
    );
  });

  group('block', () {
    test(
      'shows the blocking screen and caps assurance at AL0 [SEC-071]',
      () async {
        detector.findings = [_root()];
        final security = module(rootResponse: 'block');
        security.attach(host);
        await security.scanNow();
        expect(host.reported.single.response, 'block');
        expect(host.ceilings, [0]);
        expect(security.status.value.blocked, isTrue);
        security.detach();
      },
    );

    test('a setting that tightens later applies without a second report '
        '[SEC-071] [SEC-182]', () async {
      detector.findings = [_root()];
      final security = module();
      security.attach(host);
      await security.scanNow();
      expect(security.status.value.blocked, isFalse);
      host.settings.value = settingsWithRaspResponse('block');
      expect(security.status.value.blocked, isTrue);
      expect(host.ceilings, [0]);
      expect(host.reported, hasLength(1));
      security.detach();
    });
  });

  group('reporting', () {
    test('a detection is reported before the device attests again, once per '
        'appearance [SEC-071] [SEC-006]', () async {
      detector.findings = [_root()];
      final security = module();
      security.attach(host);
      await security.scanNow();
      await security.scanNow();
      expect(host.log, ['report:root', 'reattest']);
      security.detach();
    });

    test(
      'a new kind in a later scan is reported on its own [SEC-071]',
      () async {
        detector.findings = [_root()];
        final security = module();
        security.attach(host);
        await security.scanNow();
        detector.findings = [
          _root(),
          PluxDetection(PluxDetectionKind.hooking, ['frida_port']),
        ];
        await security.scanNow();
        expect(host.reported.map((r) => r.kind), ['root', 'hooking']);
        expect(host.reattestations, 2);
        security.detach();
      },
    );

    test('screen capture and overlays are reported but do not make the '
        'device attest again [SEC-070] [SEC-006]', () async {
      detector.findings = [
        PluxDetection(PluxDetectionKind.screenCapture, ['external_display']),
        PluxDetection(PluxDetectionKind.overlay, ['obscured_touch']),
        PluxDetection(PluxDetectionKind.accessibility, [
          'a11y_content_gestures',
        ]),
      ];
      final security = module();
      security.attach(host);
      await security.scanNow();
      expect(host.reported.map((r) => r.kind), [
        'screen_capture',
        'overlay',
        'accessibility',
      ]);
      expect(host.reattestations, 0);
      security.detach();
    });

    test(
      'the severity of each kind travels with the report [SEC-071]',
      () async {
        detector.findings = [
          for (final k in PluxDetectionKind.values) PluxDetection(k),
        ];
        final security = module();
        security.attach(host);
        await security.scanNow();
        expect(
          {for (final r in host.reported) r.kind: r.severity},
          {
            'root': 'high',
            'hooking': 'high',
            'repackaging': 'high',
            'debugger': 'medium',
            'emulator': 'medium',
            'overlay': 'medium',
            'accessibility': 'medium',
            'untrusted_installer': 'low',
            'screen_capture': 'low',
          },
        );
        security.detach();
      },
    );
  });

  group('state over time', () {
    test('a hostile environment stays in force after it goes away '
        '[SEC-070]', () async {
      detector.findings = [_root()];
      final security = module(rootResponse: 'degrade');
      security.attach(host);
      await security.scanNow();
      detector.findings = [];
      await security.scanNow();
      expect(
        security.status.value.detections,
        contains(PluxDetectionKind.root),
      );
      expect(host.ceilings, [2]);
      security.detach();
    });

    test('a live condition ends with its cause, and is reported again when '
        'it returns [SEC-070] [SEC-071]', () async {
      final security = module(
        policy: const PluxRaspPolicy(
          responses: {
            PluxDetectionKind.screenCapture: PluxRaspResponse.degrade,
          },
        ),
      );
      security.attach(host);
      await security.scanNow();
      detector.findings = [PluxDetection(PluxDetectionKind.screenCapture)];
      await security.scanNow();
      expect(host.ceilings, [2]);
      detector.findings = [];
      await security.scanNow();
      expect(security.status.value, PluxSecurityStatus.clear);
      expect(host.ceilings, [2, 3]);
      detector.findings = [PluxDetection(PluxDetectionKind.screenCapture)];
      await security.scanNow();
      expect(host.reported, hasLength(2));
      security.detach();
    });

    test('a detector that fails leaves what was found in force and throws '
        'nothing [SEC-070]', () async {
      detector.findings = [_root()];
      final security = module();
      security.attach(host);
      await security.scanNow();
      detector.failure = Exception('channel');
      await security.scanNow();
      expect(
        security.status.value.detections,
        contains(PluxDetectionKind.root),
      );
      security.detach();
    });

    test('scans started together are one scan [SEC-070]', () async {
      final security = module();
      security.attach(host);
      await Future.wait([security.scanNow(), security.scanNow()]);
      expect(detector.scans, hasLength(1), reason: 'attach started it');
      security.detach();
    });
  });

  group('scheduling', () {
    testWidgets('scans at attach and then every interval [SEC-070]', (
      tester,
    ) async {
      final security = PluxSecurity(
        detector: detector,
        scanInterval: const Duration(seconds: 30),
      );
      host = FakeHost();
      security.attach(host);
      await tester.pump();
      expect(detector.scans, hasLength(1));
      await tester.pump(const Duration(seconds: 30));
      expect(detector.scans, hasLength(2));
      await tester.pump(const Duration(seconds: 30));
      expect(detector.scans, hasLength(3));
      security.detach();
      await tester.pump(const Duration(minutes: 5));
      expect(detector.scans, hasLength(3), reason: 'detached');
    });

    test('a detection after detach changes nothing [SEC-071]', () async {
      detector.findings = [_root()];
      final security = module();
      security.attach(host);
      final scan = security.scanNow();
      security.detach();
      await scan;
      expect(host.reported, isEmpty);
    });

    test('the scan carries the expectations of the app [SEC-070]', () async {
      final security = PluxSecurity(
        detector: detector,
        expectations: PluxRaspExpectations(
          signingCertSha256: ['AB:CD:ef'],
          iosTeamId: 'TEAM123456',
        ),
      );
      host = FakeHost();
      security.attach(host);
      await security.scanNow();
      expect(detector.scans.single.signingCertSha256, ['abcdef']);
      expect(detector.scans.single.iosTeamId, 'TEAM123456');
      security.detach();
    });
  });
}
