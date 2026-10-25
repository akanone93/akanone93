// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Layout conformance goldens (STU-005, ADR-0013 "Conformance suite and
// tolerance"): Flutter itself lays out every scene in `scenes/` under every
// configuration and records the global rect of every identified node. With
// UPDATE_GOLDENS=1 the goldens are written; otherwise the test fails when a
// golden is missing, stale or orphaned.

import 'dart:convert';
import 'dart:math' as math;
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

/// The Flutter framework revision the goldens are captured with.
const String flutterRevision = '6a19cca56475dbfba1478ee68d7bd0c2ef891da1';

class _Frame {
  const _Frame(this.name, this.width, this.height, this.devicePixelRatio);
  final String name;
  final double width;
  final double height;
  final double devicePixelRatio;
}

class _Direction {
  const _Direction(this.name, this.textDirection, this.locale);
  final String name;
  final TextDirection textDirection;
  final String locale;
}

const List<_Frame> _frames = <_Frame>[_Frame('phone', 360, 800, 3), _Frame('tablet', 800, 1280, 2)];

const List<_Direction> _directions = <_Direction>[
  _Direction('ltr', TextDirection.ltr, 'en'),
  _Direction('rtl', TextDirection.rtl, 'ar'),
];

/// Scenes have no text yet, so only 1.0 is captured; 2.0 joins with text.
// Scenes with text are captured at text scale 1.0 and 2.0; others at 1.0.
const List<double> _textScales = <double>[1.0, 2.0];

// The repository's Noto fonts, under the families the scenes name; Studio's
// tests register the same files with CanvasKit.
const List<(String, String)> _fonts = <(String, String)>[
  ('NotoSans', 'NotoSans_400Regular.ttf'),
  ('NotoSans', 'NotoSans_700Bold.ttf'),
  ('NotoNaskhArabic', 'NotoNaskhArabic_400Regular.ttf'),
];

Future<void> _loadFonts() async {
  final Map<String, FontLoader> loaders = <String, FontLoader>{};
  for (final (String family, String file) in _fonts) {
    final Uint8List bytes = File('../../schema/goldens/fonts/$file').readAsBytesSync();
    (loaders[family] ??= FontLoader(family)).addFont(Future<ByteData>.value(ByteData.sublistView(bytes)));
  }
  for (final FontLoader loader in loaders.values) {
    await loader.load();
  }
}

StrutStyle _strut(Map<String, Object?> strut) => StrutStyle(
  fontFamily: strut['fontFamily'] as String?,
  fontSize: _optionalNumber(strut, 'fontSize'),
  height: _optionalNumber(strut, 'height'),
  leading: _optionalNumber(strut, 'leading'),
  forceStrutHeight: strut['forceStrutHeight'] as bool?,
  leadingDistribution: strut.containsKey('leadingDistribution')
      ? TextLeadingDistribution.values.byName(strut['leadingDistribution']! as String)
      : null,
);

void _collectTextIds(Map<String, Object?> node, List<String> into) {
  if (node['type'] == 'Text') {
    into.add(node['id']! as String);
  }
  for (final Object? child in (node['children'] as List<Object?>?) ?? const <Object?>[]) {
    _collectTextIds(child! as Map<String, Object?>, into);
  }
}

bool _hasText(Map<String, Object?> node) =>
    node['type'] == 'Text' ||
    ((node['children'] as List<Object?>?) ?? <Object?>[]).any(
      (Object? child) => _hasText(child! as Map<String, Object?>),
    );

double _number(Object? value) {
  if (value == 'infinity') {
    return double.infinity;
  }
  return (value! as num).toDouble();
}

double? _optionalNumber(Map<String, Object?> props, String name) =>
    props.containsKey(name) ? _number(props[name]) : null;

EdgeInsetsGeometry _padding(Object? value) {
  if (value is List<Object?>) {
    return EdgeInsets.fromLTRB(_number(value[0]), _number(value[1]), _number(value[2]), _number(value[3]));
  }
  final Map<String, Object?> sides = value! as Map<String, Object?>;
  return EdgeInsetsDirectional.fromSTEB(
    _number(sides['start'] ?? 0),
    _number(sides['top'] ?? 0),
    _number(sides['end'] ?? 0),
    _number(sides['bottom'] ?? 0),
  );
}

AlignmentGeometry _alignment(Object? value) {
  if (value == null) {
    return Alignment.center;
  }
  if (value is List<Object?>) {
    return Alignment(_number(value[0]), _number(value[1]));
  }
  final Map<String, Object?> parts = value as Map<String, Object?>;
  return AlignmentDirectional(_number(parts['start']), _number(parts['y']));
}

T _enum<T extends Enum>(List<T> values, Object? name, T fallback) =>
    name == null ? fallback : values.byName(name as String);

/// An animation running from the node's `begin` (default 0) to `end` (default
/// 1), held at the scene's animation value [t].
Animation<double> _animation(Map<String, Object?> props, double t) =>
    Tween<double>(
      begin: _optionalNumber(props, 'begin') ?? 0,
      end: _optionalNumber(props, 'end') ?? 1,
    ).animate(AlwaysStoppedAnimation<double>(t));

Offset _offset(Object? value) {
  final List<Object?> pair = value! as List<Object?>;
  return Offset(_number(pair[0]), _number(pair[1]));
}

TableColumnWidth _columnWidth(Object? spec) {
  final Map<String, Object?> props = spec! as Map<String, Object?>;
  return switch (props['type']) {
    'fixed' => FixedColumnWidth(_number(props['value'])),
    'fraction' => FractionColumnWidth(_number(props['value'])),
    'flex' => FlexColumnWidth(_optionalNumber(props, 'value') ?? 1),
    'intrinsic' => IntrinsicColumnWidth(flex: _optionalNumber(props, 'flex')),
    'max' => MaxColumnWidth(_columnWidth(props['a']), _columnWidth(props['b'])),
    'min' => MinColumnWidth(_columnWidth(props['a']), _columnWidth(props['b'])),
    _ => throw ArgumentError('unknown table column width $spec'),
  };
}

const Duration _animationDuration = Duration(seconds: 1);


