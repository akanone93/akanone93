// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_rive/plux_rive.dart';

final class FakeNode implements NodeContext {
  FakeNode(this.context, this.props, {this.fired});

  @override
  final BuildContext context;
  final Map<int, Object?> props;
  final List<int>? fired;

  @override
  Object? prop(int id) => props[id];

  @override
  T? decode<T>(int id, T? Function(Decoding d, Object? v) decoder) {
    final v = props[id];
    return v == null ? null : decoder(this, v);
  }

  @override
  Never missing(String name) => throw DecodeError('$name is missing');

  @override
  bool handles(int id) => fired != null;

  @override
  void fire(int id, [Object? payload]) => fired!.add(id);

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

void main() {
  Future<Widget> build(
    WidgetTester tester,
    Map<int, Object?> props, {
    List<int>? fired,
  }) async {
    late Widget built;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          built = const PluxRiveWidgets().builders[PluxRiveIds.riveView]!(
            FakeNode(context, props, fired: fired),
          );
          return const SizedBox();
        },
      ),
    );
    return built;
  }

  test('the package IDs are those of the descriptor [WGT-020] [RT-060]', () {
    final d = jsonDecode(
      File('../../schema/widgets/layer2/RiveView.json').readAsStringSync(),
    ) as Map<String, Object?>;
    expect(d['id'], PluxRiveIds.riveView);
    expect(d['package'], const PluxRiveWidgets().name);
    final props = (d['props']! as List<Object?>).cast<Map<String, Object?>>();
    expect(
      {for (final p in props) p['id']! as int: p['name']},
      {
        1: 'url',
        2: 'artboard',
        3: 'stateMachine',
        4: 'inputs',
        5: 'flags',
        6: 'triggers',
        7: 'fit',
        8: 'reduceMotion',
        9: 'semanticLabel',
      },
    );
  });

  testWidgets('only https addresses play [WGT-020] [SEC-054]', (tester) async {
    final built = await build(tester, {1: 'http://example.com/a.riv'});
    expect(built, isA<SizedBox>());
  });

  testWidgets('numbers, flags and triggers reach the state machine inputs '
      '[WGT-020] [A11Y-001]', (tester) async {
    final fired = <int>[];
    final built = await build(tester, {
      1: 'https://example.com/a.riv',
      2: 'Main',
      3: 'machine',
      4: {'level': 2.0},
      5: {'on': true},
      6: {'go': 1.0},
      8: 'ignore',
      9: 'A rocket',
    }, fired: fired);
    expect(built, isA<Semantics>());
    final view = (built as Semantics).child! as PluxRiveView;
    expect(view.artboard, 'Main');
    expect(view.stateMachine, 'machine');
    expect(view.inputs, {'level': 2.0, 'on': true});
    expect(view.triggers, {'go': 1.0});
    expect(view.ignoreReduceMotion, isTrue);
    expect(view.cacheKey, 'https://example.com/a.riv');
    view.onLoaded!();
    view.onFailed!();
    expect(fired, [1, 2]);
  });

  testWidgets('without a label the animation is hidden from screen readers '
      '[A11Y-001]', (tester) async {
    final built = await build(tester, {1: 'https://example.com/a.riv'});
    expect(built, isA<ExcludeSemantics>());
    final view = (built as ExcludeSemantics).child! as PluxRiveView;
    expect(view.inputs, isEmpty);
    expect(view.ignoreReduceMotion, isFalse);
  });
}
