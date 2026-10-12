// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/core/confidential_bundles.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/software_keys.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/downloader.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';
import 'package:plux_flutter/src/sync/sync_event.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';

import 'confidential_support.dart';
import 'fake_device.dart';
import 'fake_server.dart';

void main() {
  final goldens = Goldens.load();
  final demo = goldens.bundles['loan-calculator/demo.pxb']!;
  final loans = goldens.bundles['loan-calculator/loans.pxb']!;
  final key = Uint8List.fromList([for (var i = 0; i < 32; i++) 200 - i]);
  final logo = File(
    '../../schema/testdata/documents/loan-calculator/assets/images/logo.png',
  ).readAsBytesSync();

  late FakePluxServer server;
  late String root;
  late http.Client client;
  late SoftwareDeviceKeys keys;
  late MemoryCredentialStore credentials;

  setUp(() async {
    server = await FakePluxServer.start();
    server.serveObject('assets', logo);
    root = Directory.systemTemp.createTempSync('plux_conf_sync').path;
    client = http.Client();
    keys = SoftwareDeviceKeys();
    credentials = MemoryCredentialStore();
  });
  tearDown(() async {
    client.close();
    await server.close();
    Directory(root).deleteSync(recursive: true);
  });

  SyncEngine engine() => SyncEngine(
    config: SyncConfig(
      appId: FakePluxServer.app,
      environment: FakePluxServer.environment,
      channel: FakePluxServer.channel,
      keys: [server.trustedKey],
      device: const DeviceInfo(
        platform: 'android',
        osVersion: '14',
        runtimeVersion: '0.1.0',
        hostBuild: '1',
      ),
      supportsFeature: (_) => true,
      verifierLimits: const VerifierLimits(maxDepth: 64, maxVisits: 1000000),
    ),
    store: ReleaseStore.open(root),
    api: PluxApiClient(client, server.endpoint),
    downloader: Downloader(client, sleep: (_) async {}, random: Random(1)),
    credentials: credentials,
    keys: keys,
    attestation: FakeAttestation(),
  );

  Future<SyncResult> sync() => engine().run((_) {});

  FakeRelease release(int sequence) => FakeRelease(sequence, demo, {
    'loans': loans,
  }, sealed: SealedRelease.of({'': demo, 'loans': loans}, key));

  test('SEC-053 syncs a confidential release, stored encrypted with its key '
      '[SEC-052]', () async {
    server.release = release(7);
    final result = await sync();
    expect(result.outcome, SyncOutcome.staged, reason: '${result.error}');
    final store = ReleaseStore.open(root);
    final record = store.record(7)!;
    expect(record.bundles.every((b) => b.confidential), isTrue);
    expect(record.contentKey, isNotNull);
    expect(record.contentKey!.deviceId, 'dev_1');
    // The stored objects are the ciphertext the manifest names, not the
    // plain bundles: nothing readable is on disk.
    for (final b in record.bundles) {
      final stored = File(store.objectPath(ObjectKind.bundles, b.hash))
          .readAsBytesSync();
      expect(stored, server.release!.sealed!.bundles[b.key]);
      expect(stored, isNot(b.key.isEmpty ? demo : loans));
    }
    // The runtime then opens them with the device's key-agreement key.
    final opened = await ConfidentialBundles(
      keys: keys,
      appId: FakePluxServer.app,
      environment: FakePluxServer.environment,
    ).open(record, store);
    expect(opened[''], demo);
    expect(opened['loans'], loans);
  });

  test('SEC-053 a confidential bundle is always downloaded whole '
      '[SYN-011]', () async {
    server.release = release(7);
    await sync();
    server.release = FakeRelease(8, demo, {
      'loans': loans,
    }, sealed: SealedRelease.of({'': demo, 'loans': loans}, key));
    // The next release seals the same bundles again under the same key
    // from the same counter, so the bundles are the ones the device holds.
    final second = await sync();
    expect(second.outcome, SyncOutcome.staged, reason: '${second.error}');
    expect(
      server.requests.where((p) => p.contains('/deltas/')),
      isEmpty,
      reason: 'no delta is ever planned for ciphertext',
    );
  });

  test('SEC-052 a tampered download is refused before it is stored', () async {
    server.release = release(7);
    final hash = Goldens.hashOf(server.release!.sealed!.bundles['loans']!);
    server.faults['/v1/objects/bundles/${hash.substring(0, 2)}/$hash'] = [
      const CorruptFault(),
      const CorruptFault(),
      const CorruptFault(),
      const CorruptFault(),
    ];
    final result = await sync();
    expect(result.outcome, SyncOutcome.failed);
    final store = ReleaseStore.open(root);
    expect(store.hasObject(ObjectKind.bundles, hash), isFalse);
    expect(store.pointer.staged, isNull);
  });

  test(
    'SEC-053 a server that sends no content key fails with PLX-6050',
    () async {
      server.release = release(7);
      server.withholdContentKey = true;
      final result = await sync();
      expect(result.outcome, SyncOutcome.failed);
      expect(result.error!.code, PluxErrorCode.confidentialKeyUnavailable);
      expect(ReleaseStore.open(root).pointer.staged, isNull);
    },
  );

  test('SEC-053 a content key for another release does not open it', () async {
    server.release = release(7);
    server.contentKeySequence = 99;
    final result = await sync();
    expect(result.outcome, SyncOutcome.failed);
    expect(result.error!.code, PluxErrorCode.confidentialKeyUnavailable);
  });

  test(
    'SEC-053 an unsupported runtime refuses the release before downloading',
    () async {
      server.release = release(7);
      final e = SyncEngine(
        config: SyncConfig(
          appId: FakePluxServer.app,
          environment: FakePluxServer.environment,
          channel: FakePluxServer.channel,
          keys: [server.trustedKey],
          device: const DeviceInfo(
            platform: 'android',
            osVersion: '14',
            runtimeVersion: '0.1.0',
            hostBuild: '1',
          ),
          supportsFeature: (f) => f != 'security.confidential.v1',
          verifierLimits: const VerifierLimits(
            maxDepth: 64,
            maxVisits: 1000000,
          ),
        ),
        store: ReleaseStore.open(root),
        api: PluxApiClient(client, server.endpoint),
        downloader: Downloader(client, sleep: (_) async {}, random: Random(1)),
        credentials: credentials,
        keys: keys,
        attestation: FakeAttestation(),
      );
      final result = await e.run((_) {});
      expect(result.outcome, SyncOutcome.failed);
      expect(result.error!.code, PluxErrorCode.unsupportedRequiredFeature);
      expect(
        server.requests.where((p) => p.startsWith('/v1/objects/bundles')),
        isEmpty,
      );
    },
  );
}
