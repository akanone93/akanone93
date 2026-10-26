// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Layout goldens of the Layer 2 components (STU-005, S4.5): the real runtime
// renders every `layer2-*` scene of test/layout-conformance/scenes, compiled by
// the Go compiler, and records the global rect of each node by its `testId`,
// in the format of test/layout-conformance/goldens. With UPDATE_GOLDENS=1 the
// goldens are written; otherwise the test fails when one is missing or stale.
// Studio's layout engine is compared with these files (make studio-conformance).

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_charts/plux_charts.dart';
import 'package:plux_components/plux_components.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_lottie/plux_lottie.dart';
import 'package:plux_maps/plux_maps.dart';
import 'package:plux_markdown/plux_markdown.dart';
import 'package:plux_media/plux_media.dart';
import 'package:plux_player/plux_player.dart';
import 'package:plux_rive/plux_rive.dart';

import '../support/harness.dart';

const _flutterRevision = '6a19cca56475dbfba1478ee68d7bd0c2ef891da1';
const _suite = '../../test/layout-conformance';
const _frames = [('phone', 360.0, 800.0, 3.0), ('tablet', 800.0, 1280.0, 2.0)];
const _directions = [('ltr', 'en'), ('rtl', 'ar')];
const _scales = [1.0, 2.0];

String _uuid(int n) =>
    '01c0c450-6c00-7000-8f00-${n.toRadixString(16).padLeft(12, '0')}';

/// A scene node as a Plux node: `id` becomes the `testId`.
Map<String, Object?> _node(Map<String, Object?> scene, _Ids ids) => {
  'id': _uuid(ids.next()),
  'type': scene['type'],
  'testId': scene['id'],
  if (scene['props'] != null) 'props': scene['props'],
  if (scene['slots'] != null)
    'slots': {
      for (final e in (scene['slots']! as Map<String, Object?>).entries)
        e.key: _node(e.value! as Map<String, Object?>, ids),
    },
  if (scene['children'] != null)
    'children': [
      for (final c in scene['children']! as List<Object?>)
        _node(c! as Map<String, Object?>, ids),
    ],
};

final class _Ids {
  int _n = 0x1000;
  int next() => _n++;
}

/// The page every scene is laid out in: a Scaffold whose body is a Column
/// that starts its children, so a component gets loose width.
Map<String, Object?> _page(int index, Map<String, Object?> root) {
  final ids = _Ids().._n = 0x1000 + index * 0x100;
  return {
    'id': _uuid(0x100 + index),
    'key': 'scene$index',
    'kind': 'page',
    'pageKind': 'screen',
    'route': 'scene$index',
    'schemaVersion': '1.0.0',
    'title': 'scene',
    'root': {
      'id': _uuid(ids.next()),
      'type': 'Scaffold',
      'slots': {
        'body': {
          'id': _uuid(ids.next()),
          'type': 'Column',
          'props': {'crossAxisAlignment': 'start'},
          'children': [_node(root, ids)],
        },
      },
    },
  };
}

Set<String> _testIds(Map<String, Object?> n, [Set<String>? into]) {
  into ??= {};
  into.add(n['id']! as String);
  for (final s in (n['slots'] as Map<String, Object?>? ?? {}).values) {
    _testIds(s! as Map<String, Object?>, into);
  }
  for (final c in (n['children'] as List<Object?>? ?? [])) {
    _testIds(c! as Map<String, Object?>, into);
  }
  return into;
}

/// Compiles the pages into bundles with the Go CLI.
Future<Map<String, Uint8List>> _compile(List<Map<String, Object?>> pages) async {
  final work = Directory.systemTemp.createTempSync('plux_l2');
  addTearDown(() => work.deleteSync(recursive: true));
  final project = Directory('${work.path}/project');
  await Process.run('cp', ['-r', '../../schema/testdata/documents/widgets', project.path]);
  final pagesDir = Directory('${project.path}/plugins/gallery/pages');
  for (final f in pagesDir.listSync()) {
    f.deleteSync();
  }
  Directory('${project.path}/components').deleteSync(recursive: true);
  for (var i = 0; i < pages.length; i++) {
    File('${pagesDir.path}/scene$i.page.json').writeAsStringSync(jsonEncode(pages[i]));
  }
  void edit(String path, void Function(Map<String, Object?>) f) {
    final file = File('${project.path}/$path');
    final doc = jsonDecode(file.readAsStringSync()) as Map<String, Object?>;
    f(doc);
    file.writeAsStringSync(jsonEncode(doc));
  }

  edit('plugins/gallery/plugin.json', (d) {
    d['pages'] = [for (final p in pages) p['id']];
    d['entryPage'] = pages.first['id'];
    d['dataSources'] = [
      for (final s in d['dataSources']! as List<Object?>)
        if ((s! as Map<String, Object?>)['name'] == 'uploads') s,
    ];
  });
  edit('app.json', (d) {
    d['entryRoute'] = 'scene0';
    d['supportedLocales'] = ['en', 'ar'];
  });
  final ar = jsonDecode(
    File('${project.path}/translations/en.json').readAsStringSync(),
  ) as Map<String, Object?>;
  ar['id'] = _uuid(0x90);
  ar['locale'] = 'ar';
  File('${project.path}/translations/ar.json').writeAsStringSync(jsonEncode(ar));
  final binary = '${work.path}/plux';
  final built = await Process.run('go', [
    'build',
    '-o',
    binary,
    './cmd/plux',
  ], workingDirectory: '../../backend');
  if (built.exitCode != 0) fail('go build failed: ${built.stderr}');
  final out = '${work.path}/out';
  final run = await Process.run(binary, ['build', '-o', out, project.path]);
  if (run.exitCode != 0) fail('plux build failed: ${run.stdout}${run.stderr}');
  return {
    'app': File('$out/app/widgets.pxb').readAsBytesSync(),
    'gallery': File('$out/plugins/gallery.pxb').readAsBytesSync(),
  };
}

