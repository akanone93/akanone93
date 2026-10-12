// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:file_picker/file_picker.dart' show FileType;
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_media/plux_media.dart';
import 'package:plux_media/src/kyc.dart';

final class FakeImages implements ImageBackend {
  PickedEntry? captured;
  final List<bool> front = [];

  @override
  Future<List<PickedEntry>> pick({
    required bool multiple,
    required int limit,
    int? maxDimension,
  }) async => const [];

  @override
  Future<PickedEntry?> capture({int? maxDimension, bool front = false}) async {
    this.front.add(front);
    return captured;
  }
}

final class NoFiles implements FileBackend {
  @override
  Future<List<PickedEntry>> pick({
    required FileType type,
    required List<String> extensions,
    required bool multiple,
  }) async => const [];
}

/// RGBA pixels of [width] x [height] with luminance [at] gives per pixel.
Uint8List pixels(int width, int height, int Function(int x, int y) at) {
  final out = Uint8List(width * height * 4);
  for (var y = 0; y < height; y++) {
    for (var x = 0; x < width; x++) {
      final v = at(x, y);
      final o = (y * width + x) * 4;
      out
        ..[o] = v
        ..[o + 1] = v
        ..[o + 2] = v
        ..[o + 3] = 255;
    }
  }
  return out;
}

int checker(int x, int y) => ((x ~/ 2) + (y ~/ 2)).isEven ? 20 : 235;

/// A PNG of [width] x [height] with luminance [at] per pixel.
Future<Uint8List> png(int width, int height, int Function(int, int) at) async {
  final completer = await ui.ImmutableBuffer.fromUint8List(
    pixels(width, height, at),
  );
  final descriptor = ui.ImageDescriptor.raw(
    completer,
    width: width,
    height: height,
    pixelFormat: ui.PixelFormat.rgba8888,
  );
  final codec = await descriptor.instantiateCodec();
  final image = (await codec.getNextFrame()).image;
  final data = await image.toByteData(format: ui.ImageByteFormat.png);
  image.dispose();
  codec.dispose();
  return data!.buffer.asUint8List();
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('the quality measure', () {
    const t = KycThresholds();

    test('a sharp, evenly lit photo passes [SEC-090]', () {
      final q = measureRgba(pixels(64, 64, checker), 64, 64);
      expect(q.sharpness, greaterThan(t.minSharpness));
      expect(q.brightness, closeTo(127.5, 2));
      expect(q.problems(t), isEmpty);
    });

    test('a smooth gradient is blurry [SEC-090]', () {
      final q = measureRgba(pixels(64, 64, (x, y) => 60 + x * 2), 64, 64);
      expect(q.sharpness, lessThan(1));
      expect(q.problems(t), ['blurry']);
    });

    test('too dark and too bright are told apart, and may come with blur [SEC-090]', () {
      expect(measureRgba(pixels(64, 64, (x, y) => 10), 64, 64).problems(t), [
        'blurry',
        'dark',
      ]);
      expect(measureRgba(pixels(64, 64, (x, y) => 250), 64, 64).problems(t), [
        'blurry',
        'bright',
      ]);
      expect(
        measureRgba(
          pixels(64, 64, (x, y) => (x + y).isEven ? 150 : 255),
          64,
          64,
        ).problems(t),
        ['bright'],
      );
    });

    test('the thresholds are the host\'s to set [SEC-090]', () {
      final q = measureRgba(pixels(64, 64, (x, y) => 60 + x * 2), 64, 64);
      expect(q.problems(const KycThresholds(minSharpness: 0)), isEmpty);
    });

    test('an image too small to measure has no sharpness [SEC-090]', () {
      expect(measureRgba(pixels(2, 2, (x, y) => 128), 2, 2).sharpness, 0);
    });
  });

  group('analysing an encoded photo', () {
    testWidgets(
      'decodes it reduced and measures it off the UI isolate [SEC-090]',
      (tester) async {
        await tester.runAsync(() async {
          final sharp = await analysePhoto(await png(640, 480, checker));
          expect(sharp.problems(const KycThresholds()), isEmpty);
          final flat = await analysePhoto(await png(640, 480, (x, y) => 240));
          expect(flat.problems(const KycThresholds()), ['blurry', 'bright']);
        });
      },
    );
  });

  group('the capture service', () {
    late FakeImages images;
    late List<String> analysed;
    KycQuality quality = const KycQuality(sharpness: 500, brightness: 120);

    PluxKycCapture service({KycThresholds? thresholds}) {
      final media = PluxMedia(
        images: images,
        files: NoFiles(),
        kyc: thresholds ?? const KycThresholds(),
        analyse: (path) async {
          analysed.add(path);
          return quality;
        },
      );
      return media.services[PluxKycCapture]! as PluxKycCapture;
    }

    setUp(() {
      images = FakeImages();
      analysed = [];
      quality = const KycQuality(sharpness: 500, brightness: 120);
    });

    PickedEntry entry() =>
        (path: '/cache/id.jpg', name: 'id.jpg', mimeType: null, size: 9);

    test('the package registers it beside the picker [RT-060]', () {
      final services = PluxMedia(images: images, files: NoFiles()).services;
      expect(services.keys, containsAll([PluxMediaPicker, PluxKycCapture]));
    });

    test(
      'a document uses the rear camera and a selfie the front one [SEC-090]',
      () async {
        images.captured = entry();
        final kyc = service();
        await kyc.capture('document');
        await kyc.capture('selfie');
        expect(images.front, [false, true]);
      },
    );

    test(
      'a photo that passes comes back with no problems and its file [SEC-090]',
      () async {
        images.captured = entry();
        final photo = (await service().capture('document'))!;
        expect(photo.problems, isEmpty);
        expect(photo.file.path, '/cache/id.jpg');
        expect(photo.file.mimeType, 'image/jpeg');
        expect(photo.file.sizeBytes, 9);
        expect(analysed, ['/cache/id.jpg']);
      },
    );

    test('a photo that fails the checks says why [SEC-090]', () async {
      images.captured = entry();
      quality = const KycQuality(sharpness: 5, brightness: 30);
      expect((await service().capture('selfie'))!.problems, ['blurry', 'dark']);
      expect(
        (await service(
          thresholds: const KycThresholds(minSharpness: 1, minBrightness: 10),
        ).capture('selfie'))!.problems,
        isEmpty,
      );
    });

    test('cancelling takes no photo and measures nothing [SEC-090]', () async {
      expect(await service().capture('document'), isNull);
      expect(analysed, isEmpty);
    });

    test('an unknown mode, or a photo that cannot be measured, is a device failure [SEC-090]', () async {
      images.captured = entry();
      await expectLater(
        service().capture('passport'),
        throwsA(isA<PluxDeviceException>()),
      );
      final broken =
          PluxMedia(
                images: images,
                files: NoFiles(),
                analyse: (_) async =>
                    throw const FormatException('not an image'),
              ).services[PluxKycCapture]!
              as PluxKycCapture;
      await expectLater(
        broken.capture('document'),
        throwsA(
          isA<PluxDeviceException>().having(
            (e) => e.message,
            'message',
            isNot(contains('/cache')),
          ),
        ),
      );
    });
  });
}
