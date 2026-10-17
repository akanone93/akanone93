// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/device/guard.dart';
import 'package:plux_flutter/src/render/min_target.dart';
import 'package:plux_flutter/src/render/renderer.dart';
import 'package:plux_flutter/src/render/semantics_wrap.dart';
import 'package:plux_flutter/src/render/theme.dart';
import 'package:plux_flutter/src/state/providers.dart';

import '../actions/engine_test.dart' show FakeNavigator;
import '../support/harness.dart';

fbs.Semantics sem({
  bool header = false,
  bool group = false,
  bool decorative = false,
  int headingLevel = 0,
  double? focusOrder,
}) => fbs.Semantics(
  fbs.SemanticsObjectBuilder(
    header: header,
    group: group,
    decorative: decorative,
    headingLevel: headingLevel,
    focusOrder: focusOrder ?? -1,
  ).toBytes(),
);

Widget wrap(
  Widget child, {
  fbs.Semantics? s,
  fbs.TextDirectionOverride direction = fbs.TextDirectionOverride.Inherit,
  String? testId,
  bool minTarget = false,
}) => wrapSemantics(
  child,
  sem: s,
  direction: direction,
  testId: testId,
  text: (_) => null,
  minTarget: minTarget,
);

Widget host(Widget child) => Directionality(
  textDirection: TextDirection.ltr,
  child: Center(child: child),
);

