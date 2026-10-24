// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Layout conformance goldens (STU-005, ADR-0013 "Conformance suite and
// tolerance"): Flutter itself lays out every scene in `scenes/` under every
// configuration and records the global rect of every identified node. With
// UPDATE_GOLDENS=1 the goldens are written; otherwise the test fails when a
// golden is missing, stale or orphaned.

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
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

const List<_Frame> _frames = <_Frame>[
  _Frame('phone', 360, 800, 3),
  _Frame('tablet', 800, 1280, 2),
];

const List<_Direction> _directions = <_Direction>[
  _Direction('ltr', TextDirection.ltr, 'en'),
  _Direction('rtl', TextDirection.rtl, 'ar'),
];

/// Scenes have no text yet, so only 1.0 is captured; 2.0 joins with text.
const List<double> _textScales = <double>[1.0];

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
    return EdgeInsets.fromLTRB(
      _number(value[0]),
      _number(value[1]),
      _number(value[2]),
      _number(value[3]),
    );
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

Widget _build(Map<String, Object?> node) {
  final String id = node['id']! as String;
  final String type = node['type']! as String;
  final Map<String, Object?> props =
      (node['props'] as Map<String, Object?>?) ?? <String, Object?>{};
  final List<Widget> children = <Widget>[
    for (final Object? child in (node['children'] as List<Object?>?) ?? <Object?>[])
      _build(child! as Map<String, Object?>),
  ];
  const Set<String> multiChild = <String>{'Row', 'Column', 'Flex'};
  if (!multiChild.contains(type) && children.length > 1) {
    throw ArgumentError('node "$id" of type $type takes at most one child');
  }
  final Widget? child = children.length == 1 ? children.single : null;
  final Key key = ValueKey<String>(id);
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
        crossAxisAlignment:
            _enum(CrossAxisAlignment.values, props['crossAxisAlignment'], CrossAxisAlignment.center),
        verticalDirection: _enum(VerticalDirection.values, props['verticalDirection'], VerticalDirection.down),
        spacing: _optionalNumber(props, 'spacing') ?? 0,
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
  return ((node['children'] as List<Object?>?) ?? <Object?>[])
      .any((Object? child) => _expectsOverflow(child! as Map<String, Object?>));
}

String _goldenName(String scene, _Frame frame, _Direction direction, double scale) =>
    '$scene.${frame.name}.${direction.name}.${scale.toStringAsFixed(1)}.json';

void main() {
  final bool update = Platform.environment['UPDATE_GOLDENS'] == '1';
  final List<File> sceneFiles = Directory('scenes')
      .listSync()
      .whereType<File>()
      .where((File file) => file.path.endsWith('.json'))
      .toList()
    ..sort((File a, File b) => a.path.compareTo(b.path));
  final Set<String> expectedGoldens = <String>{};

  for (final File sceneFile in sceneFiles) {
    final Map<String, Object?> scene = jsonDecode(sceneFile.readAsStringSync()) as Map<String, Object?>;
    final String name = scene['name']! as String;
    final Map<String, Object?> root = scene['root']! as Map<String, Object?>;
    final List<String> ids = <String>[];
    _collectIds(root, ids);
    ids.sort();

    for (final _Frame frame in _frames) {
      for (final _Direction direction in _directions) {
        for (final double scale in _textScales) {
          final String goldenName = _goldenName(name, frame, direction, scale);
          expectedGoldens.add(goldenName);
          testWidgets(goldenName, (WidgetTester tester) async {
            tester.view.physicalSize = Size(frame.width * frame.devicePixelRatio, frame.height * frame.devicePixelRatio);
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

            final Map<String, List<double>> rects = <String, List<double>>{
              for (final String id in ids)
                id: () {
                  final Rect rect = tester.getRect(find.byKey(ValueKey<String>(id)));
                  return <double>[rect.left, rect.top, rect.width, rect.height];
                }(),
            };
            final String content = '${const JsonEncoder.withIndent('  ').convert(<String, Object?>{
              'flutterRevision': flutterRevision,
              'scene': name,
              'frame': <String, Object?>{
                'width': frame.width,
                'height': frame.height,
                'devicePixelRatio': frame.devicePixelRatio,
              },
              'locale': direction.locale,
              'textScale': scale,
              'rects': rects,
            })}\n';

            final File golden = File('goldens/$goldenName');
            if (update) {
              golden.writeAsStringSync(content);
            } else {
              expect(golden.existsSync(), isTrue, reason: '$goldenName is missing: run make studio-conformance-update');
              expect(golden.readAsStringSync(), content, reason: '$goldenName is stale: run make studio-conformance-update');
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
      expect(present.difference(expectedGoldens), isEmpty, reason: 'goldens without a scene: run make studio-conformance-update');
    }
  });
}