/// Viewport and sliver scene nodes (STU-068, S4.1): their children are built lazily, so a scene
/// golden holds only the nodes Flutter has built.
const Set<String> _lazyTypes = <String>{
  'ListView', 'GridView', 'PageView', 'ReorderableListView', 'SliverList', 'SliverFixedExtentList',
  'SliverVariedExtentList', 'SliverGrid', 'SliverFillViewport',
};

bool _hasLazy(Map<String, Object?> node) =>
    _lazyTypes.contains(node['type']) ||
    ((node['children'] as List<Object?>?) ?? <Object?>[]).any(
      (Object? child) => _hasLazy(child! as Map<String, Object?>),
    );

/// A header delegate with fixed extents and a fixed child (SliverPersistentHeader, SliverAppBar).
class _FixedHeaderDelegate extends SliverPersistentHeaderDelegate {
  const _FixedHeaderDelegate(this.min, this.max, this.child);
  final double min;
  final double max;
  final Widget child;

  @override
  double get minExtent => min;

  @override
  double get maxExtent => max;

  @override
  Widget build(BuildContext context, double shrinkOffset, bool overlapsContent) => child;

  @override
  bool shouldRebuild(covariant _FixedHeaderDelegate oldDelegate) => true;
}

SliverGridDelegate _gridDelegate(Map<String, Object?> props) {
  final double mainAxisSpacing = _optionalNumber(props, 'mainAxisSpacing') ?? 0;
  final double crossAxisSpacing = _optionalNumber(props, 'crossAxisSpacing') ?? 0;
  final double childAspectRatio = _optionalNumber(props, 'childAspectRatio') ?? 1;
  final double? mainAxisExtent = _optionalNumber(props, 'mainAxisExtent');
  if (props.containsKey('maxCrossAxisExtent')) {
    return SliverGridDelegateWithMaxCrossAxisExtent(
      maxCrossAxisExtent: _number(props['maxCrossAxisExtent']),
      mainAxisSpacing: mainAxisSpacing,
      crossAxisSpacing: crossAxisSpacing,
      childAspectRatio: childAspectRatio,
      mainAxisExtent: mainAxisExtent,
    );
  }
  return SliverGridDelegateWithFixedCrossAxisCount(
    crossAxisCount: props['crossAxisCount']! as int,
    mainAxisSpacing: mainAxisSpacing,
    crossAxisSpacing: crossAxisSpacing,
    childAspectRatio: childAspectRatio,
    mainAxisExtent: mainAxisExtent,
  );
}

SliverChildListDelegate _listDelegate(List<Widget> children) => SliverChildListDelegate(
  children,
  addAutomaticKeepAlives: false,
  addRepaintBoundaries: false,
  addSemanticIndexes: false,
);

ScrollCacheExtent? _cacheExtent(Map<String, Object?> props) =>
    props.containsKey('cacheExtent') ? ScrollCacheExtent.pixels(_number(props['cacheExtent'])) : null;

Widget _buildScroll(String type, Key key, Map<String, Object?> props, List<Widget> children) {
  final Axis scrollDirection = _enum(Axis.values, props['scrollDirection'], Axis.vertical);
  final bool reverse = props['reverse'] == true;
  final bool shrinkWrap = props['shrinkWrap'] == true;
  final ScrollController controller = ScrollController(initialScrollOffset: _optionalNumber(props, 'scrollOffset') ?? 0);
  final EdgeInsetsGeometry? padding = props.containsKey('padding') ? _padding(props['padding']) : null;
  final double? itemExtent = _optionalNumber(props, 'itemExtent');
  switch (type) {
    case 'Viewport':
      return Viewport(
        key: key,
        axisDirection: _enum(AxisDirection.values, props['axisDirection'], AxisDirection.down),
        anchor: _optionalNumber(props, 'anchor') ?? 0,
        offset: ViewportOffset.fixed(_optionalNumber(props, 'offset') ?? 0),
        scrollCacheExtent: _cacheExtent(props),
        slivers: children,
      );
    case 'ShrinkWrappingViewport':
      return ShrinkWrappingViewport(
        key: key,
        axisDirection: _enum(AxisDirection.values, props['axisDirection'], AxisDirection.down),
        offset: ViewportOffset.fixed(_optionalNumber(props, 'offset') ?? 0),
        slivers: children,
      );
    case 'CustomScrollView':
      return CustomScrollView(
        key: key,
        scrollDirection: scrollDirection,
        reverse: reverse,
        controller: controller,
        primary: false,
        shrinkWrap: shrinkWrap,
        anchor: _optionalNumber(props, 'anchor') ?? 0,
        scrollCacheExtent: _cacheExtent(props),
        slivers: children,
      );
    case 'ListView':
      return ListView(
        key: key,
        scrollDirection: scrollDirection,
        reverse: reverse,
        controller: controller,
        primary: false,
        shrinkWrap: shrinkWrap,
        padding: padding,
        itemExtent: itemExtent,
        scrollCacheExtent: _cacheExtent(props),
        addAutomaticKeepAlives: false,
        addRepaintBoundaries: false,
        addSemanticIndexes: false,
        children: children,
      );
    case 'GridView':
      return GridView(
        key: key,
        scrollDirection: scrollDirection,
        reverse: reverse,
        controller: controller,
        primary: false,
        shrinkWrap: shrinkWrap,
        padding: padding,
        gridDelegate: _gridDelegate(props),
        scrollCacheExtent: _cacheExtent(props),
        addAutomaticKeepAlives: false,
        addRepaintBoundaries: false,
        addSemanticIndexes: false,
        children: children,
      );
    case 'PageView':
      return PageView(
        key: key,
        scrollDirection: scrollDirection,
        reverse: reverse,
        controller: PageController(
          initialPage: (props['initialPage'] as int?) ?? 0,
          viewportFraction: _optionalNumber(props, 'viewportFraction') ?? 1,
        ),
        padEnds: props['padEnds'] != false,
        children: children,
      );
    case 'NestedScrollView':
      return NestedScrollView(
        key: key,
        scrollDirection: scrollDirection,
        reverse: reverse,
        headerSliverBuilder: (BuildContext context, bool innerBoxIsScrolled) =>
            children.sublist(0, children.length - 1),
        body: children.last,
      );
    case 'ReorderableListView':
      return Localizations(
        locale: const Locale('en'),
        delegates: const <LocalizationsDelegate<dynamic>>[
          DefaultWidgetsLocalizations.delegate,
          DefaultMaterialLocalizations.delegate,
        ],
        child: Material(
          type: MaterialType.transparency,
          child: Overlay(initialEntries: <OverlayEntry>[OverlayEntry(builder: (BuildContext context) => ReorderableListView(
            key: key,
            scrollDirection: scrollDirection,
            reverse: reverse,
            scrollController: controller,
            primary: false,
            shrinkWrap: shrinkWrap,
            padding: padding is EdgeInsets ? padding : null,
            itemExtent: itemExtent,
            onReorder: (int oldIndex, int newIndex) {},
            children: children,
          ))]),
        ),
      );
  }
  throw ArgumentError('unknown scroll view type "$type"');
}

