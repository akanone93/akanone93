// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// A camera session: what a scanner widget reads codes from, whichever
/// engine is behind it.
library;

import 'dart:async';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:mobile_scanner/mobile_scanner.dart' as ms;
import 'package:plux_scanner/src/engine.dart';
import 'package:plux_scanner/src/formats.dart';
import 'package:plux_scanner/src/zxing_ffi.dart';

/// The error codes a scanner reports; they never hold what was scanned.
abstract final class ScanErrorCodes {
  /// The camera permission was denied.
  static const String permissionDenied = 'permissionDenied';

  /// Scanning cannot work on this device.
  static const String unsupported = 'unsupported';

  /// The camera or the engine failed.
  static const String failed = 'failed';
}

/// A scanner cannot start.
final class ScanSourceException implements Exception {
  /// Creates the error with one of [ScanErrorCodes].
  const ScanSourceException(this.code);

  /// What went wrong.
  final String code;

  @override
  String toString() => 'ScanSourceException($code)';
}

/// A code a session read.
@immutable
final class ScanHit {
  /// Creates a hit.
  const ScanHit(this.value, this.format);

  /// The decoded text; it can be sensitive (SEC-092).
  final String value;

  /// The schema `BarcodeFormat` member.
  final String format;
}

/// What a scanner asks of a session.
@immutable
final class ScanRequest {
  /// Creates a request.
  const ScanRequest({
    this.formats = const [],
    this.torch = false,
    this.backCamera = true,
    this.scanWindow,
  });

  /// The schema `BarcodeFormat` members to read; every format when empty.
  final List<String> formats;

  /// Whether the torch starts on.
  final bool torch;

  /// Whether to open the back camera.
  final bool backCamera;

  /// The part of the preview that is read, as fractions: left, top, width,
  /// height; the whole preview when null.
  final List<double>? scanWindow;
}

/// Reads codes from the camera.
abstract interface class ScanSession {
  /// The camera preview, fitted to its box. [scanWindow] is the part of it
  /// that is read, in the preview's own coordinates, when the request has
  /// one.
  Widget preview(BuildContext context, Rect? scanWindow);

  /// The codes it reads, repeats included.
  Stream<ScanHit> get hits;

  /// The codes of [ScanErrorCodes] for failures after the start.
  Stream<String> get errors;

  /// Turns the torch on or off.
  Future<void> setTorch(bool on);

  /// Stops the camera.
  Future<void> dispose();
}

/// Opens a session on an engine.
typedef ScanSessionFactory = ScanSession Function(
  ScanRequest request,
  ScannerEngine engine,
);

/// A camera that delivers luminance frames, for the ZXing engine.
abstract interface class ScanFrameSource {
  /// The camera preview.
  Widget preview(BuildContext context);

  /// The frames; an error of type [ScanSourceException] ends the session
  /// with its code.
  Stream<ScanFrame> get frames;

  /// Turns the torch on or off.
  Future<void> setTorch(bool on);

  /// Stops the camera.
  Future<void> dispose();
}

/// Opens a frame source for a request.
typedef ScanFrameSourceFactory = ScanFrameSource Function(ScanRequest request);

/// The part of [frame] that [window] selects: left, top, width and height as
/// fractions of the frame. The frame itself for a window that selects all of
/// it; a copy otherwise.
ScanFrame cropFrame(ScanFrame frame, List<double> window) {
  final left = (window[0].clamp(0.0, 1.0) * frame.width).floor();
  final top = (window[1].clamp(0.0, 1.0) * frame.height).floor();
  final w = (window[2].clamp(0.0, 1.0) * frame.width).ceil().clamp(
    1,
    frame.width - left,
  );
  final h = (window[3].clamp(0.0, 1.0) * frame.height).ceil().clamp(
    1,
    frame.height - top,
  );
  if (left == 0 && top == 0 && w == frame.width && h == frame.height) {
    return frame;
  }
  final out = Uint8List(w * h);
  for (var row = 0; row < h; row++) {
    final from = (top + row) * frame.stride + left;
    out.setRange(row * w, (row + 1) * w, frame.bytes, from);
  }
  return ScanFrame(out, w, h);
}

/// Reads frames from a [ScanFrameSource] with ZXing-C++, one frame at a time:
/// frames that arrive while one is being read are dropped.
final class ZxingSession implements ScanSession {
  /// Opens the session on [source].
  ZxingSession(
    this._request,
    this._source, {
    Future<ZxingResult?> Function(ScanFrame frame, int formats)? decode,
  }) : _decode =
           decode ??
           ((frame, formats) =>
               const ZxingDecoder().decodeAsync(frame, formats: formats)) {
    _mask = formatMask(_request.formats);
    _sub = _source.frames.listen(_frame, onError: _failed);
  }