void main() {
  group('semantics wrapper', () {
    testWidgets('headingLevel marks a heading of that level [A11Y-006]', (
      tester,
    ) async {
      final handle = tester.ensureSemantics();
      await tester.pumpWidget(
        host(wrap(const Text('Title'), s: sem(headingLevel: 2))),
      );
      final node = tester.getSemantics(find.text('Title'));
      expect(node.flagsCollection.isHeader, isTrue);
      expect(node.headingLevel, 2);
      handle.dispose();
    });

    testWidgets('decorative excludes the subtree [A11Y-001]', (tester) async {
      final handle = tester.ensureSemantics();
      await tester.pumpWidget(
        host(wrap(const Text('Ornament'), s: sem(decorative: true))),
      );
      expect(find.bySemanticsLabel('Ornament'), findsNothing);
      handle.dispose();
    });

    testWidgets('group merges its descendants into one node [A11Y-006]', (
      tester,
    ) async {
      final handle = tester.ensureSemantics();
      await tester.pumpWidget(
        host(
          wrap(
            const Column(
              mainAxisSize: MainAxisSize.min,
              children: [Text('Price'), Text('9 EUR')],
            ),
            s: sem(group: true),
          ),
        ),
      );
      final node = tester.getSemantics(find.byType(Column));
      expect(node.label, contains('Price'));
      expect(node.label, contains('9 EUR'));
      handle.dispose();
    });

    testWidgets('focusOrder becomes an OrdinalSortKey [A11Y-006]', (
      tester,
    ) async {
      await tester.pumpWidget(
        host(wrap(const Text('b'), s: sem(focusOrder: 2.5))),
      );
      final w = tester.widget<Semantics>(
        find
            .ancestor(of: find.text('b'), matching: find.byType(Semantics))
            .first,
      );
      expect((w.properties.sortKey as OrdinalSortKey).order, 2.5);
    });

    testWidgets('direction overrides the inherited one; inherit does not '
        '[I18N-004]', (tester) async {
      late TextDirection inside;
      Widget probe() => Builder(
        builder: (c) {
          inside = Directionality.of(c);
          return const SizedBox();
        },
      );
      await tester.pumpWidget(
        host(wrap(probe(), direction: fbs.TextDirectionOverride.Rtl)),
      );
      expect(inside, TextDirection.rtl);
      await tester.pumpWidget(
        host(wrap(probe(), direction: fbs.TextDirectionOverride.Inherit)),
      );
      expect(inside, TextDirection.ltr);
    });

    testWidgets('a minTarget widget is at least 44 x 44 [A11Y-004]', (
      tester,
    ) async {
      await tester.pumpWidget(
        host(wrap(const SizedBox(width: 10, height: 10), minTarget: true)),
      );
      final size = tester.getSize(find.byType(ConstrainedBox).first);
      expect(size.width, greaterThanOrEqualTo(44));
      expect(size.height, greaterThanOrEqualTo(44));
    });

    testWidgets('a sliver with semantics and a testId stays in its viewport '
        '[A11Y-001]', (tester) async {
      final handle = tester.ensureSemantics();
      await tester.pumpWidget(
        host(
          SizedBox(
            height: 300,
            child: CustomScrollView(
              slivers: [
                wrapSliverSemantics(
                  const SliverToBoxAdapter(child: Text('row')),
                  sem: sem(header: true),
                  testId: 'list',
                  text: (_) => null,
                ),
                wrapSliverSemantics(
                  SliverList.list(children: const [Text('item')]),
                  sem: sem(group: true),
                  testId: 'items',
                  text: (_) => null,
                ),
              ],
            ),
          ),
        ),
      );
      expect(tester.takeException(), isNull);
      expect(find.byType(SliverSemantics), findsNWidgets(2));
      expect(find.text('item'), findsOneWidget);
      handle.dispose();
    });
  });

  group('settings (A11Y-007)', () {
    test('deviceRoot carries the accessibility settings', () {
      final d = deviceRoot(
        width: 400,
        locale: const Locale('en'),
        textScale: 1,
        dark: false,
        reduceMotion: true,
        boldText: true,
        highContrast: true,
        screenReader: true,
        textDirection: TextDirection.rtl,
      );
      expect(d['reduceMotion'], true);
      expect(d['boldText'], true);
      expect(d['highContrast'], true);
      expect(d['screenReader'], true);
      expect(d['textDirection'], 'rtl');
      final off = deviceRoot(
        width: 400,
        locale: const Locale('en'),
        textScale: 1,
        dark: false,
      );
      expect(
        [off['reduceMotion'], off['boldText'], off['highContrast']],
        [false, false, false],
      );
      expect(off['textDirection'], 'ltr');
    });

    test('boldText bolds every style of the text theme', () {
      final bold = boldTextTheme(ThemeData().textTheme);
      for (final s in [bold.bodyMedium, bold.titleLarge, bold.labelSmall]) {
        expect(s!.fontWeight!.value, greaterThanOrEqualTo(700));
      }
    });

    test('Material themes pad tap targets', () {
      final t = PluxTheme.resolve(
        host: ThemeData(),
        source: PluxThemeSource.plux,
        dark: false,
        tokens: (token: (_) => null, raw: (_) => null, paths: (_) => const []),
        boldText: true,
      );
      expect(t.material.materialTapTargetSize, MaterialTapTargetSize.padded);
      expect(t.material.textTheme.bodyMedium!.fontWeight, FontWeight.bold);
    });

    test('minTarget types match the widget descriptors in schema/widgets', () {
      final marked = <String>{};
      for (final f in Directory(
        '../../schema/widgets',
      ).listSync(recursive: true)) {
        if (f is! File || !f.path.endsWith('.json')) continue;
        final doc = jsonDecode(f.readAsStringSync());
        final a11y = doc is Map ? doc['accessibility'] : null;
        if (doc is Map && a11y is Map && a11y['minTarget'] == true) {
          marked.add(doc['type'] as String);
        }
      }
      expect(minTargetTypes, marked);
      expect(minTargetWidgets, isNotEmpty);
    });
  });

  group('actions', () {
    late Harness h;
    setUp(() async {
      h = await Harness.create();
    });
    tearDown(() => h.close());

    StepContext ctx(BuildContext? context) => StepContext(
      navigator: FakeNavigator(),
      emit: (_, _) {},
      nativeActions: const NoNativeActions(),
      device: DeviceScope(
        plugin: '',
        guard: DeviceGuard(
          capabilities: (_) => (deviceApis: const [], networkDomains: const []),
          deepLinks: () => null,
          report: (_) {},
        ),
        context: () => context,
      ),
    );

    testWidgets('announce sends a polite or assertive announcement '
        '[A11Y-006]', (tester) async {
      final sent = <Map<Object?, Object?>>[];
      tester.binding.defaultBinaryMessenger
          .setMockDecodedMessageHandler<Object?>(SystemChannels.accessibility, (
            m,
          ) async {
            sent.add(m! as Map<Object?, Object?>);
            return null;
          });
      addTearDown(
        () => tester.binding.defaultBinaryMessenger
            .setMockDecodedMessageHandler<Object?>(
              SystemChannels.accessibility,
              null,
            ),
      );
      late BuildContext context;
      await tester.pumpWidget(
        host(
          Builder(
            builder: (c) {
              context = c;
              return const SizedBox();
            },
          ),
        ),
      );
      final handler = builtInHandlers['announce']!;
      await tester.runAsync(() async {
        await handler.run(ctx(context), {'message': 'Saved'});
        await handler.run(ctx(context), {
          'message': 'Error',
          'assertiveness': 'assertive',
        });
      });
      final data = [for (final m in sent) m['data']! as Map<Object?, Object?>];
      expect(data.map((d) => d['message']), ['Saved', 'Error']);
      expect(data[0]['assertiveness'], isNot(data[1]['assertiveness']));
    });

    testWidgets('announce without a view fails typed [A11Y-006]', (
      tester,
    ) async {
      expect(
        () => builtInHandlers['announce']!.run(ctx(null), {'message': 'x'}),
        throwsA(isA<ActionError>()),
      );
    });

    testWidgets('setThemeMode sets the environment path of '
        'Plux.setThemeMode [THM-002]', (tester) async {
      await tester.runAsync(
        () => h.startFrom(Harness.goldens.bundles['widgets/widgets.pxb']!, {
          'gallery': Harness.goldens.bundles['widgets/gallery.pxb']!,
        }),
      );
      final handler = builtInHandlers['setThemeMode']!;
      handler.run(ctx(null), {'mode': 'dark'});
      expect(
        Plux.container.read(environmentProvider).themeMode,
        ThemeMode.dark,
      );
      handler.run(ctx(null), {'mode': 'light'});
      expect(
        Plux.container.read(environmentProvider).themeMode,
        ThemeMode.light,
      );
      expect(
        () => handler.run(ctx(null), {'mode': 'sepia'}),
        throwsA(isA<ActionError>()),
      );
    });
  });
}
