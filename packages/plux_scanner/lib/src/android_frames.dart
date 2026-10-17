// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The default camera of the ZXing engine on Android: CameraX in
/// `PluxScannerPlugin` shows its preview in a Flutter texture and streams
/// the luminance plane of at most ten frames a second (decision B21).
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:plux_scanner/src/session.dart';
import 'package:plux_scanner/src/zxing_ffi.dart';

/// The most pixels along either side of a frame that is accepted.
const int maxFrameSide = 4096;

/// A [ScanFrameSource] on the Android plugin's CameraX camera.
final class AndroidFrameSource implements ScanFrameSource {
  /// Starts the camera for [request]. The channels are the plugin's.
  AndroidFrameSource(
    ScanRequest request, {
    this._methods = const MethodChannel('dev.plux/scanner'),
    this._events = const EventChannel('dev.plux/scanner/frames'),
  }) : _torch = request.torch {
    unawaited(_start(request));
  }

  final MethodChannel _methods;
  final EventChannel _events;
  final StreamController<ScanFrame> _frames = StreamController<ScanFrame>();
  final ValueNotifier<_Preview?> _preview = ValueNotifier<_Preview?>(null);
  StreamSubscription<Object?>? _sub;
  bool _torch;
  bool _disposed = false;

  Future<void> _start(ScanRequest request) async {
    try {
      final info = await _methods.invokeMapMethod<String, Object?>('start', {
        'back': request.backCamera,
        'torch': request.torch,
      });
      final id = info?['textureId'], w = info?['width'], h = info?['height'];
      final rotation = info?['rotation'];
      if (id is! int || w is! int || h is! int || w <= 0 || h <= 0) {
        throw const ScanSourceException(ScanErrorCodes.failed);
      }
      if (_disposed) return;
      _preview.value = _Preview(id, w, h, rotation is int ? rotation : 0);
      _sub = _events.receiveBroadcastStream().listen(
        _frame,
        onError: _failed,
        cancelOnError: false,
      );
    } on PlatformException catch (e) {
      _failed(ScanSourceException(_code(e.code)));
    } on MissingPluginException {
      _failed(const ScanSourceException(ScanErrorCodes.unsupported));
    } on ScanSourceException catch (e) {
      _failed(e);
    }
  }

  static String _code(String platformCode) => switch (platformCode) {
    ScanErrorCodes.permissionDenied => ScanErrorCodes.permissionDenied,
    ScanErrorCodes.unsupported => ScanErrorCodes.unsupported,
    _ => ScanErrorCodes.failed,
  };

  void _failed(Object error) {
    if (_disposed) return;
    _frames.addError(
      error is PlatformException
          ? ScanSourceException(_code(error.code))
          : error,
    );
  }

  /// A frame of the plugin: a map of `bytes`, `width`, `height`, `rowStride`
  /// and `rotation`. Anything else, or a frame beyond [maxFrameSide], is
  /// dropped.
  void _frame(Object? event) {
    if (_disposed || event is! Map) return;
    final bytes = event['bytes'], w = event['width'], h = event['height'];
    final stride = event['rowStride'], rotation = event['rotation'];
    if (bytes is! Uint8List || w is! int || h is! int || stride is! int) {
      return;
    }
    if (w <= 0 || h <= 0 || w > maxFrameSide || h > maxFrameSide) return;
    if (stride < w || stride > maxFrameSide * 4) return;
    if (bytes.length < stride * (h - 1) + w) return;
    final turns = rotation is int ? rotation : 0;
    if (turns % 90 != 0) return;
    _frames.add(ScanFrame(bytes, w, h, stride: stride, rotation: turns));
  }

  @override
  Stream<ScanFrame> get frames => _frames.stream;

  @override
  Widget preview(BuildContext context) => ValueListenableBuilder<_Preview?>(
    valueListenable: _preview,
    builder: (context, p, _) {
      if (p == null) return const SizedBox.shrink();
      return ClipRect(
        child: RotatedBox(
          quarterTurns: p.rotation ~/ 90,
          child: FittedBox(
            fit: BoxFit.cover,
            child: SizedBox(
              width: p.width.toDouble(),
              height: p.height.toDouble(),
              child: Texture(textureId: p.textureId),
            ),
          ),
        ),
      );
    },
  );

  @override
  Future<void> setTorch(bool on) async {
    if (_torch == on) return;
    _torch = on;
    try {
      await _methods.invokeMethod<void>('setTorch', {'on': on});
    } on PlatformException {
      // The torch is a convenience: a camera without one keeps scanning.
    } on MissingPluginException {
      // Nothing to switch.
    }
  }

  @override
  Future<void> dispose() async {
    if (_disposed) return;
    _disposed = true;
    unawaited(_sub?.cancel());
    try {
      await _methods.invokeMethod<void>('stop');
    } on PlatformException {
      // The camera is gone with the engine.
    } on MissingPluginException {
      // No plugin, no camera.
    }
    _preview.dispose();
    unawaited(_frames.close());
  }
}

final class _Preview {
  const _Preview(this.textureId, this.width, this.height, this.rotation);

  final int textureId;
  final int width;
  final int height;
  final int rotation;
}
