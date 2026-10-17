// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The inline scanner: a camera preview with a viewfinder, a torch button,
/// a hint, debounced detection, an error state with a retry, and a live
/// region that announces a read without speaking it.
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:plux_scanner/src/engine.dart';
import 'package:plux_scanner/src/session.dart';

/// A camera preview that reads codes (`QrScanner`, `BarcodeScanner`).
final class PluxScannerView extends StatefulWidget {
  /// Creates the scanner. [onDetected] gets every code that passes the
  /// debounce, once in total unless [continuous]; [onError] gets one of
  /// [ScanErrorCodes].
  const PluxScannerView({
    required this.config,
    this.formats = const [],
    this.torch = false,
    this.backCamera = true,
    this.scanWindow,
    this.continuous = false,
    this.overlayHint,
    this.semanticLabel,
    this.onDetected,
    this.onError,
    this.clock = DateTime.now,
    super.key,
  });

  /// The host's configuration.
  final PluxScannerConfig config;

  /// The schema `BarcodeFormat` members to read; every format when empty.
  final List<String> formats;

  /// Whether the torch is on.
  final bool torch;

  /// Whether to use the back camera.
  final bool backCamera;

  /// The part of the preview that is read: left, top, width, height as
  /// fractions; all of it when null.
  final List<double>? scanWindow;

  /// Whether to keep reading after the first code.
  final bool continuous;

  /// The line under the viewfinder.
  final String? overlayHint;

  /// What a screen reader announces for the scanner.
  final String? semanticLabel;

  /// A code was read.
  final void Function(ScanHit hit)? onDetected;

  /// The scanner failed with one of [ScanErrorCodes].
  final void Function(String code)? onError;

  /// The time, for the debounce.
  final DateTime Function() clock;

  @override
  State<PluxScannerView> createState() => _PluxScannerViewState();
}

final class _PluxScannerViewState extends State<PluxScannerView> {
  ScanSession? _session;
  StreamSubscription<ScanHit>? _hitSub;
  StreamSubscription<String>? _errorSub;
  String? _error;
  bool _torch = false;
  bool _done = false;
  String? _announcement;
  String? _lastValue;
  DateTime? _lastAt;
  int _generation = 0;

  @override
  void initState() {
    super.initState();
    _torch = widget.torch;
    unawaited(_open());
  }

  @override
  void didUpdateWidget(PluxScannerView old) {
    super.didUpdateWidget(old);
    final reopen =
        old.backCamera != widget.backCamera ||
        old.config != widget.config ||
        !_sameList(old.formats, widget.formats) ||
        !_sameList(old.scanWindow, widget.scanWindow);
    if (reopen) {
      _torch = widget.torch;
      unawaited(_open());
    } else if (old.torch != widget.torch) {
      _setTorch(widget.torch);
    }
  }

  @override
  void dispose() {
    _generation++;
    unawaited(_close());
    super.dispose();
  }

  Future<void> _close() async {
    final session = _session;
    _session = null;
    // The subscriptions end with the session; nothing waits for them.
    unawaited(_hitSub?.cancel());
    unawaited(_errorSub?.cancel());
    _hitSub = _errorSub = null;
    await session?.dispose();
  }

  Future<void> _open() async {
    final generation = ++_generation;
    await _close();
    if (!mounted) return;
    setState(() {
      _error = null;
      _done = false;
    });
    final ScannerEngine engine;
    try {
      engine = await widget.config.resolve();
    } on Object {
      _fail(ScanErrorCodes.failed, generation);
      return;
    }
    if (!mounted || generation != _generation) return;
    try {
      final session = widget.config.open(
        ScanRequest(
          formats: widget.formats,
          torch: _torch,
          backCamera: widget.backCamera,
          scanWindow: widget.scanWindow,
        ),
        engine,
      );
      _session = session;
      _hitSub = session.hits.listen(_hit);
      _errorSub = session.errors.listen((c) => _fail(c, generation));
      setState(() {});
    } on ScanSourceException catch (e) {
      _fail(e.code, generation);
    }
  }

  void _fail(String code, int generation) {
    if (!mounted || generation != _generation) return;
    unawaited(_close());
    setState(() => _error = code);
    widget.onError?.call(code);
  }

  void _hit(ScanHit hit) {
    if (_done) return;
    final now = widget.clock();
    final last = _lastAt;
    if (hit.value == _lastValue &&
        last != null &&
        now.difference(last) < widget.config.debounce) {
      return;
    }
    _lastValue = hit.value;
    _lastAt = now;
    if (!widget.continuous) _done = true;
    setState(() => _announcement = widget.config.texts.detected);
    widget.onDetected?.call(hit);
  }

