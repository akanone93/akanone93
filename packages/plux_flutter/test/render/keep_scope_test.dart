// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/render/keep_scope.dart';
import 'package:plux_flutter/src/render/scope.dart';
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

/// Overlays keep the page scope (X1.7, RT-060): what an overlay shows
/// through `keepScope` reads the page's render scope and providers, as the
/// bound values of menus, suggestions, options, previews, drag feedback and
/// reorder proxies do.
void main() {
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  final column = _widget('Column');
  final line = _widget('LineChart');
  final bar = _widget('BarChart');

  // The probe sits where a bound value would: it finds the scope and the
  // providers, or the build fails.
  final probed = <String>[];
  Widget probe(String name) => Builder(
    builder: (context) {
      RenderScope.of(context);
      ProviderScope.containerOf(context, listen: false);
      probed.add(name);
      return Text(name);
    },
  );

  Future<void> show(
    WidgetTester tester,
    Widget Function(NodeContext c) make,
  ) async {
    probed.clear();
    await tester.runAsync(
      () => h.startFrom(
        Harness.goldens.bundles['widgets/widgets.pxb']!,
        {
          'gallery': _galleryWith([
            fbs.NodeObjectBuilder(widget: column.id, children: [1, 2]),
            fbs.NodeObjectBuilder(widget: line.id),
            fbs.NodeObjectBuilder(widget: bar.id),
          ]),
        },
        widgetPackages: [
          _Package({line.id: make, bar.id: (c) => const SizedBox()}),
        ],
      ),
    );
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: PluxScope(child: PluxView('layout'))),
      ),
    );
    await settle(tester);
  }

  testWidgets('a popup menu item keeps the scope [RT-060]', (tester) async {
    await show(
      tester,
      (c) => PopupMenuButton<String>(
        itemBuilder: (_) => [
          PopupMenuItem(value: 'a', child: keepScope(c.context, probe('menu'))),
        ],
      ),
    );
    await tester.tap(find.byType(PopupMenuButton<String>));
    await settle(tester);
    expect(find.text('menu'), findsOneWidget);
    expect(probed, contains('menu'));
  });

  testWidgets('a search suggestion keeps the scope [RT-060]', (tester) async {
    await show(
      tester,
      (c) => SearchAnchor(
        builder: (context, controller) => IconButton(
          icon: const Icon(Icons.search),
          onPressed: controller.openView,
        ),
        suggestionsBuilder: (context, controller) => [
          keepScope(c.context, probe('suggestion')),
        ],
      ),
    );
    await tester.tap(find.byIcon(Icons.search));
    await settle(tester);
    expect(find.text('suggestion'), findsOneWidget);
  });

  testWidgets('an autocomplete option list keeps the scope [RT-060]', (
    tester,
  ) async {
    await show(
      tester,
      (c) => Autocomplete<String>(
        optionsBuilder: (v) => const ['one'],
        optionsViewBuilder: (context, onSelected, options) =>
            keepScope(c.context, probe('options')),
      ),
    );
    await tester.enterText(find.byType(EditableText), 'o');
    await settle(tester);
    expect(find.text('options'), findsOneWidget);
  });

  testWidgets('a context menu action keeps the scope [RT-060]', (tester) async {
    await show(
      tester,
      (c) => CupertinoContextMenu(
        actions: [
          CupertinoContextMenuAction(
            child: keepScope(c.context, probe('action')),
          ),
        ],
        child: const SizedBox(width: 80, height: 80, child: Text('hold')),
      ),
    );
    final g = await tester.startGesture(tester.getCenter(find.text('hold')));
    for (var i = 0; i < 20; i++) {
      await tester.pump(const Duration(milliseconds: 100));
    }
    await g.up();
    for (var i = 0; i < 10; i++) {
      await tester.pump(const Duration(milliseconds: 100));
    }
    expect(probed, contains('action'));
  });

  testWidgets('drag feedback keeps the scope [RT-060]', (tester) async {
    await show(
      tester,
      (c) => Draggable<Object>(
        data: 1,
        feedback: keepScope(c.context, probe('feedback')),
        child: const SizedBox(width: 80, height: 80, child: Text('drag')),
      ),
    );
    final g = await tester.startGesture(tester.getCenter(find.text('drag')));
    await tester.pump(const Duration(milliseconds: 100));
    await g.moveBy(const Offset(40, 40));
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('feedback'), findsOneWidget);
    await g.up();
  });

  testWidgets('a reorder proxy keeps the scope [RT-060]', (tester) async {
    await show(
      tester,
      (c) => SizedBox(
        height: 200,
        child: ReorderableListView(
          proxyDecorator: (child, _, _) => keepScope(c.context, probe('proxy')),
          onReorder: (_, _) {},
          children: [
            for (final i in [0, 1])
              ListTile(key: ValueKey(i), title: Text('item $i')),
          ],
        ),
      ),
    );
    final g = await tester.startGesture(tester.getCenter(find.text('item 0')));
    await tester.pump(const Duration(milliseconds: 800));
    await g.moveBy(const Offset(0, 30));
    await tester.pump(const Duration(milliseconds: 100));
    expect(probed, contains('proxy'));
    await g.up();
  });
}
