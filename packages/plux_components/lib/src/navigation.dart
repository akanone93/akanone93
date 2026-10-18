// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Layer 1 search, date and rail widgets of this package (WGT-001,
/// ADR-0071): `SearchBar`, `SearchAnchor`, `Autocomplete`,
/// `CalendarDatePicker` and `NavigationRail`. Flutter's controllers map to
/// state: a widget shows the index its prop gives, keeps the user's choice
/// until the prop changes, and fires the event with it.
library;

import 'package:flutter/material.dart';
import 'package:plux_components/src/ids.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// The builders of this group, by permanent widget ID.
final Map<int, NodeBuilder> navigationBuilders = {
  PluxComponentIds.searchBar: _searchBar,
  PluxComponentIds.searchAnchor: _searchAnchor,
  PluxComponentIds.autocomplete: _autocomplete,
  PluxComponentIds.calendarDatePicker: _calendarDatePicker,
  PluxComponentIds.navigationRail: _navigationRail,
};

// ── Destinations ───────────────────────────────────────────────────────────

/// One decoded `Destination`.
final class _Entry {
  const _Entry({required this.label, this.icon, this.selectedIcon});

  final String label;
  final Widget? icon;
  final Widget? selectedIcon;
}

_Entry? _destination(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final label = asString(d, f.get(1, 'label'));
  final icon = decodeIconWidget(d, f.get(2, 'icon'));
  if (label == null || icon == null) return null;
  return _Entry(
    label: label,
    icon: icon,
    selectedIcon: decodeIconWidget(d, f.get(3, 'selectedIcon')),
  );
}

List<_Entry>? _destinations(Decoding d, Object? v) =>
    asList(d, v, _destination);

int _clamp(int index, int length) =>
    length <= 0 ? 0 : index.clamp(0, length - 1);

/// The destinations of a rail; at least two, as Flutter's rail needs.
List<_Entry> _atLeastTwo(NodeContext c, int prop, String name) {
  final list = c.decode(prop, _destinations) ?? c.missing(name);
  if (list.length < 2) c.missing('$name (at least two)');
  return list;
}

// ── Search ─────────────────────────────────────────────────────────────────

Widget _searchBar(NodeContext c) {
  return c.text(
    c.decode(1, asString) ?? '',
    (controller) => SearchBar(
      controller: controller,
      hintText: c.decode(2, asString),
      leading: c.slot(1),
      trailing: c.hasSlot(2) ? c.slotList(2) : null,
      onChanged: c.handles(1) ? (v) => c.fire(1, v) : null,
      onSubmitted: c.handles(2) ? (v) => c.fire(2, v) : null,
      onTap: c.handles(3) ? () => c.fire(3) : null,
    ),
  );
}

Widget _searchAnchor(NodeContext c) {
  final suggestions = c.decode(1, asStrings) ?? c.missing('suggestions');
  final hint = c.decode(4, asString);
  return SearchAnchor(
    isFullScreen: c.decode(2, asBool),
    viewHintText: c.decode(3, asString),
    builder: (context, controller) => SearchBar(
      controller: controller,
      hintText: hint,
      leading: const Icon(Icons.search),
      onTap: controller.openView,
      onChanged: (_) => controller.openView(),
    ),
    suggestionsBuilder: (context, controller) {
      final query = controller.text.toLowerCase();
      return [
        for (final s in suggestions)
          if (s.toLowerCase().contains(query))
            keepScope(
              c.context,
              ListTile(
                title: Text(s),
                onTap: () {
                  controller.closeView(s);
                  if (c.handles(1)) c.fire(1, s);
                },
              ),
            ),
      ];
    },
  );
}

Widget _autocomplete(NodeContext c) {
  final options = c.decode(1, asStrings) ?? c.missing('options');
  final hint = c.decode(2, asString);
  return Autocomplete<String>(
    optionsBuilder: (value) {
      final query = value.text.toLowerCase();
      if (query.isEmpty) return const Iterable<String>.empty();
      return options.where((o) => o.toLowerCase().contains(query));
    },
    optionsMaxHeight: c.decode(3, asDouble) ?? 200.0,
    initialValue: TextEditingValue(text: c.decode(4, asString) ?? ''),
    onSelected: c.handles(1) ? (v) => c.fire(1, v) : null,
    optionsViewBuilder: (context, onSelected, opts) => keepScope(
      c.context,
      Align(
        alignment: Alignment.topLeft,
        child: Material(
          elevation: 4,
          child: ConstrainedBox(
            constraints: BoxConstraints(
              maxHeight: c.decode(3, asDouble) ?? 200.0,
            ),
            child: ListView(
              padding: EdgeInsets.zero,
              shrinkWrap: true,
              children: [
                for (final o in opts)
                  ListTile(title: Text(o), onTap: () => onSelected(o)),
              ],
            ),
          ),
        ),
      ),
    ),
    fieldViewBuilder: (context, controller, focusNode, onSubmitted) =>
        TextField(
          controller: controller,
          focusNode: focusNode,
          decoration: InputDecoration(hintText: hint),
          onSubmitted: (_) => onSubmitted(),
        ),
  );
}

// ── Date picker ────────────────────────────────────────────────────────────

DateTime? _date(Decoding d, Object? v) {
  if (v is! PxlDate) return null;
  final u = DateTime.fromMillisecondsSinceEpoch(v.days * 86400000, isUtc: true);
  return DateTime(u.year, u.month, u.day);
}

PxlDate _pxlDate(DateTime t) => PxlDate(
  DateTime.utc(t.year, t.month, t.day).millisecondsSinceEpoch ~/ 86400000,
);

Widget _calendarDatePicker(NodeContext c) {
  final first = c.decode(2, _date) ?? c.missing('firstDate');
  final last = c.decode(3, _date) ?? c.missing('lastDate');
  if (last.isBefore(first)) c.missing('lastDate (not before firstDate)');
  var initial = c.decode(1, _date) ?? c.missing('initialDate');
  if (initial.isBefore(first)) initial = first;
  if (initial.isAfter(last)) initial = last;
  return CalendarDatePicker(
    initialDate: initial,
    firstDate: first,
    lastDate: last,
    currentDate: c.decode(4, _date),
    onDateChanged: (date) {
      if (c.handles(1)) c.fire(1, _pxlDate(date));
    },
    onDisplayedMonthChanged: c.handles(2)
        ? (date) => c.fire(2, _pxlDate(date))
        : null,
  );
}

Widget _navigationRail(NodeContext c) {
  final dests = _atLeastTwo(c, 1, 'destinations');
  final extended = c.decode(3, asBool) ?? false;
  return c.controlled<int>(
    1,
    c.decode(2, asInt) ?? 0,
    (selected, onChanged) => NavigationRail(
      selectedIndex: _clamp(selected, dests.length),
      onDestinationSelected: onChanged,
      extended: extended,
      // An extended rail shows its labels beside the icons (Flutter asserts
      // that no label type is set).
      labelType: extended ? null : c.decode(4, decodeNavigationRailLabelType),
      backgroundColor: c.decode(5, asColor),
      elevation: c.decode(6, asDouble),
      groupAlignment: c.decode(7, asDouble),
      minWidth: c.decode(8, asDouble),
      minExtendedWidth: c.decode(9, asDouble),
      useIndicator: c.decode(10, asBool),
      indicatorColor: c.decode(11, asColor),
      leading: c.slot(1),
      trailing: c.slot(2),
      destinations: [
        for (final e in dests)
          NavigationRailDestination(
            icon: e.icon!,
            selectedIcon: e.selectedIcon,
            label: Text(e.label),
          ),
      ],
    ),
  );
}
