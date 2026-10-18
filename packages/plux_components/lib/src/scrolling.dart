// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Layer 1 scrolling and display widgets of this package (WGT-020,
/// ADR-0071): `Table`, `ReorderableListView`, `NestedScrollView`,
/// `DataTable` and `Stepper`. They pull large parts of the Flutter
/// framework into a build, so apps that do not use them leave them out
/// (RT-060, RT-061). Props are read by the permanent IDs of their
/// descriptors in `schema/widgets/layer1`.
library;

import 'package:flutter/gestures.dart' show DragStartBehavior;
import 'package:flutter/material.dart';
import 'package:plux_components/src/ids.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// The builders of this group, by permanent widget ID.
final Map<int, NodeBuilder> scrollingBuilders = {
  PluxComponentIds.table: _table,
  PluxComponentIds.reorderableListView: _reorderableListView,
  PluxComponentIds.nestedScrollView: _nestedScrollView,
  PluxComponentIds.dataTable: _dataTable,
  PluxComponentIds.stepper: _stepper,
};

List<Object?> _list(NodeContext c, int id) => switch (c.prop(id)) {
  final List<Object?> v => v,
  _ => const [],
};

/// Decodes a `ColumnWidth` (value type 48).
TableColumnWidth? _decodeColumnWidth(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final value = asDouble(d, f.get(2, 'value')) ?? 1;
  return switch (nameColumnWidthKind(d, f.get(1, 'kind')) ?? 'flex') {
    'fixed' => FixedColumnWidth(value),
    'fraction' => FractionColumnWidth(value),
    'intrinsic' => const IntrinsicColumnWidth(),
    _ => FlexColumnWidth(value),
  };
}

TableBorder? _tableBorder(Decoding d, Object? v) {
  final side = decodeBorderSide(d, v);
  if (side == null) return null;
  return TableBorder.all(
    color: side.color,
    width: side.width,
    style: side.style,
  );
}

// ── Table ──────────────────────────────────────────────────────────────────

Widget _table(NodeContext c) {
  final columns = (c.decode(1, asInt) ?? 1).clamp(1, 1 << 16);
  final cells = c.children();
  final rows = <TableRow>[];
  for (var start = 0; start < cells.length; start += columns) {
    rows.add(
      TableRow(
        children: [
          for (var i = start; i < start + columns; i++)
            i < cells.length ? cells[i] : const SizedBox.shrink(),
        ],
      ),
    );
  }
  final widths = <int, TableColumnWidth>{};
  final given = _list(c, 2);
  for (var i = 0; i < given.length; i++) {
    final w = _decodeColumnWidth(c, given[i]);
    if (w != null) widths[i] = w;
  }
  return Table(
    columnWidths: widths.isEmpty ? null : widths,
    defaultColumnWidth:
        c.decode(3, _decodeColumnWidth) ?? const FlexColumnWidth(),
    textDirection: c.decode(4, decodeTextDirection),
    border: c.decode(5, _tableBorder),
    defaultVerticalAlignment:
        _verticalAlignment(c, c.prop(6)) ?? TableCellVerticalAlignment.top,
    textBaseline: c.decode(7, decodeTextBaseline),
    children: rows,
  );
}

TableCellVerticalAlignment? _verticalAlignment(Decoding d, Object? v) =>
    switch (nameTableCellVerticalAlignment(d, v)) {
      'middle' => TableCellVerticalAlignment.middle,
      'bottom' => TableCellVerticalAlignment.bottom,
      'baseline' => TableCellVerticalAlignment.baseline,
      'fill' => TableCellVerticalAlignment.fill,
      'intrinsicHeight' => TableCellVerticalAlignment.intrinsicHeight,
      'top' => TableCellVerticalAlignment.top,
      _ => null,
    };

// ── ReorderableListView ────────────────────────────────────────────────────

