// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_media/plux_media.dart';

final class FakeNode implements NodeContext {
  FakeNode(this.context, this.props, {this.fired});

  @override
  final BuildContext context;
  final Map<int, Object?> props;
  final List<Object?>? fired;

  @override
  T? decode<T>(int id, T? Function(Decoding d, Object? v) decoder) {
    final v = props[id];
    return v == null ? null : decoder(this, v);
  }

  @override
  bool handles(int id) => fired != null;

  @override
  void fire(int id, [Object? payload]) => fired!.add(payload);

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Widget _app(Widget child) => MaterialApp(
  home: Scaffold(
    body: Center(child: SizedBox(width: 300, child: child)),
  ),
);

Future<void> _stroke(WidgetTester tester, Offset from, Offset to) async {
  final g = await tester.startGesture(from);
  await g.moveTo(to);
  await g.up();
}

void main() {
  test('the package ID is that of the descriptor [WGT-020] [RT-060]', () {
    final d = jsonDecode(
      File('../../schema/widgets/layer2/SignaturePad.json').readAsStringSync(),
    ) as Map<String, Object?>;
    expect(d['id'], PluxMediaWidgetIds.signaturePad);
    expect(d['package'], const PluxMediaWidgets().name);
    expect(const PluxMediaWidgets().builders.keys, [202]);
  });

  test('path data has one M per stroke and a dot for a single point '
      '[WGT-020]', () {
    expect(
      signaturePathData([
        [const Offset(1, 2), const Offset(3.25, 4)],
        [const Offset(10, 10)],
      ]),
      'M 1 2 L 3.3 4 M 10 10 L 10 10',
    );
    expect(signaturePathData(const []), '');
  });

  testWidgets('a stroke reports SVG path data; clear reports an empty '
      'string [WGT-020]', (tester) async {
    final got = <String>[];
    await tester.pumpWidget(_app(PluxSignaturePad(onChanged: got.add)));
    final origin = tester.getTopLeft(find.byType(ClipRRect));
    expect(
      tester.widget<TextButton>(find.byType(TextButton)).onPressed,
      isNull,
    );
    await _stroke(
      tester,
      origin + const Offset(10, 10),
      origin + const Offset(50, 60),
    );
    await tester.pump();
    expect(got, hasLength(1));
    expect(got.single, startsWith('M 10 10 L '));
    await tester.tap(find.text('Clear'));
    await tester.pump();
    expect(got.last, '');
  });

  testWidgets('png format reports a base64 PNG [WGT-020]', (tester) async {
    final got = <String>[];
    await tester.pumpWidget(
      _app(PluxSignaturePad(format: SignatureFormat.png, onChanged: got.add)),
    );
    final origin = tester.getTopLeft(find.byType(ClipRRect));
    await _stroke(
      tester,
      origin + const Offset(10, 10),
      origin + const Offset(50, 60),
    );
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 200)),
    );
    await tester.pump();
    expect(got, hasLength(1));
    final bytes = base64.decode(got.single);
    // The PNG signature.
    expect(bytes.sublist(0, 4), [0x89, 0x50, 0x4e, 0x47]);
  });

  testWidgets('the pad and the clear button are reachable by screen readers '
      '[A11Y-001]', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(
      _app(const PluxSignaturePad(semanticLabel: 'Sign here')),
    );
    expect(find.bySemanticsLabel('Sign here'), findsOneWidget);
    expect(find.bySemanticsLabel('Clear'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('the builder reads props and fires onChanged [WGT-020]', (
    tester,
  ) async {
    final fired = <Object?>[];
    late Widget pad;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          pad = const PluxMediaWidgets().builders[202]!(
            FakeNode(context, {1: 'png', 2: 100.0, 4: 'Erase'}, fired: fired),
          );
          return const SizedBox();
        },
      ),
    );
    final p = pad as PluxSignaturePad;
    expect(p.format, SignatureFormat.png);
    expect(p.strokeWidth, 12);
    expect(p.clearLabel, 'Erase');
    p.onChanged!('x');
    expect(fired, ['x']);
  });
}
