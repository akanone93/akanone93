// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Builders of the Cupertino and structural widgets of K1d (ADR-0071,
/// WGT-020): `CupertinoNavigationBar`, `CupertinoTabBar`, `CupertinoPicker`,
/// `CupertinoDatePicker`, `CupertinoContextMenu`, `DataScope` and
/// `NativeSlot`. Prop, event and slot IDs are the permanent IDs of the
/// descriptors in `schema/widgets/layer1`.
library;

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart' show Theme;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:plux_flutter/src/assets/icon_fonts.dart';
import 'package:plux_flutter/src/pxl/values.dart';
import 'package:plux_flutter/src/render/decoders.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/node_context.dart';
import 'package:plux_flutter/src/render/plux_icon.dart';
import 'package:plux_flutter/src/render/scope.dart';

/// The hand-written builders of this group by permanent widget ID.
const Map<int, NodeBuilder> cupertinoMoreBuilders = {
  dataScopeWidgetId: _buildDataScope,
  nativeSlotWidgetId: _buildNativeSlot,
  cupertinoNavigationBarWidgetId: _buildNavigationBar,
  cupertinoTabBarWidgetId: _buildTabBar,
  cupertinoPickerWidgetId: _buildPicker,
  cupertinoDatePickerWidgetId: _buildDatePicker,
  cupertinoContextMenuWidgetId: _buildContextMenu,
};

/// The permanent widget ID of `DataScope`.
const int dataScopeWidgetId = 109;

/// The permanent widget ID of `NativeSlot`.
const int nativeSlotWidgetId = 110;

/// The permanent widget ID of `CupertinoNavigationBar`.
const int cupertinoNavigationBarWidgetId = 154;

/// The permanent widget ID of `CupertinoTabBar`.
const int cupertinoTabBarWidgetId = 155;

/// The permanent widget ID of `CupertinoPicker`.
const int cupertinoPickerWidgetId = 156;

/// The permanent widget ID of `CupertinoDatePicker`.
const int cupertinoDatePickerWidgetId = 157;

/// The permanent widget ID of `CupertinoContextMenu`.
const int cupertinoContextMenuWidgetId = 158;

// ── DataScope ───────────────────────────────────────────────────────────────

/// Props `status` (1) and `isEmpty` (2); slots `success` (1), `loading` (2),
/// `empty` (3), `error` (4).
Widget _buildDataScope(NodeContext c) {
  final status = nameListStatus(c, c.prop(1)) ?? 'ready';
  final shown = switch (status) {
    'loading' => c.slot(2),
    'error' => c.slot(4),
    _ when c.decode(2, asBool) ?? false => c.slot(3),
    _ => c.slot(1),
  };
  return shown ?? const SizedBox.shrink();
}

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

// ── CupertinoPicker ─────────────────────────────────────────────────────────

/// Props `itemExtent` (1), `selectedIndex` (2), `diameterRatio` (3),
/// `backgroundColor` (4), `offAxisFraction` (5), `useMagnifier` (6),
/// `magnification` (7), `squeeze` (8), `looping` (9); event
/// `onSelectedItemChanged` (1); children.
Widget _buildPicker(NodeContext c) {
  final children = c.children();
  if (children.isEmpty) return const SizedBox.shrink();
  return _Wheel(
    selected: c.decode(2, asInt) ?? 0,
    count: children.length,
    onChanged: c.handles(1) ? (i) => c.fire(1, i) : null,
    build: (controller, onChanged) => CupertinoPicker(
      itemExtent: c.decode(1, asDouble) ?? c.missing('itemExtent'),
      scrollController: controller,
      onSelectedItemChanged: onChanged,
      diameterRatio: c.decode(3, asDouble) ?? 1.07,
      backgroundColor: c.decode(4, asColor),
      offAxisFraction: c.decode(5, asDouble) ?? 0.0,
      useMagnifier: c.decode(6, asBool) ?? false,
      magnification: c.decode(7, asDouble) ?? 1.0,
      squeeze: c.decode(8, asDouble) ?? 1.45,
      looping: c.decode(9, asBool) ?? false,
      children: children,
    ),
  );
}

/// Owns the wheel's scroll controller: `selected` is the state the wheel
/// follows, and the user's scrolling is reported through `onChanged`.
final class _Wheel extends StatefulWidget {
  const _Wheel({
    required this.selected,
    required this.count,
    required this.onChanged,
    required this.build,
  });

  final int selected;
  final int count;
  final ValueChanged<int>? onChanged;
  final Widget Function(
    FixedExtentScrollController controller,
    ValueChanged<int> onChanged,
  )
  build;

  @override
  State<_Wheel> createState() => _WheelState();
}

final class _WheelState extends State<_Wheel> {
  late final FixedExtentScrollController _controller =
      FixedExtentScrollController(initialItem: _clamp(widget.selected));
  int _reported = -1;

  int _clamp(int i) => i.clamp(0, widget.count - 1);

