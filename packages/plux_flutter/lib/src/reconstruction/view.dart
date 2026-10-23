// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `PluxReconstructionView` (SEC-142, ADR-0082): the page of a reconstructed
/// moment, shown as the device presented it, under a banner that names the
/// moment and cannot be dismissed.
library;

import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:plux_flutter/src/core/plux.dart';
import 'package:plux_flutter/src/core/plux_view.dart';
import 'package:plux_flutter/src/reconstruction/reconstruction.dart';

/// The logical width a phone is shown at when the screen is wider.
const double _phoneWidth = 390;

/// The logical width a tablet is shown at when the screen is narrower; it is
/// scaled down to fit, so that the page lays out as it did on a tablet.
const double _tabletWidth = 834;

/// The first width, in logical pixels, of the tablet size classes (PXL's
/// `device.sizeClass`): a screen at least this wide shows a tablet as it is.
const double _tabletClassFrom = 600;

/// Shows the page of the reconstruction that `PluxConfig.reconstruction`
/// opened (SEC-142): the recorded route, laid out for the recorded device
/// class and text scale, in the recorded locale and theme.
///
/// The view has a [Navigator] of its own, so the banner stays above every
/// page the reconstructed page opens. Place it below a `MaterialApp` and
/// beside nothing that could cover the banner. Pages open without
/// parameters, since none are recorded.
final class PluxReconstructionView extends StatelessWidget {
  /// Creates the view.
  const PluxReconstructionView({super.key});

  @override
  Widget build(BuildContext context) {
    final moment = Plux.reconstruction;
    final route = moment?.route;
    if (moment == null || route == null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Text(
            moment == null
                ? 'No reconstruction is open.'
                : 'No page was recorded for this moment.',
            textAlign: TextAlign.center,
          ),
        ),
      );
    }
    return PluxScope(
      child: Column(
        children: [
          PluxReconstructionBanner(moment),
          Expanded(
            child: _Viewport(
              moment: moment,
              child: Navigator(
                onGenerateInitialRoutes: (_, _) => [
                  MaterialPageRoute<void>(
                    builder: (_) => PluxView(route),
                    settings: RouteSettings(name: route),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// The banner of a reconstruction: it names the moment and the release that
/// ran, and has no way to be closed (SEC-142).
final class PluxReconstructionBanner extends StatelessWidget {
  /// Creates the banner for [moment].
  const PluxReconstructionBanner(this.moment, {super.key});

  /// The reconstructed moment.
  final PluxReconstructionMoment moment;

  /// The text the banner shows.
  static String textFor(PluxReconstructionMoment moment) =>
      'Reconstructed moment ${formatMoment(moment.at)}, release '
      '${moment.releaseSequence}. Read-only: nothing here is live.';

  /// [at] as `2026-10-21 14:03:07 UTC`.
  static String formatMoment(DateTime at) {
    final t = at.toUtc();
    String two(int n) => n.toString().padLeft(2, '0');
    return '${t.year.toString().padLeft(4, '0')}-${two(t.month)}-${two(t.day)} '
        '${two(t.hour)}:${two(t.minute)}:${two(t.second)} UTC';
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final text = textFor(moment);
    return Semantics(
      container: true,
      liveRegion: true,
      label: text,
      child: Material(
        color: scheme.tertiaryContainer,
        child: SafeArea(
          bottom: false,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Row(
              children: [
                Icon(Icons.history, color: scheme.onTertiaryContainer),
                const SizedBox(width: 12),
                Expanded(
                  child: ExcludeSemantics(
                    child: Text(
                      text,
                      style: TextStyle(color: scheme.onTertiaryContainer),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Lays [child] out for the recorded device class and text scale.
final class _Viewport extends StatelessWidget {
  const _Viewport({required this.moment, required this.child});

  final PluxReconstructionMoment moment;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final media = MediaQuery.of(context);
    return LayoutBuilder(
      builder: (context, constraints) {
        final available = constraints.biggest;
        final width = moment.isTablet
            ? (available.width >= _tabletClassFrom
                  ? available.width
                  : _tabletWidth)
            : math.min(available.width, _phoneWidth);
        // The page sees the recorded class's width; a tablet shown on a
        // smaller screen is scaled to fit rather than reflowed.
        final scale = width > available.width ? available.width / width : 1.0;
        final height = available.height / scale;
        final page = SizedBox(
          width: width,
          height: height,
          child: MediaQuery(
            data: media.copyWith(
              size: Size(width, height),
              textScaler: TextScaler.linear(moment.textScale),
            ),
            child: child,
          ),
        );
        if (scale < 1) {
          return FittedBox(alignment: Alignment.topCenter, child: page);
        }
        return Align(alignment: Alignment.topCenter, child: page);
      },
    );
  }
}
