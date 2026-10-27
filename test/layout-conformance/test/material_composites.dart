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
  'Autocomplete', 'BottomAppBar', 'CalendarDatePicker', 'DataTable', 'DropdownMenu', 'MaterialBanner',
  'NavigationDrawer', 'PopupMenuButton', 'SegmentedButton', 'Stepper',
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
  const _SheetHost({required this.node, required this.child});
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

List<Map<String, Object?>> _list(Object? value) => <Map<String, Object?>>[
  for (final Object? raw in (value as List<Object?>?) ?? <Object?>[]) raw! as Map<String, Object?>,
];

Widget _autocomplete(Map<String, Object?> node, Key key) {
  final Map<String, Object?> props = _props(node);
  return Autocomplete<String>(
    key: key,
    optionsBuilder: (TextEditingValue value) => const Iterable<String>.empty(),
    fieldViewBuilder:
        (BuildContext context, TextEditingController controller, FocusNode focusNode, VoidCallback onSubmitted) =>
            TextFormField(
              controller: controller,
              focusNode: focusNode,
              decoration: InputDecoration(hintText: props['hintText'] as String?),
              onFieldSubmitted: (String _) => onSubmitted(),
            ),
  );
}

/// A `BottomAppBar` needs a `Scaffold` above it; the scaffold is as high as the bar, so the bar fills it.
Widget _bottomAppBar(Map<String, Object?> node, Key key, BuildScene build) {
  final Map<String, Object?> props = _props(node);
  final double height = _num(props['height']) ?? 80;
  return SizedBox(
    height: height,
    child: Scaffold(
      primary: false,
      bottomNavigationBar: BottomAppBar(key: key, height: _num(props['height']), child: _one(node, 'child', build)),
    ),
  );
}

Widget _dataTable(Map<String, Object?> node, Key key) {
  final Map<String, Object?> props = _props(node);
  return DataTable(
    key: key,
    columns: <DataColumn>[
      for (final Map<String, Object?> column in _list(props['columns']))
        DataColumn(label: Text(column['label']! as String), numeric: column['numeric'] == true),
    ],
    rows: <DataRow>[
      for (final Object? row in (props['rows'] as List<Object?>?) ?? <Object?>[])
        DataRow(cells: <DataCell>[for (final Object? cell in row! as List<Object?>) DataCell(Text(cell! as String))]),
    ],
    horizontalMargin: _num(props['horizontalMargin']),
    columnSpacing: _num(props['columnSpacing']),
    headingRowHeight: _num(props['headingRowHeight']),
    dataRowMinHeight: _num(props['dataRowMinHeight']),
    dataRowMaxHeight: _num(props['dataRowMaxHeight']),
    dividerThickness: _num(props['dividerThickness']),
    showBottomBorder: props['showBottomBorder'] == true,
  );
}

Widget _dropdownMenu(Map<String, Object?> node, Key key, BuildScene build) {
  final Map<String, Object?> props = _props(node);
  return DropdownMenu<String>(
    key: key,
    width: _num(props['width']),
    hintText: props['hintText'] as String?,
    helperText: props['helperText'] as String?,
    showTrailingIcon: props['showTrailingIcon'] != false,
    leadingIcon: _one(node, 'leadingIcon', build),
    label: _one(node, 'label', build),
    dropdownMenuEntries: <DropdownMenuEntry<String>>[
      for (final Map<String, Object?> entry in _list(props['dropdownMenuEntries']))
        DropdownMenuEntry<String>(value: entry['value']! as String, label: entry['label']! as String),
    ],
  );
}

Widget _materialBanner(Map<String, Object?> node, Key key, BuildScene build) {
  final Map<String, Object?> props = _props(node);
  return MaterialBanner(
    key: key,
    content: _one(node, 'content', build)!,
    leading: _one(node, 'leading', build),
    forceActionsBelow: props['forceActionsBelow'] == true,
    minActionBarHeight: _num(props['minActionBarHeight']) ?? 52,
    actions: <Widget>[for (final Map<String, Object?> action in _slot(node, 'actions')) build(action)],
  );
}