  final ScanRequest _request;
  final ScanFrameSource _source;
  final Future<ZxingResult?> Function(ScanFrame frame, int formats) _decode;
  final StreamController<ScanHit> _hits = StreamController<ScanHit>.broadcast();
  final StreamController<String> _errors = StreamController<String>.broadcast();
  late final int _mask;
  late final StreamSubscription<ScanFrame> _sub;
  bool _busy = false;
  bool _disposed = false;

  @override
  Stream<ScanHit> get hits => _hits.stream;

  @override
  Stream<String> get errors => _errors.stream;

  @override
  Widget preview(BuildContext context, Rect? scanWindow) =>
      _source.preview(context);

  void _frame(ScanFrame frame) {
    if (_busy || _disposed) return;
    _busy = true;
    final window = _request.scanWindow;
    final input = window == null ? frame : cropFrame(frame, window);
    unawaited(
      _decode(input, _mask)
          .then<void>((r) {
            if (r != null && !_disposed) _hits.add(ScanHit(r.value, r.format));
          })
          .catchError((Object _) {
            if (!_disposed) _errors.add(ScanErrorCodes.failed);
          })
          .whenComplete(() => _busy = false),
    );
  }

  void _failed(Object error) {
    if (_disposed) return;
    _errors.add(
      error is ScanSourceException ? error.code : ScanErrorCodes.failed,
    );
  }

  @override
  Future<void> setTorch(bool on) => _source.setTorch(on);

  @override
  Future<void> dispose() async {
    _disposed = true;
    await _sub.cancel();
    await _source.dispose();
    await _hits.close();
    await _errors.close();
  }
}

/// Reads the camera with `mobile_scanner`: ML Kit on Android, Vision on iOS.
final class MobileScannerSession implements ScanSession {
  /// Opens the session; the camera starts when the preview is built.
  MobileScannerSession(ScanRequest request)
    : _formats = request.formats,
      _wantTorch = request.torch,
      _controller = ms.MobileScannerController(
        formats: scannerFormats(request.formats),
        facing: request.backCamera
            ? ms.CameraFacing.back
            : ms.CameraFacing.front,
        torchEnabled: request.torch,
      ) {
    _controller.addListener(_changed);
    _sub = _controller.barcodes.listen(_captured);
  }

  final List<String> _formats;
  final ms.MobileScannerController _controller;
  final StreamController<ScanHit> _hits = StreamController<ScanHit>.broadcast();
  final StreamController<String> _errors = StreamController<String>.broadcast();
  late final StreamSubscription<ms.BarcodeCapture> _sub;
  bool _wantTorch;
  bool _disposed = false;
  String? _reported;

  @override
  Stream<ScanHit> get hits => _hits.stream;

  @override
  Stream<String> get errors => _errors.stream;

  @override
  Widget preview(BuildContext context, Rect? scanWindow) => ms.MobileScanner(
    controller: _controller,
    scanWindow: scanWindow,
    errorBuilder: (context, error) => const ColoredBox(color: Colors.black),
  );

  void _captured(ms.BarcodeCapture capture) {
    for (final code in capture.barcodes) {
      final value = code.rawValue, format = formatName(code.format);
      if (value == null || format == null) continue;
      if (_formats.isNotEmpty && !_formats.contains(format)) continue;
      _hits.add(ScanHit(value, format));
    }
  }

  void _changed() {
    if (_disposed) return;
    final error = _controller.value.error;
    if (error != null) {
      final code = switch (error.errorCode) {
        ms.MobileScannerErrorCode.permissionDenied =>
          ScanErrorCodes.permissionDenied,
        ms.MobileScannerErrorCode.unsupported => ScanErrorCodes.unsupported,
        _ => ScanErrorCodes.failed,
      };
      // One report per kind of failure until the camera runs again.
      if (_reported != code) {
        _reported = code;
        _errors.add(code);
      }
      return;
    }
    _reported = null;
    final value = _controller.value;
    if (value.isRunning &&
        value.torchState != ms.TorchState.unavailable &&
        (value.torchState == ms.TorchState.on) != _wantTorch) {
      unawaited(_controller.toggleTorch());
    }
  }

  @override
  Future<void> setTorch(bool on) async {
    _wantTorch = on;
    _changed();
  }

  @override
  Future<void> dispose() async {
    _disposed = true;
    _controller.removeListener(_changed);
    await _sub.cancel();
    await _controller.dispose();
    await _hits.close();
    await _errors.close();
  }
}
