// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Builders of the P5 scrolling, layout and display widgets (WGT-020,
/// ADR-0071): `BackdropFilter`, `RefreshIndicator`,
/// `SliverPersistentHeader` and `MaterialBanner`. `Table`,
/// `ReorderableListView`, `NestedScrollView`, `DataTable` and `Stepper` are
/// built by `plux_components` (RT-060, RT-061).
///
/// The generator writes builders for P3 widgets only, so these read their
/// props by the permanent IDs of their descriptors in `schema/widgets/layer1`.
library;

import 'dart:ui' show ImageFilter;

import 'package:flutter/material.dart';
import 'package:plux_flutter/src/render/decoders.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/node_context.dart';

/// The permanent ID of `BackdropFilter`.
const int backdropFilterWidgetId = 112;

/// The permanent ID of `RefreshIndicator`.
const int refreshIndicatorWidgetId = 113;

/// The permanent ID of `SliverPersistentHeader`.
const int sliverPersistentHeaderWidgetId = 116;

/// The permanent ID of `MaterialBanner`.
const int materialBannerWidgetId = 119;

/// The builders of this group, by permanent widget ID.
const Map<int, NodeBuilder> scrollingBuilders = {
  backdropFilterWidgetId: _backdropFilter,
  refreshIndicatorWidgetId: _refreshIndicator,
  sliverPersistentHeaderWidgetId: _sliverPersistentHeader,
  materialBannerWidgetId: _materialBanner,
};

// ── Value types ────────────────────────────────────────────────────────────

/// Decodes a `BlurFilter` (value type 47).
ImageFilter? _decodeBlur(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  return ImageFilter.blur(
    sigmaX: asDouble(d, f.get(1, 'sigmaX')) ?? 0,
    sigmaY: asDouble(d, f.get(2, 'sigmaY')) ?? 0,
    tileMode: decodeTileMode(d, f.get(3, 'tileMode')),
  );
}

// ── BackdropFilter ─────────────────────────────────────────────────────────

Widget _backdropFilter(NodeContext c) {
  final child = c.slot(1);
  final filter = c.decode(1, _decodeBlur);
  if (filter == null) return child ?? const SizedBox.shrink();
  return BackdropFilter(
    filter: filter,
    blendMode: c.decode(2, decodeBlendMode) ?? BlendMode.srcOver,
    enabled: c.decode(3, asBool) ?? true,
    child: child,
  );
}

// ── RefreshIndicator ───────────────────────────────────────────────────────

Widget _refreshIndicator(NodeContext c) {
  return RefreshIndicator(
    displacement: c.decode(1, asDouble) ?? 40,
    edgeOffset: c.decode(2, asDouble) ?? 0,
    color: c.decode(3, asColor),
    backgroundColor: c.decode(4, asColor),
    semanticsLabel: c.decode(5, asString),
    semanticsValue: c.decode(6, asString),
    strokeWidth:
        c.decode(7, asDouble) ?? RefreshProgressIndicator.defaultStrokeWidth,
    triggerMode: switch (nameRefreshIndicatorTriggerMode(c, c.prop(8))) {
      'anywhere' => RefreshIndicatorTriggerMode.anywhere,
      _ => RefreshIndicatorTriggerMode.onEdge,
    },
    elevation: c.decode(9, asDouble) ?? 2,
    // The handlers run unawaited: the indicator completes once they have
    // been started, as the engine's fire is not awaitable from a builder.
    onRefresh: () async {
      if (c.handles(1)) c.fire(1);
    },
    child: c.slot(1) ?? c.missing('child'),
  );
}

// ── SliverPersistentHeader ─────────────────────────────────────────────────

final class _HeaderDelegate extends SliverPersistentHeaderDelegate {
  const _HeaderDelegate(this.min, this.max, this.child);

  final double min;
  final double max;
  final Widget child;

  @override
  double get minExtent => min;

  @override
  double get maxExtent => max;

  @override
  Widget build(
    BuildContext context,
    double shrinkOffset,
    bool overlapsContent,
  ) => SizedBox.expand(child: child);

  @override
  bool shouldRebuild(_HeaderDelegate old) =>
      old.min != min || old.max != max || old.child != child;
}

Widget _sliverPersistentHeader(NodeContext c) {
  final min = c.decode(3, asDouble) ?? c.missing('minExtent');
  final max = c.decode(4, asDouble) ?? c.missing('maxExtent');
  return SliverPersistentHeader(
    pinned: c.decode(1, asBool) ?? false,
    floating: c.decode(2, asBool) ?? false,
    delegate: _HeaderDelegate(
      min,
      max < min ? min : max,
      c.slot(1) ?? c.missing('child'),
    ),
  );
}

// ── MaterialBanner ─────────────────────────────────────────────────────────

Widget _materialBanner(NodeContext c) {
  final actions = c.slotList(3);
  return Semantics(
    container: true,
    liveRegion: true,
    child: MaterialBanner(
      content: c.slot(1) ?? c.missing('content'),
      contentTextStyle: c.decode(1, decodeTextStyle),
      elevation: c.decode(2, asDouble),
      backgroundColor: c.decode(3, asColor),
      surfaceTintColor: c.decode(4, asColor),
      shadowColor: c.decode(5, asColor),
      dividerColor: c.decode(6, asColor),
      padding: c.decode(7, decodeEdgeInsets),
      margin: c.decode(8, decodeEdgeInsets),
      leadingPadding: c.decode(9, decodeEdgeInsets),
      leading: c.slot(2),
      forceActionsBelow: c.decode(10, asBool) ?? false,
      overflowAlignment: switch (nameOverflowBarAlignment(c, c.prop(11))) {
        'start' => OverflowBarAlignment.start,
        'center' => OverflowBarAlignment.center,
        _ => OverflowBarAlignment.end,
      },
      minActionBarHeight: c.decode(12, asDouble) ?? 52,
      onVisible: c.handles(1) ? () => c.fire(1) : null,
      actions: actions.isEmpty ? const [SizedBox.shrink()] : actions,
    ),
  );
}

/// Gives a list its own [Overlay], so the item being dragged is drawn below
/// the node's scope and keeps its providers, theme and bindings.