Widget _buildSliver(String type, Key key, Map<String, Object?> props, List<Widget> children) {
  final Widget? child = children.length == 1 ? children.single : null;
  switch (type) {
    case 'SliverToBoxAdapter':
      return SliverToBoxAdapter(key: key, child: child);
    case 'SliverList':
      return SliverList(key: key, delegate: _listDelegate(children));
    case 'SliverFixedExtentList':
      return SliverFixedExtentList(key: key, itemExtent: _number(props['itemExtent']), delegate: _listDelegate(children));
    case 'SliverVariedExtentList':
      final List<double> extents = <double>[for (final Object? e in props['itemExtents']! as List<Object?>) _number(e)];
      return SliverVariedExtentList(
        key: key,
        itemExtentBuilder: (int index, SliverLayoutDimensions dimensions) => index < extents.length ? extents[index] : null,
        delegate: _listDelegate(children),
      );
    case 'SliverGrid':
      return SliverGrid(key: key, gridDelegate: _gridDelegate(props), delegate: _listDelegate(children));
    case 'SliverFillViewport':
      return SliverFillViewport(
        key: key,
        viewportFraction: _optionalNumber(props, 'viewportFraction') ?? 1,
        padEnds: props['padEnds'] != false,
        delegate: _listDelegate(children),
      );
    case 'SliverFillRemaining':
      return SliverFillRemaining(
        key: key,
        hasScrollBody: props['hasScrollBody'] != false,
        fillOverscroll: props['fillOverscroll'] == true,
        child: child,
      );
    case 'SliverPadding':
      return SliverPadding(key: key, padding: _padding(props['padding']), sliver: child);
    case 'SliverPersistentHeader':
    case 'SliverAppBar':
      final double min;
      final double max;
      if (type == 'SliverAppBar') {
        // SliverAppBar's delegate: collapsedHeight or toolbarHeight collapsed, expandedHeight or toolbarHeight expanded.
        final double toolbarHeight = _optionalNumber(props, 'toolbarHeight') ?? kToolbarHeight;
        min = _optionalNumber(props, 'collapsedHeight') ?? toolbarHeight;
        max = math.max(_optionalNumber(props, 'expandedHeight') ?? toolbarHeight, min);
      } else {
        min = _number(props['minExtent']);
        max = _number(props['maxExtent']);
      }
      return SliverPersistentHeader(
        key: key,
        pinned: props['pinned'] == true,
        floating: props['floating'] == true,
        delegate: _FixedHeaderDelegate(min, max, child ?? const SizedBox.expand()),
      );
  }
  throw ArgumentError('unknown sliver type "$type"');
}

/// The rect of the render object a scene id keys: a box's global rect, a sliver's paint area.
List<double> _rectOf(WidgetTester tester, String id) {
  final Finder finder = find.byKey(ValueKey<String>(id), skipOffstage: false);
  final RenderObject renderObject = tester.element(finder).renderObject!;
  if (renderObject is RenderSliver) {
    final Offset origin = MatrixUtils.transformPoint(renderObject.getTransformTo(null), Offset.zero);
    final Rect bounds = renderObject.paintBounds;
    return <double>[origin.dx, origin.dy, bounds.width, bounds.height];
  }
  final Rect rect = tester.getRect(finder);
  return <double>[rect.left, rect.top, rect.width, rect.height];
}


/// The Material composites (S4.3): scenes name the part of a composite a child
/// fills with `slot` (default `child`); the golden writer builds the real widget.
const Set<String> _materialTypes = <String>{
  'Icon', 'ElevatedButton', 'FilledButton', 'FilledTonalButton', 'OutlinedButton', 'TextButton', 'IconButton',
  'FloatingActionButton', 'ListTile', 'PreferredSize', 'AppBar', 'Scaffold',
  'Chip', 'FilterChip', 'InputChip', 'TextField', 'NavigationBar', 'NavigationDestination',
};

bool _usesMaterial(Map<String, Object?> node) =>
    _materialTypes.contains(node['type']) ||
    ((node['children'] as List<Object?>?) ?? <Object?>[]).any(
      (Object? child) => _usesMaterial(child! as Map<String, Object?>),
    );

/// Material 3 in the `NotoSans` the goldens load, with the English strings
/// `Localizations` needs, a transparent `Material` for ink, and no animation.
Widget _materialScope(Widget child, TextDirection direction) => Localizations(
  locale: const Locale('en'),
  delegates: const <LocalizationsDelegate<dynamic>>[
    DefaultMaterialLocalizations.delegate,
    DefaultWidgetsLocalizations.delegate,
  ],
  child: Theme(
    data: ThemeData(useMaterial3: true, fontFamily: 'NotoSans'),
    child: Directionality(
      textDirection: direction,
      child: Material(type: MaterialType.transparency, child: child),
    ),
  ),
);

MaterialTapTargetSize _tapTarget(Map<String, Object?> props) =>
    props['tapTargetSize'] == 'shrinkWrap' ? MaterialTapTargetSize.shrinkWrap : MaterialTapTargetSize.padded;

VisualDensity _density(Map<String, Object?> props) => switch (props['density']) {
  'comfortable' => VisualDensity.comfortable,
  'compact' => VisualDensity.compact,
  _ => VisualDensity.standard,
};

