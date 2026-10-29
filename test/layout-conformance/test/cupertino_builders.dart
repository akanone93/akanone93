// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The Cupertino composites of the layout conformance scenes (STU-005, S4.4):
// each scene type builds the real Flutter widget, so the goldens hold what
// Flutter lays out. SF Pro is not available to the harness, so the theme
// below gives every Cupertino text style the harness font (Noto Sans) and no
// letter spacing, keeping Flutter's sizes.

import 'package:flutter/cupertino.dart';

const Set<String> cupertinoTypes = <String>{
  'CupertinoDatePicker', //
  'CupertinoContextMenu', 'CupertinoContextMenuAction', 'CupertinoAlertDialog', 'CupertinoDialogAction', 'CupertinoActionSheet', 'CupertinoActionSheetAction', //
  'CupertinoButton', 'CupertinoSwitch', 'CupertinoSlider', 'CupertinoActivityIndicator', 'CupertinoTabBar', 'CupertinoTabItem', //
  'CupertinoNavigationBar', 'CupertinoNavSlot', 'CupertinoPageScaffold', 'CupertinoSlot', 'CupertinoTabScaffold', 'CupertinoPicker', 'CupertinoListTile', //
  'CupertinoListSection', 'CupertinoTextField', 'CupertinoSegmentedControl', 'CupertinoSlidingSegmentedControl', 'CupertinoSliverNavigationBar',
};

/// Types that take several children.
const Set<String> cupertinoMultiChild = <String>{
  'CupertinoContextMenu', 'CupertinoAlertDialog', 'CupertinoActionSheet', //
  'CupertinoTabBar', 'CupertinoNavigationBar', 'CupertinoPageScaffold', 'CupertinoTabScaffold', 'CupertinoListTile', //
  'CupertinoListSection',
  'CupertinoSegmentedControl', 'CupertinoPicker', 'CupertinoSlidingSegmentedControl', 'CupertinoSliverNavigationBar',
};

/// The families the dialog styles name (`CupertinoSystemText`, `CupertinoSystemDisplay`), registered with the harness font.
const List<(String, String)> cupertinoFonts = <(String, String)>[
  ('CupertinoSystemText', 'NotoSans_400Regular.ttf'),
  ('CupertinoSystemText', 'NotoSans_700Bold.ttf'),
  ('CupertinoSystemDisplay', 'NotoSans_400Regular.ttf'),
  ('CupertinoSystemDisplay', 'NotoSans_700Bold.ttf'),
];

TextStyle _style(double size, [FontWeight weight = FontWeight.w400]) =>
    TextStyle(fontFamily: 'NotoSans', fontSize: size, fontWeight: weight, letterSpacing: 0);

/// The text theme every Cupertino scene runs under.
final CupertinoThemeData cupertinoTestTheme = CupertinoThemeData(
  textTheme: CupertinoTextThemeData(
    textStyle: _style(17),
    actionTextStyle: _style(17),
    actionSmallTextStyle: _style(15),
    tabLabelTextStyle: _style(10),
    navTitleTextStyle: _style(17, FontWeight.w700),
    navLargeTitleTextStyle: _style(34, FontWeight.w700),
    navActionTextStyle: _style(17),
    pickerTextStyle: _style(21),
    dateTimePickerTextStyle: _style(21),
  ),
);

bool usesCupertino(Map<String, Object?> node) =>
    (node['type']! as String).startsWith('Cupertino') ||
    ((node['children'] as List<Object?>?) ?? <Object?>[]).any((Object? c) => usesCupertino(c! as Map<String, Object?>));

double? _num(Object? value) => value == null ? null : (value as num).toDouble();

/// Gives [node]'s subtree the status-bar and home-indicator insets its props ask for.
Widget _withSafeArea(Map<String, Object?> props, Widget child) {
  if (!props.containsKey('safeTop') && !props.containsKey('safeBottom')) {
    return child;
  }
  return Builder(
    builder: (BuildContext context) => MediaQuery(
      data: MediaQuery.of(context).copyWith(
        padding: EdgeInsets.only(top: _num(props['safeTop']) ?? 0, bottom: _num(props['safeBottom']) ?? 0),
        viewPadding: EdgeInsets.only(top: _num(props['safeTop']) ?? 0, bottom: _num(props['safeBottom']) ?? 0),
      ),
      child: child,
    ),
  );
}

