// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/sync/api_client.dart' show DeviceInfo;
import 'package:plux_flutter/src/sync/background_sync.dart';
import 'package:plux_flutter/src/sync/background_sync_entry.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';
import 'package:plux_flutter/src/sync/sync_worker.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

import '../store/store_test_support.dart';
import 'fake_device.dart';
import 'fake_server.dart';

bool _all(String _) => true;

const _pinA = 'AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=';
const _pinB = 'AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=';

/// A platform scheduler that records its calls.
final class _RecordingPlatform implements BackgroundSyncPlatform {
  final List<BackgroundSyncRequest> scheduled = [];
  int cancelled = 0;
  PluxException? failure;

  @override
  Future<void> schedule(BackgroundSyncRequest request) async {
    final f = failure;
    if (f != null) throw f;
    scheduled.add(request);
  }

  @override
  Future<void> cancel() async => cancelled++;
}

BackgroundSyncDescriptor _descriptor(String root) => BackgroundSyncDescriptor(
  storeRoot: root,
  endpoint: Uri.parse('https://plux.example/'),
  appId: 'app',
  environment: 'production',
  channel: 'production',
  keys: [
    TrustedKey(
      keyId: 'k1',
      algorithm: 'ed25519',
      role: 'targets',
      publicKey: Uint8List.fromList([1, 2, 3]),
    ),
  ],
  rootDocument: Uint8List.fromList([9, 8]),
  pins: const [_pinA, _pinB],
  hostBuild: '42',
  parallelism: 2,
  diskQuota: 1000,
  credentialName: 'device.app.production',
  locale: 'en-US',
  pixelRatio: 2.5,
  avif: true,
  cloudProjectNumber: 123,
  enrolSca: true,
  deviceFunctions: true,
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  group('the configuration', () {
    test('is off by default and the interval defaults to 360 minutes '
        '[SYN-014]', () {
      const config = BackgroundSyncConfig();
      expect(config.enabled, isFalse);
      expect(config.intervalMinutes, 360);
      expect(const BackgroundSyncConfig.enabled().enabled, isTrue);
      expect(
        PluxConfig(
          appId: 'a',
          endpoint: Uri.parse('https://plux.example/'),
        ).backgroundSync.enabled,
        isFalse,
      );
    });

    test('clamps the interval to at least 15 minutes [SYN-014]', () {
      int minutes(Duration d) =>
          BackgroundSyncConfig(enabled: true, interval: d).intervalMinutes;
      expect(minutes(const Duration(minutes: 1)), 15);
      expect(minutes(Duration.zero), 15);
      expect(minutes(const Duration(minutes: -5)), 15);
      expect(minutes(const Duration(minutes: 14, seconds: 59)), 15);
      expect(minutes(const Duration(minutes: 15)), 15);
      expect(minutes(const Duration(hours: 12)), 720);
    });

    test('carries the constraints to the platform [SYN-014]', () {
      final request = BackgroundSyncRequest(
        callbackHandle: 7,
        descriptor: '{}',
        config: const BackgroundSyncConfig(
          enabled: true,
          interval: Duration(minutes: 5),
          requiresUnmeteredNetwork: true,
          requiresCharging: true,
          requiresBatteryNotLow: false,
        ),
      );
      expect(request.toArguments(), {
        'callback': 7,
        'descriptor': '{}',
        'intervalMinutes': 15,
        'requiresUnmeteredNetwork': true,
        'requiresCharging': true,
        'requiresBatteryNotLow': false,
      });
    });
  });

  group('the descriptor', () {
    test('survives its JSON form and holds no secret [SYN-014]', () {
      final d = _descriptor('/store');
      final back = BackgroundSyncDescriptor.fromJson(d.toJson());
      expect(back.toJson(), d.toJson());
      expect(back.keys.single.publicKey, [1, 2, 3]);
      expect(back.rootDocument, [9, 8]);
      expect(back.pins, [_pinA, _pinB]);
      expect(back.cloudProjectNumber, 123);
      expect(back.endpoint, Uri.parse('https://plux.example/'));
      expect(d.toJson(), isNot(contains('token')));
      expect(d.toJson(), isNot(contains('secret')));
    });

    test('refuses what is not a descriptor with PLX-3500 [SYN-014]', () {
      expect(
        () => BackgroundSyncDescriptor.fromJson('{"appId": 1}'),
        throwsA(
          isA<PluxException>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.backgroundSyncFailed,
          ),
        ),
      );
    });
  });

  group('the scheduler', () {
    late Directory dir;
    late _RecordingPlatform platform;
    late BackgroundSyncScheduler scheduler;

    setUp(() {
      dir = Directory.systemTemp.createTempSync('plux_bg');
      platform = _RecordingPlatform();
      scheduler = BackgroundSyncScheduler(
        platform: platform,
        storeRoot: dir.path,
      );
    });
    tearDown(() => dir.deleteSync(recursive: true));

    Future<void> apply(BackgroundSyncConfig config) => scheduler.apply(
      config,
      descriptor: () => _descriptor(dir.path),
      callbackHandle: () => 11,
    );

    test('a disabled configuration schedules nothing and calls nothing '
        '[SYN-014]', () async {
      await apply(const BackgroundSyncConfig());
      expect(platform.scheduled, isEmpty);
      expect(platform.cancelled, 0);
    });

    test(
      'an enabled configuration schedules once per start [SYN-014]',
      () async {
        await apply(
          const BackgroundSyncConfig(
            enabled: true,
            interval: Duration(minutes: 1),
          ),
        );
        final request = platform.scheduled.single;
        expect(request.callbackHandle, 11);
        expect(request.config.intervalMinutes, 15);
        expect(
          BackgroundSyncDescriptor.fromJson(request.descriptor).storeRoot,
          dir.path,
        );
      },
    );

    test('turning it off again cancels the task once [SYN-014]', () async {
      await apply(const BackgroundSyncConfig.enabled());
      await apply(const BackgroundSyncConfig());
      await apply(const BackgroundSyncConfig());
      expect(platform.cancelled, 1);
    });

    test(
      'a refusal reaches the caller and leaves no marker [SYN-014]',
      () async {
        platform.failure = const PluxException(
          PluxErrorCode.backgroundSyncNotConfigured,
          'not declared',
        );
        await expectLater(
          apply(const BackgroundSyncConfig.enabled()),
          throwsA(
            isA<PluxException>().having(
              (e) => e.code,
              'code',
              PluxErrorCode.backgroundSyncNotConfigured,
            ),
          ),
        );
        await apply(const BackgroundSyncConfig());
        expect(platform.cancelled, 0);
      },
    );
  });

  group('the platform channel', () {
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    const channel = MethodChannel('dev.plux/runtime');

    setUp(() => debugDefaultTargetPlatformOverride = TargetPlatform.android);
    tearDown(() {
      debugDefaultTargetPlatformOverride = null;
      messenger.setMockMethodCallHandler(channel, null);
    });

    test('sends the request and the cancellation [SYN-014]', () async {
      final calls = <MethodCall>[];
      messenger.setMockMethodCallHandler(channel, (call) async {
        calls.add(call);
        return null;
      });
      const platform = ChannelBackgroundSyncPlatform();
      await platform.schedule(
        const BackgroundSyncRequest(
          callbackHandle: 3,
          descriptor: '{}',
          config: BackgroundSyncConfig.enabled(),
        ),
      );
      await platform.cancel();
      expect(calls.map((c) => c.method), [
        'backgroundSyncSchedule',
        'backgroundSyncCancel',
      ]);
      expect((calls.first.arguments as Map)['intervalMinutes'], 360);
    });

    test('reports PLX-3501 when the host did not declare the task '
        '[SYN-014]', () async {
      messenger.setMockMethodCallHandler(channel, (call) async {
        throw PlatformException(code: 'PLX-3501');
      });
      await expectLater(
        const ChannelBackgroundSyncPlatform().schedule(
          const BackgroundSyncRequest(
            callbackHandle: 3,
            descriptor: '{}',
            config: BackgroundSyncConfig.enabled(),
          ),
        ),
        throwsA(
          isA<PluxException>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.backgroundSyncNotConfigured,
          ),
        ),
      );
    });

    test('reports PLX-3500 for any other platform failure [SYN-014]', () async {
      messenger.setMockMethodCallHandler(channel, (call) async {
        throw PlatformException(code: 'PLUX_PLATFORM');
      });
      await expectLater(
        const ChannelBackgroundSyncPlatform().cancel(),
        throwsA(
          isA<PluxException>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.backgroundSyncFailed,
          ),
        ),
      );
    });

    test('reports PLX-3501 where the platform has no scheduler [SYN-014]', () {
      debugDefaultTargetPlatformOverride = TargetPlatform.linux;
      expect(
        const ChannelBackgroundSyncPlatform().cancel(),
        throwsA(
          isA<PluxException>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.backgroundSyncNotConfigured,
          ),
        ),
      );
    });
  });

  group('the headless run', () {
    late Goldens goldens;
    late FakePluxServer server;
    late String root;

    setUp(() async {
      goldens = Goldens.load();
      server = await FakePluxServer.start();
      root = Directory.systemTemp.createTempSync('plux_bg_run').path;
    });
    tearDown(() async {
      await server.close();
      Directory(root).deleteSync(recursive: true);
    });

    SyncWorkerConfig workerConfig({TrustedKey? key}) => SyncWorkerConfig(
      storeRoot: root,
      endpoint: server.endpoint,
      httpClient: http.Client.new,
      credentials: MemoryCredentialStore.new,
      deviceKeys: FakeDeviceKeys.new,
      attestation: FakeAttestation.new,
      sync: SyncConfig(
        appId: FakePluxServer.app,
        environment: FakePluxServer.environment,
        channel: FakePluxServer.channel,
        keys: [key ?? server.trustedKey],
        device: const DeviceInfo(
          platform: 'android',
          osVersion: '14',
          runtimeVersion: '0.6.0',
          hostBuild: '1',
        ),
        supportsFeature: _all,
        verifierLimits: const VerifierLimits(maxDepth: 64, maxVisits: 1000000),
      ),
      baseline: (_) async => null,
    );

    FakeRelease release(int sequence) => FakeRelease(
      sequence,
      goldens.bundles['loan-calculator/demo.pxb']!,
      {'loans': goldens.bundles['loan-calculator/loans.pxb']!},
    );

    test('verifies a release and stages it without activating it '
        '[SYN-014] [SEC-052] [SYN-004]', () async {
      server.release = release(10);
      final report = await BackgroundSyncRunner(workerConfig()).run();
      expect(
        report.outcome,
        BackgroundSyncOutcome.staged,
        reason: '${report.error}',
      );
      expect(report.sequence, 10);
      final pointer = ReleaseStore.open(root).pointer;
      expect(pointer.staged, 10);
      expect(
        pointer.active,
        isNull,
        reason: 'a background run never activates',
      );
      expect(pointer.lastKnownGood, isNull);
      expect(report.toArguments(), {'outcome': 'staged', 'sequence': 10});
    });

    test('keeps the active release when it stages a newer one '
        '[SYN-014] [SYN-004]', () async {
      server.release = release(10);
      final worker = await SyncWorker.start(workerConfig());
      await worker.sync((_) {});
      await worker.activate();
      await worker.close();
      expect(checkComplete(root), 10);

      server.release = release(11);
      final report = await BackgroundSyncRunner(workerConfig()).run();
      expect(
        report.outcome,
        BackgroundSyncOutcome.staged,
        reason: '${report.error}',
      );
      final pointer = ReleaseStore.open(root).pointer;
      expect(pointer.active, 10);
      expect(pointer.staged, 11);
      expect(checkComplete(root), 10, reason: 'the active release is intact');
    });

    test('reports up to date when nothing is newer [SYN-014]', () async {
      server.release = release(10);
      await BackgroundSyncRunner(workerConfig()).run();
      final report = await BackgroundSyncRunner(workerConfig()).run();
      expect(report.outcome, BackgroundSyncOutcome.upToDate);
      expect(ReleaseStore.open(root).pointer.staged, 10);
    });

    test('refuses to stage a release that fails verification with PLX-3503 '
        '[SYN-014] [SEC-052]', () async {
      server.release = release(10);
      final trusted = server.trustedKey;
      final other = TrustedKey(
        keyId: trusted.keyId,
        algorithm: trusted.algorithm,
        role: trusted.role,
        publicKey: Uint8List.fromList([
          for (final b in trusted.publicKey) b ^ 0x5a,
        ]),
      );
      final report = await BackgroundSyncRunner(workerConfig(key: other)).run();
      expect(report.outcome, BackgroundSyncOutcome.rejected);
      expect(report.error!.code, PluxErrorCode.stagedReleaseVerificationFailed);
      expect(report.error!.details['cause'], 'PLX-3001');
      expect(report.toArguments()['code'], 'PLX-3503');
      final pointer = ReleaseStore.open(root).pointer;
      expect(pointer.staged, isNull);
      expect(pointer.active, isNull);
    });

    test('an expired manifest is a verification failure too [SYN-014] '
        '[SEC-052]', () async {
      server.release = release(10);
      server.expires = DateTime.utc(2000);
      final report = await BackgroundSyncRunner(workerConfig()).run();
      expect(report.outcome, BackgroundSyncOutcome.rejected);
      expect(report.error!.code, PluxErrorCode.stagedReleaseVerificationFailed);
      expect(ReleaseStore.open(root).pointer.staged, isNull);
    });

    test(
      'a server that is down is PLX-3500, not a rejection [SYN-014]',
      () async {
        server.release = release(10);
        server.faults['/plux.v1.ManifestService/GetManifest'] = [
          const StatusFault(503),
          const StatusFault(503),
          const StatusFault(503),
          const StatusFault(503),
        ];
        final report = await BackgroundSyncRunner(workerConfig()).run();
        expect(report.outcome, BackgroundSyncOutcome.failed);
        expect(report.error!.code, PluxErrorCode.backgroundSyncFailed);
        expect(ReleaseStore.open(root).pointer.staged, isNull);
      },
    );

    test('never throws when the worker cannot start [SYN-014]', () async {
      final report = await BackgroundSyncRunner(
        workerConfig(),
        startWorker: (_) async => throw StateError('no isolate'),
      ).run();
      expect(report.outcome, BackgroundSyncOutcome.failed);
      expect(report.error!.code, PluxErrorCode.backgroundSyncFailed);
      expect(report.error!.message, isNot(contains('no isolate')));
    });

    test(
      'fails the run if the active release changed under it [SYN-004]',
      () async {
        server.release = release(10);
        var reads = 0;
        final report = await BackgroundSyncRunner(
          workerConfig(),
          readPointer: (_) {
            reads++;
            return reads == 1
                ? ReleaseStore.open(root).pointer
                : ReleaseStore.open(root).pointer.copyWith(active: 99);
          },
        ).run();
        expect(report.outcome, BackgroundSyncOutcome.failed);
        expect(report.error!.code, PluxErrorCode.backgroundSyncFailed);
      },
    );
  });

  group('the entry point', () {
    test('builds the sync worker from the descriptor [SYN-014]', () {
      final dir = Directory.systemTemp.createTempSync('plux_bg_cfg');
      addTearDown(() => dir.deleteSync(recursive: true));
      final config = backgroundWorkerConfig(_descriptor(dir.path));
      expect(config.storeRoot, dir.path);
      expect(config.parallelism, 2);
      expect(config.sync.appId, 'app');
      expect(config.sync.keys.single.keyId, 'k1');
      expect(config.sync.assets.pixelRatio, 2.5);
      expect(config.sync.assets.avif, isTrue);
      expect(config.sync.locale, 'en-US');
      expect(config.sync.diskQuota, 1000);
      expect(config.sync.enrolSca, isTrue);
      expect(config.sync.pins!.pins, {_pinA, _pinB});
    });

    test('reports a descriptor it cannot read instead of throwing '
        '[SYN-014]', () async {
      final report = await runBackgroundSync('not json');
      expect(report.outcome, BackgroundSyncOutcome.failed);
      expect(report.error!.code, PluxErrorCode.backgroundSyncFailed);
    });

    group('the platform entry', () {
      final messenger =
          TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      const channel = MethodChannel('dev.plux/background-sync');
      tearDown(() => messenger.setMockMethodCallHandler(channel, null));

      /// Runs the entry point against a platform whose `descriptor` answers
      /// [descriptor]; the arguments of the `done` it receives.
      Future<Map<Object?, Object?>> entry(
        Future<Object?> Function() descriptor,
      ) async {
        Map<Object?, Object?>? done;
        messenger.setMockMethodCallHandler(channel, (call) async {
          if (call.method == 'descriptor') return descriptor();
          done = call.arguments as Map<Object?, Object?>;
          return null;
        });
        await pluxBackgroundSyncMain();
        return done!;
      }

      test('tells the platform it has no descriptor with PLX-3501 '
          '[SYN-014]', () async {
        final done = await entry(() async => null);
        expect(done, {'outcome': 'failed', 'code': 'PLX-3501'});
      });

      test('tells the platform about a descriptor it cannot read '
          '[SYN-014]', () async {
        final done = await entry(() async => 'not json');
        expect(done, {'outcome': 'failed', 'code': 'PLX-3500'});
      });

      test(
        'tells the platform when the descriptor call fails [SYN-014]',
        () async {
          final done = await entry(
            () async => throw PlatformException(code: 'PLUX_PLATFORM'),
          );
          expect(done, {'outcome': 'failed', 'code': 'PLX-3500'});
        },
      );
    });
  });
}