/// The slot children of [node] built, keyed by slot name.
Map<String, List<Widget>> _buildSlots(Map<String, Object?> node, String phase, double t) {
  final Map<String, List<Widget>> slots = <String, List<Widget>>{};
  for (final Object? raw in (node['children'] as List<Object?>?) ?? <Object?>[]) {
    final Map<String, Object?> child = raw! as Map<String, Object?>;
    (slots[(child['slot'] as String?) ?? 'child'] ??= <Widget>[]).add(_build(child, phase, t));
  }
  return slots;
}

/// A [NavigationDestination] node: the icon slot and the `label` prop. The tooltip is left empty,
/// as it needs an `Overlay` that has no effect on layout.
NavigationDestination _destination(Map<String, Object?> node, String phase, double t) {
  final Map<String, Object?> props = (node['props'] as Map<String, Object?>?) ?? <String, Object?>{};
  return NavigationDestination(
    key: ValueKey<String>(node['id']! as String),
    icon: _buildSlots(node, phase, t)['icon']!.single,
    label: (props['label'] as String?) ?? '',
    tooltip: '',
  );
}

Widget _buildNavigationBar(Map<String, Object?> node, Map<String, Object?> props, Key key, String phase, double t) {
  return NavigationBar(
    key: key,
    selectedIndex: (props['selectedIndex'] as num?)?.toInt() ?? 0,
    height: _optionalNumber(props, 'height'),
    labelBehavior: switch (props['labelBehavior']) {
      'alwaysHide' => NavigationDestinationLabelBehavior.alwaysHide,
      'onlyShowSelected' => NavigationDestinationLabelBehavior.onlyShowSelected,
      _ => NavigationDestinationLabelBehavior.alwaysShow,
    },
    destinations: <Widget>[
      for (final Object? raw in (node['children'] as List<Object?>?) ?? <Object?>[])
        _destination(raw! as Map<String, Object?>, phase, t),
    ],
  );
}

Widget _buildMaterial(Map<String, Object?> node, String phase, double t) {
  final String id = node['id']! as String;
  final String type = node['type']! as String;
  final Key key = ValueKey<String>(id);
  final Map<String, Object?> props = (node['props'] as Map<String, Object?>?) ?? <String, Object?>{};
  if (type == 'NavigationBar') {
    return _buildNavigationBar(node, props, key, phase, t);
  }
  final Map<String, List<Widget>> slots = <String, List<Widget>>{};
  for (final Object? raw in (node['children'] as List<Object?>?) ?? <Object?>[]) {
    final Map<String, Object?> child = raw! as Map<String, Object?>;
    (slots[(child['slot'] as String?) ?? 'child'] ??= <Widget>[]).add(_build(child, phase, t));
  }
  Widget? one(String name) {
    final List<Widget>? found = slots[name];
    if (found != null && found.length > 1) {
      throw ArgumentError('node "$id" slot $name takes one child');
    }
    return found?.single;
  }

  final ButtonStyle style = ButtonStyle(tapTargetSize: _tapTarget(props), visualDensity: _density(props));
  void onPressed() {}
  switch (type) {
    case 'Icon':
      return Icon(Icons.star, key: key, size: _optionalNumber(props, 'size'));
    case 'ElevatedButton':
      final Widget? icon = one('icon');
      return icon == null
          ? ElevatedButton(key: key, onPressed: onPressed, style: style, child: one('child'))
          : ElevatedButton.icon(key: key, onPressed: onPressed, style: style, icon: icon, label: one('child')!);
    case 'FilledButton':
    case 'FilledTonalButton':
      final Widget? icon = one('icon');
      final bool tonal = type == 'FilledTonalButton';
      if (icon == null) {
        return tonal
            ? FilledButton.tonal(key: key, onPressed: onPressed, style: style, child: one('child'))
            : FilledButton(key: key, onPressed: onPressed, style: style, child: one('child'));
      }
      return tonal
          ? FilledButton.tonalIcon(key: key, onPressed: onPressed, style: style, icon: icon, label: one('child')!)
          : FilledButton.icon(key: key, onPressed: onPressed, style: style, icon: icon, label: one('child')!);
    case 'OutlinedButton':
      final Widget? icon = one('icon');
      return icon == null
          ? OutlinedButton(key: key, onPressed: onPressed, style: style, child: one('child'))
          : OutlinedButton.icon(key: key, onPressed: onPressed, style: style, icon: icon, label: one('child')!);
    case 'TextButton':
      final Widget? icon = one('icon');
      return icon == null
          ? TextButton(key: key, onPressed: onPressed, style: style, child: one('child')!)
          : TextButton.icon(key: key, onPressed: onPressed, style: style, icon: icon, label: one('child')!);
    case 'IconButton':
      return switch (props['variant']) {
        'filled' => IconButton.filled(key: key, onPressed: onPressed, style: style, icon: one('child')!),
        'tonal' => IconButton.filledTonal(key: key, onPressed: onPressed, style: style, icon: one('child')!),
        'outlined' => IconButton.outlined(key: key, onPressed: onPressed, style: style, icon: one('child')!),
        _ => IconButton(key: key, onPressed: onPressed, style: style, icon: one('child')!),
      };
    case 'FloatingActionButton':
      final MaterialTapTargetSize target = _tapTarget(props);
      return switch (props['kind']) {
        'small' => FloatingActionButton.small(key: key, onPressed: onPressed, materialTapTargetSize: target, child: one('child')),
        'large' => FloatingActionButton.large(key: key, onPressed: onPressed, materialTapTargetSize: target, child: one('child')),
        'extended' => FloatingActionButton.extended(
          key: key,
          onPressed: onPressed,
          materialTapTargetSize: target,
          icon: one('child'),
          label: one('label')!,
        ),
        _ => FloatingActionButton(key: key, onPressed: onPressed, materialTapTargetSize: target, child: one('child')),
      };
    case 'PreferredSize':
      return PreferredSize(key: key, preferredSize: Size.fromHeight(_number(props['height'])), child: one('child')!);
    case 'AppBar':
      return AppBar(
        key: key,
        automaticallyImplyLeading: false,
        leading: one('leading'),
        title: one('title'),
        actions: slots['actions'],
        bottom: one('bottom') as PreferredSizeWidget?,
        toolbarHeight: _optionalNumber(props, 'toolbarHeight'),
        centerTitle: props['centerTitle'] == true,
        titleSpacing: _optionalNumber(props, 'titleSpacing'),
        leadingWidth: _optionalNumber(props, 'leadingWidth'),
      );
    case 'Scaffold':
      return Scaffold(
        key: key,
        appBar: one('appBar') as PreferredSizeWidget?,
        body: one('child'),
        floatingActionButton: one('floatingActionButton'),
        floatingActionButtonLocation: switch (props['floatingActionButtonLocation']) {
          'startTop' => FloatingActionButtonLocation.startTop,
          'centerTop' => FloatingActionButtonLocation.centerTop,
          'endTop' => FloatingActionButtonLocation.endTop,
          'startFloat' => FloatingActionButtonLocation.startFloat,
          'centerFloat' => FloatingActionButtonLocation.centerFloat,
          'startDocked' => FloatingActionButtonLocation.startDocked,
          'centerDocked' => FloatingActionButtonLocation.centerDocked,
          'endDocked' => FloatingActionButtonLocation.endDocked,
          _ => FloatingActionButtonLocation.endFloat,
        },
        bottomNavigationBar: one('bottomNavigationBar'),
        bottomSheet: one('bottomSheet'),
      );
    case 'TextField':
      return TextField(
        key: key,
        controller: TextEditingController(text: props['text'] as String?),
        decoration: InputDecoration(
          labelText: props['labelText'] as String?,
          helperText: props['helperText'] as String?,
          errorText: props['errorText'] as String?,
          filled: props['filled'] == true,
          border: props['border'] == 'outline' ? const OutlineInputBorder() : const UnderlineInputBorder(),
          isDense: props['isDense'] == true ? true : null,
          prefixIcon: one('prefixIcon'),
          suffixIcon: one('suffixIcon'),
        ),
      );
    case 'Chip':
    case 'FilterChip':
    case 'InputChip':
      final Widget label = one('label')!;
      final Widget? avatar = one('avatar');
      final Widget? deleteIcon = props['deletable'] == true ? one('deleteIcon') : null;
      final VoidCallback? onDeleted = deleteIcon == null ? null : () {};
      final bool selected = props['selected'] == true;
      final VisualDensity density = _density(props);
      final MaterialTapTargetSize target = _tapTarget(props);
      return switch (type) {
        'FilterChip' => FilterChip(
          key: key,
          label: label,
          avatar: avatar,
          selected: selected,
          onSelected: (bool _) {},
          deleteIcon: deleteIcon,
          deleteButtonTooltipMessage: '',
          visualDensity: density,
          materialTapTargetSize: target,
        ),
        'InputChip' => InputChip(
          key: key,
          label: label,
          avatar: avatar,
          selected: selected,
          onPressed: () {},
          onDeleted: onDeleted,
          deleteIcon: deleteIcon,
          deleteButtonTooltipMessage: '',
          visualDensity: density,
          materialTapTargetSize: target,
        ),
        _ => Chip(
          key: key,
          label: label,
          avatar: avatar,
          onDeleted: onDeleted,
          deleteIcon: deleteIcon,
          deleteButtonTooltipMessage: '',
          visualDensity: density,
          materialTapTargetSize: target,
        ),
      };
    case 'ListTile':
      return ListTile(
        key: key,
        leading: one('leading'),
        title: one('title'),
        subtitle: one('subtitle'),
        trailing: one('trailing'),
        dense: props['dense'] == true ? true : null,
        isThreeLine: props['isThreeLine'] == true,
        visualDensity: _density(props),
      );
  }
  throw ArgumentError('unknown Material type "$type"');
}