  void _setTorch(bool on) {
    setState(() => _torch = on);
    unawaited(_session?.setTorch(on));
  }

  Rect? _window(Size size) {
    final w = widget.scanWindow;
    if (w == null || w.length != 4) return null;
    return Rect.fromLTWH(
      w[0] * size.width,
      w[1] * size.height,
      w[2] * size.width,
      w[3] * size.height,
    );
  }

  @override
  Widget build(BuildContext context) {
    final texts = widget.config.texts;
    return Semantics(
      container: true,
      explicitChildNodes: true,
      label: widget.semanticLabel,
      child: ColoredBox(
        color: Colors.black,
        child: _error != null
            ? _ErrorState(
                message: texts.forError(_error!),
                retry: texts.retry,
                onRetry: () => unawaited(_open()),
              )
            : _scanning(context, texts),
      ),
    );
  }

  Widget _scanning(BuildContext context, ScannerTexts texts) {
    final session = _session;
    return LayoutBuilder(
      builder: (context, box) {
        final size = box.biggest;
        final window = _window(size);
        final frame = window ?? _centered(size);
        return Stack(
          fit: StackFit.expand,
          children: [
            ExcludeSemantics(
              child: session == null
                  ? const SizedBox.shrink()
                  : session.preview(context, window),
            ),
            ExcludeSemantics(
              child: CustomPaint(
                painter: _ViewfinderPainter(
                  frame,
                  _done && !widget.continuous
                      ? Colors.greenAccent
                      : Colors.white,
                ),
              ),
            ),
            if (widget.overlayHint != null)
              Positioned(
                left: 16,
                right: 16,
                top: (frame.bottom + 12).clamp(0.0, size.height),
                child: Text(
                  widget.overlayHint!,
                  textAlign: TextAlign.center,
                  style: const TextStyle(color: Colors.white),
                ),
              ),
            Positioned(
              top: 4,
              right: 4,
              child: IconButton(
                tooltip: _torch ? texts.torchOff : texts.torchOn,
                color: Colors.white,
                icon: Icon(_torch ? Icons.flash_on : Icons.flash_off),
                onPressed: () => _setTorch(!_torch),
              ),
            ),
            // The live region speaks that a code was read, never the code.
            Semantics(
              container: true,
              liveRegion: true,
              label: _announcement,
              // A node of no size would not reach the semantics tree.
              child: const SizedBox.square(dimension: 1),
            ),
          ],
        );
      },
    );
  }

  static Rect _centered(Size size) {
    final side = size.shortestSide * 0.6;
    return Rect.fromCenter(
      center: size.center(Offset.zero),
      width: side,
      height: side,
    );
  }
}

bool _sameList<T>(List<T>? a, List<T>? b) {
  if (identical(a, b)) return true;
  if (a == null || b == null || a.length != b.length) return false;
  for (var i = 0; i < a.length; i++) {
    if (a[i] != b[i]) return false;
  }
  return true;
}

final class _ErrorState extends StatelessWidget {
  const _ErrorState({
    required this.message,
    required this.retry,
    required this.onRetry,
  });

  final String message;
  final String retry;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Center(
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const ExcludeSemantics(
            child: Icon(Icons.no_photography, color: Colors.white70, size: 48),
          ),
          const SizedBox(height: 12),
          Semantics(
            liveRegion: true,
            child: Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(color: Colors.white),
            ),
          ),
          const SizedBox(height: 16),
          FilledButton.tonal(onPressed: onRetry, child: Text(retry)),
        ],
      ),
    ),
  );
}

/// Four corner brackets around the part of the preview that is read.
final class _ViewfinderPainter extends CustomPainter {
  const _ViewfinderPainter(this.frame, this.color);

  final Rect frame;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4
      ..strokeCap = StrokeCap.round;
    final arm = (frame.shortestSide * 0.18).clamp(8.0, 32.0);
    void corner(Offset p, double dx, double dy) {
      canvas.drawLine(p, p + Offset(dx * arm, 0), paint);
      canvas.drawLine(p, p + Offset(0, dy * arm), paint);
    }

    corner(frame.topLeft, 1, 1);
    corner(frame.topRight, -1, 1);
    corner(frame.bottomLeft, 1, -1);
    corner(frame.bottomRight, -1, -1);
  }

  @override
  bool shouldRepaint(_ViewfinderPainter old) =>
      old.frame != frame || old.color != color;
}