Widget _reorderableListView(NodeContext c) {
  final values = _list(c, 1);
  final status = nameListStatus(c, c.prop(15)) ?? 'ready';
  final other = switch (status) {
    'loading' => c.slot(5),
    'error' => c.slot(6),
    _ when values.isEmpty => c.slot(4),
    _ => null,
  };
  if (other != null) return other;
  final list = ReorderableListView.builder(
    scrollDirection: c.decode(2, decodeAxis) ?? Axis.vertical,
    reverse: c.decode(3, asBool) ?? false,
    physics: c.decode(4, decodeScrollPhysics),
    shrinkWrap: c.decode(5, asBool) ?? false,
    padding: c.decode(6, decodeEdgeInsetsResolved),
    buildDefaultDragHandles: c.decode(7, asBool) ?? true,
    itemExtent: c.decode(8, asDouble),
    anchor: c.decode(9, asDouble) ?? 0,
    dragStartBehavior:
        c.decode(11, decodeDragStartBehavior) ?? DragStartBehavior.start,
    keyboardDismissBehavior: c.decode(
      12,
      decodeScrollViewKeyboardDismissBehavior,
    ),
    clipBehavior: c.decode(13, decodeClip) ?? Clip.hardEdge,
    autoScrollerVelocityScalar: c.decode(14, asDouble),
    header: c.slot(2),
    footer: c.slot(3),
    itemCount: values.length,
    itemBuilder: (context, i) =>
        KeyedSubtree(key: ValueKey(i), child: c.item(1, values[i], i)),
    proxyDecorator: (child, _, _) => keepScope(
      c.context,
      Material(elevation: 4, color: Colors.transparent, child: child),
    ),
    onReorderItem: (from, to) {
      if (c.handles(1)) c.fire(1, {'oldIndex': from, 'newIndex': to});
    },
    onReorderStart: c.handles(2) ? (i) => c.fire(2, i) : null,
    onReorderEnd: c.handles(3) ? (i) => c.fire(3, i) : null,
  );
  return _OverlayHost(child: list);
}

// ── NestedScrollView ───────────────────────────────────────────────────────

Widget _nestedScrollView(NodeContext c) {
  return NestedScrollView(
    scrollDirection: c.decode(1, decodeAxis) ?? Axis.vertical,
    reverse: c.decode(2, asBool) ?? false,
    physics: c.decode(3, decodeScrollPhysics),
    dragStartBehavior:
        c.decode(4, decodeDragStartBehavior) ?? DragStartBehavior.start,
    floatHeaderSlivers: c.decode(5, asBool) ?? false,
    clipBehavior: c.decode(6, decodeClip) ?? Clip.hardEdge,
    hitTestBehavior:
        c.decode(7, decodeHitTestBehavior) ?? HitTestBehavior.opaque,
    headerSliverBuilder: (context, innerBoxIsScrolled) => c.slotList(1),
    body: c.slot(2) ?? c.missing('body'),
  );
}

// ── DataTable ──────────────────────────────────────────────────────────────

Widget _dataTable(NodeContext c) {
  final heads = <Map<int, Object?>>[];
  final names = <String>[];
  for (final v in _list(c, 1)) {
    final f = Fields.of(c, v);
    final label = f == null ? null : asString(c, f.get(1, 'label'));
    if (f == null || label == null) continue;
    names.add(label);
    heads.add({
      2: asBool(c, f.get(2, 'numeric')) ?? false,
      3: asString(c, f.get(3, 'tooltip')),
    });
  }
  if (heads.isEmpty) return const SizedBox.shrink();
  final sortable = c.handles(1);
  final columns = [
    for (var i = 0; i < heads.length; i++)
      DataColumn(
        label: Text(names[i]),
        numeric: heads[i][2]! as bool,
        tooltip: heads[i][3] as String?,
        onSort: sortable
            ? (index, ascending) =>
                  c.fire(1, {'index': index, 'value': ascending})
            : null,
      ),
  ];
  final selected = {
    for (final v in _list(c, 3))
      if (v is int) v,
  };
  final rows = <DataRow>[];
  final given = _list(c, 2);
  for (var r = 0; r < given.length; r++) {
    final row = given[r] is List<Object?> ? given[r]! as List<Object?> : null;
    rows.add(
      DataRow(
        selected: selected.contains(r),
        onSelectChanged: c.handles(2)
            ? (on) => c.fire(2, {'index': r, 'value': on ?? false})
            : null,
        cells: [
          for (var i = 0; i < columns.length; i++)
            DataCell(
              Text(row != null && i < row.length ? _cellText(row[i]) : ''),
            ),
        ],
      ),
    );
  }
  final sortIndex = c.decode(4, asInt);
  return DataTable(
    columns: columns,
    rows: rows,
    sortColumnIndex:
        sortIndex != null && sortIndex >= 0 && sortIndex < columns.length
        ? sortIndex
        : null,
    sortAscending: c.decode(5, asBool) ?? true,
    showCheckboxColumn: c.decode(6, asBool) ?? true,
    decoration: c.decode(7, decodeBoxDecoration),
    dataRowColor: c.decode(8, decodeWidgetStateColor),
    dataRowMinHeight: c.decode(9, asDouble),
    dataRowMaxHeight: c.decode(10, asDouble),
    dataTextStyle: c.decode(11, decodeTextStyle),
    headingRowColor: c.decode(12, decodeWidgetStateColor),
    headingRowHeight: c.decode(13, asDouble),
    headingTextStyle: c.decode(14, decodeTextStyle),
    horizontalMargin: c.decode(15, asDouble),
    columnSpacing: c.decode(16, asDouble),
    showBottomBorder: c.decode(17, asBool) ?? false,
    dividerThickness: c.decode(18, asDouble),
    checkboxHorizontalMargin: c.decode(19, asDouble),
    border: c.decode(20, _tableBorder),
    clipBehavior: c.decode(21, decodeClip) ?? Clip.none,
    onSelectAll: c.handles(3) ? (on) => c.fire(3, on ?? false) : null,
  );
}