/// Builds the widget for [node]. Implicitly animated nodes take their `from`
/// or `to` props according to [phase]; transitions are held at the scene's
/// animation value [t].
Widget _build(Map<String, Object?> node, String phase, double t) {
  final String id = node['id']! as String;
  final String type = node['type']! as String;
  final Map<String, Object?> props = (node['props'] as Map<String, Object?>?) ?? <String, Object?>{};
  if (_materialTypes.contains(type)) {
    return _buildMaterial(node, phase, t);
  }
  final List<Widget> children = <Widget>[
    for (final Object? child in (node['children'] as List<Object?>?) ?? <Object?>[])
      _build(child! as Map<String, Object?>, phase, t),
  ];
  const Set<String> multiChild = <String>{
    'Row', 'Column', 'Flex', 'Stack', 'IndexedStack', 'Wrap', 'Table', //
    'Viewport', 'ShrinkWrappingViewport', 'CustomScrollView', 'ListView', 'GridView', 'PageView', 'NestedScrollView', //
    'ReorderableListView', 'SliverList', 'SliverFixedExtentList', 'SliverVariedExtentList', 'SliverGrid', 'SliverFillViewport',
  };
  if (!multiChild.contains(type) && children.length > 1) {
    throw ArgumentError('node "$id" of type $type takes at most one child');
  }
  final Widget? child = children.length == 1 ? children.single : null;
  final Key key = ValueKey<String>(id);
  const Set<String> scrollTypes = <String>{
    'Viewport', 'ShrinkWrappingViewport', 'CustomScrollView', 'ListView', 'GridView', 'PageView', 'NestedScrollView',
    'ReorderableListView',
  };
  if (scrollTypes.contains(type)) {
    return _buildScroll(type, key, props, children);
  }
  if (type.startsWith('Sliver')) {
    return _buildSliver(type, key, props, children);
  }
  switch (type) {
    case 'TestBox':
      return SizedBox(key: key, width: _number(props['width']), height: _number(props['height']));
    case 'SizedBox':
      return SizedBox(
        key: key,
        width: _optionalNumber(props, 'width'),
        height: _optionalNumber(props, 'height'),
        child: child,
      );
    case 'ConstrainedBox':
      return ConstrainedBox(
        key: key,
        constraints: BoxConstraints(
          minWidth: _optionalNumber(props, 'minWidth') ?? 0,
          maxWidth: _optionalNumber(props, 'maxWidth') ?? double.infinity,
          minHeight: _optionalNumber(props, 'minHeight') ?? 0,
          maxHeight: _optionalNumber(props, 'maxHeight') ?? double.infinity,
        ),
        child: child,
      );
    case 'Padding':
      return Padding(key: key, padding: _padding(props['padding']), child: child);
    case 'Align':
      return Align(
        key: key,
        alignment: _alignment(props['alignment']),
        widthFactor: _optionalNumber(props, 'widthFactor'),
        heightFactor: _optionalNumber(props, 'heightFactor'),
        child: child,
      );
    case 'Center':
      return Center(
        key: key,
        widthFactor: _optionalNumber(props, 'widthFactor'),
        heightFactor: _optionalNumber(props, 'heightFactor'),
        child: child,
      );
    case 'DecoratedBox':
      final String hex = (props['color'] as String?) ?? '#000000';
      return DecoratedBox(
        key: key,
        decoration: BoxDecoration(color: Color(0xff000000 | int.parse(hex.substring(1), radix: 16))),
        child: child,
      );
    case 'Row':
    case 'Column':
    case 'Flex':
      return Flex(
        key: key,
        direction: switch (type) {
          'Row' => Axis.horizontal,
          'Column' => Axis.vertical,
          _ => _enum(Axis.values, props['direction'], Axis.horizontal),
        },
        mainAxisAlignment: _enum(MainAxisAlignment.values, props['mainAxisAlignment'], MainAxisAlignment.start),
        mainAxisSize: _enum(MainAxisSize.values, props['mainAxisSize'], MainAxisSize.max),
        crossAxisAlignment: _enum(CrossAxisAlignment.values, props['crossAxisAlignment'], CrossAxisAlignment.center),
        verticalDirection: _enum(VerticalDirection.values, props['verticalDirection'], VerticalDirection.down),
        spacing: _optionalNumber(props, 'spacing') ?? 0,
        children: children,
      );
    case 'Stack':
      return Stack(
        key: key,
        alignment: props.containsKey('alignment') ? _alignment(props['alignment']) : AlignmentDirectional.topStart,
        fit: _enum(StackFit.values, props['fit'], StackFit.loose),
        children: children,
      );
    case 'IndexedStack':
      return IndexedStack(
        key: key,
        alignment: props.containsKey('alignment') ? _alignment(props['alignment']) : AlignmentDirectional.topStart,
        sizing: _enum(StackFit.values, props['sizing'], StackFit.loose),
        index: (props['index'] as int?) ?? 0,
        children: children,
      );
    case 'Positioned':
      return Positioned(
        key: key,
        left: _optionalNumber(props, 'left'),
        top: _optionalNumber(props, 'top'),
        right: _optionalNumber(props, 'right'),
        bottom: _optionalNumber(props, 'bottom'),
        width: _optionalNumber(props, 'width'),
        height: _optionalNumber(props, 'height'),
        child: child!,
      );
    case 'PositionedDirectional':
      return PositionedDirectional(
        key: key,
        start: _optionalNumber(props, 'start'),
        top: _optionalNumber(props, 'top'),
        end: _optionalNumber(props, 'end'),
        bottom: _optionalNumber(props, 'bottom'),
        width: _optionalNumber(props, 'width'),
        height: _optionalNumber(props, 'height'),
        child: child!,
      );
    case 'Wrap':
      return Wrap(
        key: key,
        direction: _enum(Axis.values, props['direction'], Axis.horizontal),
        alignment: _enum(WrapAlignment.values, props['alignment'], WrapAlignment.start),
        spacing: _optionalNumber(props, 'spacing') ?? 0,
        runAlignment: _enum(WrapAlignment.values, props['runAlignment'], WrapAlignment.start),
        runSpacing: _optionalNumber(props, 'runSpacing') ?? 0,
        crossAxisAlignment:
            _enum(WrapCrossAlignment.values, props['crossAxisAlignment'], WrapCrossAlignment.start),
        verticalDirection: _enum(VerticalDirection.values, props['verticalDirection'], VerticalDirection.down),
        children: children,
      );
    case 'Expanded':
      return Expanded(key: key, flex: (props['flex'] as int?) ?? 1, child: child!);
    case 'Flexible':
      return Flexible(
        key: key,
        flex: (props['flex'] as int?) ?? 1,
        fit: _enum(FlexFit.values, props['fit'], FlexFit.loose),
        child: child!,
      );
    case 'Text':
      final Map<String, Object?> style = (props['style'] as Map<String, Object?>?) ?? <String, Object?>{};
      return Text(
        props['text']! as String,
        key: key,
        style: TextStyle(
          fontFamily: style['fontFamily'] as String?,
          fontSize: _optionalNumber(style, 'fontSize'),
          fontWeight: style.containsKey('fontWeight')
              ? FontWeight.values.firstWhere((FontWeight w) => w.value == style['fontWeight'])
              : null,
          height: _optionalNumber(style, 'height'),
          letterSpacing: _optionalNumber(style, 'letterSpacing'),
          leadingDistribution: style.containsKey('leadingDistribution')
              ? TextLeadingDistribution.values.byName(style['leadingDistribution']! as String)
              : null,
        ),
        strutStyle: props.containsKey('strutStyle') ? _strut(props['strutStyle']! as Map<String, Object?>) : null,
        textAlign: props.containsKey('textAlign') ? TextAlign.values.byName(props['textAlign']! as String) : null,
        maxLines: props['maxLines'] as int?,
        overflow: props.containsKey('overflow') ? TextOverflow.values.byName(props['overflow']! as String) : null,
        softWrap: props['softWrap'] as bool?,
      );
    case 'LimitedBox':
      return LimitedBox(
        key: key,
        maxWidth: _optionalNumber(props, 'maxWidth') ?? double.infinity,
        maxHeight: _optionalNumber(props, 'maxHeight') ?? double.infinity,
        child: child,
      );
    case 'FittedBox':
      return FittedBox(
        key: key,
        fit: _enum(BoxFit.values, props['fit'], BoxFit.contain),
        alignment: _alignment(props['alignment']),
        child: child,
      );
    case 'AspectRatio':
      return AspectRatio(key: key, aspectRatio: _number(props['aspectRatio']), child: child);
    case 'FractionallySizedBox':
      return FractionallySizedBox(
        key: key,
        alignment: _alignment(props['alignment']),
        widthFactor: _optionalNumber(props, 'widthFactor'),
        heightFactor: _optionalNumber(props, 'heightFactor'),
        child: child,
      );
    case 'Baseline':
      return Baseline(
        key: key,
        baseline: _number(props['baseline']),
        baselineType: _enum(TextBaseline.values, props['baselineType'], TextBaseline.alphabetic),
        child: child,
      );
    case 'IntrinsicWidth':
      return IntrinsicWidth(
        key: key,
        stepWidth: _optionalNumber(props, 'stepWidth'),
        stepHeight: _optionalNumber(props, 'stepHeight'),
        child: child,
      );
    case 'IntrinsicHeight':
      return IntrinsicHeight(key: key, child: child);
    case 'Table':
      final int columns = props['columns']! as int;
      return Table(
        key: key,
        columnWidths: <int, TableColumnWidth>{
          for (final MapEntry<String, Object?> entry
              in ((props['columnWidths'] as Map<String, Object?>?) ?? <String, Object?>{}).entries)
            int.parse(entry.key): _columnWidth(entry.value),
        },
        defaultColumnWidth: props.containsKey('defaultColumnWidth')
            ? _columnWidth(props['defaultColumnWidth'])
            : const FlexColumnWidth(),
        defaultVerticalAlignment: _enum(
          TableCellVerticalAlignment.values,
          props['defaultVerticalAlignment'],
          TableCellVerticalAlignment.top,
        ),
        textBaseline: props.containsKey('textBaseline') ? TextBaseline.values.byName(props['textBaseline']! as String) : null,
        children: <TableRow>[
          for (int row = 0; row < children.length; row += columns)
            TableRow(children: children.sublist(row, row + columns)),
        ],
      );
    case 'TableCell':
      return TableCell(
        key: key,
        verticalAlignment: props.containsKey('verticalAlignment')
            ? TableCellVerticalAlignment.values.byName(props['verticalAlignment']! as String)
            : null,
        child: child!,
      );
    case 'Transform':
      final Object? scale = props['scale'] ?? 1;
      final List<Object?> translate = (props['translate'] as List<Object?>?) ?? <Object?>[0, 0];
      return Transform(
        key: key,
        alignment: props.containsKey('alignment') ? _alignment(props['alignment']) : null,
        transform: Matrix4.translationValues(_number(translate[0]), _number(translate[1]), 0)
          ..rotateZ(_optionalNumber(props, 'rotate') ?? 0)
          ..scaleByDouble(
            scale is List<Object?> ? _number(scale[0]) : _number(scale),
            scale is List<Object?> ? _number(scale[1]) : _number(scale),
            1,
            1,
          ),
        child: child,
      );
    case 'ClipRect':
      return ClipRect(key: key, child: child);
    case 'ClipRRect':
      return ClipRRect(key: key, borderRadius: BorderRadius.circular(8), child: child);
    case 'ClipOval':
      return ClipOval(key: key, child: child);
    case 'ClipPath':
      return ClipPath(key: key, child: child);
    case 'Opacity':
      return Opacity(key: key, opacity: _optionalNumber(props, 'opacity') ?? 1, child: child);
    case 'AnimatedContainer':
      final Map<String, Object?> p = (props[phase] as Map<String, Object?>?) ?? <String, Object?>{};
      return AnimatedContainer(
        key: key,
        duration: _animationDuration,
        curve: Curves.linear,
        alignment: p.containsKey('alignment') ? _alignment(p['alignment']) : null,
        padding: p.containsKey('padding') ? _padding(p['padding']) : null,
        margin: p.containsKey('margin') ? _padding(p['margin']) : null,
        width: _optionalNumber(p, 'width'),
        height: _optionalNumber(p, 'height'),
        child: child,
      );
    case 'AnimatedPadding':
      final Map<String, Object?> p = (props[phase] as Map<String, Object?>?) ?? <String, Object?>{};
      return AnimatedPadding(
        key: key,
        duration: _animationDuration,
        curve: Curves.linear,
        padding: _padding(p['padding']),
        child: child,
      );
    case 'AnimatedAlign':
      final Map<String, Object?> p = (props[phase] as Map<String, Object?>?) ?? <String, Object?>{};
      return AnimatedAlign(
        key: key,
        duration: _animationDuration,
        curve: Curves.linear,
        alignment: _alignment(p['alignment']),
        widthFactor: _optionalNumber(p, 'widthFactor'),
        heightFactor: _optionalNumber(p, 'heightFactor'),
        child: child,
      );
    case 'AnimatedPositioned':
      final Map<String, Object?> p = (props[phase] as Map<String, Object?>?) ?? <String, Object?>{};
      return AnimatedPositioned(
        key: key,
        duration: _animationDuration,
        curve: Curves.linear,
        left: _optionalNumber(p, 'left'),
        top: _optionalNumber(p, 'top'),
        right: _optionalNumber(p, 'right'),
        bottom: _optionalNumber(p, 'bottom'),
        width: _optionalNumber(p, 'width'),
        height: _optionalNumber(p, 'height'),
        child: child!,
      );
    case 'AnimatedOpacity':
      final Map<String, Object?> p = (props[phase] as Map<String, Object?>?) ?? <String, Object?>{};
      return AnimatedOpacity(
        key: key,
        duration: _animationDuration,
        curve: Curves.linear,
        opacity: _number(p['opacity']),
        child: child,
      );
    case 'SizeTransition':
      return SizeTransition(
        key: key,
        axis: _enum(Axis.values, props['axis'], Axis.vertical),
        sizeFactor: _animation(props, t),
        alignment: props.containsKey('alignment') ? _alignment(props['alignment']) : null,
        fixedCrossAxisSizeFactor: _optionalNumber(props, 'fixedCrossAxisSizeFactor'),
        child: child,
      );
    case 'ScaleTransition':
      return ScaleTransition(key: key, scale: _animation(props, t),
        alignment: _alignment(props['alignment']) as Alignment,
        child: child,
      );
    case 'SlideTransition':
      return SlideTransition(
        key: key,
        position: Tween<Offset>(
          begin: props.containsKey('begin') ? _offset(props['begin']) : Offset.zero,
          end: props.containsKey('end') ? _offset(props['end']) : Offset.zero,
        ).animate(AlwaysStoppedAnimation<double>(t)),
        child: child,
      );
    case 'FadeTransition':
      return FadeTransition(key: key, opacity: _animation(props, t), child: child);
    case 'Spacer':
      return Spacer(key: key, flex: (props['flex'] as int?) ?? 1);
  }
  throw ArgumentError('unknown scene node type "$type" (node "$id")');
}