Widget _navigationDrawer(Map<String, Object?> node, Key key, BuildScene build) {
  final Map<String, Object?> props = _props(node);
  return NavigationDrawer(
    key: key,
    selectedIndex: (props['selectedIndex'] as num?)?.toInt(),
    children: <Widget>[
      ?_one(node, 'header', build),
      for (final Map<String, Object?> destination in _list(props['destinations']))
        NavigationDrawerDestination(
          icon: const Icon(Icons.star),
          selectedIcon: destination['selectedIcon'] == null ? null : const Icon(Icons.star_border),
          label: Text(destination['label']! as String),
        ),
      ?_one(node, 'footer', build),
    ],
  );
}

Widget _popupMenuButton(Map<String, Object?> node, Key key, BuildScene build) {
  final Map<String, Object?> props = _props(node);
  return PopupMenuButton<String>(
    key: key,
    icon: _one(node, 'icon', build),
    iconSize: _num(props['iconSize']),
    enabled: props['enabled'] != false,
    itemBuilder: (BuildContext context) => <PopupMenuEntry<String>>[
      for (final Map<String, Object?> item in _list(props['items']))
        PopupMenuItem<String>(value: item['value']! as String, child: Text(item['label']! as String)),
    ],
    child: _one(node, 'child', build),
  );
}

Widget _segmentedButton(Map<String, Object?> node, Key key) {
  final Map<String, Object?> props = _props(node);
  return SegmentedButton<String>(
    key: key,
    showSelectedIcon: props['showSelectedIcon'] != false,
    multiSelectionEnabled: props['multiSelectionEnabled'] == true,
    emptySelectionAllowed: props['emptySelectionAllowed'] == true,
    segments: <ButtonSegment<String>>[
      for (final Map<String, Object?> segment in _list(props['segments']))
        ButtonSegment<String>(
          value: segment['value']! as String,
          label: segment['label'] == null ? null : Text(segment['label']! as String),
          icon: segment['icon'] == null ? null : const Icon(Icons.star),
        ),
    ],
    selected: <String>{for (final Object? value in (props['selected'] as List<Object?>?) ?? <Object?>[]) value! as String},
    onSelectionChanged: (Set<String> _) {},
  );
}

Widget _stepper(Map<String, Object?> node, Key key) {
  final Map<String, Object?> props = _props(node);
  return Stepper(
    key: key,
    type: props['type'] == 'horizontal' ? StepperType.horizontal : StepperType.vertical,
    currentStep: (props['currentStep'] as num?)?.toInt() ?? 0,
    // The default controls (two text buttons in a 48-px row under 16 px of padding) overflow a phone at text
    // scale 2; a box of their size keeps the step's height, which `material-stepper` at scale 1 checks.
    controlsBuilder: (BuildContext context, ControlsDetails details) =>
        const Padding(padding: EdgeInsets.only(top: 16), child: SizedBox(height: 48)),
    steps: <Step>[
      for (final Map<String, Object?> step in _list(props['steps']))
        Step(
          title: Text(step['title']! as String),
          subtitle: step['subtitle'] == null ? null : Text(step['subtitle']! as String),
          content: const SizedBox.shrink(),
        ),
    ],
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
    case 'Autocomplete':
      return _autocomplete(node, key);
    case 'BottomAppBar':
      return _bottomAppBar(node, key, build);
    case 'CalendarDatePicker':
      return CalendarDatePicker(
        key: key,
        initialDate: DateTime(2026, 1, 15),
        firstDate: DateTime(2020),
        lastDate: DateTime(2030),
        onDateChanged: (DateTime _) {},
      );
    case 'DataTable':
      return _dataTable(node, key);
    case 'DropdownMenu':
      return _dropdownMenu(node, key, build);
    case 'MaterialBanner':
      return _materialBanner(node, key, build);
    case 'NavigationDrawer':
      return _navigationDrawer(node, key, build);
    case 'PopupMenuButton':
      return _popupMenuButton(node, key, build);
    case 'SegmentedButton':
      return _segmentedButton(node, key);
    case 'Stepper':
      return _stepper(node, key);
  }
  throw ArgumentError('unknown Material composite "${node['type']}"');
}
