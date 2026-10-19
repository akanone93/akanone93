// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_lottie/plux_lottie.dart';

final class FakeNode implements NodeContext {
  FakeNode(this.context, this.props, {this.fired, this.asked});

  @override
  final BuildContext context;
  final Map<int, Object?> props;
  final List<int>? fired;
  final List<String>? asked;

  @override
  Future<Uint8List?> remoteBytes(String url) async {
    asked?.add(url);
    return Uri.parse(url).host == 'example.com'
        ? Uint8List.fromList(utf8.encode('{}'))
        : null;
  }

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
    List<String>? asked,
  }) async {
    late Widget built;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          built = const PluxLottieWidgets().builders[PluxLottieIds.lottieView]!(
            FakeNode(context, props, fired: fired, asked: asked),
          );
          return const SizedBox();
        },
      ),
    );
    return built;
  }

  test('the package IDs are those of the descriptor [WGT-020] [RT-060]', () {
    final d = jsonDecode(
      File('../../schema/widgets/layer2/LottieView.json').readAsStringSync(),
    ) as Map<String, Object?>;
    expect(d['id'], PluxLottieIds.lottieView);
    expect(d['package'], const PluxLottieWidgets().name);
    final props = (d['props']! as List<Object?>).cast<Map<String, Object?>>();
    expect(
      {for (final p in props) p['id']! as int: p['name']},
      {
        1: 'url',
        2: 'animate',
        3: 'repeat',
        4: 'reverse',
        5: 'progress',
        6: 'fit',
        7: 'width',
        8: 'height',
        9: 'reduceMotion',
        10: 'semanticLabel',
      },
    );
  });

  Future<PluxLottieView?> shown(
    WidgetTester tester,
    Map<int, Object?> props, {
    List<int>? fired,
    List<String>? asked,
  }) async {
    final built = await build(tester, props, fired: fired, asked: asked);
    await tester.pumpWidget(
      Directionality(textDirection: TextDirection.ltr, child: built),
    );
    await tester.runAsync(() => Future<void>.delayed(Duration.zero));
    await tester.pump();
    final found = find.byType(PluxLottieView);
    return found.evaluate().isEmpty
        ? null
        : tester.widget<PluxLottieView>(found);
  }

  testWidgets('a URL outside the declared domains loads nothing '
      '[WGT-020] [SEC-080] [SEC-041] [AST-002]', (tester) async {
    final asked = <String>[];
    expect(
      await shown(tester, {1: 'https://evil.example.net/a.json'}, asked: asked),
      isNull,
    );
    expect(asked, ['https://evil.example.net/a.json']);
  });

  testWidgets('props reach the view and the label makes it an image '
      '[WGT-020] [A11Y-001]', (tester) async {
    final fired = <int>[];
    final view = (await shown(tester, {
      1: 'https://example.com/a.json',
      2: false,
      3: false,
      5: 0.5,
      6: 'cover',
      9: 'shorten',
      10: 'Loading spinner',
    }, fired: fired))!;
    expect(find.bySemanticsLabel('Loading spinner'), findsOneWidget);
    expect(view.animate, isFalse);
    expect(view.repeat, isFalse);
    expect(view.progress, 0.5);
    expect(view.fit, BoxFit.cover);
    expect(view.reduceMotion, PluxReduceMotion.shorten);
    view.onCompleted!();
    expect(fired, [1]);
  });

  testWidgets('without a label the animation is hidden from screen readers '
      '[A11Y-001]', (tester) async {
    final view = (await shown(tester, {1: 'https://example.com/a.json'}))!;
    expect(
      find.ancestor(
        of: find.byType(PluxLottieView),
        matching: find.byType(ExcludeSemantics),
      ),
      findsWidgets,
    );
    expect(view.animate, isTrue);
    expect(view.repeat, isTrue);
    expect(view.reduceMotion, PluxReduceMotion.skip);
    expect(view.onCompleted, isNull);
  });
}
