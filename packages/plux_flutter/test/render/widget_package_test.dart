// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/schema/registry.g.dart';

import '../support/harness.dart';

WidgetDescriptor _widget(String type) =>
    widgetDescriptors.firstWhere((d) => d.type == type);

/// The gallery bundle with its `layout` page replaced by [nodes].
Uint8List _galleryWith(List<fbs.NodeObjectBuilder> nodes) {
  final c = BundleContainer.parse(
    Harness.goldens.bundles['widgets/gallery.pxb']!,
  );
  final sections = <Section>[];
  for (final s in c.sections) {
    if (s.kind == SectionKind.page) {
      final page = fbs.Page(s.data);
      if (page.strings![page.route] == 'layout') {
        final data = fbs.PageObjectBuilder(
          id: fbs.UuidObjectBuilder(hi: page.id!.hi, lo: page.id!.lo),
          key: 1,
          route: 1,
          nodes: nodes,
          strings: const ['', 'layout'],
        ).toBytes('PXPG');
        sections.add(Section(s.kind, s.id, s.hash, data));
        continue;
      }
    }
    sections.add(s);
  }
  return encodeBundle(BundleKinds.plugin, sections);
}

final class _Package implements PluxWidgetPackage {
  _Package(this.builders);

  @override
  final String name = 'plux_charts';

  @override
  final Map<int, NodeBuilder> builders;
}

/// Widget packages (RT-060, ADR-0071): a registered package's builder
/// renders its widget; without it the node falls back and PLX-4403 is
/// reported once per type.
void main() {
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  final column = _widget('Column');
  final line = _widget('LineChart');
  final bar = _widget('BarChart');

  Future<void> show(
    WidgetTester tester,
    List<PluxWidgetPackage> packages,
  ) async {
    await tester.runAsync(
      () => h.startFrom(Harness.goldens.bundles['widgets/widgets.pxb']!, {
        'gallery': _galleryWith([
          fbs.NodeObjectBuilder(widget: column.id, children: [1, 2, 3]),
          fbs.NodeObjectBuilder(widget: line.id),
          fbs.NodeObjectBuilder(widget: line.id),
          fbs.NodeObjectBuilder(widget: bar.id),
        ]),
      }, widgetPackages: packages),
    );
    await tester.pumpWidget(
      const MaterialApp(home: PluxScope(child: PluxView('layout'))),
    );
    await settle(tester);
  }

  List<PluxException> missing() => [
    for (final e in h.errors)
      if (e.code == PluxErrorCode.widgetPackageMissing) e,
  ];

  test('the descriptors name their package [RT-060]', () {
    expect(line.package, 'plux_charts');
    expect(_widget('Text').package, isNull);
  });

  testWidgets(
    'a missing package reports PLX-4403 once per widget type [RT-060]',
    (tester) async {
      await show(tester, const []);
      expect(missing(), hasLength(2));
      expect(find.textContaining('plux_charts missing'), findsWidgets);
    },
  );

  testWidgets('a registered package renders its builders [RT-060]', (
    tester,
  ) async {
    await show(tester, [
      _Package({
        line.id: (c) => const Text('line', textDirection: TextDirection.ltr),
        bar.id: (c) => const Text('bar', textDirection: TextDirection.ltr),
      }),
    ]);
    expect(find.text('line'), findsNWidgets(2));
    expect(find.text('bar'), findsOneWidget);
    expect(missing(), isEmpty);
  });
}
