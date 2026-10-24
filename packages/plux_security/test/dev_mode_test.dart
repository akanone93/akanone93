// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_security/plux_security.dart';
import 'package:plux_security/testing.dart';

import 'support/fake_host.dart';

final class _Sink implements DevEventSink {
  final List<Map<String, Object?>> events = [];

  @override
  void emit(Map<String, Object?> payload, {String? traceId}) =>
      events.add(payload);
}

PluxDetection _root() => PluxDetection(PluxDetectionKind.root, ['su_binary']);

void main() {
  late FakeRaspDetector detector;
  late FakeHost host;
  late _Sink sink;

  PluxSecurity module({
    PluxSecurityMode mode = PluxSecurityMode.developmentReportOnly,
    String environment = 'dev',
    bool devtools = true,
    String rootResponse = 'block',
  }) {
    host = FakeHost(settingsWithRaspResponse(rootResponse));
    return PluxSecurity(
      detector: detector,
      mode: mode,
      environment: environment,
      devtools: devtools,
    )..devEventSink = sink;
  }

  setUp(() {
    detector = FakeRaspDetector();
    sink = _Sink();
  });

  group('the mode is refused', () {
    test('in a production environment [DEV-001]', () async {
      detector.findings = [_root()];
      final security = module(environment: 'production');
      expect(security.developmentMode, isFalse);
      expect(security.developmentRefusal, contains('production'));
      security.attach(host);
      await security.scanNow();
      // The production behaviour is in force: blocked, reported to the
      // server, capped at AL0 [SEC-071].
      expect(security.status.value.blocked, isTrue);
      expect(host.reported.single.response, 'block');
      expect(host.ceilings, [0]);
      expect(sink.events, isEmpty);
      security.detach();
    });

    test('when the environment is left out [DEV-001]', () {
      final security = PluxSecurity(
        detector: detector,
        mode: PluxSecurityMode.developmentReportOnly,
      );
      expect(security.developmentMode, isFalse);
      expect(security.developmentRefusal, isNotNull);
    });

    test('in a build without the dev channel [DEV-001]', () async {
      detector.findings = [_root()];
      final security = module(devtools: false);
      expect(security.developmentMode, isFalse);
      expect(security.developmentRefusal, contains('no dev channel'));
      security.attach(host);
      await security.scanNow();
      expect(security.status.value.blocked, isTrue);
      expect(host.ceilings, [0]);
      expect(security.simulateDetection(PluxDetectionKind.root), isFalse);
      security.detach();
    });

    test('is not asked for in the production mode [DEV-001]', () {
      final security = module(mode: PluxSecurityMode.production);
      expect(security.developmentMode, isFalse);
      expect(security.developmentRefusal, isNull);
    });
  });

  group('report-only', () {
    test('reports the detection to the dev stream, not the server '
        '[DEV-001] [SEC-070]', () async {
      detector.findings = [_root()];
      final security = module();
      expect(security.developmentMode, isTrue);
      security.attach(host);
      await security.scanNow();
      expect(sink.events, [
        {
          'securityDetection': {
            'kind': 'DEV_SECURITY_DETECTION_KIND_ROOT',
            'response': 'report',
          },
        },
      ]);
      expect(security.status.value.detections.keys, [PluxDetectionKind.root]);
      expect(host.reported, isEmpty);
      security.detach();
    });

    test('leaves the assurance level and the responses alone '
        '[DEV-001] [SEC-071] [SEC-007]', () async {
      detector.findings = [
        _root(),
        PluxDetection(PluxDetectionKind.emulator, ['qemu']),
      ];
      final security = module();
      security.attach(host);
      await security.scanNow();
      expect(host.ceilings, isEmpty);
      expect(host.assurance.value, 3);
      expect(host.reattestations, 0);
      final status = security.status.value;
      expect(status.blocked, isFalse);
      expect(status.warnings, isEmpty);
      expect(status.assuranceCeiling, 3);
      security.detach();
    });

    test(
      'names every kind as the dev stream does [DEV-001] [SEC-070]',
      () async {
        detector.findings = [
          for (final k in PluxDetectionKind.values) PluxDetection(k),
        ];
        final security = module();
        security.attach(host);
        await security.scanNow();
        final kinds = [
          for (final e in sink.events)
            (e['securityDetection']! as Map<String, Object?>)['kind'],
        ];
        expect(kinds, [
          'DEV_SECURITY_DETECTION_KIND_ROOT',
          'DEV_SECURITY_DETECTION_KIND_HOOKING',
          'DEV_SECURITY_DETECTION_KIND_DEBUGGER',
          'DEV_SECURITY_DETECTION_KIND_EMULATOR',
          'DEV_SECURITY_DETECTION_KIND_REPACKAGING',
          'DEV_SECURITY_DETECTION_KIND_UNTRUSTED_INSTALLER',
          'DEV_SECURITY_DETECTION_KIND_SCREEN_CAPTURE',
          'DEV_SECURITY_DETECTION_KIND_OVERLAY',
          'DEV_SECURITY_DETECTION_KIND_ACCESSIBILITY',
        ]);
        security.detach();
      },
    );

    test('a detection already in force is reported once [DEV-001]', () async {
      detector.findings = [_root()];
      final security = module();
      security.attach(host);
      await security.scanNow();
      await security.scanNow();
      expect(sink.events, hasLength(1));
      security.detach();
    });
  });

  group('simulateSecurityDetection', () {
    Future<Map<String, Object?>> run(PluxSecurity security, Object? kind) =>
        pluxSecuritySimulateDetectionHandler(security).handle({
          'simulateSecurityDetection': {'kind': kind},
        });

    test(
      'emits the event and drives the pipeline [DEV-001] [SEC-071]',
      () async {
        final security = module();
        security.attach(host);
        await security.scanNow();
        final result = await run(
          security,
          'DEV_SECURITY_DETECTION_KIND_HOOKING',
        );
        expect(result['status'], 'DEV_COMMAND_STATUS_OK');
        expect(sink.events.single, {
          'securityDetection': {
            'kind': 'DEV_SECURITY_DETECTION_KIND_HOOKING',
            'simulated': true,
            'response': 'report',
          },
        });
        final status = security.status.value;
        expect(status.detections.keys, [PluxDetectionKind.hooking]);
        expect(status.detections.values.single.indicators, ['simulated']);
        // The server's view of the device is unchanged.
        expect(host.reported, isEmpty);
        expect(host.ceilings, isEmpty);
        expect(host.assurance.value, 3);
        security.detach();
      },
    );

    test('a sticky kind stays in force through the next scan [DEV-001] '
        '[SEC-070]', () async {
      final security = module();
      security.attach(host);
      await security.scanNow();
      await run(security, 'DEV_SECURITY_DETECTION_KIND_ROOT');
      await security.scanNow();
      expect(security.status.value.detections.keys, [PluxDetectionKind.root]);
      security.detach();
    });

    test(
      'a current-state kind goes with the next scan [DEV-001] [SEC-070]',
      () async {
        final security = module();
        security.attach(host);
        await security.scanNow();
        await run(security, 'DEV_SECURITY_DETECTION_KIND_OVERLAY');
        expect(security.status.value.detections, isNotEmpty);
        await security.scanNow();
        expect(security.status.value, PluxSecurityStatus.clear);
        security.detach();
      },
    );

    test('accepts the number of the kind [DEV-001]', () async {
      final security = module();
      security.attach(host);
      final result = await run(security, 4);
      expect(result['status'], 'DEV_COMMAND_STATUS_OK');
      expect(security.status.value.detections.keys, [
        PluxDetectionKind.emulator,
      ]);
      security.detach();
    });

    test('an unknown or unspecified kind fails [DEV-001]', () async {
      final security = module();
      security.attach(host);
      for (final kind in [
        'DEV_SECURITY_DETECTION_KIND_UNSPECIFIED',
        'DEV_SECURITY_DETECTION_KIND_TELEPORT',
        0,
        10,
        null,
      ]) {
        final result = await run(security, kind);
        expect(result['status'], 'DEV_COMMAND_STATUS_FAILED', reason: '$kind');
      }
      expect(sink.events, isEmpty);
      expect(security.status.value, PluxSecurityStatus.clear);
      security.detach();
    });

    test('a body that is not an object fails [DEV-001]', () async {
      final security = module();
      security.attach(host);
      final result = await pluxSecuritySimulateDetectionHandler(security)
          .handle({'simulateSecurityDetection': 'root'});
      expect(result['status'], 'DEV_COMMAND_STATUS_FAILED');
      security.detach();
    });

    test('is refused where the mode does not run [DEV-001]', () async {
      final security = module(environment: 'production');
      security.attach(host);
      final result = await run(security, 'DEV_SECURITY_DETECTION_KIND_ROOT');
      expect(result['status'], 'DEV_COMMAND_STATUS_REFUSED');
      expect(security.status.value, PluxSecurityStatus.clear);
      expect(host.reported, isEmpty);
      security.detach();
    });
  });
}
