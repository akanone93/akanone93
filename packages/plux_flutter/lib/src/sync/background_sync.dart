// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Opt-in background sync (SYN-014, ADR-0092): the descriptor and the
/// request the foreground sends to the platform's scheduler, and the
/// headless run that fetches, verifies and stages a release.
///
/// A background run never activates a release (SYN-004): activation stays
/// with the next start or safe point of the foreground runtime. What it
/// stages has been verified by the same sync worker as a foreground sync,
/// before anything is parsed beyond the container header (SEC-052).
library;

import 'dart:convert';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/platform/runtime_channel.dart';
import 'package:plux_flutter/src/store/pointer.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/sync/background_sync_config.dart';
import 'package:plux_flutter/src/sync/sync_event.dart';
import 'package:plux_flutter/src/sync/sync_worker.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

/// What a headless run needs, written when the foreground schedules the
/// task and read by the background engine, which does not run the host's
/// `main`. It holds public data only: identifiers, the embedded public keys
/// and pins. No secret, token or key is kept in it.
final class BackgroundSyncDescriptor {
  /// Creates the descriptor.
  const BackgroundSyncDescriptor({
    required this.storeRoot,
    required this.endpoint,
    required this.appId,
    required this.environment,
    required this.channel,
    required this.keys,
    required this.pins,
    required this.hostBuild,
    required this.parallelism,
    required this.diskQuota,
    required this.credentialName,
    required this.locale,
    required this.pixelRatio,
    required this.avif,
    this.rootDocument,
    this.cloudProjectNumber,
    this.enrolSca = false,
    this.deviceFunctions = false,
  });

  /// Reads the descriptor [toJson] wrote; a [PluxException] when it is not
  /// one.
  factory BackgroundSyncDescriptor.fromJson(String json) {
    try {
      final m = jsonDecode(json) as Map<String, Object?>;
      final root = m['rootDocument'] as String?;
      return BackgroundSyncDescriptor(
        storeRoot: m['storeRoot']! as String,
        endpoint: Uri.parse(m['endpoint']! as String),
        appId: m['appId']! as String,
        environment: m['environment']! as String,
        channel: m['channel']! as String,
        keys: [
          for (final k
              in (m['keys']! as List<Object?>).cast<Map<String, Object?>>())
            TrustedKey(
              keyId: k['keyId']! as String,
              algorithm: k['algorithm']! as String,
              role: k['role']! as String,
              publicKey: base64.decode(k['publicKey']! as String),
            ),
        ],
        pins: [...(m['pins']! as List<Object?>).cast<String>()],
        hostBuild: m['hostBuild']! as String,
        parallelism: m['parallelism']! as int,
        diskQuota: m['diskQuota']! as int,
        credentialName: m['credentialName']! as String,
        locale: m['locale']! as String,
        pixelRatio: (m['pixelRatio']! as num).toDouble(),
        avif: m['avif']! as bool,
        rootDocument: root == null ? null : base64.decode(root),
        cloudProjectNumber: m['cloudProjectNumber'] as int?,
        enrolSca: m['enrolSca']! as bool,
        deviceFunctions: m['deviceFunctions']! as bool,
      );
    } on Object {
      throw const PluxException(
        PluxErrorCode.backgroundSyncFailed,
        'the background sync descriptor cannot be read',
      );
    }
  }

  /// The release store's directory.
  final String storeRoot;

  /// The Plux server.
  final Uri endpoint;

  /// The app.
  final String appId;

  /// The environment key.
  final String environment;

  /// The channel key.
  final String channel;

  /// The embedded public keys (SEC-051).
  final List<TrustedKey> keys;

  /// The embedded root document, the anchor of the update metadata.
  final Uint8List? rootDocument;

  /// The pins of the server (SEC-041), as `PluxConfig.pins`.
  final List<String> pins;

  /// The host app's build.
  final String hostBuild;

  /// Downloads at once.
  final int parallelism;

  /// The disk quota cap in bytes.
  final int diskQuota;

  /// The name of the device credential in the platform's secret storage.
  final String credentialName;

  /// The device's locale, as a BCP 47 tag.
  final String locale;

  /// The device pixel ratio, which picks the asset files to fetch.
  final double pixelRatio;

  /// Whether the engine decodes AVIF here.
  final bool avif;

  /// The Play Integrity cloud project number.
  final int? cloudProjectNumber;

  /// Whether registration also creates the SCA key.
  final bool enrolSca;

  /// Whether the host registered a device function engine.
  final bool deviceFunctions;

