// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/store/pointer.dart';
import 'package:plux_flutter/src/sync/api_client.dart' show DeviceInfo;
import 'package:plux_flutter/src/sync/background_sync.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';
import 'package:plux_flutter/src/sync/sync_lock.dart';
import 'package:plux_flutter/src/sync/sync_worker.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';

/// A lock whose calls fail.
final class _BrokenLock implements SyncLock {
  int releases = 0;

  @override
  Future<bool> tryAcquire() async => throw StateError('no lock');

  @override
  Future<void> release() async => releases++;
}

SyncWorkerConfig _config() => SyncWorkerConfig(
  storeRoot: '/nonexistent',
  endpoint: Uri.parse('https://plux.example/'),
  httpClient: () => throw StateError('unused'),
  credentials: () => throw StateError('unused'),
  deviceKeys: () => throw StateError('unused'),
  attestation: () => throw StateError('unused'),
  sync: SyncConfig(
    appId: 'app',
    environment: 'production',
    channel: 'production',
    keys: const [],
    device: const DeviceInfo(
      platform: 'android',
      osVersion: '14',
      runtimeVersion: '0.6.0',
      hostBuild: '1',
    ),
    supportsFeature: (_) => true,
    verifierLimits: const VerifierLimits(maxDepth: 64, maxVisits: 1000000),
  ),
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('the background run and the sync lock', () {
    BackgroundSyncRunner runner(
      SyncLock lock, {
      Future<SyncWorker> Function(SyncWorkerConfig)? startWorker,
    }) => BackgroundSyncRunner(
      _config(),
      lock: lock,
      startWorker: startWorker,
      readPointer: (_) => StorePointer.empty,
    );

    test('skips, without an error, when a foreground sync holds the lock '
        '[SYN-014]', () async {
      final lock = InMemorySyncLock();
      await lock.tryAcquire();
      var started = false;
      final report = await runner(
        lock,
        startWorker: (_) async {
          started = true;
          throw StateError('must not start');
        },
      ).run();
      expect(report.outcome, BackgroundSyncOutcome.skipped);
      expect(report.error, isNull);
      expect(report.toArguments(), {'outcome': 'skipped'});
      expect(started, isFalse);
      expect(lock.held, isTrue, reason: 'the holder is not the runner');
    });

    test(
      'takes the lock and releases it when the run fails [SYN-014]',
      () async {
        final lock = InMemorySyncLock();
        var heldDuringRun = false;
        final report = await runner(
          lock,
          startWorker: (_) async {
            heldDuringRun = lock.held;
            throw StateError('no isolate');
          },
        ).run();
        expect(report.outcome, BackgroundSyncOutcome.failed);
        expect(heldDuringRun, isTrue);
        expect(lock.held, isFalse);
      },
    );

    test(
      'fails, never throws, when the lock cannot be asked [SYN-014]',
      () async {
        final report = await runner(_BrokenLock()).run();
        expect(report.outcome, BackgroundSyncOutcome.failed);
        expect(report.error!.code, PluxErrorCode.backgroundSyncFailed);
      },
    );
  });

  group('the foreground and the sync lock', () {
    ForegroundSyncLock foreground(
      SyncLock lock, {
      List<String>? warnings,
      Duration timeout = const Duration(milliseconds: 200),
    }) => ForegroundSyncLock(
      lock,
      retryInterval: const Duration(milliseconds: 10),
      timeout: timeout,
      onWarning: warnings?.add,
    );

    test(
      'holds the lock while it runs and releases it after [SYN-014]',
      () async {
        final lock = InMemorySyncLock();
        final held = await foreground(lock).guard(() async => lock.held);
        expect(held, isTrue);
        expect(lock.held, isFalse);
      },
    );

    test(
      'waits for a background run to end, then proceeds [SYN-014]',
      () async {
        final lock = InMemorySyncLock();
        await lock.tryAcquire();
        Future<void>.delayed(const Duration(milliseconds: 50), lock.release);
        final warnings = <String>[];
        var ranWith = false;
        await foreground(lock, warnings: warnings).guard(() async {
          ranWith = lock.held;
        });
        expect(ranWith, isTrue, reason: 'it took the lock once it was free');
        expect(warnings, isEmpty);
        expect(lock.held, isFalse);
      },
    );

    test('proceeds with a warning when the lock stays busy, and leaves the '
        'other holder alone [SYN-014]', () async {
      final lock = InMemorySyncLock();
      await lock.tryAcquire();
      final warnings = <String>[];
      var ran = false;
      await foreground(lock, warnings: warnings).guard(() async => ran = true);
      expect(ran, isTrue);
      expect(warnings, hasLength(1));
      expect(lock.held, isTrue, reason: 'it never took the lock');
    });

    test('proceeds with a warning when the lock cannot be asked '
        '[SYN-014]', () async {
      final warnings = <String>[];
      var ran = false;
      await foreground(
        _BrokenLock(),
        warnings: warnings,
      ).guard(() async => ran = true);
      expect(ran, isTrue);
      expect(warnings, hasLength(1));
    });

    test('releases when the run throws [SYN-014]', () async {
      final lock = InMemorySyncLock();
      await expectLater(
        foreground(lock).guard<void>(() async => throw StateError('x')),
        throwsStateError,
      );
      expect(lock.held, isFalse);
    });

    test('lets overlapping runs share one hold [SYN-014]', () async {
      final lock = InMemorySyncLock();
      final guard = foreground(lock);
      final gate = Future<void>.delayed(const Duration(milliseconds: 30));
      final a = guard.guard(() => gate);
      final b = guard.guard(() async => lock.held);
      expect(await b, isTrue);
      await a;
      expect(lock.held, isFalse);
    });
  });

  group('the channel lock', () {
    test('is free on a platform without the plugin [SYN-014]', () async {
      const lock = ChannelSyncLock();
      expect(await lock.tryAcquire(), isTrue);
      await lock.release();
    });

    test('calls the native lock over the runtime channel [SYN-014]', () async {
      final calls = <String>[];
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(const MethodChannel('dev.plux/runtime'), (
            call,
          ) async {
            calls.add(call.method);
            return call.method == 'syncLock.tryAcquire' ? false : null;
          });
      addTearDown(
        () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
            .setMockMethodCallHandler(
              const MethodChannel('dev.plux/runtime'),
              null,
            ),
      );
      const lock = ChannelSyncLock();
      expect(await lock.tryAcquire(), isFalse);
      await lock.release();
      expect(calls, ['syncLock.tryAcquire', 'syncLock.release']);
    });
  });
}