Widget _slot(List<Map<String, Object?>> raw, List<Widget> built, String name) {
  for (int i = 0; i < raw.length; i++) {
    if ((raw[i]['type'] == 'CupertinoNavSlot' || raw[i]['type'] == 'CupertinoSlot') && (raw[i]['props'] as Map<String, Object?>)['slot'] == name) {
      return built[i];
    }
  }
  throw ArgumentError('no $name slot');
}

bool _hasSlot(List<Map<String, Object?>> raw, String name) => raw.any(
  (Map<String, Object?> n) =>
      (n['type'] == 'CupertinoNavSlot' || n['type'] == 'CupertinoSlot') &&
      (n['props'] as Map<String, Object?>)['slot'] == name,
);

List<Widget> _ofType(List<Map<String, Object?>> raw, List<Widget> built, String type) => <Widget>[
  for (int i = 0; i < raw.length; i++)
    if (raw[i]['type'] == type) built[i],
];

/// Builds the Cupertino node [node] of [type] from its built [children].
Widget buildCupertino(
  Map<String, Object?> node,
  Key key,
  Map<String, Object?> props,
  List<Widget> children,
  EdgeInsetsGeometry Function(Object?) padding,
) {
  final List<Map<String, Object?>> raw = <Map<String, Object?>>[
    for (final Object? c in (node['children'] as List<Object?>?) ?? <Object?>[]) c! as Map<String, Object?>,
  ];
  switch (node['type']! as String) {
    case 'CupertinoContextMenu':
      return CupertinoContextMenu(
        key: key,
        // The actions are the open sheet's rows; they are not part of the scene tree.
        actions: <Widget>[
          for (final Object? label in (props['actions'] as List<Object?>?) ?? <Object?>['Action'])
            CupertinoContextMenuAction(onPressed: () {}, child: Text(label! as String)),
        ],
        child: children.single,
      );
    case 'CupertinoContextMenuAction':
      return CupertinoContextMenuAction(
        key: key,
        isDefaultAction: props['isDefaultAction'] == true,
        isDestructiveAction: props['isDestructiveAction'] == true,
        onPressed: () {},
        child: children.single,
      );
    case 'CupertinoDatePicker':
      return CupertinoDatePicker(
        key: key,
        mode: CupertinoDatePickerMode.values.byName((props['mode'] as String?) ?? 'dateAndTime'),
        initialDateTime: DateTime(2018, 4, 9, 10, 30),
        use24hFormat: props['use24hFormat'] == true,
        itemExtent: _num(props['itemExtent']) ?? 32,
        onDateTimeChanged: (DateTime _) {},
      );
    case 'CupertinoAlertDialog':
      return _withSafeArea(
        props,
        CupertinoAlertDialog(
          key: key,
          title: _hasSlot(raw, 'title') ? _slot(raw, children, 'title') : null,
          content: _hasSlot(raw, 'content') ? _slot(raw, children, 'content') : null,
          actions: _ofType(raw, children, 'CupertinoDialogAction'),
        ),
      );
    case 'CupertinoDialogAction':
      return CupertinoDialogAction(
        key: key,
        isDefaultAction: props['isDefaultAction'] == true,
        isDestructiveAction: props['isDestructiveAction'] == true,
        onPressed: () {},
        child: children.single,
      );
    case 'CupertinoActionSheet':
      return _withSafeArea(
        props,
        CupertinoActionSheet(
          key: key,
          title: _hasSlot(raw, 'title') ? _slot(raw, children, 'title') : null,
          message: _hasSlot(raw, 'message') ? _slot(raw, children, 'message') : null,
          actions: _ofType(raw, children, 'CupertinoActionSheetAction'),
          cancelButton: _hasSlot(raw, 'cancelButton') ? _slot(raw, children, 'cancelButton') : null,
        ),
      );
    case 'CupertinoActionSheetAction':
      return CupertinoActionSheetAction(
        key: key,
        isDefaultAction: props['isDefaultAction'] == true,
        isDestructiveAction: props['isDestructiveAction'] == true,
        onPressed: () {},
        child: children.single,
      );
    case 'CupertinoButton':
      final CupertinoButtonSize size = CupertinoButtonSize.values.byName((props['size'] as String?) ?? 'large');
      final double min = switch (size) {
        CupertinoButtonSize.small => 28,
        CupertinoButtonSize.medium => 32,
        CupertinoButtonSize.large => 44,
      };
      final Size minimumSize = Size(_num(props['minWidth']) ?? min, _num(props['minHeight']) ?? min);
      final EdgeInsetsGeometry? pad = props.containsKey('padding') ? padding(props['padding']) : null;
      void onPressed() {}
      return switch (props['kind'] as String? ?? 'plain') {
        'filled' => CupertinoButton.filled(
          key: key, sizeStyle: size, padding: pad, minimumSize: minimumSize, onPressed: onPressed, child: children.single),
        'tinted' => CupertinoButton.tinted(
          key: key, sizeStyle: size, padding: pad, minimumSize: minimumSize, onPressed: onPressed, child: children.single),
        _ => CupertinoButton(
          key: key, sizeStyle: size, padding: pad, minimumSize: minimumSize, onPressed: onPressed, child: children.single),
      };
    case 'CupertinoSwitch':
      return CupertinoSwitch(key: key, value: props['value'] == true, onChanged: (bool _) {});
    case 'CupertinoActivityIndicator':
      return CupertinoActivityIndicator(key: key);
    case 'CupertinoSlider':
      return CupertinoSlider(key: key, value: 0.5, onChanged: (double _) {});
    case 'CupertinoTabItem':
    case 'CupertinoNavSlot':
    case 'CupertinoSlot':
      return KeyedSubtree(key: key, child: children.single);
    case 'CupertinoTabBar':
      final int active = raw.indexWhere((Map<String, Object?> n) => (n['props'] as Map<String, Object?>?)?['active'] == true);
      return _withSafeArea(
        props,
        CupertinoTabBar(
          key: key,
          backgroundColor: props['opaque'] == true ? const Color(0xFFFFFFFF) : null,
          height: _num(props['height']) ?? 50,
          currentIndex: active < 0 ? 0 : active,
          items: <BottomNavigationBarItem>[
            for (int i = 0; i < children.length; i++)
              BottomNavigationBarItem(icon: children[i], label: (raw[i]['props'] as Map<String, Object?>)['label'] as String?),
          ],
        ),
      );
    case 'CupertinoNavigationBar':
      final Widget? leading = _hasSlot(raw, 'leading') ? _slot(raw, children, 'leading') : null;
      final Widget? trailing = _hasSlot(raw, 'trailing') ? _slot(raw, children, 'trailing') : null;
      final Color? background = props['opaque'] == true ? const Color(0xFFFFFFFF) : null;
      return _withSafeArea(
        props,
        _hasSlot(raw, 'largeTitle')
            ? CupertinoNavigationBar.large(
                key: key,
                automaticallyImplyLeading: false,
                transitionBetweenRoutes: false,
                backgroundColor: background,
                leading: leading,
                trailing: trailing,
                largeTitle: _slot(raw, children, 'largeTitle'),
              )
            : CupertinoNavigationBar(
                key: key,
                automaticallyImplyLeading: false,
                automaticallyImplyMiddle: false,
                transitionBetweenRoutes: false,
                backgroundColor: background,
                leading: leading,
                middle: _hasSlot(raw, 'middle') ? _slot(raw, children, 'middle') : null,
                trailing: trailing,
              ),
      );
    case 'CupertinoSliverNavigationBar':
      return _withSafeArea(
        props,
        CupertinoSliverNavigationBar(
          key: key,
          automaticallyImplyLeading: false,
          automaticallyImplyTitle: false,
          transitionBetweenRoutes: false,
          leading: _hasSlot(raw, 'leading') ? _slot(raw, children, 'leading') : null,
          middle: _hasSlot(raw, 'middle') ? _slot(raw, children, 'middle') : null,
          trailing: _hasSlot(raw, 'trailing') ? _slot(raw, children, 'trailing') : null,
          largeTitle: _slot(raw, children, 'largeTitle'),
        ),
      );
    case 'CupertinoPicker':
      return CupertinoPicker(
        key: key,
        itemExtent: _num(props['itemExtent']) ?? 32,
        scrollController: FixedExtentScrollController(initialItem: (props['selectedItem'] as int?) ?? 0),
        onSelectedItemChanged: (int _) {},
        children: children.isNotEmpty
            ? children
            : <Widget>[
                for (int i = 0; i < ((props['itemCount'] as int?) ?? 3); i++)
                  Text('Item $i', key: ValueKey<String>('${node['id']}.item$i'), style: _style(21)),
              ],
      );
    case 'CupertinoTabScaffold':
      final int tabBar = raw.indexWhere((Map<String, Object?> n) => n['type'] == 'CupertinoTabBar');
      final int tab = raw.indexWhere((Map<String, Object?> n) => n['type'] != 'CupertinoTabBar');
      return _withSafeArea(
        props,
        CupertinoTabScaffold(
          key: key,
          tabBar: children[tabBar] as CupertinoTabBar,
          tabBuilder: (BuildContext context, int index) => children[tab],
        ),
      );
    case 'CupertinoListTile':
      Widget? slot(String name) => _hasSlot(raw, name) ? _slot(raw, children, name) : null;
      final EdgeInsetsGeometry? tilePadding = props.containsKey('padding') ? padding(props['padding']) : null;
      return props['type'] == 'notched'
          ? CupertinoListTile.notched(
              key: key,
              title: slot('title')!,
              subtitle: slot('subtitle'),
              additionalInfo: slot('additionalInfo'),
              leading: slot('leading'),
              trailing: slot('trailing'),
              padding: tilePadding,
            )
          : CupertinoListTile(
              key: key,
              title: slot('title')!,
              subtitle: slot('subtitle'),
              additionalInfo: slot('additionalInfo'),
              leading: slot('leading'),
              trailing: slot('trailing'),
              padding: tilePadding,
            );
    case 'CupertinoListSection':
      Widget? part(String name) => _hasSlot(raw, name) ? _slot(raw, children, name) : null;
      final List<Widget> rows = <Widget>[
        for (int i = 0; i < raw.length; i++)
          if (raw[i]['type'] == 'CupertinoListTile') children[i],
      ];
      return props['type'] == 'insetGrouped'
          ? CupertinoListSection.insetGrouped(
              key: key,
              header: part('header'),
              footer: part('footer'),
              hasLeading: props['hasLeading'] != false,
              children: rows,
            )
          : CupertinoListSection(
              key: key,
              header: part('header'),
              footer: part('footer'),
              topMargin: _num(props['topMargin']) ?? 22,
              children: rows,
            );
    case 'CupertinoTextField':
      return CupertinoTextField(
        key: key,
        controller: TextEditingController(text: (props['text'] as String?) ?? ''),
        placeholder: props['placeholder'] as String?,
        padding: props.containsKey('padding') ? padding(props['padding']) : const EdgeInsets.all(7),
      );
    case 'CupertinoSegmentedControl':
      return CupertinoSegmentedControl<int>(
        key: key,
        children: <int, Widget>{for (int i = 0; i < children.length; i++) i: children[i]},
        groupValue: 0,
        onValueChanged: (int _) {},
        padding: props.containsKey('padding') ? padding(props['padding']) : null,
      );
    case 'CupertinoSlidingSegmentedControl':
      return CupertinoSlidingSegmentedControl<int>(
        key: key,
        children: <int, Widget>{for (int i = 0; i < children.length; i++) i: children[i]},
        groupValue: props.containsKey('groupValue') ? props['groupValue'] as int? : 0,
        onValueChanged: (int? _) {},
        proportionalWidth: (props['proportionalWidth'] as bool?) ?? false,
        padding: props.containsKey('padding') ? padding(props['padding']) : const EdgeInsets.symmetric(vertical: 2, horizontal: 3),
      );
    case 'CupertinoPageScaffold':
      final int bar = raw.indexWhere((Map<String, Object?> n) => n['type'] == 'CupertinoNavigationBar');
      final int content = raw.indexWhere((Map<String, Object?> n) => n['type'] != 'CupertinoNavigationBar');
      return _withSafeArea(
        props,
        CupertinoPageScaffold(
          key: key,
          navigationBar: bar < 0 ? null : children[bar] as ObstructingPreferredSizeWidget,
          child: children[content],
        ),
      );
  }
  throw ArgumentError('unknown Cupertino type "${node['type']}"');
}