String _cellText(Object? v) => switch (v) {
  null => '',
  final String s => s,
  _ => '$v',
};

// ── Stepper ────────────────────────────────────────────────────────────────

Widget _stepper(NodeContext c) {
  final contents = c.children();
  final steps = <Step>[];
  for (final v in _list(c, 1)) {
    final f = Fields.of(c, v);
    final title = f == null ? null : asString(c, f.get(1, 'title'));
    if (f == null || title == null) continue;
    final subtitle = asString(c, f.get(2, 'subtitle'));
    final label = asString(c, f.get(3, 'label'));
    final i = steps.length;
    steps.add(
      Step(
        title: Text(title),
        subtitle: subtitle == null ? null : Text(subtitle),
        label: label == null ? null : Text(label),
        content: i < contents.length ? contents[i] : const SizedBox.shrink(),
        state: switch (nameStepState(c, f.get(4, 'state'))) {
          'editing' => StepState.editing,
          'complete' => StepState.complete,
          'disabled' => StepState.disabled,
          'error' => StepState.error,
          _ => StepState.indexed,
        },
        isActive: asBool(c, f.get(5, 'isActive')) ?? false,
      ),
    );
  }
  if (steps.isEmpty) return const SizedBox.shrink();
  final connector = c.decode(7, asColor);
  return Stepper(
    steps: steps,
    physics: c.decode(4, decodeScrollPhysics),
    type: switch (nameStepperType(c, c.prop(2))) {
      'horizontal' => StepperType.horizontal,
      _ => StepperType.vertical,
    },
    currentStep: (c.decode(3, asInt) ?? 0).clamp(0, steps.length - 1),
    onStepTapped: c.handles(1) ? (i) => c.fire(1, i) : null,
    onStepContinue: c.handles(2) ? () => c.fire(2) : null,
    onStepCancel: c.handles(3) ? () => c.fire(3) : null,
    elevation: c.decode(5, asDouble),
    margin: c.decode(6, decodeEdgeInsets),
    connectorColor: connector == null
        ? null
        : WidgetStatePropertyAll<Color>(connector),
    connectorThickness: c.decode(8, asDouble),
    stepIconHeight: c.decode(9, asDouble),
    stepIconWidth: c.decode(10, asDouble),
    stepIconMargin: c.decode(11, decodeEdgeInsetsResolved),
    clipBehavior: c.decode(12, decodeClip) ?? Clip.none,
    headerPadding: c.decode(13, decodeEdgeInsets),
    contentPadding: c.decode(14, decodeEdgeInsets),
  );
}

final class _OverlayHost extends StatefulWidget {
  const _OverlayHost({required this.child});

  final Widget child;

  @override
  State<_OverlayHost> createState() => _OverlayHostState();
}

final class _OverlayHostState extends State<_OverlayHost> {
  late final OverlayEntry _entry = OverlayEntry(builder: (_) => widget.child);

  @override
  void didUpdateWidget(_OverlayHost old) {
    super.didUpdateWidget(old);
    _entry.markNeedsBuild();
  }

  @override
  void dispose() {
    _entry
      ..remove()
      ..dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Overlay(initialEntries: [_entry]);
}
