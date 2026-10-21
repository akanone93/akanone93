// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/device_targeting.dart';
import 'package:plux_flutter/src/sync/downloader.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';

import 'fake_device.dart';
import 'fake_server.dart';

const _device = DeviceInfo(
  platform: 'ios',
  osVersion: '17.2',
  runtimeVersion: '0.1.0',
  hostBuild: '42',
);

const _context = SyncContext(
  deviceClass: 'tablet',
  locale: 'de-CH',
  region: 'CH',
  userId: 'user-9',
  userAttributes: {'tier': 'gold'},
);

/// What the device tells the server for rollout targeting (REL-051) and for
/// a reconstruction (SEC-142).
void main() {
  group('hashes', () {
    test('a user attribute is the hex SHA-256 of its name, a zero byte and '
        'its value, as the server hashes it [REL-051]', () {
      expect(
        hashUserAttribute('tier', 'gold'),
        '24b20542c515da63790c6560a90ae13e744225c4b52d2b8446b34c0d2b08554a',
      );
      expect(
        hashUserAttribute('region', 'gold'),
        isNot(hashUserAttribute('tier', 'gold')),
        reason: 'the name keeps equal values of different attributes apart',
      );
    });

    test('the user identifier is the hex SHA-256 of the identifier '
        '[REL-051]', () {
      expect(
        hashUserId('user-9'),
        '14817f52178f6bc3d9bde29d8e163b179bd99de06cb908953ac31369377c759f',
      );
    });
  });

  group('the targeting of a manifest request', () {
    test('carries the platform, runtime, class, locale and region, and only '
        'hashes of the user [REL-051]', () {
      final json = DeviceTargeting.of(_device, _context).toJson();
      expect(json, {
        'os': 'ios',
        'osVersion': '17.2',
        'runtimeVersion': '0.1.0',
        'deviceClass': 'tablet',
        'locale': 'de-CH',
        'region': 'CH',
        'userHash': hashUserId('user-9'),
        'userAttributes': {'tier': hashUserAttribute('tier', 'gold')},
      });
      final text = jsonEncode(json);
      expect(text, isNot(contains('user-9')));
      expect(text, isNot(contains('gold')));
    });

    test('leaves out what the device does not know, and the user when none is '
        'set [REL-051]', () {
      expect(DeviceTargeting.of(_device, const SyncContext()).toJson(), {
        'os': 'ios',
        'osVersion': '17.2',
        'runtimeVersion': '0.1.0',
      });
      expect(
        DeviceTargeting.of(
          _device,
          const SyncContext(userId: ''),
        ).toJson().containsKey('userHash'),
        isFalse,
      );
    });

    test(
      'is sent with the manifest request of the API client [REL-051]',
      () async {
        final bodies = <Map<String, Object?>>[];
        final client = PluxApiClient(
          MockClient((req) async {
            bodies.add(jsonDecode(req.body) as Map<String, Object?>);
            return http.Response(
              jsonEncode({'notModified': true, 'etag': '"e"'}),
              200,
            );
          }),
          Uri.parse('https://x'),
        );
        Future<void> ask({DeviceTargeting? targeting}) => client.manifest(
          token: fakeToken('t'),
          appId: 'a',
          environment: 'e',
          channel: 'c',
          installedSequence: 0,
          installed: const {},
          ifNoneMatch: '',
          targeting: targeting,
        );
        await ask(targeting: DeviceTargeting.of(_device, _context));
        await ask();
        expect(
          bodies.first['deviceTargeting'],
          DeviceTargeting.of(_device, _context).toJson(),
        );
        expect(bodies.last.containsKey('deviceTargeting'), isFalse);
      },
    );

    test('is sent by a sync of the engine, from the context the runtime '
        'gives it, and never in the clear [REL-051]', () async {
      final goldens = Goldens.load();
      final server = await FakePluxServer.start();
      final root = Directory.systemTemp.createTempSync('plux_targeting').path;
      final client = http.Client();
      addTearDown(() async {
        client.close();
        await server.close();
        Directory(root).deleteSync(recursive: true);
      });
      server.release = FakeRelease(
        10,
        goldens.bundles['loan-calculator/demo.pxb']!,
        {'loans': goldens.bundles['loan-calculator/loans.pxb']!},
      );
      final engine = SyncEngine(
        config: SyncConfig(
          appId: FakePluxServer.app,
          environment: FakePluxServer.environment,
          channel: FakePluxServer.channel,
          keys: [server.trustedKey],
          device: _device,
          supportsFeature: (_) => true,
          verifierLimits: const VerifierLimits(
            maxDepth: 64,
            maxVisits: 1000000,
          ),
        ),
        store: ReleaseStore.open(root),
        api: PluxApiClient(client, server.endpoint),
        downloader: Downloader(client),
        credentials: MemoryCredentialStore(),
        keys: FakeDeviceKeys(),
        attestation: FakeAttestation(),
      );
      await engine.run((_) {}, context: _context);
      expect(server.deviceTargetingSeen.single, {
        'os': 'ios',
        'osVersion': '17.2',
        'runtimeVersion': '0.1.0',
        'deviceClass': 'tablet',
        'locale': 'de-CH',
        'region': 'CH',
        'userHash': hashUserId('user-9'),
        'userAttributes': {'tier': hashUserAttribute('tier', 'gold')},
      });
    });
  });

  group('the installed-release report', () {
    test('carries the theme, the text scale and the user reference '
        '[SEC-142]', () async {
      final bodies = <Map<String, Object?>>[];
      final client = PluxApiClient(
        MockClient((req) async {
          bodies.add(jsonDecode(req.body) as Map<String, Object?>);
          return http.Response('{}', 200);
        }),
        Uri.parse('https://x'),
      );
      await client.reportInstalled(
        fakeToken('t'),
        'd',
        3,
        context: const InstalledContext(
          theme: 'dark',
          textScalePercent: 130,
          userRef: 'user-9',
        ),
      );
      await client.reportInstalled(fakeToken('t'), 'd', 4);
      expect(bodies, [
        {
          'deviceId': 'd',
          'releaseSequence': '3',
          'theme': 'dark',
          'textScalePercent': 130,
          'userRef': 'user-9',
        },
        {'deviceId': 'd', 'releaseSequence': '4'},
      ]);
    });
  });
}
