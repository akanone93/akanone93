// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/render/builders/cupertino_more.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/node_context.dart';

/// A DataScope node: props by permanent ID and the sources it can read.
final class _Node implements NodeContext {
  _Node(this.props, this.sources);

  final Map<int, Object?> props;
  final Map<String, Map<String, Object?>> sources;

  @override
  Object? prop(int id) => props[id];

  @override
  T? decode<T>(int id, T? Function(Decoding d, Object? v) decoder) {
    final v = props[id];
    return v == null ? null : decoder(this, v);
  }

  @override
  Map<String, Object?>? dataSource(String name) => sources[name];

  @override
  Widget? slot(int id) => switch (id) {
    1 => const Text('success'),
    2 => const Text('loading'),
    3 => const Text('empty'),
    4 => const Text('error'),
    _ => null,
  };

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Map<String, Object?> _src(String status, [Object? value]) => {
  'status': status,
  'value': value,
};

void main() {
  final build = cupertinoMoreBuilders[109]!;

  Future<void> shows(
    WidgetTester tester,
    Map<int, Object?> props,
    Map<String, Object?> source,
    String slot,
  ) async {
    await tester.pumpWidget(
      Directionality(
        textDirection: TextDirection.ltr,
        child: build(_Node(props, {'s': source})),
      ),
    );
    expect(find.text(slot), findsOneWidget);
  }

  // Verifies: WGT-020.
  testWidgets('a loading source shows the loading slot', (tester) async {
    await shows(tester, {3: 's'}, _src('loading'), 'loading');
  });

  testWidgets('a failed source shows the error slot', (tester) async {
    await shows(tester, {3: 's'}, _src('error'), 'error');
  });

  testWidgets('a null, empty list or empty map value shows the empty slot', (
    tester,
  ) async {
    await shows(tester, {3: 's'}, _src('ready'), 'empty');
    await shows(tester, {3: 's'}, _src('ready', <Object?>[]), 'empty');
    await shows(tester, {3: 's'}, _src('ready', <String, Object?>{}), 'empty');
  });

  testWidgets('a source with a value shows the success slot', (tester) async {
    await shows(tester, {3: 's'}, _src('ready', [1]), 'success');
  });

  testWidgets('an explicit status or isEmpty wins over the source', (
    tester,
  ) async {
    await shows(tester, {3: 's', 1: 'ready'}, _src('loading'), 'empty');
    await shows(tester, {3: 's', 1: 'loading'}, _src('ready', [1]), 'loading');
    await shows(tester, {3: 's', 2: false}, _src('ready'), 'success');
  });
}
