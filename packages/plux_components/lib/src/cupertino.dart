// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Cupertino Layer 1 widgets of this package (WGT-020, ADR-0071):
/// `CupertinoPicker`, `CupertinoDatePicker` and `CupertinoContextMenu`.
library;

import 'package:flutter/cupertino.dart';
import 'package:plux_components/src/ids.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// The builders of this group, by permanent widget ID.
final Map<int, NodeBuilder> cupertinoBuilders = {
  PluxComponentIds.cupertinoPicker: _buildPicker,
  PluxComponentIds.cupertinoDatePicker: _buildDatePicker,
  PluxComponentIds.cupertinoContextMenu: _buildContextMenu,
};

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
  final child = keepScope(c.context, slot);
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
