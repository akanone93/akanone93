// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `SignaturePad` (ADR-0071): a box the user signs in with a finger, drawn
/// in house with no dependency. A signature leaves the widget as SVG path
/// data or as a base64 PNG, through `onChanged`.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:math' as math;
import 'dart:ui' as ui;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';

/// How a signature is handed to the page.
enum SignatureFormat {
  /// SVG path data, in the pad's own logical pixels: `M x y L x y ...`, one
  /// `M` per stroke.
  svg,

  /// A base64 PNG of the pad's size, dark strokes on a transparent
  /// background.
  png,
}

/// The SVG path data of [strokes]: one `M` per stroke, coordinates rounded
/// to a tenth of a pixel. A stroke of one point is a dot.
String signaturePathData(List<List<Offset>> strokes) {
  String n(double v) {
    final s = v.toStringAsFixed(1);
    return s.endsWith('.0') ? s.substring(0, s.length - 2) : s;
  }

  final b = StringBuffer();
  for (final stroke in strokes) {
    if (stroke.isEmpty) continue;
    if (b.isNotEmpty) b.write(' ');
    b.write('M ${n(stroke.first.dx)} ${n(stroke.first.dy)}');
    for (final p in stroke.length == 1 ? [stroke.first] : stroke.skip(1)) {
      b.write(' L ${n(p.dx)} ${n(p.dy)}');
    }
  }
  return b.toString();
}

/// Draws [strokes] with a round pen of [width] in [color].
void paintSignature(
  Canvas canvas,
  List<List<Offset>> strokes,
  double width,
  Color color,
) {
  final pen = Paint()
    ..color = color
    ..style = PaintingStyle.stroke
    ..strokeWidth = width
    ..strokeCap = StrokeCap.round
    ..strokeJoin = StrokeJoin.round;
  for (final stroke in strokes) {
    if (stroke.length == 1) {
      canvas.drawCircle(stroke.first, width / 2, Paint()..color = color);
      continue;
    }
    final path = Path()..moveTo(stroke.first.dx, stroke.first.dy);
    for (final p in stroke.skip(1)) {
      path.lineTo(p.dx, p.dy);
    }
    canvas.drawPath(path, pen);
  }
}

/// A base64 PNG of [strokes] on a transparent [size] image.
Future<String> signaturePng(
  List<List<Offset>> strokes,
  Size size,
  double width,
) async {
  final recorder = ui.PictureRecorder();
  paintSignature(Canvas(recorder), strokes, width, const Color(0xFF000000));
  final picture = recorder.endRecording();
  final image = await picture.toImage(
    math.max(1, size.width.ceil()),
    math.max(1, size.height.ceil()),
  );
  try {
    final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
    return base64.encode(bytes!.buffer.asUint8List());
  } finally {
    image.dispose();
    picture.dispose();
  }
}

/// A pad to sign in. [onChanged] gets the signature after each stroke, and
/// an empty string once it is cleared.
final class PluxSignaturePad extends StatefulWidget {
  /// Creates a pad.
  const PluxSignaturePad({
    this.format = SignatureFormat.svg,
    this.strokeWidth = 2,
    this.height = 160,
    this.clearLabel = 'Clear',
    this.semanticLabel,
    this.onChanged,
    super.key,
  });

  /// What [onChanged] receives.
  final SignatureFormat format;

  /// The pen width in logical pixels.
  final double strokeWidth;

  /// The pad's height.
  final double height;

  /// The clear button's label.
  final String clearLabel;

  /// What the signature is for, spoken for the pad.
  final String? semanticLabel;

  /// Called after each stroke with the signature, and with an empty string
  /// when cleared.
  final ValueChanged<String>? onChanged;

  @override
  State<PluxSignaturePad> createState() => _PluxSignaturePadState();
}

final class _PluxSignaturePadState extends State<PluxSignaturePad> {
  final List<List<Offset>> _strokes = [];
  Size _size = Size.zero;
  int _revision = 0;

  void _down(PointerDownEvent e) =>
      setState(() => _strokes.add([e.localPosition]));

  void _move(PointerMoveEvent e) {
    if (_strokes.isEmpty) return;
    setState(() => _strokes.last.add(e.localPosition));
  }

  void _up(PointerEvent e) => unawaited(_emit());

  Future<void> _emit() async {
    final revision = ++_revision;
    final strokes = [for (final s in _strokes) List<Offset>.of(s)];
    final String value;
    if (strokes.isEmpty) {
      value = '';
    } else if (widget.format == SignatureFormat.svg) {
      value = signaturePathData(strokes);
    } else {
      value = await signaturePng(strokes, _size, widget.strokeWidth);
    }
    // A newer stroke or a clear has superseded this one.
    if (!mounted || revision != _revision) return;
    widget.onChanged?.call(value);
  }

  void _clear() {
    setState(_strokes.clear);
    unawaited(_emit());
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Semantics(
          container: true,
          label: widget.semanticLabel,
          image: true,
          child: ExcludeSemantics(
            child: SizedBox(
              height: widget.height,
              child: DecoratedBox(
                decoration: BoxDecoration(
                  color: scheme.surfaceContainerHighest,
                  border: Border.all(color: scheme.outline),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(8),
                  // Wins the gesture arena at once, so that a page that
                  // scrolls does not take the stroke.
                  child: RawGestureDetector(
                    gestures: {
                      EagerGestureRecognizer:
                          GestureRecognizerFactoryWithHandlers<
                            EagerGestureRecognizer
                          >(EagerGestureRecognizer.new, (_) {}),
                    },
                    child: Listener(
                      behavior: HitTestBehavior.opaque,
                      onPointerDown: _down,
                      onPointerMove: _move,
                      onPointerUp: _up,
                      onPointerCancel: _up,
                      child: LayoutBuilder(
                        builder: (context, constraints) {
                          _size = constraints.biggest;
                          return CustomPaint(
                            painter: _SignaturePainter(
                              [for (final s in _strokes) List.of(s)],
                              widget.strokeWidth,
                              scheme.onSurface,
                            ),
                            size: Size.infinite,
                          );
                        },
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
        Align(
          alignment: AlignmentDirectional.centerEnd,
          child: TextButton(
            onPressed: _strokes.isEmpty ? null : _clear,
            child: Text(widget.clearLabel),
          ),
        ),
      ],
    );
  }
}

final class _SignaturePainter extends CustomPainter {
  _SignaturePainter(this.strokes, this.width, this.color);

  final List<List<Offset>> strokes;
  final double width;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) =>
      paintSignature(canvas, strokes, width, color);

  @override
  bool shouldRepaint(_SignaturePainter old) => true;
}
