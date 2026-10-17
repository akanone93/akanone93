// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';

import '../support/harness.dart';

/// The interaction and animation widgets (ADR-0071, WGT-020, ANI-001,
/// ANI-002, ANI-004, ANI-007): the gallery page `animation`, compiled by the
/// Go compiler and rendered here.
void main() {
  final g = Harness.goldens;
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<void> open(
    WidgetTester tester, {
    bool dark = false,
    bool rtl = false,
    double scale = 1.0,
    bool reduceMotion = false,
  }) async {
    tester.view.physicalSize = const Size(800, 2600);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    if (reduceMotion) {
      tester.platformDispatcher.accessibilityFeaturesTestValue =
          const FakeAccessibilityFeatures(disableAnimations: true);
      addTearDown(
        tester.platformDispatcher.clearAccessibilityFeaturesTestValue,
      );
    }
    await tester.runAsync(() async {
      await h.startFrom(g.bundles['widgets/widgets.pxb']!, {
        'gallery': g.bundles['widgets/gallery.pxb']!,
      }, themeSource: PluxThemeSource.plux);
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        builder: (context, child) => Directionality(
          textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
          child: MediaQuery(
            data: MediaQuery.of(context)
                .copyWith(textScaler: TextScaler.linear(scale)),
            child: child!,
          ),
        ),
        home: PluxScope(child: PluxView('animation')),
      ),
    );
    await settle(tester);
    if (dark) {
      Plux.setThemeMode(ThemeMode.dark);
      await settle(tester);
      await tester.pump(const Duration(milliseconds: 300));
    }
  }

  Finder over(String text, Type type) =>
      find.ancestor(of: find.text(text), matching: find.byType(type));

  Future<void> toggle(WidgetTester tester) async {
    await tester.tap(find.text('Toggle'));
    await settle(tester);
  }

  double opacityOf(WidgetTester tester, Finder of) => tester
      .widget<FadeTransition>(
        find.descendant(of: of, matching: find.byType(FadeTransition)),
      )
      .opacity
      .value;

  testWidgets('every widget of the group renders without a problem '
      '[WGT-020] [WGT-003]', (tester) async {
    await open(tester);
    for (final (text, type) in [
      ('container', AnimatedContainer),
      ('Opacity', AnimatedOpacity),
      ('Align', AnimatedAlign),
      ('Padding', AnimatedPadding),
      ('Scale', AnimatedScale),
      ('Rotation', AnimatedRotation),
      ('Slide', AnimatedSlide),
      ('switch off', AnimatedSwitcher),
      ('second', AnimatedCrossFade),
      ('size', AnimatedSize),
      ('Pinch and pan', InteractiveViewer),
      ('Swipe me', Dismissible),
    ]) {
      expect(over(text, type), findsOneWidget, reason: '$type');
    }
    expect(find.byType(AnimatedPositioned), findsOneWidget);
    expect(find.byType(Draggable<Object>), findsOneWidget);
    expect(find.byType(DragTarget<Object>), findsOneWidget);
    for (final (text, type) in [
      ('t-fade', FadeTransition),
      ('t-slide', SlideTransition),
      ('t-scale', ScaleTransition),
      ('t-rotation', RotationTransition),
    ]) {
      expect(over(text, type), findsWidgets, reason: '$type');
    }
    expect(
      find.byWidgetPredicate((w) => w is Hero && w.tag == 'gallery-hero'),
      findsOneWidget,
    );
    expect(find.text('second'), findsOneWidget);
    expect(h.errors.where((e) => e.code != PluxErrorCode.syncFailed), isEmpty);
  });

  testWidgets('Dismissible fires onDismissed after a swipe [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    expect(find.text('dismissed: no'), findsOneWidget);
    await tester.fling(find.text('Swipe me'), const Offset(-700, 0), 2000);
    await tester.pump(const Duration(milliseconds: 500));
    await tester.pump(const Duration(milliseconds: 500));
    await settle(tester);
    expect(
      find.byType(Text).evaluate().map((e) => (e.widget as Text).data).toList(),
      contains('dismissed: yes'),
      reason: '${h.errors}',
    );
    expect(find.text('Swipe me'), findsNothing);
  });

  testWidgets('Draggable carries its data to a DragTarget, which highlights '
      'while the candidate hovers [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('drag: idle, drop: none'), findsOneWidget);
    final gesture = await tester.startGesture(
      tester.getCenter(find.text('Drag')),
    );
    await tester.pump(const Duration(milliseconds: 100));
    await gesture.moveTo(tester.getCenter(find.text('Drop here')));
    await tester.pump(const Duration(milliseconds: 100));
    final highlighted = find.byWidgetPredicate(
      (w) => w is DecoratedBox && w.position == DecorationPosition.foreground,
    );
    expect(
      find.descendant(
        of: find.byType(DragTarget<Object>),
        matching: highlighted,
      ),
      findsOneWidget,
      reason: 'a candidate hovers',
    );
    await gesture.up();
    await settle(tester);
    expect(find.text('drag: started, drop: dropped'), findsOneWidget);
    expect(
      find.descendant(
        of: find.byType(DragTarget<Object>),
        matching: highlighted,
      ),
      findsNothing,
    );
  });

  testWidgets('InteractiveViewer carries its scale limits [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    final v = tester.widget<InteractiveViewer>(find.byType(InteractiveViewer));
    expect(v.minScale, 0.5);
    expect(v.maxScale, 4.0);
    expect(v.panEnabled, isTrue);
    expect(v.scaleEnabled, isTrue);
  });

  testWidgets('implicit widgets animate to the new value over their duration '
      '[ANI-001] [WGT-020]', (tester) async {
    await open(tester);
    double width() =>
        tester.getSize(over('container', AnimatedContainer)).width;
    expect(width(), 80);
    expect(opacityOf(tester, find.byType(AnimatedOpacity)), 0.25);
    final x0 = tester.getTopLeft(find.byType(AnimatedPositioned)).dx;
    await toggle(tester);
    await tester.pump(const Duration(milliseconds: 100));
    expect(width(), inExclusiveRange(80, 160), reason: 'mid-animation');
    expect(
      opacityOf(tester, find.byType(AnimatedOpacity)),
      inExclusiveRange(0.25, 1.0),
    );
    await tester.pump(const Duration(milliseconds: 300));
    expect(width(), 160);
    expect(opacityOf(tester, find.byType(AnimatedOpacity)), 1.0);
    final scale = tester.widget<ScaleTransition>(
      find.descendant(
        of: find.byType(AnimatedScale),
        matching: find.byType(ScaleTransition),
      ),
    );
    expect(scale.scale.value, 1.5);
    final turns = tester.widget<RotationTransition>(
      find.descendant(
        of: find.byType(AnimatedRotation),
        matching: find.byType(RotationTransition),
      ),
    );
    expect(turns.turns.value, 0.25);
    expect(
      tester.getTopLeft(find.byType(AnimatedPositioned)).dx - x0,
      160,
      reason: 'AnimatedPositioned moved by its left',
    );
    expect(find.text('switch on'), findsOneWidget);
    expect(find.text('switch off'), findsNothing);
  });

  testWidgets('implicit widgets jump to the value under reduce motion '
      '[ANI-007] [WGT-020]', (tester) async {
    await open(tester, reduceMotion: true);
    double width() =>
        tester.getSize(over('container', AnimatedContainer)).width;
    expect(width(), 80);
    await tester.tap(find.text('Toggle'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 1));
    expect(width(), 160, reason: 'no animation');
    expect(
      find.byWidgetPredicate((w) => w is Hero),
      findsNothing,
      reason: 'a hero flight is motion',
    );
  });

  testWidgets('FadeTransition, SlideTransition, ScaleTransition and '
      'RotationTransition follow a timeline track [ANI-002] [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    double fade() => tester
        .widget<FadeTransition>(over('t-fade', FadeTransition).first)
        .opacity
        .value;
    double scale() => tester
        .widget<ScaleTransition>(over('t-scale', ScaleTransition).first)
        .scale
        .value;
    double turns() => tester
        .widget<RotationTransition>(
          over('t-rotation', RotationTransition).first,
        )
        .turns
        .value;
    Offset slide() => tester
        .widget<SlideTransition>(over('t-slide', SlideTransition).first)
        .position
        .value;
    expect(fade(), 0.0, reason: 'the track start value before it plays');
    expect(scale(), 0.5);
    expect(turns(), 0.0);
    expect(slide(), Offset.zero);
    await tester.tap(find.text('Play'));
    await settle(tester);
    await tester.pump(const Duration(milliseconds: 200));
    expect(fade(), inExclusiveRange(0.0, 1.0));
    expect(scale(), inExclusiveRange(0.5, 1.5));
    await tester.pump(const Duration(milliseconds: 500));
    expect(fade(), 1.0);
    expect(scale(), 1.5);
    expect(turns(), 0.25);
    expect(slide(), const Offset(0.5, 0));
  });

  testWidgets('Timeline plays its timeline when shown [ANI-002] [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    double pulse() =>
        tester.widget<Opacity>(over('Timeline pulse', Opacity).first).opacity;
    await tester.pump(const Duration(milliseconds: 200));
    expect(pulse(), inExclusiveRange(0.2, 1.0));
    await tester.pump(const Duration(milliseconds: 600));
    expect(pulse(), closeTo(0.2, 0.001));
  });

  testWidgets('the group reads as its roles say [A11Y-001]', (tester) async {
    final semantics = tester.ensureSemantics();
    await open(tester);
    expect(
      tester.getSemantics(find.text('Toggle')),
      isSemantics(
        label: 'Toggle',
        isButton: true,
        isEnabled: true,
        hasTapAction: true,
      ),
    );
    expect(find.bySemanticsLabel('Swipe me'), findsOneWidget);
    expect(find.bySemanticsLabel('t-fade'), findsOneWidget);
    semantics.dispose();
  });

  group('the group in light, dark, right-to-left and 200% text '
      '[WGT-020] [A11Y-001]', () {
    for (final (name, dark, rtl, scale) in [
      ('light', false, false, 1.0),
      ('dark', true, false, 1.0),
      ('rtl', false, true, 1.0),
      ('text200', false, false, 2.0),
    ]) {
      testWidgets(name, (tester) async {
        await open(tester, dark: dark, rtl: rtl, scale: scale);
        await tester.pump(const Duration(milliseconds: 1000));
        expect(find.text('Swipe me'), findsOneWidget);
        await expectLater(
          find.byType(MaterialApp),
          matchesGoldenFile('goldens/animation_$name.png'),
        );
      });
    }
  });
}
