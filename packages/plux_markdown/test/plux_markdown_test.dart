// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_markdown/plux_markdown.dart';

/// A node whose props are given by permanent ID; enough of [NodeContext]
/// for builders that read props and fire events.
final class FakeNode implements NodeContext {
  FakeNode(this.context, this.props, {this.events = const {}});

  @override
  final BuildContext context;
  final Map<int, Object?> props;
  final Map<int, void Function(Object?)> events;

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
  bool handles(int id) => events.containsKey(id);

  @override
  void fire(int id, [Object? payload]) => events[id]!(payload);

  final List<String> remote = [];

  @override
  ImageProvider<Object>? image({String? asset, String? url}) => null;

  @override
  ImageProvider<Object>? remoteImage(String url) {
    remote.add(url);
    return null;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Map<String, Object?> _descriptor(String type) => jsonDecode(
  File('../../schema/widgets/layer2/$type.json').readAsStringSync(),
) as Map<String, Object?>;

const _doc = '''
# Terms of service

Welcome to **Plux**, the *friendly* platform with `inline code`.

## Your rights

- First right
- Second right
  - nested right

1. Step one
2. Step two

> Quoted promise

```
var x = 1;
```

Read the [privacy policy](https://example.com/privacy) first.

<script>alert(1)</script>

![Logo](logo.png)
''';

Widget _app(
  Widget child, {
  bool dark = false,
  bool rtl = false,
  double scale = 1,
}) => MaterialApp(
  debugShowCheckedModeBanner: false,
  theme: ThemeData(
    colorScheme: ColorScheme.fromSeed(
      seedColor: Colors.indigo,
      brightness: dark ? Brightness.dark : Brightness.light,
    ),
  ),
  builder: (context, c) => Directionality(
    textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
    child: MediaQuery(
      data: MediaQuery.of(context)
          .copyWith(textScaler: TextScaler.linear(scale)),
      child: c!,
    ),
  ),
  home: Scaffold(
    body: Padding(padding: const EdgeInsets.all(16), child: child),
  ),
);

void main() {
  test('the package IDs are those of the descriptors [WGT-020] [RT-060]', () {
    for (final (type, id) in [
      ('TermsViewer', PluxMarkdownIds.termsViewer),
      ('MarkdownView', PluxMarkdownIds.markdownView),
    ]) {
      final d = _descriptor(type);
      expect(d['id'], id, reason: type);
      expect(d['package'], const PluxMarkdown().name);
    }
    expect(const PluxMarkdown().builders.keys, unorderedEquals([191, 192]));
  });

  testWidgets('blocks render; headings are headers; HTML is shown as text '
      '[WGT-020] [A11Y-001]', (tester) async {
    final handle = tester.ensureSemantics();
    tester.view.physicalSize = const Size(600, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_app(const PluxMarkdownView(source: _doc)));
    expect(find.textContaining('Terms of service'), findsOneWidget);
    expect(find.textContaining('Welcome to Plux'), findsOneWidget);
    expect(find.text('First right'), findsOneWidget);
    expect(find.text('nested right'), findsOneWidget);
    expect(find.text('2.'), findsOneWidget);
    expect(find.text('Quoted promise'), findsOneWidget);
    expect(find.text('var x = 1;'), findsOneWidget);
    expect(find.textContaining('<script>alert(1)</script>'), findsOneWidget);
    // No resolver: an image is its alt text.
    expect(find.textContaining('Logo'), findsOneWidget);
    expect(find.byType(Image), findsNothing);
    final header = tester.getSemantics(find.text('Terms of service'));
    expect(header.flagsCollection.isHeader, isTrue);
    handle.dispose();
  });

  testWidgets('links go to onLink and never open by themselves '
      '[WGT-020] [SEC-054]', (tester) async {
    final taps = <String>[];
    tester.view.physicalSize = const Size(600, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      _app(PluxMarkdownView(source: _doc, onLink: taps.add)),
    );
    await tester.tapOnText(find.textRange.ofSubstring('privacy policy'));
    expect(taps, ['https://example.com/privacy']);
  });

  testWidgets('images resolve only through the resolver [WGT-020]', (
    tester,
  ) async {
    final asked = <String>[];
    await tester.pumpWidget(
      _app(
        PluxMarkdownView(
          source: '![Logo](logo.png)',
          imageResolver: (s) {
            asked.add(s);
            return MemoryImage(Uint8List(0));
          },
        ),
      ),
    );
    expect(asked, ['logo.png']);
    expect(find.byType(Image), findsOneWidget);
  });

  testWidgets('the builders wire props and events [WGT-020]', (tester) async {
    final fired = <(int, Object?)>[];
    late Widget view;
    late Widget terms;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          const pkg = PluxMarkdown();
          view = pkg.builders[192]!(
            FakeNode(
              context,
              {1: 'a [b](https://x.y)'},
              events: {1: (p) => fired.add((1, p))},
            ),
          );
          terms = pkg.builders[191]!(
            FakeNode(
              context,
              {1: 'Terms', 2: '3.1'},
              events: {1: (p) => fired.add((1, p))},
            ),
          );
          return const SizedBox();
        },
      ),
    );
    expect(view, isA<PluxMarkdownView>());
    expect((view as PluxMarkdownView).onLink, isNotNull);
    (view as PluxMarkdownView).onLink!('https://x.y');
    expect(terms, isA<PluxTermsViewer>());
    (terms as PluxTermsViewer).onAccepted!('3.1');
    expect(fired, [(1, 'https://x.y'), (1, '3.1')]);
  });

  testWidgets('terms accept only after the end, with the version '
      '[WGT-020] [A11Y-001]', (tester) async {
    final accepted = <String>[];
    tester.view.physicalSize = const Size(400, 500);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final long = [for (var i = 0; i < 60; i++) 'Clause $i.'].join('\n\n');
    await tester.pumpWidget(
      _app(
        PluxTermsViewer(source: long, version: '2.0', onAccepted: accepted.add),
      ),
    );
    FilledButton button() => tester.widget(find.byType(FilledButton));
    expect(button().onPressed, isNull);
    expect(find.text('Scroll to the end to accept'), findsOneWidget);
    await tester.drag(
      find.byType(SingleChildScrollView),
      const Offset(0, -20000),
    );
    await tester.pumpAndSettle();
    expect(button().onPressed, isNotNull);
    expect(find.text('Scroll to the end to accept'), findsNothing);
    await tester.tap(find.byType(FilledButton));
    expect(accepted, ['2.0']);
  });

  testWidgets('short terms can be accepted at once [WGT-020]', (tester) async {
    await tester.pumpWidget(
      _app(PluxTermsViewer(source: 'Short.', version: '1', onAccepted: (_) {})),
    );
    await tester.pump();
    expect(
      tester.widget<FilledButton>(find.byType(FilledButton)).onPressed,
      isNotNull,
    );
  });

  group('goldens in light, dark, right-to-left and 200% text [WGT-020]', () {
    for (final (name, dark, rtl, scale) in [
      ('light', false, false, 1.0),
      ('dark', true, false, 1.0),
      ('rtl', false, true, 1.0),
      ('text200', false, false, 2.0),
    ]) {
      testWidgets(name, (tester) async {
        tester.view.physicalSize = const Size(600, 2600);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.reset);
        await tester.pumpWidget(
          _app(
            SingleChildScrollView(
              child: PluxMarkdownView(source: _doc, onLink: (_) {}),
            ),
            dark: dark,
            rtl: rtl,
            scale: scale,
          ),
        );
        await expectLater(
          find.byType(Scaffold),
          matchesGoldenFile('goldens/markdown_$name.png'),
        );
      });
    }
  });

  testWidgets('an image address goes through the domain-checked remote '
      'image, a name stays an asset [SEC-080][AST-002][WGT-020]', (
    tester,
  ) async {
    late FakeNode node;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: Builder(
              builder: (context) {
                node = FakeNode(context, {
                  1: '![a](https://evil.example.net/a.png)',
                });
                return const PluxMarkdown().builders[PluxMarkdownIds
                    .markdownView]!(node);
              },
            ),
          ),
        ),
      ),
    );
    expect(node.remote, ['https://evil.example.net/a.png']);
  });
}
