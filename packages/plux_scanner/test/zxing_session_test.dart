// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The ZXing session on a fake frame source: frames in, hits out, through the
// real offline engine (decision B21).
//
// Verifies: RT-060.

import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_scanner/plux_scanner.dart';

import 'support/code_images.dart';

final class FakeFrameSource implements ScanFrameSource {
  final StreamController<ScanFrame> controller = StreamController();
  final List<bool> torches = [];
  bool disposed = false;

  @override
  Widget preview(BuildContext context) => const SizedBox();

  @override
  Stream<ScanFrame> get frames => controller.stream;

  @override
  Future<void> setTorch(bool on) async => torches.add(on);

  @override
  Future<void> dispose() async {
    disposed = true;
    unawaited(controller.close());
  }
}

void main() {
  test('a frame with a code becomes a hit', () async {
    final source = FakeFrameSource();
    final session = ZxingSession(const ScanRequest(), source);
    final hit = session.hits.first;
    source.controller.add(testCodes.first.draw(4).frame);
    final got = await hit;
    expect(got.value, testCodes.first.value);
    expect(got.format, 'qrCode');
    await session.dispose();
    expect(source.disposed, isTrue);
  });

  test('the request limits the formats that are read', () async {
    final source = FakeFrameSource();
    final session = ZxingSession(const ScanRequest(formats: ['ean13']), source);
    final hits = <ScanHit>[];
    session.hits.listen(hits.add);
    source.controller
      ..add(testCodes.first.draw(4).frame)
      ..add(testCodes[3].draw(4).frame);
    await pumpEventQueue(times: 200);
    // The QR frame is dropped while the first frame is being read, or read
    // with the mask and rejected: either way only the EAN-13 can be a hit.
    expect(hits.every((h) => h.format == 'ean13'), isTrue);
    await session.dispose();
  });

  test('frames that arrive while one is read are dropped', () async {
    final source = FakeFrameSource();
    final gate = Completer<ZxingResult?>();
    var reads = 0;
    final session = ZxingSession(
      const ScanRequest(),
      source,
      decode: (frame, formats) {
        reads++;
        return gate.future;
      },
    );
    final frame = GrayImage(20, 20).frame;
    source.controller
      ..add(frame)
      ..add(frame)
      ..add(frame);
    await pumpEventQueue();
    expect(reads, 1);
    gate.complete(null);
    await pumpEventQueue();
    source.controller.add(frame);
    await pumpEventQueue();
    expect(reads, 2);
    await session.dispose();
  });

  test('the scan window crops the frame before it is read', () async {
    final source = FakeFrameSource();
    final sizes = <(int, int)>[];
    final session = ZxingSession(
      const ScanRequest(scanWindow: [0.25, 0.25, 0.5, 0.5]),
      source,
      decode: (frame, formats) async {
        sizes.add((frame.width, frame.height));
        return null;
      },
    );
    source.controller.add(GrayImage(200, 100).frame);
    await pumpEventQueue();
    expect(sizes, [(100, 50)]);
    await session.dispose();
  });

  test('a source error ends the session with its code', () async {
    final source = FakeFrameSource();
    final session = ZxingSession(const ScanRequest(), source);
    final errors = <String>[];
    session.errors.listen(errors.add);
    source.controller.addError(
      const ScanSourceException(ScanErrorCodes.permissionDenied),
    );
    source.controller.addError(StateError('boom'));
    await pumpEventQueue();
    expect(errors, [ScanErrorCodes.permissionDenied, ScanErrorCodes.failed]);
    await session.dispose();
  });

  test(
    'a decoder failure is reported as failed, and reading goes on',
    () async {
      final source = FakeFrameSource();
      var calls = 0;
      final session = ZxingSession(
        const ScanRequest(),
        source,
        decode: (frame, formats) async {
          if (calls++ == 0) throw StateError('native failure');
          return const ZxingResult('v', 'qrCode');
        },
      );
      final errors = <String>[];
      final hits = <ScanHit>[];
      session.errors.listen(errors.add);
      session.hits.listen(hits.add);
      source.controller.add(GrayImage(20, 20).frame);
      await pumpEventQueue();
      source.controller.add(GrayImage(20, 20).frame);
      await pumpEventQueue();
      expect(errors, [ScanErrorCodes.failed]);
      expect(hits.single.value, 'v');
      await session.dispose();
    },
  );

  test('the torch goes to the source', () async {
    final source = FakeFrameSource();
    final session = ZxingSession(const ScanRequest(), source);
    await session.setTorch(true);
    expect(source.torches, [true]);
    await session.dispose();
  });
}