  /// The descriptor as JSON.
  String toJson() => jsonEncode({
    'storeRoot': storeRoot,
    'endpoint': endpoint.toString(),
    'appId': appId,
    'environment': environment,
    'channel': channel,
    'keys': [
      for (final k in keys)
        {
          'keyId': k.keyId,
          'algorithm': k.algorithm,
          'role': k.role,
          'publicKey': base64.encode(k.publicKey),
        },
    ],
    'rootDocument': ?(rootDocument == null
        ? null
        : base64.encode(rootDocument!)),
    'pins': pins,
    'hostBuild': hostBuild,
    'parallelism': parallelism,
    'diskQuota': diskQuota,
    'credentialName': credentialName,
    'locale': locale,
    'pixelRatio': pixelRatio,
    'avif': avif,
    'cloudProjectNumber': ?cloudProjectNumber,
    'enrolSca': enrolSca,
    'deviceFunctions': deviceFunctions,
  });
}

/// What the foreground asks the platform scheduler for.
final class BackgroundSyncRequest {
  /// Creates the request.
  const BackgroundSyncRequest({
    required this.callbackHandle,
    required this.descriptor,
    required this.config,
  });

  /// The raw handle of the headless entry point, which the platform's
  /// background engine runs.
  final int callbackHandle;

  /// The descriptor, as JSON, the platform keeps for the headless run.
  final String descriptor;

  /// The configuration; its interval counts at least 15 minutes.
  final BackgroundSyncConfig config;

  /// The arguments of the `backgroundSyncSchedule` call.
  Map<String, Object?> toArguments() => {
    'callback': callbackHandle,
    'descriptor': descriptor,
    'intervalMinutes': config.intervalMinutes,
    'requiresUnmeteredNetwork': config.requiresUnmeteredNetwork,
    'requiresCharging': config.requiresCharging,
    'requiresBatteryNotLow': config.requiresBatteryNotLow,
  };
}

/// The platform side of the scheduler: WorkManager on Android,
/// `BGTaskScheduler` on iOS.
abstract interface class BackgroundSyncPlatform {
  /// Schedules the periodic task, replacing the one scheduled before. A
  /// [PluxException] when the platform cannot: `PLX-3501` when the host did
  /// not declare the task, `PLX-3500` otherwise.
  Future<void> schedule(BackgroundSyncRequest request);

  /// Cancels the task.
  Future<void> cancel();
}

/// The platform scheduler behind the `dev.plux/runtime` channel.
final class ChannelBackgroundSyncPlatform implements BackgroundSyncPlatform {
  /// Creates the platform with the runtime's channel.
  const ChannelBackgroundSyncPlatform([this._channel = const RuntimeChannel()]);

  final RuntimeChannel _channel;

  @override
  Future<void> schedule(BackgroundSyncRequest request) =>
      _call('backgroundSyncSchedule', request.toArguments());

  @override
  Future<void> cancel() => _call('backgroundSyncCancel');

  Future<void> _call(String method, [Object? arguments]) async {
    try {
      await _channel.invokeMethod<void>(method, arguments);
    } on MissingPluginException {
      throw const PluxException(
        PluxErrorCode.backgroundSyncNotConfigured,
        'this platform has no background scheduling',
      );
    } on PlatformException catch (e) {
      throw PluxException(
        e.code == PluxErrorCode.backgroundSyncNotConfigured.id
            ? PluxErrorCode.backgroundSyncNotConfigured
            : PluxErrorCode.backgroundSyncFailed,
        e.code == PluxErrorCode.backgroundSyncNotConfigured.id
            ? 'the host did not declare the background task: '
                  '$backgroundSyncTaskIdentifier is missing from '
                  'BGTaskSchedulerPermittedIdentifiers'
            : 'the platform scheduler refused the request',
        details: {'platformCode': e.code},
      );
    }
  }
}

/// Schedules or cancels the task as [BackgroundSyncConfig] says.
///
/// A marker file in the store records that a task was scheduled, so an app
/// that never opted in makes no platform call at all, and one that opts out
/// again has its task cancelled once.
final class BackgroundSyncScheduler {
  /// Creates a scheduler over [platform] that keeps its marker in
  /// [storeRoot].
  BackgroundSyncScheduler({required this.platform, required String storeRoot})
    : _marker = File('$storeRoot/background-sync');

  /// The platform's scheduler.
  final BackgroundSyncPlatform platform;
  final File _marker;

  /// Applies [config]: schedules the task with the descriptor and the entry
  /// point [callbackHandle] when it is enabled, cancels a task
  /// scheduled before when it is not. Both are made only when needed.
  /// Throws a [PluxException] when the platform refuses.
  Future<void> apply(
    BackgroundSyncConfig config, {
    required BackgroundSyncDescriptor Function() descriptor,
    required int Function() callbackHandle,
  }) async {
    if (!config.enabled) {
      if (_marker.existsSync()) {
        await platform.cancel();
        _marker.deleteSync();
      }
      return;
    }
    await platform.schedule(
      BackgroundSyncRequest(
        callbackHandle: callbackHandle(),
        descriptor: descriptor().toJson(),
        config: config,
      ),
    );
    if (!_marker.existsSync()) _marker.writeAsBytesSync(const []);
  }
}

