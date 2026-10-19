// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Builders of the Cupertino and structural widgets of K1d (ADR-0071,
/// WGT-020): `CupertinoNavigationBar`, `CupertinoTabBar`, `DataScope` and
/// `NativeSlot`; the pickers and the context menu are built by
/// `plux_components`. Prop, event and slot IDs are the permanent IDs of the
/// descriptors in `schema/widgets/layer1`.
library;

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart' show Theme;
import 'package:plux_flutter/src/render/decoders.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/node_context.dart';

/// The hand-written builders of this group by permanent widget ID.
const Map<int, NodeBuilder> cupertinoMoreBuilders = {
  dataScopeWidgetId: _buildDataScope,
  nativeSlotWidgetId: _buildNativeSlot,
  cupertinoNavigationBarWidgetId: _buildNavigationBar,
  cupertinoTabBarWidgetId: _buildTabBar,
};

/// The permanent widget ID of `DataScope`.
const int dataScopeWidgetId = 109;

/// The permanent widget ID of `NativeSlot`.
const int nativeSlotWidgetId = 110;

/// The permanent widget ID of `CupertinoNavigationBar`.
const int cupertinoNavigationBarWidgetId = 154;

/// The permanent widget ID of `CupertinoTabBar`.
const int cupertinoTabBarWidgetId = 155;

// ── DataScope ───────────────────────────────────────────────────────────────

/// Props `status` (1), `isEmpty` (2) and `source` (3); slots `success` (1),
/// `loading` (2), `empty` (3), `error` (4). With a `source`, the state comes
/// from that data source: its status, and empty when its value is null, an
/// empty list or an empty map; an explicit `status` or `isEmpty` wins.
Widget _buildDataScope(NodeContext c) {
  final name = c.decode(3, asString);
  final src = name == null ? null : c.dataSource(name);
  final status =
      nameListStatus(c, c.prop(1)) ?? (src?['status'] as String?) ?? 'ready';
  final empty =
      c.decode(2, asBool) ?? (src != null && _isEmptyValue(src['value']));
  final shown = switch (status) {
    'loading' => c.slot(2),
    'error' => c.slot(4),
    _ when empty => c.slot(3),
    _ => c.slot(1),
  };
  return shown ?? const SizedBox.shrink();
}

bool _isEmptyValue(Object? v) =>
    v == null || (v is List && v.isEmpty) || (v is Map && v.isEmpty);

// ── NativeSlot ──────────────────────────────────────────────────────────────

/// The compiler turns a `NativeSlot` with a literal `name` into the node
/// whose type is that name, so a `NativeSlot` that reaches the runtime has
/// a name that is not a literal: the node fails.
Widget _buildNativeSlot(NodeContext c) => c.missing('name');

// ── CupertinoNavigationBar ──────────────────────────────────────────────────

/// Props 1–8; slots `leading` (1), `middle` (2), `trailing` (3).
Widget _buildNavigationBar(NodeContext c) => CupertinoNavigationBar(
  leading: c.slot(1),
  middle: c.slot(2),
  trailing: c.slot(3),
  automaticallyImplyLeading: c.decode(1, asBool) ?? true,
  automaticallyImplyMiddle: c.decode(2, asBool) ?? true,
  previousPageTitle: c.decode(3, asString),
  backgroundColor: c.decode(4, asColor),
  automaticBackgroundVisibility: c.decode(5, asBool) ?? true,
  enableBackgroundFilterBlur: c.decode(6, asBool) ?? true,
  brightness: c.decode(7, decodeBrightness),
  transitionBetweenRoutes: c.decode(8, asBool) ?? true,
);

// ── CupertinoTabBar ─────────────────────────────────────────────────────────

/// A `TabBarItem`: `label` (1), `icon` (2).
BottomNavigationBarItem? _decodeTabBarItem(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final label = asString(d, f.get(1, 'label'));
  final icon = decodeIconWidget(d, f.get(2, 'icon'));
  if (label == null || icon == null) return null;
  return BottomNavigationBarItem(icon: icon, label: label);
}

List<BottomNavigationBarItem>? _tabItems(Decoding d, Object? v) =>
    asList(d, v, _decodeTabBarItem);

/// Props `items` (1), `currentIndex` (2), `backgroundColor` (3),
/// `activeColor` (4), `inactiveColor` (5), `iconSize` (6), `height` (7);
/// event `onTap` (1).
Widget _buildTabBar(NodeContext c) {
  final items = c.decode(1, _tabItems) ?? c.missing('items');
  if (items.length < 2) c.missing('items');
  return c.controlled<int>(
    1,
    (c.decode(2, asInt) ?? 0).clamp(0, items.length - 1),
    // Cupertino's own grey inactive colour is 2.5:1 on its default
    // background; the theme's variant text colour meets A11Y-004 instead.
    (value, onChanged) => Builder(
      builder: (context) => CupertinoTabBar(
        items: items,
        currentIndex: value.clamp(0, items.length - 1),
        onTap: onChanged,
        backgroundColor: c.decode(3, asColor),
        activeColor: c.decode(4, asColor),
        inactiveColor:
            c.decode(5, asColor) ??
            Theme.of(context).colorScheme.onSurfaceVariant,
        iconSize: c.decode(6, asDouble) ?? 30.0,
        height: c.decode(7, asDouble) ?? 50.0,
      ),
    ),
  );
}
