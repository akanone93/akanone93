// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `KycCameraCapture` (SEC-090, plan P6 §5.10): a capture step for an ID
/// document or a selfie. It shows a framing guide, takes the photo through
/// the registered `PluxKycCapture` (`plux_media`), shows the photo under the
/// guide with what the quality checks found, and lets the user use it or
/// retake it. Only a photo that passed the checks can be used.
library;

import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';

/// A photo the capture service took, as the widget shows it.
final class KycShot {
  /// Creates the shot.
  const KycShot({
    required this.path,
    required this.reference,
    this.problems = const [],
  });

  /// Where the photo is, for the preview.
  final String path;

  /// The opaque reference the `onCaptured` event carries.
  final String reference;

  /// Why the photo is not good enough: `blurry`, `dark` or `bright`.
  final List<String> problems;
}

/// What each quality problem tells the user.
const Map<String, String> kycProblemMessages = {
  'blurry': 'The photo is blurry. Hold the camera steady and retake it.',
  'dark': 'The photo is too dark. Move to a brighter place and retake it.',
  'bright': 'The photo is too bright. Avoid glare and retake it.',
};

/// The capture step: guide, capture button, review.
final class KycCameraCaptureView extends StatefulWidget {
  /// Creates the step for [mode], a `KycCaptureMode` member name. [capture]
  /// takes the photo, or is null where it cannot be taken, which disables
  /// the button. [onCaptured] gets the reference of the photo the user
  /// accepts.
  const KycCameraCaptureView({
    super.key,
    required this.mode,
    required this.capture,
    required this.onCaptured,
    this.onActivity,
  });

  /// `document` or `selfie`.
  final String mode;

  /// Takes a photo; null when the user cancels.
  final Future<KycShot?> Function()? capture;

  /// Called with the reference of the accepted photo.
  final ValueChanged<String>? onCaptured;

  /// Called on every press, for the inactivity lock (SEC-094).
  final VoidCallback? onActivity;

  @override
  State<KycCameraCaptureView> createState() => _KycCameraCaptureViewState();
}

final class _KycCameraCaptureViewState extends State<KycCameraCaptureView> {
  KycShot? _shot;
  bool _busy = false;

  bool get _selfie => widget.mode == 'selfie';

  Future<void> _take() async {
    final capture = widget.capture;
    if (capture == null || _busy) return;
    widget.onActivity?.call();
    setState(() => _busy = true);
    KycShot? shot;
    try {
      shot = await capture();
    } finally {
      if (mounted) {
        setState(() {
          _busy = false;
          _shot = shot ?? _shot;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final shot = _shot;
    final theme = Theme.of(context);
    final hint = _selfie
        ? 'Center your face in the oval.'
        : 'Fit the whole document inside the frame.';
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Semantics(
          label: hint,
          image: true,
          child: AspectRatio(
            aspectRatio: _selfie ? 3 / 4 : 1.586,
            child: Stack(
              fit: StackFit.expand,
              children: [
                ColoredBox(color: theme.colorScheme.surfaceContainerHighest),
                if (shot != null)
                  Image.file(
                    File(shot.path),
                    fit: BoxFit.cover,
                    cacheWidth: 800,
                    excludeFromSemantics: true,
                    errorBuilder: (context, error, stack) =>
                        const SizedBox.shrink(),
                  ),
                CustomPaint(
                  painter: _GuidePainter(
                    oval: _selfie,
                    color: shot != null && shot.problems.isNotEmpty
                        ? theme.colorScheme.error
                        : theme.colorScheme.primary,
                  ),
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 8),
        if (shot == null)
          Text(hint, style: theme.textTheme.bodyMedium)
        else if (shot.problems.isNotEmpty)
          Semantics(
            liveRegion: true,
            child: Column(
              children: [
                for (final p in shot.problems)
                  Text(
                    kycProblemMessages[p] ?? 'The photo is not good enough.',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: theme.colorScheme.error,
                    ),
                  ),
              ],
            ),
          )
        else
          Semantics(
            liveRegion: true,
            child: Text(
              'The photo looks good.',
              style: theme.textTheme.bodyMedium,
            ),
          ),
        const SizedBox(height: 8),
        Wrap(
          spacing: 12,
          children: [
            OutlinedButton(
              onPressed: widget.capture == null || _busy
                  ? null
                  : () => unawaited(_take()),
              child: Text(shot == null ? 'Take photo' : 'Retake'),
            ),
            if (shot != null)
              FilledButton(
                onPressed:
                    shot.problems.isNotEmpty ||
                        _busy ||
                        widget.onCaptured == null
                    ? null
                    : () {
                        widget.onActivity?.call();
                        widget.onCaptured!(shot.reference);
                      },
                child: const Text('Use photo'),
              ),
          ],
        ),
      ],
    );
  }
}

/// The framing guide: the area outside the frame dimmed, and the frame.
final class _GuidePainter extends CustomPainter {
  const _GuidePainter({required this.oval, required this.color});

  final bool oval;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final frame = oval
        ? Rect.fromCenter(
            center: size.center(Offset.zero),
            width: size.width * 0.62,
            height: size.height * 0.72,
          )
        : Rect.fromLTWH(
            size.width * 0.06,
            size.height * 0.08,
            size.width * 0.88,
            size.height * 0.84,
          );
    final hole = Path()
      ..addRect(Offset.zero & size)
      ..addPath(
        oval
            ? (Path()..addOval(frame))
            : (Path()..addRRect(RRect.fromRectXY(frame, 12, 12))),
        Offset.zero,
      )
      ..fillType = PathFillType.evenOdd;
    canvas
      ..drawPath(hole, Paint()..color = const Color(0x66000000))
      ..drawPath(
        oval
            ? (Path()..addOval(frame))
            : (Path()..addRRect(RRect.fromRectXY(frame, 12, 12))),
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = 3
          ..color = color,
      );
  }

  @override
  bool shouldRepaint(_GuidePainter old) =>
      old.oval != oval || old.color != color;
}
