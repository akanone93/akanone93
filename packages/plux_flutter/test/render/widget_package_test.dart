// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/device/guard.dart';
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

/// What the mock client of this file was asked for; the client is made by a
/// top-level function because the runtime sends its factory to the sync
/// worker.
final List<String> _asked = [];

http.Client _mockClient() => MockClient.streaming((r, body) async {
  _asked.add(r.url.toString());
  return http.StreamedResponse(
    Stream.value(Uint8List.fromList([1, 2, 3])),
    200,
    contentLength: 3,
  );
});

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

  testWidgets('remoteClient and remoteImage apply the domain rule '
      '[SEC-080][AST-002][RT-060]', (tester) async {
    final seen = <Object?>[];
    await show(tester, [
      _Package({
        line.id: (c) {
          seen
            ..add(c.remoteClient('https://example.com/t/1/2.png'))
            ..add(c.remoteClient('https://example.com/t/1/3.png'))
            ..add(c.remoteImage('https://example.com/a.png'))
            ..add(c.remoteClient('https://evil.example.net/t.png'))
            ..add(c.remoteClient('http://example.com/t.png'))
            ..add(c.remoteImage('https://evil.example.net/a.png'));
          return const SizedBox();
        },
        bar.id: (c) => const SizedBox(),
      }),
    ]);
    final first = seen.take(2).toList();
    expect(first[0], isNotNull);
    expect(first[1], same(first[0]));
    expect(seen[2], isNotNull);
    expect(seen.skip(3).take(2), everyElement(isNull));
    expect(seen[5], isNull);
    expect(
      h.errors.where((e) => e.code == PluxErrorCode.outboundRequestBlocked),
      hasLength(6),
    );
  });

  testWidgets('a widget of a plugin without the device API gets the '
      'standard refusal and nothing opens [SEC-080][RT-060]', (tester) async {
    var opened = 0;
    await show(tester, [
      _Package({
        line.id: (c) {
          final refusal = c.deviceRefusal('location');
          if (refusal != null) return refusal;
          opened++;
          return const SizedBox();
        },
        bar.id: (c) => const SizedBox(),
      }),
    ]);
    expect(opened, 0);
    expect(
      h.errors.where((e) => e.code == PluxErrorCode.deviceCapabilityBlocked),
      isNotEmpty,
    );
    expect(DeviceGuard.widgetApiOf['QrScanner'], 'camera');
    expect(DeviceGuard.widgetApiOf['FileUploader'], 'files');
  });

  group('media and remote bytes', () {
    Future<List<Object?>> run(
      WidgetTester tester,
      Future<Object?> Function(NodeContext c) ask, {
      required String profile,
    }) async {
      final out = <Object?>[];
      late NodeContext node;
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
          client: _mockClient,
          widgetPackages: [
            _Package({
              line.id: (c) {
                node = c;
                return const SizedBox();
              },
              bar.id: (c) => const SizedBox(),
            }),
          ],
        ),
      );
      await tester.pumpWidget(
        const MaterialApp(home: PluxScope(child: PluxView('layout'))),
      );
      h.runtime.settings.value = SecuritySettings.fromDocument({
        'profile': profile,
      });
      out.add(await tester.runAsync<Object?>(() => ask(node)));
      return out;
    }

    testWidgets('under maximum the media is downloaded through the pinned '
        'client to a temporary file [SEC-041][SEC-080][AST-002]', (
      tester,
    ) async {
      _asked.clear();
      final out = await run(
        tester,
        (c) => c.mediaSource('https://example.com/a.mp4'),
        profile: 'maximum',
      );
      expect(_asked, ['https://example.com/a.mp4']);
      final uri = out.single! as Uri;
      expect(uri.scheme, 'file');
      expect(File.fromUri(uri).readAsBytesSync(), [1, 2, 3]);
      File.fromUri(uri).parent.deleteSync(recursive: true);
    });

    testWidgets('under standard the platform player gets the address and '
        'nothing is downloaded [SEC-041]', (tester) async {
      _asked.clear();
      final out = await run(
        tester,
        (c) => c.mediaSource('https://example.com/a.mp4'),
        profile: 'standard',
      );
      expect(_asked, isEmpty);
      expect(out.single, Uri.parse('https://example.com/a.mp4'));
    });

    testWidgets('an undeclared domain is refused with PLX-6030 and nothing '
        'is loaded [SEC-080][AST-002]', (tester) async {
      _asked.clear();
      final out = await run(
        tester,
        (c) async => [
          await c.mediaSource('https://evil.example.net/a.mp4'),
          await c.remoteBytes('https://evil.example.net/a.json'),
        ],
        profile: 'maximum',
      );
      expect(out.single, [null, null]);
      expect(_asked, isEmpty);
      expect(
        h.errors.where((e) => e.code == PluxErrorCode.outboundRequestBlocked),
        hasLength(2),
      );
    });

    testWidgets('remoteBytes fetches a declared domain through the pinned '
        'client [SEC-041][AST-002]', (tester) async {
      _asked.clear();
      final out = await run(
        tester,
        (c) => c.remoteBytes('https://example.com/a.json'),
        profile: 'standard',
      );
      expect(out.single, [1, 2, 3]);
      expect(_asked, ['https://example.com/a.json']);
    });
  });
}
