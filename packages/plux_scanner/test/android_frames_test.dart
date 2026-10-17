// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The Android camera of the ZXing engine on fake platform channels (no
// camera): start, frames, bounds, errors, torch, stop (decision B21).
//
// Verifies: RT-060.

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_scanner/plux_scanner.dart';

import 'scanner_widgets_test.dart' show FakeProbe;
import 'support/code_images.dart';

const _methods = MethodChannel('dev.plux/scanner');
const _events = EventChannel('dev.plux/scanner/frames');

/// The plugin: answers the method channel and lets the test push frames.
final class FakePlugin {
  FakePlugin(this.binding);

  final TestDefaultBinaryMessenger binding;
  final List<MethodCall> calls = [];
  Object? startResult = {
    'textureId': 7,
    'width': 640,
    'height': 480,
    'rotation': 90,
  };
  PlatformException? startError;

  void install() {
    binding.setMockMethodCallHandler(_methods, (call) async {
      calls.add(call);
      if (call.method == 'start') {
        if (startError != null) throw startError!;
        return startResult;
      }
      return null;
    });
    binding.setMockStreamHandler(
      _events,
      MockStreamHandler.inline(
        onListen: (arguments, events) {
          sink = events;
        },
        onCancel: (arguments) {
          sink = null;
        },
      ),
    );
  }

  MockStreamHandlerEventSink? sink;

  void remove() {
    binding.setMockMethodCallHandler(_methods, null);
    binding.setMockStreamHandler(_events, null);
  }
}

