// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Which engine reads the camera, and how the choice is made (decision B21,
/// ADR-0051): ML Kit's unbundled model through Google Play services where it
/// is ready, ZXing-C++ through FFI where Play services is missing or the
/// model is not downloaded, Apple's Vision on iOS.
library;

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:plux_scanner/src/android_frames.dart';
import 'package:plux_scanner/src/session.dart';

/// The engine a scanner uses.
enum ScannerEngine {
  /// Pick by platform and by what the device has: the default.
  auto,

  /// The platform's own: ML Kit on Android, Vision on iOS.
  platform,

  /// ZXing-C++ through FFI, offline; frames come from the host's
  /// [PluxScannerConfig.frameSource].
  zxing,
}

/// What Android reports of the unbundled ML Kit barcode model.
enum MlKitState {
  /// Google Play services has the model: scanning works now.
  ready,

  /// Play services is there and the model is downloading or was just
  /// requested.
  downloading,

  /// There is no Play services, or the model cannot be had.
  unavailable,
}

/// Asks Android about the unbundled ML Kit model.
abstract interface class ScannerProbe {
  /// The model's state now.
  Future<MlKitState> mlKitState();

  /// Requests the model's download in the background; does not wait for it.
  Future<void> requestModel();
}

/// The probe of the Android side of the package (`PluxScannerPlugin`).
final class ChannelScannerProbe implements ScannerProbe {
  /// Creates the probe on [channel].
  const ChannelScannerProbe([
    this.channel = const MethodChannel('dev.plux/scanner'),
  ]);

  /// The platform channel of the Android plugin.
  final MethodChannel channel;

  @override
  Future<MlKitState> mlKitState() async {
    try {
      return switch (await channel.invokeMethod<String>('mlKitState')) {
        'ready' => MlKitState.ready,
        'downloading' => MlKitState.downloading,
        _ => MlKitState.unavailable,
      };
    } on PlatformException {
      return MlKitState.unavailable;
    } on MissingPluginException {
      return MlKitState.unavailable;
    }
  }

  @override
  Future<void> requestModel() async {
    try {
      await channel.invokeMethod<void>('requestModel');
    } on PlatformException {
      // The next open asks again.
    } on MissingPluginException {
      // No plugin, no model.
    }
  }
}

/// Chooses the engine for one scanner (decision B21).
///
/// A configured engine is taken as it is. With [ScannerEngine.auto], iOS and
/// every other platform use the platform engine; Android uses it when the
/// model is ready and otherwise falls back to ZXing-C++, asking for the
/// model's download when it is not there yet so that the next scanner can
/// use it.
Future<ScannerEngine> resolveScannerEngine({
  required ScannerEngine configured,
  required bool android,
  required ScannerProbe probe,
}) async {
  if (configured != ScannerEngine.auto) return configured;
  if (!android) return ScannerEngine.platform;
  final state = await probe.mlKitState();
  if (state == MlKitState.ready) return ScannerEngine.platform;
  await probe.requestModel();
  return ScannerEngine.zxing;
}

/// Texts the scanners show and announce. A host replaces them with its own
/// strings; a plugin's own text comes from the descriptor's translatable
/// props.
@immutable
final class ScannerTexts {
  /// Creates the texts; the defaults are English.
  const ScannerTexts({
    this.detected = 'Code read',
    this.retry = 'Try again',
    this.torchOn = 'Turn the torch on',
    this.torchOff = 'Turn the torch off',
    this.denied = 'The camera permission was denied',
    this.unsupported = 'Scanning is not available on this device',
    this.failed = 'The camera cannot be used',
  });

  /// Announced to screen readers when a code is read; never the code.
  final String detected;

  /// The retry button of the error state.
  final String retry;

  /// The tooltip of the torch button while the torch is off.
  final String torchOn;

  /// The tooltip of the torch button while the torch is on.
  final String torchOff;

  /// The error state when the camera permission is denied.
  final String denied;

  /// The error state when scanning cannot work on this device.
  final String unsupported;

  /// The error state for any other camera failure.
  final String failed;

  /// The text of the error state for [code].
  String forError(String code) => switch (code) {
    ScanErrorCodes.permissionDenied => denied,
    ScanErrorCodes.unsupported => unsupported,
    _ => failed,
  };
}

/// How the scanners of a host behave.
@immutable
final class PluxScannerConfig {
  /// Creates the configuration.
  const PluxScannerConfig({
    this.engine = ScannerEngine.auto,
    this.probe = const ChannelScannerProbe(),
    this.frameSource,
    this.texts = const ScannerTexts(),
    this.debounce = const Duration(milliseconds: 1500),
    this.android,
    this.sessionFactory,
  });

  /// The engine; [ScannerEngine.zxing] forces the offline fallback.
  final ScannerEngine engine;

  /// Where Android's ML Kit state comes from.
  final ScannerProbe probe;

  /// Opens the camera for [ScannerEngine.zxing]: the fallback needs frames
  /// and `mobile_scanner` hands out none. Null on Android uses CameraX in the
  /// package's plugin ([AndroidFrameSource]); elsewhere the fallback then
  /// reports `unsupported`.
  final ScanFrameSourceFactory? frameSource;

  /// The texts.
  final ScannerTexts texts;

  /// How long a continuous scanner ignores a code it has just reported.
  final Duration debounce;

  /// Whether this is Android; the target platform when null.
  final bool? android;

  /// Replaces the sessions the engines open; for tests.
  @visibleForTesting
  final ScanSessionFactory? sessionFactory;

  bool get _isAndroid =>
      android ?? defaultTargetPlatform == TargetPlatform.android;

  /// The engine for a scanner opened now.
  Future<ScannerEngine> resolve() => resolveScannerEngine(
    configured: engine,
    android: _isAndroid,
    probe: probe,
  );

  /// Opens a session on [resolved] for [request].
  ScanSession open(ScanRequest request, ScannerEngine resolved) {
    final custom = sessionFactory;
    if (custom != null) return custom(request, resolved);
    if (resolved == ScannerEngine.zxing) {
      final source = frameSource;
      if (source != null) return ZxingSession(request, source(request));
      // Android's own camera is the default; elsewhere the host provides one.
      if (_isAndroid) return ZxingSession(request, AndroidFrameSource(request));
      throw const ScanSourceException(ScanErrorCodes.unsupported);
    }
    return MobileScannerSession(request);
  }
}
