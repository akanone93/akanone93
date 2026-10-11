// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Photos for identity verification (`KycCameraCapture`, SEC-090): the
/// camera for a document or a selfie, and the quality checks on the photo
/// taken. The checks decode the photo at a reduced size, which the engine
/// does off the Dart UI isolate, and measure it in a separate isolate, so
/// neither touches the UI isolate with more than a small result.
library;

import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/foundation.dart' show compute, visibleForTesting;
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_media/src/media.dart' show PickedEntry;

/// The width a photo is reduced to before it is measured.
const int kycAnalysisWidth = 320;

/// How good a photo has to be. The defaults suit photos of documents and
/// faces read at [kycAnalysisWidth]; a host with other needs passes its own
/// to [PluxMedia].
final class KycThresholds {
  /// Creates thresholds.
  const KycThresholds({
    this.minSharpness = 100,
    this.minBrightness = 60,
    this.maxBrightness = 200,
  });

  /// The least variance of the Laplacian of the luminance: below it the
  /// photo is `blurry`.
  final double minSharpness;

  /// The least mean luminance, 0 to 255: below it the photo is `dark`.
  final double minBrightness;

  /// The greatest mean luminance, 0 to 255: above it the photo is `bright`.
  final double maxBrightness;
}

/// What the quality checks measured.
final class KycQuality {
  /// Creates a measurement.
  const KycQuality({required this.sharpness, required this.brightness});

  /// The variance of the Laplacian of the luminance: high for a sharp
  /// photo, near zero for a blurred or flat one.
  final double sharpness;

  /// The mean luminance, 0 (black) to 255 (white).
  final double brightness;

  /// The problems against [thresholds], by stable code (`blurry`, `dark`,
  /// `bright`); empty when the photo passes.
  List<String> problems(KycThresholds thresholds) => [
    if (sharpness < thresholds.minSharpness) 'blurry',
    if (brightness < thresholds.minBrightness) 'dark',
    if (brightness > thresholds.maxBrightness) 'bright',
  ];
}

/// Measures RGBA pixels of [width] x [height]: the mean luminance, and the
/// variance of the 4-neighbour Laplacian of the luminance over the inside
/// of the image. Pure, so it runs in any isolate.
@visibleForTesting
KycQuality measureRgba(Uint8List rgba, int width, int height) {
  final luma = Float64List(width * height);
  var sum = 0.0;
  for (var i = 0; i < luma.length; i++) {
    final o = i * 4;
    final y = (299 * rgba[o] + 587 * rgba[o + 1] + 114 * rgba[o + 2]) / 1000;
    luma[i] = y;
    sum += y;
  }
  final mean = luma.isEmpty ? 0.0 : sum / luma.length;
  if (width < 3 || height < 3) {
    return KycQuality(sharpness: 0, brightness: mean);
  }
  var count = 0;
  var total = 0.0;
  var totalSquares = 0.0;
  for (var y = 1; y < height - 1; y++) {
    for (var x = 1; x < width - 1; x++) {
      final i = y * width + x;
      final l =
          luma[i - 1] +
          luma[i + 1] +
          luma[i - width] +
          luma[i + width] -
          4 * luma[i];
      total += l;
      totalSquares += l * l;
      count++;
    }
  }
  final meanL = total / count;
  return KycQuality(
    sharpness: totalSquares / count - meanL * meanL,
    brightness: mean,
  );
}

Future<KycQuality> _measure((Uint8List, int, int) args) async =>
    measureRgba(args.$1, args.$2, args.$3);

/// Decodes the photo in [bytes] at [kycAnalysisWidth] and measures it in
/// another isolate.
Future<KycQuality> analysePhoto(Uint8List bytes) async {
  final codec = await ui.instantiateImageCodec(
    bytes,
    targetWidth: kycAnalysisWidth,
  );
  try {
    final frame = await codec.getNextFrame();
    final image = frame.image;
    try {
      final data = await image.toByteData();
      if (data == null) throw StateError('the photo has no pixels');
      return await compute(_measure, (
        data.buffer.asUint8List(data.offsetInBytes, data.lengthInBytes),
        image.width,
        image.height,
      ));
    } finally {
      image.dispose();
    }
  } finally {
    codec.dispose();
  }
}

/// Measures the photo at a path; replaced in tests.
typedef KycAnalyser = Future<KycQuality> Function(String path);

/// The default analyser: reads the file and measures it.
Future<KycQuality> analyseFile(String path) async =>
    analysePhoto(await File(path).readAsBytes());

/// Takes the photo for [KycCaptureService] with [camera] and returns the
/// entry; null when the user cancels. [front] says which camera.
typedef KycCamera = Future<PickedEntry?> Function({required bool front});

/// The service behind `KycCameraCapture`: takes a photo with the rear
/// camera for a document or the front camera for a selfie, and checks its
/// quality against [thresholds].
final class KycCaptureService implements PluxKycCapture {
  /// Creates the service.
  KycCaptureService({
    required this.camera,
    required this.thresholds,
    required this.analyse,
    required this.mimeOf,
  });

  /// Takes the photo.
  final KycCamera camera;

  /// The quality the photo must reach.
  final KycThresholds thresholds;

  /// Measures the photo.
  final KycAnalyser analyse;

  /// The media type of the entry the camera returned.
  final String Function(PickedEntry entry) mimeOf;

  @override
  Future<PluxKycPhoto?> capture(String mode) async {
    if (mode != 'document' && mode != 'selfie') {
      throw PluxDeviceException.unavailable('$mode is not a capture mode');
    }
    final entry = await camera(front: mode == 'selfie');
    if (entry == null) return null;
    final KycQuality quality;
    try {
      quality = await analyse(entry.path);
    } on Object {
      // A photo that cannot be measured cannot be accepted.
      throw const PluxDeviceException.unavailable(
        'the photo could not be checked',
      );
    }
    return PluxKycPhoto(
      file: PluxPickedFile(
        path: entry.path,
        name: entry.name,
        mimeType: mimeOf(entry),
        sizeBytes: entry.size,
      ),
      problems: quality.problems(thresholds),
    );
  }
}
