// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The small pill that says whether the phone is live with Studio (DEV-020).
library;

import 'package:flutter/material.dart';
import 'package:plux_dev/src/ui/theme.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// The words and colour of a link state; null is no session.
(String, Color) statusOf(DevLinkState? state) => switch (state) {
  DevLinkState.live => ('Live', DevColors.live),
  DevLinkState.connecting => ('Connecting', DevColors.warn),
  DevLinkState.reconnecting => ('Reconnecting', DevColors.warn),
  DevLinkState.ended || null => ('Offline', DevColors.muted),
};

/// "Live", "Connecting", "Reconnecting" or "Offline", with a coloured dot
/// that pulses while the phone connects.
final class StatusPill extends StatefulWidget {
  /// Creates the pill for [state].
  const StatusPill({super.key, required this.state, this.onTap});

  /// Where the link stands; null when the phone is in no session.
  final DevLinkState? state;

  /// Runs on a tap.
  final VoidCallback? onTap;

  @override
  State<StatusPill> createState() => _StatusPillState();
}

final class _StatusPillState extends State<StatusPill>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 900),
  );

  bool get _connecting =>
      widget.state == DevLinkState.connecting ||
      widget.state == DevLinkState.reconnecting;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _sync();
  }

  @override
  void didUpdateWidget(StatusPill old) {
    super.didUpdateWidget(old);
    _sync();
  }

  void _sync() {
    final still = MediaQuery.maybeDisableAnimationsOf(context) ?? false;
    if (_connecting && !still) {
      if (!_pulse.isAnimating) _pulse.repeat(reverse: true);
    } else {
      _pulse
        ..stop()
        ..value = 1;
    }
  }

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final (label, colour) = statusOf(widget.state);
    return Semantics(
      label: 'Connection: $label',
      button: widget.onTap != null,
      liveRegion: true,
      excludeSemantics: true,
      child: Material(
        color: Color.alphaBlend(
          colour.withValues(alpha: 0.12),
          DevColors.surface,
        ),
        shape: const StadiumBorder(),
        child: InkWell(
          customBorder: const StadiumBorder(),
          onTap: widget.onTap,
          child: ConstrainedBox(
            constraints: const BoxConstraints(minHeight: 30),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  FadeTransition(
                    opacity: Tween<double>(begin: 0.35, end: 1).animate(_pulse),
                    child: DecoratedBox(
                      decoration: BoxDecoration(
                        color: colour,
                        shape: BoxShape.circle,
                      ),
                      child: const SizedBox.square(dimension: 7),
                    ),
                  ),
                  const SizedBox(width: 6),
                  Text(
                    label,
                    style: TextStyle(
                      color: colour == DevColors.muted
                          ? DevColors.muted
                          : Color.lerp(colour, DevColors.ink, 0.25),
                      fontWeight: FontWeight.w600,
                      fontSize: 13,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
