// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The headless entry point of background sync (SYN-014, ADR-0092). The
/// platform scheduler starts a background Flutter engine at
/// [pluxBackgroundSyncMain]; there is no screen and no host `main`, so
/// everything the sync needs comes from the descriptor the foreground left
/// with the platform.
library;

import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/assets/assets.dart';
import 'package:plux_flutter/src/core/features.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/platform/platform_services.dart';
import 'package:plux_flutter/src/runtime_info.dart';
import 'package:plux_flutter/src/schema/limits.g.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/pins.dart';
import 'package:plux_flutter/src/security/platform_attestation.dart';
import 'package:plux_flutter/src/store/metadata_state.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/background_sync.dart';
import 'package:plux_flutter/src/sync/metadata_sync.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';
import 'package:plux_flutter/src/sync/sync_worker.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';

const MethodChannel _channel = MethodChannel('dev.plux/background-sync');

/// The sync worker configuration of a headless run described by [d]: the
/// platform's pinned HTTP client, credential store, hardware keys and
/// attestation, as the foreground runtime makes them.
///
/// The closures capture plain values only, since they run on the sync
/// isolate.
SyncWorkerConfig backgroundWorkerConfig(
  BackgroundSyncDescriptor d, {
  RootIsolateToken? rootIsolateToken,
}) {
  final pinSet = pinSetFor(d.endpoint, d.pins);
  if (pinSet != null) {
    MetadataSync.applyStoredPins(pinSet, MetadataStore(d.storeRoot).read());
  }
  final features = RuntimeFeatures(deviceFunctions: d.deviceFunctions);
  final appId = d.appId;
  final environment = d.environment;
  final cloudProjectNumber = d.cloudProjectNumber;
  final buildId = d.hostBuild;
  final credentialName = d.credentialName;
  return SyncWorkerConfig(
    storeRoot: d.storeRoot,
    endpoint: d.endpoint,
    httpClient: PlatformHttpClients(d.endpoint, pinSet).create,
    credentials: () => PlatformCredentialStore(credentialName),
    configSecrets: PlatformSecretStore.new,
    deviceKeys: PlatformDeviceKeys.new,
    attestation: () => PlatformAttestation(
      appId: appId,
      environment: environment,
      cloudProjectNumber: cloudProjectNumber,
      buildId: buildId,
    ),
    parallelism: d.parallelism,
    rootIsolateToken: rootIsolateToken,
    sync: SyncConfig(
      appId: d.appId,
      environment: d.environment,
      channel: d.channel,
      keys: d.keys,
      rootDocument: d.rootDocument,
      pins: pinSet,
      device: DeviceInfo(
        platform: Platform.operatingSystem,
        osVersion: deviceOsVersion(Platform.operatingSystemVersion),
        runtimeVersion: PluxRuntimeInfo.version,
        hostBuild: d.hostBuild,
      ),
      enrolSca: d.enrolSca,
      supportsFeature: features.supports,
      verifierLimits: VerifierLimits(
        maxDepth: PluxLimit.bundleVerifierDepth.defaultValue,
        maxVisits: PluxLimit.bundleVerifierTables.defaultValue,
      ),
      diskQuota: d.diskQuota,
      assets: AssetDevice(pixelRatio: d.pixelRatio, avif: d.avif),
      locale: d.locale,
    ),
  );
}

/// Runs the sync described by the descriptor JSON [json] and reports how it
/// ended. Never throws.
Future<BackgroundSyncReport> runBackgroundSync(
  String json, {
  RootIsolateToken? rootIsolateToken,
}) async {
  try {
    final descriptor = BackgroundSyncDescriptor.fromJson(json);
    return await BackgroundSyncRunner(
      backgroundWorkerConfig(descriptor, rootIsolateToken: rootIsolateToken),
    ).run();
  } on PluxException catch (e) {
    return BackgroundSyncReport(BackgroundSyncOutcome.failed, error: e);
  } on Object catch (e) {
    return BackgroundSyncReport(
      BackgroundSyncOutcome.failed,
      error: PluxException(
        PluxErrorCode.backgroundSyncFailed,
        'the background sync could not start: ${e.runtimeType}',
      ),
    );
  }
}

/// The entry point the platform's background engine runs (SYN-014). It
/// fetches the descriptor from the platform, runs one sync that verifies
/// and stages a release (never activates it), and tells the platform how it
/// ended with `done`, after which the platform stops the engine.
@pragma('vm:entry-point')
Future<void> pluxBackgroundSyncMain() async {
  WidgetsFlutterBinding.ensureInitialized();
  ui.DartPluginRegistrant.ensureInitialized();
  BackgroundSyncReport report;
  try {
    final json = await _channel.invokeMethod<String>('descriptor');
    report = json == null
        ? const BackgroundSyncReport(
            BackgroundSyncOutcome.failed,
            error: PluxException(
              PluxErrorCode.backgroundSyncNotConfigured,
              'the platform holds no background sync descriptor',
            ),
          )
        : await runBackgroundSync(
            json,
            rootIsolateToken: RootIsolateToken.instance,
          );
  } on Object catch (e) {
    report = BackgroundSyncReport(
      BackgroundSyncOutcome.failed,
      error: PluxException(
        PluxErrorCode.backgroundSyncFailed,
        'the descriptor cannot be read: ${e.runtimeType}',
      ),
    );
  }
  await _channel.invokeMethod<void>('done', report.toArguments());
}
