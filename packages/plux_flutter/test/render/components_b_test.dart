// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/device/types.dart';
import 'package:plux_flutter/src/render/builders/components_b.dart';

import '../support/harness.dart';

/// The Layer 2 components of group B (WGT-020, A11Y-001): commerce,
/// campaign, rating and media components, rendered from the gallery page
/// `components-b` and, for the ones with injected services, directly.
void main() {
  final g = Harness.goldens;
  // The instant every countdown of the page ends, and the fake "now": 2 days,
  // 3 hours, 4 minutes and 5 seconds before it.
  final ends = DateTime.utc(2099);
  late DateTime now;
  late Harness h;
  late List<String> haptics;

  setUp(() async {
    h = await Harness.create();
    haptics = [];
    now = ends.subtract(
      const Duration(days: 2, hours: 3, minutes: 4, seconds: 5),
    );
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (call) async {
          if (call.method.startsWith('HapticFeedback.')) {
            haptics.add(call.method);
          }
          return null;
        });
  });
  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null);
    return h.close();
  });

  Future<void> open(
    WidgetTester tester, {
    bool reduceMotion = false,
    bool rtl = false,
    double scale = 1,
  }) async {
    tester.view.physicalSize = const Size(800, 2600);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      await h.startFrom(g.bundles['widgets/widgets.pxb']!, {
        'gallery': g.bundles['widgets/gallery.pxb']!,
      }, themeSource: PluxThemeSource.plux);
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        builder: (context, child) => CountdownClock(
          now: () => now,
          child: Directionality(
            textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
            child: MediaQuery(
              data: MediaQuery.of(context).copyWith(
                textScaler: TextScaler.linear(scale),
                disableAnimations: reduceMotion,
              ),
              child: child!,
            ),
          ),
        ),
        home: const PluxScope(child: PluxView('components-b')),
      ),
    );
    await settle(tester);
  }

  /// Problems other than the sync that finds no server release, and the
  /// missing picker package the page's uploader reports.
  List<PluxException> problems() => [
    for (final e in h.errors)
      if (e.code != PluxErrorCode.syncFailed &&
          e.code != PluxErrorCode.devicePackageMissing)
        e,
  ];

  group('components in light, dark, right-to-left and 200% text '
      '[WGT-020]', () {
    for (final (name, dark, rtl, scale) in [
      ('light', false, false, 1.0),
      ('dark', true, false, 1.0),
      ('rtl', false, true, 1.0),
      ('text200', false, false, 2.0),
    ]) {
      testWidgets(name, (tester) async {
        await open(tester, rtl: rtl, scale: scale, reduceMotion: true);
        if (dark) {
          Plux.setThemeMode(ThemeMode.dark);
          await settle(tester);
          await tester.pump(const Duration(milliseconds: 300));
        }
        for (final t in [
          'Order 1042',
          '7 min',
          'On the way',
          'Trail shoe',
          r'$89.00',
          'Summer sale',
          'Shop now',
          '2d 03:04:05',
        ]) {
          expect(find.text(t), findsWidgets, reason: t);
        }
        expect(problems(), isEmpty);
        await expectLater(
          find.byType(MaterialApp),
          matchesGoldenFile('goldens/components_b_$name.png'),
        );
      });
    }
  });

  testWidgets('ProductCard fires onTap and onAddToCart [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    await tester.tap(find.text('Trail shoe'));
    await tester.pump();
    await tester.pump();
    expect(haptics, hasLength(1), reason: 'onTap');
    await tester.tap(find.text('Add to cart'));
    await tester.pump();
    await tester.pump();
    expect(haptics, hasLength(2), reason: 'onAddToCart');
    expect(problems(), isEmpty);
  });

  testWidgets('QuantityStepper steps, clamps and fires onChanged '
      '[WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('2'), findsOneWidget);
    await tester.tap(find.byIcon(Icons.add));
    await tester.pump();
    expect(find.text('3'), findsOneWidget);
    for (var i = 0; i < 4; i++) {
      await tester.tap(find.byIcon(Icons.add));
      await tester.pump();
    }
    expect(find.text('5'), findsOneWidget, reason: 'clamped at max');
    expect(
      tester
          .widget<IconButton>(find.widgetWithIcon(IconButton, Icons.add))
          .onPressed,
      isNull,
    );
    await tester.tap(find.byIcon(Icons.remove));
    await tester.pump();
    expect(find.text('4'), findsOneWidget);
    await tester.pump();
    expect(haptics, hasLength(4), reason: 'one onChanged per change');
  });

  testWidgets('RatingInput selects a star and fires onChanged [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    expect(find.byIcon(Icons.star), findsWidgets);
    await tester.tap(find.bySemanticsLabel('2 star'));
    await tester.pump();
    await tester.pump();
    expect(haptics, hasLength(1));
    expect(
      tester.widgetList<Icon>(
        find.descendant(
          of: find.byType(IconButton),
          matching: find.byIcon(Icons.star),
        ),
      ),
      hasLength(2),
      reason: 'two stars filled',
    );
  });

  testWidgets('Carousel pages on swipe and autoplay, which stops with reduce '
      'motion [WGT-020]', (tester) async {
    await open(tester);
    await tester.drag(find.text('slide one'), const Offset(-600, 0));
    await tester.pumpAndSettle();
    expect(haptics, hasLength(1), reason: 'swipe fires onPageChanged');
    // Autoplay: after one interval the second page shows.
    await tester.pump(const Duration(seconds: 1));
    await tester.pumpAndSettle();
    expect(haptics, hasLength(2), reason: 'autoplay advanced');
    expect(find.text('auto two'), findsOneWidget);
    expect(problems(), isEmpty);
  });

  testWidgets('Carousel autoplay pauses with reduce motion [WGT-020]', (
    tester,
  ) async {
    await open(tester, reduceMotion: true);
    await tester.pump(const Duration(seconds: 3));
    expect(haptics, isEmpty);
    expect(find.text('auto one'), findsOneWidget);
  });

  testWidgets('ImageGallery swipes between images [WGT-020]', (tester) async {
    await open(tester);
    expect(find.byType(InteractiveViewer), findsWidgets);
    await tester.drag(
      find.byType(InteractiveViewer).first,
      const Offset(-600, 0),
    );
    await tester.pumpAndSettle();
    expect(haptics, hasLength(1), reason: 'onPageChanged');
  });

  testWidgets('FileUploader without plux_media is disabled and reports '
      'PLX-5401 [WGT-020] [RT-060]', (tester) async {
    await open(tester);
    await tester.pump();
    final pick = find.widgetWithText(OutlinedButton, 'Choose file');
    expect(tester.widget<OutlinedButton>(pick).onPressed, isNull);
    expect(
      h.errors.map((e) => e.code),
      contains(PluxErrorCode.devicePackageMissing),
    );
  });

  testWidgets('Components read as their screen-reader script says '
      '[WGT-020] [A11Y-001]', (tester) async {
    final semantics = tester.ensureSemantics();
    await open(tester, reduceMotion: true);
    expect(
      tester.getSemantics(find.text('Order 1042')),
      isSemantics(label: 'Order 1042, 7 min, On the way', isLiveRegion: true),
      reason: 'the ETA is announced as it changes',
    );
    expect(
      tester.getSemantics(find.text('Trail shoe')),
      isSemantics(
        label: '-25%\nTrail shoe\n\$89.00\n\$120.00',
        isButton: true,
        hasTapAction: true,
      ),
    );
    expect(
      tester.getSemantics(find.text('2')),
      isSemantics(
        value: '2',
        increasedValue: '3',
        decreasedValue: '1',
        hasIncreaseAction: true,
        hasDecreaseAction: true,
      ),
    );
    expect(
      tester.getSemantics(find.byIcon(Icons.star_half)),
      isSemantics(label: 'Rating', value: '3.5 of 5'),
    );
    expect(
      tester.getSemantics(find.bySemanticsLabel('4 star')),
      isSemantics(
        label: '4 star',
        isButton: true,
        isSelected: true,
        hasSelectedState: true,
        isEnabled: true,
        hasEnabledState: true,
        hasTapAction: true,
      ),
    );
    expect(
      tester.getSemantics(find.text('2d 03:04:05').first),
      isSemantics(label: '2d 03:04:05'),
      reason: 'the countdown reads its time without announcing each tick',
    );
    semantics.dispose();
  });

  group('CountdownTimer', () {
    Widget host(CountdownView view) => MaterialApp(
      home: CountdownClock(
        now: () => now,
        child: Scaffold(body: view),
      ),
    );

    testWidgets('ticks once a second in each format and fires onFinished '
        'once at zero [WGT-020]', (tester) async {
      now = ends.subtract(
        const Duration(days: 1, hours: 2, minutes: 3, seconds: 3),
      );
      var finished = 0;
      for (final (format, text) in [
        ('hms', '26:03:03'),
        ('dhms', '1d 02:03:03'),
        ('compact', '1d 2h'),
      ]) {
        await tester.pumpWidget(
          host(
            CountdownView(
              endsAt: ends,
              format: format,
              onFinished: () => finished++,
            ),
          ),
        );
        expect(find.text(text), findsOneWidget, reason: format);
      }
      await tester.pumpWidget(
        host(CountdownView(endsAt: ends, onFinished: () => finished++)),
      );
      now = now.add(const Duration(seconds: 1));
      await tester.pump(const Duration(seconds: 1));
      expect(find.text('26:03:02'), findsOneWidget, reason: 'one tick');
      now = ends;
      await tester.pump(const Duration(seconds: 1));
      expect(find.text('00:00:00'), findsOneWidget);
      expect(finished, 1);
      await tester.pump(const Duration(seconds: 5));
      expect(finished, 1, reason: 'only once');
    });

    testWidgets('disposes its timer with the widget [WGT-020]', (tester) async {
      await tester.pumpWidget(host(CountdownView(endsAt: ends)));
      await tester.pumpWidget(const MaterialApp(home: SizedBox()));
      // A pending periodic timer would fail the test at its end.
      await tester.pump(const Duration(seconds: 3));
    });
  });

  group('FileUploader view', () {
    const file = PluxPickedFile(
      path: '/nonexistent/photo.jpg',
      name: 'photo.jpg',
      mimeType: 'image/jpeg',
      sizeBytes: 2048,
    );

    Future<void> show(
      WidgetTester tester, {
      Future<PluxPickedFile?> Function()? pick,
      Future<String?> Function(PluxPickedFile, void Function(double?))? upload,
      int? maxBytes,
      List<Map<String, Object?>>? uploaded,
      List<Map<String, Object?>>? failed,
      VoidCallback? onUnavailable,
    }) => tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: FileUploaderView(
            label: 'Choose file',
            uploadLabel: 'Upload',
            maxBytes: maxBytes,
            pick: pick,
            upload: upload,
            onUnavailable: onUnavailable,
            onUploaded: uploaded?.add,
            onFailed: failed?.add,
          ),
        ),
      ),
    );

    testWidgets('picks, previews and uploads with progress [WGT-020] '
        '[A11Y-001]', (tester) async {
      final semantics = tester.ensureSemantics();
      final uploaded = <Map<String, Object?>>[];
      final done = Completer<String?>();
      late void Function(double?) report;
      await show(
        tester,
        pick: () async => file,
        upload: (f, progress) {
          report = progress;
          return done.future;
        },
        uploaded: uploaded,
      );
      await tester.tap(find.text('Choose file'));
      await tester.pump();
      expect(find.text('photo.jpg'), findsOneWidget);
      expect(find.text('2.0 KB'), findsOneWidget);
      await tester.tap(find.text('Upload'));
      await tester.pump();
      report(0.42);
      await tester.pump();
      expect(
        tester.getSemantics(find.byType(LinearProgressIndicator)),
        isSemantics(label: 'photo.jpg', value: '42%'),
        reason: 'progress is spoken as a percentage',
      );
      done.complete('{"id":"f1"}');
      await tester.pump();
      await tester.pump();
      expect(uploaded, [
        {
          'name': 'photo.jpg',
          'mimeType': 'image/jpeg',
          'sizeBytes': 2048,
          'response': '{"id":"f1"}',
        },
      ]);
      semantics.dispose();
    });

    testWidgets('a failing upload fires onFailed and can be retried '
        '[WGT-020]', (tester) async {
      final failed = <Map<String, Object?>>[];
      var calls = 0;
      await show(
        tester,
        pick: () async => file,
        upload: (f, p) async {
          calls++;
          throw const FormatException('boom');
        },
        failed: failed,
      );
      await tester.tap(find.text('Choose file'));
      await tester.pump();
      await tester.tap(find.text('Upload'));
      await tester.pump();
      await tester.pump();
      expect(failed.single['code'], PluxErrorCode.dataTransferFileFailed.id);
      await tester.tap(find.text('Upload'));
      await tester.pump();
      await tester.pump();
      expect(calls, 2, reason: 'the upload button stays available');
    });

    testWidgets('a file over maxBytes is refused [WGT-020]', (tester) async {
      final failed = <Map<String, Object?>>[];
      await show(
        tester,
        pick: () async => file,
        maxBytes: 1024,
        failed: failed,
      );
      await tester.tap(find.text('Choose file'));
      await tester.pump();
      expect(failed, hasLength(1));
      expect(find.text('Upload'), findsNothing);
    });

    testWidgets('without a picker the control is disabled and the problem is '
        'reported once [WGT-020] [RT-060]', (tester) async {
      var reported = 0;
      await show(tester, pick: null, onUnavailable: () => reported++);
      await tester.pump();
      expect(
        tester.widget<OutlinedButton>(find.byType(OutlinedButton)).onPressed,
        isNull,
      );
      expect(reported, 1);
    });
  });
}
