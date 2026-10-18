// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The navigation and menu builders (WGT-001, ADR-0071): menus, tabs, bars
/// and drawers. Search, the date picker and the rail are built by
/// `plux_components` (RT-060, RT-061). Flutter's controllers
/// map to state (ADR-0071): a widget shows the index its prop gives, keeps
/// the user's choice until the prop changes, and fires the event with it, so
/// a page binds both to one state key. Entries are lists of small value
/// objects (`MenuItem`, `TabItem`, `Destination`), not widgets.
library;

import 'package:flutter/material.dart';
import 'package:plux_flutter/src/render/decoders.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/keep_scope.dart';
import 'package:plux_flutter/src/render/node_context.dart';

/// The permanent ID of `PopupMenuButton`.
const int popupMenuButtonWidgetId = 120;

/// The permanent ID of `MenuAnchor`.
const int menuAnchorWidgetId = 121;

/// The permanent ID of `TabBar`.
const int tabBarWidgetId = 126;

/// The permanent ID of `TabBarView`.
const int tabBarViewWidgetId = 127;

/// The permanent ID of `NavigationBar`.
const int navigationBarWidgetId = 128;

/// The permanent ID of `BottomNavigationBar`.
const int bottomNavigationBarWidgetId = 129;

/// The permanent ID of `Drawer`.
const int drawerWidgetId = 130;

/// The permanent ID of `NavigationDrawer`.
const int navigationDrawerWidgetId = 131;

/// The builders of the navigation group, by permanent widget ID.
const Map<int, NodeBuilder> navigationBuilders = {
  popupMenuButtonWidgetId: _popupMenuButton,
  menuAnchorWidgetId: _menuAnchor,
  tabBarWidgetId: _tabBar,
  tabBarViewWidgetId: _tabBarView,
  navigationBarWidgetId: _navigationBar,
  bottomNavigationBarWidgetId: _bottomNavigationBar,
  drawerWidgetId: _drawer,
  navigationDrawerWidgetId: _navigationDrawer,
};

// ── Entries ────────────────────────────────────────────────────────────────

/// One decoded `MenuItem`, `TabItem` or `Destination`.
final class _Entry {
  const _Entry({
    required this.label,
    this.value = '',
    this.icon,
    this.selectedIcon,
    this.enabled = true,
  });

  final String label;
  final String value;
  final Widget? icon;
  final Widget? selectedIcon;
  final bool enabled;
}

// The field IDs of the value types (`schema/widgets/types/`).
_Entry? _menuEntry(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final value = asString(d, f.get(1, 'value'));
  final label = asString(d, f.get(2, 'label'));
  if (value == null || label == null) return null;
  return _Entry(
    value: value,
    label: label,
    icon: decodeIconWidget(d, f.get(3, 'icon')),
    enabled: asBool(d, f.get(4, 'enabled')) ?? true,
  );
}