  @override
  void didUpdateWidget(_Wheel old) {
    super.didUpdateWidget(old);
    final want = _clamp(widget.selected);
    if (widget.selected != old.selected &&
        want != _reported &&
        _controller.hasClients &&
        _controller.selectedItem != want) {
      _controller.jumpToItem(want);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.build(_controller, (i) {
    _reported = i;
    widget.onChanged?.call(i);
  });
}

// ── CupertinoDatePicker ─────────────────────────────────────────────────────

/// A dateTime as the wall clock of its own offset, so that the picker shows
/// the same fields on every device.
DateTime? _wallClock(Object? v) => v is PxlDateTime
    ? DateTime.fromMillisecondsSinceEpoch(
        v.millis + v.offset * 60000,
        isUtc: true,
      )
    : null;

PxlDateTime _fromWallClock(DateTime wall, int offset) =>
    PxlDateTime(wall.millisecondsSinceEpoch - offset * 60000, offset);

int? _minuteInterval(Decoding d, Object? v) =>
    v is int && v > 0 && 60 % v == 0 ? v : null;

/// Props `mode` (1), `initialDateTime` (2), `minimumDate` (3),
/// `maximumDate` (4), `minimumYear` (5), `maximumYear` (6),
/// `minuteInterval` (7), `use24hFormat` (8), `backgroundColor` (9),
/// `showDayOfWeek` (10), `showTimeSeparator` (11), `itemExtent` (12);
/// event `onDateTimeChanged` (1).
Widget _buildDatePicker(NodeContext c) {
  final initialValue = c.prop(2);
  final offset = initialValue is PxlDateTime ? initialValue.offset : 0;
  final interval = c.decode(7, _minuteInterval) ?? 1;
  var initial = _wallClock(initialValue) ?? DateTime.utc(2000);
  final min = _wallClock(c.prop(3));
  final max = _wallClock(c.prop(4));
  if (min != null && initial.isBefore(min)) initial = min;
  if (max != null && initial.isAfter(max)) initial = max;
  initial = DateTime.utc(
    initial.year,
    initial.month,
    initial.day,
    initial.hour,
    initial.minute - initial.minute % interval,
  );
  final maxYear = c.decode(6, asInt);
  return CupertinoDatePicker(
    mode:
        c.decode(1, decodeCupertinoDatePickerMode) ??
        CupertinoDatePickerMode.dateAndTime,
    initialDateTime: initial,
    minimumDate: min,
    maximumDate: max,
    minimumYear: c.decode(5, asInt) ?? 1,
    maximumYear: maxYear,
    minuteInterval: interval,
    use24hFormat: c.decode(8, asBool) ?? false,
    backgroundColor: c.decode(9, asColor),
    showDayOfWeek: c.decode(10, asBool) ?? false,
    showTimeSeparator: c.decode(11, asBool) ?? false,
    itemExtent: c.decode(12, asDouble) ?? 32.0,
    onDateTimeChanged: (d) {
      if (c.handles(1)) c.fire(1, _fromWallClock(d.toUtc(), offset));
    },
  );
}

// ── CupertinoContextMenu ────────────────────────────────────────────────────

/// Props `actions` (1), `enableHapticFeedback` (2); event `onAction` (1);
/// slot `child` (1).
Widget _buildContextMenu(NodeContext c) {
  final slot = c.slot(1) ?? c.missing('child');
  // The preview opens in a route above the page, so it carries the page's
  // render scope and providers with it.
  final scope = RenderScope.of(c.context);
  final child = UncontrolledProviderScope(
    container: ProviderScope.containerOf(c.context, listen: false),
    child: RenderScopeWidget(scope: scope, child: slot),
  );
  final labels = c.decode(1, _menuActions) ?? c.missing('actions');
  return CupertinoContextMenu(
    enableHapticFeedback: c.decode(2, asBool) ?? false,
    actions: [
      for (var i = 0; i < labels.length; i++)
        CupertinoContextMenuAction(
          isDestructiveAction: labels[i].destructive,
          onPressed: () {
            Navigator.of(c.context).pop();
            c.fire(1, i);
          },
          child: labels[i].icon == null
              ? Text(labels[i].label)
              : Row(
                  children: [
                    Expanded(child: Text(labels[i].label)),
                    PluxIcon(labels[i].icon!, size: 18),
                  ],
                ),
        ),
    ],
    child: child,
  );
}

typedef _MenuAction = ({String label, PluxIconSource? icon, bool destructive});

/// A `ContextMenuAction`: `label` (1), `icon` (2), `destructive` (3).
_MenuAction? _decodeMenuAction(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final label = asString(d, f.get(1, 'label'));
  if (label == null) return null;
  return (
    label: label,
    icon: decodeIconData(d, f.get(2, 'icon')),
    destructive: asBool(d, f.get(3, 'destructive')) ?? false,
  );
}

List<_MenuAction>? _menuActions(Decoding d, Object? v) =>
    asList(d, v, _decodeMenuAction);