/// How a background run ended.
enum BackgroundSyncOutcome {
  /// A verified release was staged for the next start.
  staged,

  /// There was nothing newer.
  upToDate,

  /// The run failed; trying again later may succeed (`PLX-3500`).
  failed,

  /// The release failed verification and was not staged (`PLX-3503`);
  /// trying again changes nothing until the release does.
  rejected,
}

/// The result of a background run, as the platform scheduler hears it.
final class BackgroundSyncReport {
  /// Creates a report.
  const BackgroundSyncReport(this.outcome, {this.sequence, this.error});

  /// How the run ended.
  final BackgroundSyncOutcome outcome;

  /// The release staged, or already active when [outcome] is
  /// [BackgroundSyncOutcome.upToDate].
  final int? sequence;

  /// Why the run failed or was rejected.
  final PluxException? error;

  /// The report as the `done` call sends it.
  Map<String, Object?> toArguments() => {
    'outcome': outcome.name,
    'sequence': ?sequence,
    'code': ?error?.code.id,
  };
}

/// The errors of a sync that mean what it fetched did not verify (SEC-052,
/// SYN-011).
const Set<PluxErrorCode> verificationFailures = {
  PluxErrorCode.manifestSignatureInvalid,
  PluxErrorCode.manifestExpired,
  PluxErrorCode.rollbackRejected,
  PluxErrorCode.patchHashMismatch,
  PluxErrorCode.bundleMalformed,
  PluxErrorCode.sectionHashMismatch,
  PluxErrorCode.sectionVerificationFailed,
  PluxErrorCode.assetHashMismatch,
};

/// Runs one sync with the sync worker and stages what it verifies.
///
/// The run never activates: it does not call the worker's `activate`, and
/// it refuses to report success if the active release changed under it.
final class BackgroundSyncRunner {
  /// Creates a runner for the worker [config]. [startWorker] and
  /// [readPointer] are for tests.
  BackgroundSyncRunner(
    this.config, {
    Future<SyncWorker> Function(SyncWorkerConfig)? startWorker,
    StorePointer Function(String root)? readPointer,
  }) : _startWorker = startWorker ?? SyncWorker.start,
       _readPointer = readPointer ?? _pointerOf;

  /// The sync worker's configuration.
  final SyncWorkerConfig config;
  final Future<SyncWorker> Function(SyncWorkerConfig) _startWorker;
  final StorePointer Function(String root) _readPointer;

  static StorePointer _pointerOf(String root) =>
      ReleaseStore.open(root).pointer;

  /// Syncs once. Never throws.
  Future<BackgroundSyncReport> run() async {
    SyncWorker? worker;
    try {
      final before = _readPointer(config.storeRoot);
      worker = await _startWorker(config);
      final result = await worker.sync((_) {});
      final after = _readPointer(config.storeRoot);
      if (after.active != before.active) {
        return BackgroundSyncReport(
          BackgroundSyncOutcome.failed,
          error: const PluxException(
            PluxErrorCode.backgroundSyncFailed,
            'the active release changed during a background sync',
          ),
        );
      }
      return switch (result.outcome) {
        SyncOutcome.staged => BackgroundSyncReport(
          BackgroundSyncOutcome.staged,
          sequence: result.sequence,
        ),
        SyncOutcome.upToDate => BackgroundSyncReport(
          BackgroundSyncOutcome.upToDate,
          sequence: result.sequence,
        ),
        SyncOutcome.failed => _failure(result.error),
      };
    } on Object catch (e) {
      return BackgroundSyncReport(
        BackgroundSyncOutcome.failed,
        error: PluxException(
          PluxErrorCode.backgroundSyncFailed,
          'the background sync could not run: ${e.runtimeType}',
        ),
      );
    } finally {
      try {
        await worker?.close();
      } on Object {
        // The isolate is killed after its grace period; nothing else to do.
      }
    }
  }

  static BackgroundSyncReport _failure(PluxException? cause) {
    if (cause != null && verificationFailures.contains(cause.code)) {
      return BackgroundSyncReport(
        BackgroundSyncOutcome.rejected,
        error: PluxException(
          PluxErrorCode.stagedReleaseVerificationFailed,
          'a release failed verification and was not staged',
          details: {'cause': cause.code.id},
        ),
      );
    }
    return BackgroundSyncReport(
      BackgroundSyncOutcome.failed,
      error: PluxException(
        PluxErrorCode.backgroundSyncFailed,
        'the background sync failed',
        details: {if (cause != null) 'cause': cause.code.id},
      ),
    );
  }
}