Future<void> _loadFonts() async {
  final loaders = <String, FontLoader>{};
  for (final (family, file) in const [
    ('NotoSans', 'NotoSans_400Regular.ttf'),
    ('NotoSans', 'NotoSans_700Bold.ttf'),
    ('NotoNaskhArabic', 'NotoNaskhArabic_400Regular.ttf'),
  ]) {
    final bytes = File('../../schema/goldens/fonts/$file').readAsBytesSync();
    (loaders[family] ??= FontLoader(family)).addFont(
      Future.value(ByteData.sublistView(bytes)),
    );
  }
  for (final l in loaders.values) {
    await l.load();
  }
}

void main() {
  final update = Platform.environment['UPDATE_GOLDENS'] == '1';
  final files = Directory('$_suite/scenes')
      .listSync()
      .whereType<File>()
      .where((f) => f.uri.pathSegments.last.startsWith('layer2-'))
      .toList()
    ..sort((a, b) => a.path.compareTo(b.path));
  final scenes = [
    for (final f in files) jsonDecode(f.readAsStringSync()) as Map<String, Object?>,
  ];

  testWidgets('Layer 2 layout goldens are current', (tester) async {
    await tester.runAsync(_loadFonts);
    final bundles = await tester.runAsync(
      () => _compile([
        for (var i = 0; i < scenes.length; i++)
          _page(i, scenes[i]['root']! as Map<String, Object?>),
      ]),
    );
    final expected = <String>{};
    for (var i = 0; i < scenes.length; i++) {
      final name = scenes[i]['name']! as String;
      // Nodes a component does not build (the steps of a wizard but the shown one).
      final unbuilt = {...?(scenes[i]['unbuilt'] as List<Object?>?)?.cast<String>()};
      final ids = _testIds(scenes[i]['root']! as Map<String, Object?>)..removeAll(unbuilt);
      for (final (dir, locale) in _directions) {
        tester.platformDispatcher.localeTestValue = Locale(locale);
        final h = await Harness.create();
        await tester.runAsync(() async {
          await h.startFrom(
            bundles!['app']!,
            {'gallery': bundles['gallery']!},
            widgetPackages: const [
              PluxComponents(),
              PluxCharts(),
              PluxPlayer(),
              PluxLottieWidgets(),
              PluxMaps(),
              PluxMarkdown(),
              PluxMediaWidgets(),
              PluxRiveWidgets(),
            ],
          );
          await h.loadIconFonts();
        });
        for (final (frame, w, ht, dpr) in _frames) {
          for (final scale in _scales) {
            tester.view.physicalSize = Size(w * dpr, ht * dpr);
            tester.view.devicePixelRatio = dpr;
            await tester.pumpWidget(
              MaterialApp(
                key: UniqueKey(),
                debugShowCheckedModeBanner: false,
                theme: ThemeData(useMaterial3: true, fontFamily: 'NotoSans'),
                builder: (context, child) => MediaQuery(
                  data: MediaQuery.of(context).copyWith(
                    textScaler: TextScaler.linear(scale),
                    disableAnimations: true,
                  ),
                  child: Directionality(
                    textDirection: dir == 'rtl' ? TextDirection.rtl : TextDirection.ltr,
                    child: child!,
                  ),
                ),
                home: PluxScope(child: PluxView('scene$i')),
              ),
            );
            await settle(tester);
            final rects = <String, List<double>>{};
            for (final id in ids.toList()..sort()) {
              final finder = find.byWidgetPredicate(
                (w) => w is Semantics && w.properties.identifier == id,
              );
              expect(finder, findsWidgets, reason: '$name: no node "$id"');
              final r = tester.getRect(finder.first);
              rects[id] = [r.left, r.top, r.width, r.height];
            }
            final goldenName = '$name.$frame.$dir.${scale.toStringAsFixed(1)}.json';
            expected.add(goldenName);
            final content =
                '${const JsonEncoder.withIndent('  ').convert({
                  'flutterRevision': _flutterRevision,
                  'scene': name,
                  'frame': {'width': w, 'height': ht, 'devicePixelRatio': dpr},
                  'locale': locale,
                  'textScale': scale,
                  'rects': rects,
                })}\n';
            final golden = File('$_suite/goldens/$goldenName');
            if (update) {
              golden.writeAsStringSync(content);
            } else {
              expect(golden.existsSync(), isTrue, reason: '$goldenName is missing: run make studio-conformance-update');
              expect(golden.readAsStringSync(), content, reason: '$goldenName is stale: run make studio-conformance-update');
            }
          }
        }
        await tester.pumpWidget(const SizedBox());
        await tester.runAsync(h.close);
        await tester.pump(const Duration(seconds: 5));
        tester.view.reset();
        tester.platformDispatcher.clearLocaleTestValue();
      }
    }
    final present = Directory('$_suite/goldens')
        .listSync()
        .whereType<File>()
        .map((f) => f.uri.pathSegments.last)
        .where((n) => n.startsWith('layer2-'));
    final orphans = present.toSet().difference(expected);
    if (update) {
      for (final o in orphans) {
        File('$_suite/goldens/$o').deleteSync();
      }
    } else {
      expect(orphans, isEmpty, reason: 'goldens without a scene');
    }
  }, timeout: const Timeout(Duration(minutes: 10)));
}
