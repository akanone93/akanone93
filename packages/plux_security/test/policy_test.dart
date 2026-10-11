// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_security/plux_security.dart';

import 'support/fake_host.dart';

void main() {
  group('the response to root and hooking follows the setting', () {
    for (final response in PluxRaspResponse.values) {
      test('${response.name} [SEC-071]', () {
        final settings = settingsWithRaspResponse(response.name);
        for (final kind in [
          PluxDetectionKind.root,
          PluxDetectionKind.hooking,
        ]) {
          expect(const PluxRaspPolicy().responseFor(kind, settings), response);
        }
      });
    }

    test('each profile\'s default is the setting\'s default [SEC-071]', () {
      for (final (profile, response) in [
        ('standard', PluxRaspResponse.report),
        ('strict', PluxRaspResponse.degrade),
        ('maximum', PluxRaspResponse.block),
      ]) {
        final settings = SecuritySettings.fromDocument({'profile': profile});
        expect(
          const PluxRaspPolicy().responseFor(PluxDetectionKind.root, settings),
          response,
          reason: profile,
        );
      }
    });

    test('the app can make it stricter, never looser [SEC-071]', () {
      const policy = PluxRaspPolicy(
        responses: {
          PluxDetectionKind.root: PluxRaspResponse.block,
          PluxDetectionKind.hooking: PluxRaspResponse.report,
        },
      );
      final degrade = settingsWithRaspResponse('degrade');
      expect(
        policy.responseFor(PluxDetectionKind.root, degrade),
        PluxRaspResponse.block,
      );
      expect(
        policy.responseFor(PluxDetectionKind.hooking, degrade),
        PluxRaspResponse.degrade,
      );
    });

    test('a value the version does not know degrades [SEC-071]', () {
      expect(
        PluxRaspResponse.fromSetting('quarantine'),
        PluxRaspResponse.degrade,
      );
    });
  });

  group('every other kind follows its own setting', () {
    SecuritySettings with_(String key, String value) =>
        SecuritySettings.fromDocument({
          'overrides': {key: value},
        });

    for (final (kind, key) in [
      (PluxDetectionKind.debugger, 'raspDebuggerResponse'),
      (PluxDetectionKind.emulator, 'raspEmulatorResponse'),
      (PluxDetectionKind.repackaging, 'raspRepackagingResponse'),
      (PluxDetectionKind.untrustedInstaller, 'raspInstallerResponse'),
      (PluxDetectionKind.screenCapture, 'raspScreenCaptureResponse'),
      (PluxDetectionKind.overlay, 'raspOverlayResponse'),
      (PluxDetectionKind.accessibility, 'raspAccessibilityResponse'),
    ]) {
      test('${kind.name} reads $key [SEC-071]', () {
        for (final response in PluxRaspResponse.values) {
          expect(
            const PluxRaspPolicy().responseFor(kind, with_(key, response.name)),
            response,
          );
        }
        expect(PluxRaspPolicy.settingFor(kind).key, key);
      });
    }

    test('the profiles\' defaults reach the policy [SEC-071]', () {
      final strict = SecuritySettings.fromDocument({'profile': 'strict'});
      final maximum = SecuritySettings.fromDocument({'profile': 'maximum'});
      const policy = PluxRaspPolicy();
      expect(
        policy.responseFor(PluxDetectionKind.emulator, strict),
        PluxRaspResponse.warn,
      );
      expect(
        policy.responseFor(PluxDetectionKind.repackaging, maximum),
        PluxRaspResponse.block,
      );
      expect(
        policy.responseFor(PluxDetectionKind.overlay, SecuritySettings.builtIn),
        PluxRaspResponse.report,
      );
    });

    test('the app can tighten any kind, never loosen it [SEC-071]', () {
      const policy = PluxRaspPolicy(
        responses: {
          PluxDetectionKind.emulator: PluxRaspResponse.warn,
          PluxDetectionKind.debugger: PluxRaspResponse.report,
        },
      );
      expect(
        policy.responseFor(
          PluxDetectionKind.emulator,
          SecuritySettings.builtIn,
        ),
        PluxRaspResponse.warn,
      );
      expect(
        policy.responseFor(
          PluxDetectionKind.debugger,
          with_('raspDebuggerResponse', 'block'),
        ),
        PluxRaspResponse.block,
      );
    });
  });

  test('responses order from mild to strict [SEC-071]', () {
    expect(PluxRaspResponse.values.map((r) => r.name), [
      'report',
      'warn',
      'degrade',
      'block',
    ]);
    expect(
      PluxRaspResponse.warn.stricter(PluxRaspResponse.report),
      PluxRaspResponse.warn,
    );
    expect(
      PluxRaspResponse.warn.stricter(PluxRaspResponse.block),
      PluxRaspResponse.block,
    );
  });

  test(
    'every detection kind of SEC-070 has a distinct wire name [SEC-070]',
    () {
      final names = PluxDetectionKind.values.map((k) => k.wireName).toSet();
      expect(names, hasLength(PluxDetectionKind.values.length));
      expect(names, {
        'root',
        'hooking',
        'debugger',
        'emulator',
        'repackaging',
        'untrusted_installer',
        'screen_capture',
        'overlay',
        'accessibility',
      });
      for (final k in PluxDetectionKind.values) {
        expect(PluxDetectionKind.fromWire(k.wireName), k);
      }
      expect(PluxDetectionKind.fromWire('quantum'), isNull);
    },
  );
}