void main() {
  late FakePlugin plugin;

  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    plugin = FakePlugin(
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger,
    )..install();
  });
  tearDown(() => plugin.remove());

  Map<String, Object?> frame({
    int w = 8,
    int h = 4,
    int stride = 8,
    int rotation = 0,
    Object? bytes,
  }) => {
    'bytes': bytes ?? Uint8List(stride * h),
    'width': w,
    'height': h,
    'rowStride': stride,
    'rotation': rotation,
  };

  test('start sends the request and frames arrive with their stride', () async {
    final source = AndroidFrameSource(
      const ScanRequest(backCamera: false, torch: true),
    );
    final first = source.frames.first;
    await pumpEventQueue();
    expect(plugin.calls.single.method, 'start');
    expect(plugin.calls.single.arguments, {'back': false, 'torch': true});
    plugin.sink!.success(frame(w: 6, h: 3, stride: 8, rotation: 90));
    final got = await first;
    expect((got.width, got.height, got.stride, got.rotation), (6, 3, 8, 90));
    await source.dispose();
    expect(plugin.calls.last.method, 'stop');
  });

  test('frames beyond the bounds or malformed are dropped', () async {
    final source = AndroidFrameSource(const ScanRequest());
    final got = <ScanFrame>[];
    source.frames.listen(got.add);
    await pumpEventQueue();
    final sink = plugin.sink!;
    sink.success(frame(w: 4097, h: 2, stride: 4097));
    sink.success(frame(w: 2, h: 4097, stride: 2));
    sink.success(frame(w: 8, h: 4, stride: 4));
    sink.success(frame(bytes: Uint8List(5)));
    sink.success(frame(rotation: 45));
    sink.success(frame(bytes: 'not bytes'));
    sink.success('not a map');
    sink.success({'width': 1});
    sink.success(frame(w: 4096, h: 1, stride: 4096));
    sink.success(frame());
    await pumpEventQueue();
    expect(got.map((f) => f.width), [4096, 8]);
    await source.dispose();
  });

  test('a camera that cannot start is a typed error', () async {
    for (final (code, expected) in [
      ('permissionDenied', ScanErrorCodes.permissionDenied),
      ('unsupported', ScanErrorCodes.unsupported),
      ('failed', ScanErrorCodes.failed),
      ('weird', ScanErrorCodes.failed),
    ]) {
      plugin.startError = PlatformException(code: code);
      final source = AndroidFrameSource(const ScanRequest());
      final error = source.frames.first;
      await expectLater(
        error,
        throwsA(
          isA<ScanSourceException>().having((e) => e.code, 'code', expected),
        ),
      );
      await source.dispose();
    }
  });

  test('an answer without a texture is a failure', () async {
    plugin.startResult = {'width': 1};
    final source = AndroidFrameSource(const ScanRequest());
    await expectLater(source.frames.first, throwsA(isA<ScanSourceException>()));
    await source.dispose();
  });

  test('without the plugin the engine is unsupported', () async {
    plugin.remove();
    final source = AndroidFrameSource(const ScanRequest());
    await expectLater(
      source.frames.first,
      throwsA(
        isA<ScanSourceException>().having(
          (e) => e.code,
          'code',
          ScanErrorCodes.unsupported,
        ),
      ),
    );
    await source.dispose();
  });

  test('the torch is sent only when it changes', () async {
    final source = AndroidFrameSource(const ScanRequest());
    await pumpEventQueue();
    await source.setTorch(false);
    await source.setTorch(true);
    await source.setTorch(true);
    final torch = plugin.calls.where((c) => c.method == 'setTorch');
    expect(torch.map((c) => c.arguments), [
      {'on': true},
    ]);
    await source.dispose();
  });

  testWidgets('the preview is the camera texture, turned upright', (
    tester,
  ) async {
    final source = AndroidFrameSource(const ScanRequest());
    await tester.pumpWidget(
      MaterialApp(home: Builder(builder: source.preview)),
    );
    await tester.runAsync(pumpEventQueue);
    await tester.pump();
    final texture = tester.widget<Texture>(find.byType(Texture));
    expect(texture.textureId, 7);
    expect(tester.widget<RotatedBox>(find.byType(RotatedBox)).quarterTurns, 1);
    await tester.runAsync(source.dispose);
  });

  test(
    'a zxing scanner on Android uses this camera and reads its frames',
    () async {
      final config = PluxScannerConfig(
        engine: ScannerEngine.zxing,
        probe: FakeProbe(MlKitState.unavailable),
        android: true,
      );
      final session = config.open(const ScanRequest(), await config.resolve());
      final hit = session.hits.first;
      await pumpEventQueue();
      expect(plugin.calls.first.method, 'start');
      final image = testCodes.first.draw(4);
      plugin.sink!.success({
        'bytes': image.pixels,
        'width': image.width,
        'height': image.height,
        'rowStride': image.width,
        'rotation': 0,
      });
      expect((await hit).value, testCodes.first.value);
      await session.dispose();
    },
  );

  test('off Android the zxing engine still needs a host frame source', () {
    final config = PluxScannerConfig(
      engine: ScannerEngine.zxing,
      probe: FakeProbe(MlKitState.unavailable),
      android: false,
    );
    expect(
      () => config.open(const ScanRequest(), ScannerEngine.zxing),
      throwsA(isA<ScanSourceException>()),
    );
  });

  group('scan windows follow the frame rotation', () {
    test('windowInSensor maps upright fractions onto the sensor', () {
      const w = [0.1, 0.2, 0.3, 0.4];
      expect(windowInSensor(w, 0), w);
      expect(
        windowInSensor(w, 90).map((v) => double.parse(v.toStringAsFixed(6))),
        [0.2, 0.6, 0.4, 0.3],
      );
      expect(
        windowInSensor(w, 180).map((v) => double.parse(v.toStringAsFixed(6))),
        [0.6, 0.4, 0.3, 0.4],
      );
      expect(
        windowInSensor(w, 270).map((v) => double.parse(v.toStringAsFixed(6))),
        [0.4, 0.1, 0.4, 0.3],
      );
    });

    test('a rotated frame is cropped where the upright window points', () {
      // 40x20 sensor frame to be turned 90 degrees: upright it is 20x40.
      // The upright window is the top half; on the sensor that is its left
      // half (display top = sensor left).
      final bytes = Uint8List(40 * 20);
      for (var y = 0; y < 20; y++) {
        for (var x = 0; x < 40; x++) {
          bytes[y * 40 + x] = x < 20 ? 10 : 200;
        }
      }
      final f = ScanFrame(bytes, 40, 20, rotation: 90);
      final top = cropFrame(f, [0, 0, 1, 0.5]);
      expect((top.width, top.height), (20, 20));
      expect(top.bytes.every((b) => b == 10), isTrue);
      final bottom = cropFrame(f, [0, 0.5, 1, 0.5]);
      expect(bottom.bytes.every((b) => b == 200), isTrue);
    });
  });
}
