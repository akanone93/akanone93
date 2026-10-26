// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The Material composites of the layout conformance scenes that need their own
// scaffolding (STU-005, S4.3): tab bars, dialogs, sheets and menus. Each scene
// type builds the real Flutter widget; a composite names the part a child
// fills with `slot` (default `child`).

import 'package:flutter/material.dart';

const Set<String> materialCompositeTypes = <String>{'TabBar', 'Tab', 'AlertDialog'};

/// Builds one scene node (the golden writer's own builder).
typedef BuildScene = Widget Function(Map<String, Object?> node);

double? _num(Object? value) => value == null ? null : (value as num).toDouble();

Map<String, Object?> _props(Map<String, Object?> node) => (node['props'] as Map<String, Object?>?) ?? <String, Object?>{};

List<Map<String, Object?>> _children(Map<String, Object?> node) => <Map<String, Object?>>[
  for (final Object? raw in (node['children'] as List<Object?>?) ?? <Object?>[]) raw! as Map<String, Object?>,
];

/// The children of [node] filling [name].
List<Map<String, Object?>> _slot(Map<String, Object?> node, String name) => <Map<String, Object?>>[
  for (final Map<String, Object?> child in _children(node))
    if (((child['slot'] as String?) ?? 'child') == name) child,
];

Widget? _one(Map<String, Object?> node, String name, BuildScene build) {
  final List<Map<String, Object?>> found = _slot(node, name);
  return found.isEmpty ? null : build(found.single);
}

Widget _tab(Map<String, Object?> node, BuildScene build) {
  final Map<String, Object?> props = _props(node);
  return Tab(
    key: ValueKey<String>(node['id']! as String),
    text: props['text'] as String?,
    icon: _one(node, 'icon', build),
    height: _num(props['height']),
  );
}

Widget _tabBar(Map<String, Object?> node, Key key, BuildScene build) {
  final Map<String, Object?> props = _props(node);
  final List<Map<String, Object?>> tabs = _children(node);
  return DefaultTabController(
    length: tabs.length,
    child: TabBar(
      key: key,
      isScrollable: props['isScrollable'] == true,
      tabAlignment: switch (props['tabAlignment']) {
        'start' => TabAlignment.start,
        'startOffset' => TabAlignment.startOffset,
        'center' => TabAlignment.center,
        'fill' => TabAlignment.fill,
        _ => null,
      },
      indicatorSize: props['indicatorSize'] == 'label' ? TabBarIndicatorSize.label : TabBarIndicatorSize.tab,
      tabs: <Widget>[for (final Map<String, Object?> tab in tabs) _tab(tab, build)],
    ),
  );
}

Widget _alertDialog(Map<String, Object?> node, Key key, BuildScene build) {
  final List<Widget> actions = <Widget>[for (final Map<String, Object?> a in _slot(node, 'actions')) build(a)];
  return AlertDialog(
    key: key,
    title: _one(node, 'title', build),
    content: _one(node, 'content', build),
    actions: actions.isEmpty ? null : actions,
  );
}

/// Builds [node], one of [materialCompositeTypes].
Widget buildMaterialComposite(Map<String, Object?> node, Key key, BuildScene build) {
  switch (node['type']) {
    case 'TabBar':
      return _tabBar(node, key, build);
    case 'AlertDialog':
      return _alertDialog(node, key, build);
  }
  throw ArgumentError('unknown Material composite "${node['type']}"');
}
