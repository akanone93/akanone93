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

Widget _build(Map<String, Object?> node) {
  final String id = node['id']! as String;
  final String type = node['type']! as String;
  final Map<String, Object?> props = (node['props'] as Map<String, Object?>?) ?? <String, Object?>{};
  final List<Widget> children = <Widget>[
    for (final Object? child in (node['children'] as List<Object?>?) ?? <Object?>[])
      _build(child! as Map<String, Object?>),
  ];
  const Set<String> multiChild = <String>{
    'Row', 'Column', 'Flex', 'Stack', 'IndexedStack', 'Wrap', //
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

            await tester.pumpWidget(
              MediaQuery(
                data: MediaQueryData(
                  size: Size(frame.width, frame.height),
                  devicePixelRatio: frame.devicePixelRatio,
                  textScaler: TextScaler.linear(scale),
                ),
                child: Directionality(textDirection: direction.textDirection, child: _build(root)),
              ),
            );
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