void _collectIds(Map<String, Object?> node, List<String> ids) {
  ids.add(node['id']! as String);
  for (final Object? child in (node['children'] as List<Object?>?) ?? <Object?>[]) {
    _collectIds(child! as Map<String, Object?>, ids);
  }
}

bool _expectsOverflow(Map<String, Object?> node) {
  final Map<String, Object?>? props = node['props'] as Map<String, Object?>?;
  if (props?['expectOverflow'] == true) {
    return true;
  }
  return ((node['children'] as List<Object?>?) ?? <Object?>[]).any(
    (Object? child) => _expectsOverflow(child! as Map<String, Object?>),
  );
}

String _goldenName(String scene, _Frame frame, _Direction direction, double scale) =>
    '$scene.${frame.name}.${direction.name}.${scale.toStringAsFixed(1)}.json';

void main() {
  final bool update = Platform.environment['UPDATE_GOLDENS'] == '1';
  final List<File> sceneFiles =
      Directory('scenes').listSync().whereType<File>().where((File file) => file.path.endsWith('.json')).toList()
        ..sort((File a, File b) => a.path.compareTo(b.path));
  final Set<String> expectedGoldens = <String>{};
  setUpAll(_loadFonts);

  for (final File sceneFile in sceneFiles) {
    final Map<String, Object?> scene = jsonDecode(sceneFile.readAsStringSync()) as Map<String, Object?>;
    final String name = scene['name']! as String;
    final Map<String, Object?> root = scene['root']! as Map<String, Object?>;
    final double? sceneT = scene.containsKey('t') ? _number(scene['t']) : null;
    final bool useMaterial = _usesMaterial(root);
    final List<String> ids = <String>[];
    _collectIds(root, ids);
    ids.sort();

    for (final _Frame frame in _frames) {
      for (final _Direction direction in _directions) {
        for (final double scale in _hasText(root) ? _textScales : _textScales.take(1)) {
          final String goldenName = _goldenName(name, frame, direction, scale);
          expectedGoldens.add(goldenName);
          testWidgets(goldenName, (WidgetTester tester) async {
            tester.view.physicalSize = Size(
              frame.width * frame.devicePixelRatio,
              frame.height * frame.devicePixelRatio,
            );
            tester.view.devicePixelRatio = frame.devicePixelRatio;
            addTearDown(tester.view.reset);

            Widget frameWidget(String phase) => MediaQuery(
              data: MediaQueryData(
                size: Size(frame.width, frame.height),
                devicePixelRatio: frame.devicePixelRatio,
                textScaler: TextScaler.linear(scale),
              ),
              child: Directionality(
                textDirection: direction.textDirection,
                child: useMaterial ? _materialScope(_build(root, phase, sceneT ?? 1), direction.textDirection) : _build(root, phase, sceneT ?? 1),
              ),
            );
            if (sceneT == null) {
              await tester.pumpWidget(frameWidget('to'));
            } else {
              // Implicit animations run from `from` to `to`; hold them `sceneT` of the way (linear, one second).
              await tester.pumpWidget(frameWidget('from'));
              await tester.pumpWidget(frameWidget('to'));
              await tester.pump();
              await tester.pump(Duration(microseconds: (sceneT * 1000000).round()));
            }
            final Object? exception = tester.takeException();
            if (exception != null && !_expectsOverflow(root)) {
              fail('scene "$name" threw during layout: $exception');
            }

            // Lazily built children exist only while they are in or near the viewport, so a scene with
            // lists records the nodes Flutter has built; every other scene must find every node.
            final bool lazy = _hasLazy(root);
            final Map<String, List<double>> rects = <String, List<double>>{
              for (final String id in ids)
                if (!lazy || find.byKey(ValueKey<String>(id), skipOffstage: false).evaluate().isNotEmpty)
                  id: _rectOf(tester, id),
            };
            final List<String> textIds = <String>[];
            _collectTextIds(root, textIds);
            textIds.sort();
            final Map<String, double> baselines = <String, double>{
              for (final String id in textIds)
                id: () {
                  final RenderParagraph paragraph = tester.renderObject<RenderParagraph>(
                    find.descendant(
                      of: find.byKey(ValueKey<String>(id), skipOffstage: false),
                      matching: find.byType(RichText),
                      skipOffstage: false,
                    ),
                  );
                  return paragraph.getDryBaseline(paragraph.constraints, TextBaseline.alphabetic)!;
                }(),
            };
            final String content =
                '${const JsonEncoder.withIndent('  ').convert(<String, Object?>{
                  'flutterRevision': flutterRevision,
                  'scene': name,
                  'frame': <String, Object?>{'width': frame.width, 'height': frame.height, 'devicePixelRatio': frame.devicePixelRatio},
                  'locale': direction.locale,
                  'textScale': scale,
                  'rects': rects,
                  if (baselines.isNotEmpty) 'baselines': baselines,
                })}\n';

            final File golden = File('goldens/$goldenName');
            if (update) {
              golden.writeAsStringSync(content);
            } else {
              expect(golden.existsSync(), isTrue, reason: '$goldenName is missing: run make studio-conformance-update');
              expect(
                golden.readAsStringSync(),
                content,
                reason: '$goldenName is stale: run make studio-conformance-update',
              );
            }
          });
        }
      }
    }
  }

  test('no orphaned goldens', () {
    final Set<String> present = Directory('goldens')
        .listSync()
        .whereType<File>()
        .map((File file) => file.uri.pathSegments.last)
        .where((String name) => name.endsWith('.json'))
        .toSet();
    if (update) {
      for (final String orphan in present.difference(expectedGoldens)) {
        File('goldens/$orphan').deleteSync();
      }
    } else {
      expect(
        present.difference(expectedGoldens),
        isEmpty,
        reason: 'goldens without a scene: run make studio-conformance-update',
      );
    }
  });
}