_Entry? _tabEntry(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final label = asString(d, f.get(1, 'label'));
  if (label == null) return null;
  return _Entry(label: label, icon: decodeIconWidget(d, f.get(2, 'icon')));
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

List<_Entry>? _menuEntries(Decoding d, Object? v) => asList(d, v, _menuEntry);

List<_Entry>? _tabEntries(Decoding d, Object? v) => asList(d, v, _tabEntry);

List<_Entry>? _destinations(Decoding d, Object? v) =>
    asList(d, v, _destination);

int _clamp(int index, int length) =>
    length <= 0 ? 0 : index.clamp(0, length - 1);

/// An entry as a menu row: its icon, then its label.
Widget _menuRow(_Entry e) {
  final label = Text(e.label);
  final icon = e.icon;
  if (icon == null) return label;
  return Row(
    mainAxisSize: MainAxisSize.min,
    children: [icon, const SizedBox(width: 12), label],
  );
}

/// The destinations of a bar, drawer or rail; at least two, as Flutter's
/// bars need.
List<_Entry> _atLeastTwo(NodeContext c, int prop, String name) {
  final list = c.decode(prop, _destinations) ?? c.missing(name);
  if (list.length < 2) c.missing('$name (at least two)');
  return list;
}

// ── Menus ──────────────────────────────────────────────────────────────────

Widget _popupMenuButton(NodeContext c) {
  final items = c.decode(1, _menuEntries) ?? c.missing('items');
  final child = c.slot(1);
  return PopupMenuButton<String>(
    itemBuilder: (_) => [
      for (final e in items)
        PopupMenuItem<String>(
          value: e.value,
          enabled: e.enabled,
          child: keepScope(c.context, _menuRow(e)),
        ),
    ],
    initialValue: c.decode(2, asString),
    onOpened: c.handles(3) ? () => c.fire(3) : null,
    onSelected: c.handles(1) ? (v) => c.fire(1, v) : null,
    onCanceled: c.handles(2) ? () => c.fire(2) : null,
    tooltip: c.decode(3, asString),
    enabled: c.decode(4, asBool) ?? true,
    elevation: c.decode(5, asDouble),
    color: c.decode(6, asColor),
    shadowColor: c.decode(7, asColor),
    surfaceTintColor: c.decode(8, asColor),
    iconSize: c.decode(9, asDouble),
    icon: child == null ? c.slot(2) : null,
    child: child,
  );
}

Widget _menuAnchor(NodeContext c) {
  final items = c.decode(1, _menuEntries) ?? c.missing('items');
  final anchor = c.slot(1);
  return MenuAnchor(
    consumeOutsideTap: c.decode(2, asBool) ?? false,
    onOpen: c.handles(2) ? () => c.fire(2) : null,
    onClose: c.handles(3) ? () => c.fire(3) : null,
    menuChildren: [
      for (final e in items)
        MenuItemButton(
          leadingIcon: e.icon,
          onPressed: e.enabled ? () => c.fire(1, e.value) : null,
          child: Text(e.label),
        ),
    ],
    builder: (context, controller, _) => ConstrainedBox(
      constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
      child: InkWell(
        onTap: () => controller.isOpen ? controller.close() : controller.open(),
        child: Center(
          widthFactor: 1,
          child: anchor ?? const Icon(Icons.more_vert),
        ),
      ),
    ),
  );
}

// ── Tabs ───────────────────────────────────────────────────────────────────

/// Owns the [TabController] of a TabBar or TabBarView, follows the index the
/// props give and reports the page the user swipes to ([onIndex]).
final class _TabHost extends StatefulWidget {
  const _TabHost({
    required this.length,
    required this.index,
    required this.build,
    this.onIndex,
  });

  final int length;
  final int index;
  final Widget Function(TabController controller) build;
  final ValueChanged<int>? onIndex;

  @override
  State<_TabHost> createState() => _TabHostState();
}

final class _TabHostState extends State<_TabHost>
    with SingleTickerProviderStateMixin {
  late TabController _controller = _make();
  int _reported = 0;

  TabController _make() {
    final i = _clamp(widget.index, widget.length);
    _reported = i;
    return TabController(length: widget.length, vsync: this, initialIndex: i)
      ..addListener(_changed);
  }

  void _changed() {
    if (_controller.indexIsChanging || _controller.index == _reported) return;
    _reported = _controller.index;
    widget.onIndex?.call(_controller.index);
  }

  @override
  void didUpdateWidget(_TabHost old) {
    super.didUpdateWidget(old);
    if (old.length != widget.length) {
      _controller.dispose();
      _controller = _make();
      return;
    }
    final want = _clamp(widget.index, widget.length);
    if (want != _controller.index) {
      _reported = want;
      _controller.animateTo(want);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.build(_controller);
}

Widget _tabBar(NodeContext c) {
  final tabs = c.decode(1, _tabEntries) ?? c.missing('tabs');
  if (tabs.isEmpty) c.missing('tabs (at least one)');
  return _TabHost(
    length: tabs.length,
    index: c.decode(2, asInt) ?? 0,
    build: (controller) => TabBar(
      controller: controller,
      tabs: [for (final t in tabs) Tab(text: t.label, icon: t.icon)],
      isScrollable: c.decode(3, asBool) ?? false,
      indicatorColor: c.decode(4, asColor),
      indicatorSize: c.decode(5, decodeTabBarIndicatorSize),
      dividerColor: c.decode(6, asColor),
      labelColor: c.decode(7, asColor),
      unselectedLabelColor: c.decode(8, asColor),
      onTap: c.handles(1) ? (i) => c.fire(1, i) : null,
    ),
  );
}

Widget _tabBarView(NodeContext c) {
  final kids = c.children();
  if (kids.isEmpty) return const SizedBox.shrink();
  return _TabHost(
    length: kids.length,
    index: c.decode(1, asInt) ?? 0,
    onIndex: c.handles(1) ? (i) => c.fire(1, i) : null,
    build: (controller) => TabBarView(
      controller: controller,
      viewportFraction: c.decode(2, asDouble) ?? 1.0,
      children: kids,
    ),
  );
}

// ── Bars, drawers and the rail ─────────────────────────────────────────────

Widget _navigationBar(NodeContext c) {
  final dests = _atLeastTwo(c, 1, 'destinations');
  return c.controlled<int>(
    1,
    c.decode(2, asInt) ?? 0,
    (selected, onChanged) => NavigationBar(
      selectedIndex: _clamp(selected, dests.length),
      onDestinationSelected: onChanged,
      destinations: [
        for (final e in dests)
          NavigationDestination(
            icon: e.icon!,
            selectedIcon: e.selectedIcon,
            label: e.label,
          ),
      ],
      backgroundColor: c.decode(3, asColor),
      elevation: c.decode(4, asDouble),
      shadowColor: c.decode(5, asColor),
      surfaceTintColor: c.decode(6, asColor),
      indicatorColor: c.decode(7, asColor),
      height: c.decode(8, asDouble),
      labelBehavior: c.decode(9, decodeNavigationDestinationLabelBehavior),
    ),
  );
}

Widget _bottomNavigationBar(NodeContext c) {
  final items = _atLeastTwo(c, 1, 'items');
  return c.controlled<int>(
    1,
    c.decode(2, asInt) ?? 0,
    (current, onChanged) => BottomNavigationBar(
      currentIndex: _clamp(current, items.length),
      onTap: onChanged,
      items: [
        for (final e in items)
          BottomNavigationBarItem(
            icon: e.icon!,
            activeIcon: e.selectedIcon,
            label: e.label,
          ),
      ],
      elevation: c.decode(3, asDouble),
      type: c.decode(4, decodeBottomNavigationBarType),
      backgroundColor: c.decode(5, asColor),
      selectedItemColor: c.decode(6, asColor),
      unselectedItemColor: c.decode(7, asColor),
      iconSize: c.decode(8, asDouble) ?? 24.0,
      selectedFontSize: c.decode(9, asDouble) ?? 14.0,
      unselectedFontSize: c.decode(10, asDouble) ?? 12.0,
      showSelectedLabels: c.decode(11, asBool),
      showUnselectedLabels: c.decode(12, asBool),
    ),
  );
}

Widget _drawer(NodeContext c) {
  return Drawer(
    backgroundColor: c.decode(1, asColor),
    elevation: c.decode(2, asDouble),
    shadowColor: c.decode(3, asColor),
    surfaceTintColor: c.decode(4, asColor),
    shape: c.decode(5, decodeShapeBorder),
    width: c.decode(6, asDouble),
    semanticLabel: c.decode(7, asString),
    clipBehavior: c.decode(8, decodeClip),
    child: c.slot(1),
  );
}

Widget _navigationDrawer(NodeContext c) {
  final dests = _atLeastTwo(c, 1, 'destinations');
  return c.controlled<int>(
    1,
    c.decode(2, asInt) ?? 0,
    (selected, onChanged) => NavigationDrawer(
      selectedIndex: _clamp(selected, dests.length),
      onDestinationSelected: onChanged,
      header: c.slot(1),
      footer: c.slot(2),
      backgroundColor: c.decode(3, asColor),
      shadowColor: c.decode(4, asColor),
      surfaceTintColor: c.decode(5, asColor),
      elevation: c.decode(6, asDouble),
      indicatorColor: c.decode(7, asColor),
      children: [
        for (final e in dests)
          NavigationDrawerDestination(
            icon: e.icon!,
            selectedIcon: e.selectedIcon,
            label: Text(e.label),
          ),
      ],
    ),
  );
}
