// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The Material composites of the layout conformance scenes that need their own
// scaffolding (STU-005, S4.3): tab bars, dialogs, sheets and menus. Each scene
// type builds the real Flutter widget; a composite names the part a child
// fills with `slot` (default `child`).

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';

const Set<String> materialCompositeTypes = <String>{
  'TabBar', 'Tab', 'AlertDialog', 'BottomSheet', 'PopupMenu', 'PopupMenuItem', 'PopupMenuDivider',
};

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

/// The layout the modal route gives its sheet, settled: full width, at the bottom, at most 9/16 of the height
/// unless [scrollControlled].
class _ModalSheetLayout extends SingleChildLayoutDelegate {
  const _ModalSheetLayout(this.scrollControlled);
  final bool scrollControlled;

  @override
  BoxConstraints getConstraintsForChild(BoxConstraints constraints) => BoxConstraints(
    minWidth: constraints.maxWidth,
    maxWidth: constraints.maxWidth,
    maxHeight: scrollControlled ? constraints.maxHeight : constraints.maxHeight * 9.0 / 16.0,
  );

  @override
  Offset getPositionForChild(Size size, Size childSize) => Offset(0, size.height - childSize.height);

  @override
  bool shouldRelayout(_ModalSheetLayout oldDelegate) => oldDelegate.scrollControlled != scrollControlled;
}

class _SheetHost extends StatefulWidget {
  const _SheetHost({super.key, required this.node, required this.child});
  final Map<String, Object?> node;
  final Widget child;

  @override
  State<_SheetHost> createState() => _SheetHostState();
}

class _SheetHostState extends State<_SheetHost> with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(vsync: this, value: 1);

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final Map<String, Object?> props = _props(widget.node);
    return CustomSingleChildLayout(
      key: ValueKey<String>(widget.node['id']! as String),
      delegate: _ModalSheetLayout(props['isScrollControlled'] == true),
      child: BottomSheet(
        animationController: _controller,
        onClosing: () {},
        enableDrag: false,
        showDragHandle: props['showDragHandle'] == true,
        builder: (BuildContext context) => widget.child,
      ),
    );
  }
}

Widget _bottomSheet(Map<String, Object?> node, BuildScene build) =>
    _SheetHost(node: node, child: _one(node, 'child', build)!);

Widget _popupMenu(Map<String, Object?> node, Key key, BuildScene build) {
  final List<Widget> items = <Widget>[
    for (final Map<String, Object?> entry in _children(node))
      if (entry['type'] == 'PopupMenuDivider')
        PopupMenuDivider(key: ValueKey<String>(entry['id']! as String), height: _num(_props(entry)['height']) ?? 16)
      else
        PopupMenuItem<void>(
          key: ValueKey<String>(entry['id']! as String),
          height: _num(_props(entry)['height']) ?? kMinInteractiveDimension,
          child: _one(entry, 'child', build),
        ),
  ];
  return Material(
    key: key,
    shape: const RoundedRectangleBorder(borderRadius: BorderRadius.all(Radius.circular(4))),
    type: MaterialType.card,
    elevation: 3,
    child: Align(
      alignment: AlignmentDirectional.topEnd,
      widthFactor: 1,
      heightFactor: 1,
      child: ConstrainedBox(
        constraints: const BoxConstraints(minWidth: 112, maxWidth: 280),
        child: IntrinsicWidth(
          stepWidth: 56,
          child: Semantics(
            role: SemanticsRole.menu,
            scopesRoute: true,
            namesRoute: true,
            explicitChildNodes: true,
            child: SingleChildScrollView(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: ListBody(children: items),
            ),
          ),
        ),
      ),
    ),
  );
}

/// Builds [node], one of [materialCompositeTypes].
Widget buildMaterialComposite(Map<String, Object?> node, Key key, BuildScene build) {
  switch (node['type']) {
    case 'TabBar':
      return _tabBar(node, key, build);
    case 'AlertDialog':
      return _alertDialog(node, key, build);
    case 'BottomSheet':
      return _bottomSheet(node, build);
    case 'PopupMenu':
      return _popupMenu(node, key, build);
  }
  throw ArgumentError('unknown Material composite "${node['type']}"');
}
