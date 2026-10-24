// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart' as http_testing;
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/devtools_api/dev_extensions.dart';
import 'package:plux_flutter/src/devtools_api/dev_inspect.dart';
import 'package:plux_flutter/src/devtools_api/dev_network.dart';
import 'package:plux_flutter/src/devtools_api/dev_nodes.dart';
import 'package:plux_flutter/src/devtools_api/dev_overlays.dart';
import 'package:plux_flutter/src/devtools_api/dev_screenshot.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/devtools_api/network_profiles.g.dart';
import 'package:plux_flutter/src/pxl/program.dart';

const _ok = 'DEV_COMMAND_STATUS_OK';
const _failed = 'DEV_COMMAND_STATUS_FAILED';
const _refused = 'DEV_COMMAND_STATUS_REFUSED';
const _section = '01c0c450-6c00-7000-8000-000000000001';

/// `n + 3` and `n / (n - 7)`, from the PXL conformance vectors.
const _addProgram = 'AQACA2ludAIEAW4CAwAAAAAAAAABAW4HBgAABAEAEw==';
const _divideProgram = 'AQADA2ludAIEAW4CBwAAAAAAAAABAW4LBgAABgAABAEAFBY=';

final class _Sink implements DevEventSink {
  final List<Map<String, Object?>> events = [];

  @override
  void emit(Map<String, Object?> payload, {String? traceId}) =>
      events.add(payload);
}

DevNodeInfo _info(
  int index,
  String widget, {
  bool interactive = false,
  String section = _section,
}) => DevNodeInfo(
  section: section,
  index: index,
  path: 'home/$index',
  widget: widget,
  interactive: interactive,
);

Map<String, Object?> _command(String name, Map<String, Object?> body) => {
  name: body,
};

/// The tools over a small page: a 20 by 20 button (node 1), a 100 by 60
/// button (node 2), and grey text on white (node 3).
Widget _page(
  DevNodeHost host,
  GlobalKey capture, {
  List<Widget> extra = const [],
}) => DevNodeScope(
  host: host,
  child: DevToolsLayer(
    host: host,
    child: DevCaptureBoundary(
      captureKey: capture,
      child: ColoredBox(
        color: const Color(0xFFFFFFFF),
        child: Align(
          alignment: Alignment.topLeft,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              DevNodeMarker(
                host: host,
                info: _info(1, 'IconButton', interactive: true),
                child: const SizedBox(
                  width: 20,
                  height: 20,
                  child: DecoratedBox(
                    decoration: BoxDecoration(color: Color(0xFF2962FF)),
                  ),
                ),
              ),
              DevNodeMarker(
                host: host,
                info: _info(2, 'Button', interactive: true),
                child: const SizedBox(
                  width: 100,
                  height: 60,
                  child: DecoratedBox(
                    decoration: BoxDecoration(color: Color(0xFF00C853)),
                  ),
                ),
              ),
              DevNodeMarker(
                host: host,
                info: _info(3, 'Text'),
                child: const DecoratedBox(
                  decoration: BoxDecoration(color: Color(0xFFFFFFFF)),
                  child: Text(
                    'quiet',
                    style: TextStyle(color: Color(0xFFCCCCCC), fontSize: 14),
                  ),
                ),
              ),
              ...extra,
            ],
          ),
        ),
      ),
    ),
  ),
);

Map<String, Object?> _decode(String b64) => {
  'value': jsonDecode(utf8.decode(base64.decode(b64))),
};

void main() {
  late DevNodeHost host;
  late _Sink sink;
  late GlobalKey capture;
  late DevNetworkSimulation network;
  late Map<String, DevCommandHandler> tools;
  var platformViews = false;

  setUp(() {
    host = DevNodeHost(
      sourceMap: (section, index) =>
          section == _section ? '/nodes/$index' : null,
    );
    sink = _Sink();
    capture = GlobalKey();
    platformViews = false;
    network = DevNetworkSimulation(random: Random(1), delay: (_) async {});
    tools = devExtensions(
      host: host,
      sink: sink,
      compile: (expression, roots) => switch (expression) {
        'n / (n - 7)' => Program.decode(base64.decode(_divideProgram)),
        'bad' => throw const FormatException('no'),
        _ => Program.decode(base64.decode(_addProgram)),
      },
      roots: () => {'n': 7},
      captureKey: capture,
      network: network,
      platformViews: (_) => platformViews,
    );
    addTearDown(host.dispose);
  });

  Future<Map<String, Object?>> send(String name, Map<String, Object?> body) =>
      tools[name]!.handle(_command(name, body));

  test('devExtensions registers the five tool commands', () {
    expect(tools.keys.toSet(), {
      'inspectSelect',
      'toggleOverlay',
      'evaluatePxl',
      'captureScreenshot',
      'networkProfile',
    });
  });

  group('inspect', () {
    testWidgets('a tap reports the node under it [DEV-030]', (tester) async {
      await tester.pumpWidget(_page(host, capture));
      host.inspecting = true;
      await tester.pump();

      await tester.tapAt(const Offset(50, 40));
      await tester.pump();
      expect(sink.events, [
        {
          'inspect': {'nodePath': '/nodes/2', 'widget': 'Button'},
        },
      ]);
      expect(host.selected, 'home/2');

      await tester.tapAt(const Offset(10, 10));
      await tester.pump();
      expect(sink.events.last, {
        'inspect': {'nodePath': '/nodes/1', 'widget': 'IconButton'},
      });
    });

    testWidgets('without inspect mode the tap reaches the app [DEV-030]', (
      tester,
    ) async {
      var taps = 0;
      await tester.pumpWidget(
        _page(
          host,
          capture,
          extra: [
            GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () => taps++,
              child: const SizedBox(width: 50, height: 50),
            ),
          ],
        ),
      );
      await tester.tapAt(const Offset(10, 120));
      expect(taps, 1);
      expect(sink.events, isEmpty);

      host.inspecting = true;
      await tester.pump();
      await tester.tapAt(const Offset(10, 120));
      expect(taps, 1, reason: 'inspect mode keeps taps from the app');
    });

    testWidgets('inspectSelect outlines a node until cleared [DEV-030]', (
      tester,
    ) async {
      await tester.pumpWidget(_page(host, capture));
      expect(find.byKey(DevToolsLayer.selectionKey), findsNothing);

      expect(await send('inspectSelect', {'nodePath': '/nodes/2'}), {
        'status': _ok,
      });
      await tester.pump();
      expect(host.selected, 'home/2');
      expect(find.byKey(DevToolsLayer.selectionKey), findsOneWidget);

      expect(await send('inspectSelect', {'nodePath': 'home/1'}), {
        'status': _ok,
      });
      expect(host.selected, 'home/1');

      final missing = await send('inspectSelect', {'nodePath': '/nodes/99'});
      expect(missing['status'], _failed);
      expect(host.selected, 'home/1');

      expect(await send('inspectSelect', {'nodePath': ''}), {'status': _ok});
      await tester.pump();
      expect(host.selected, isNull);
      expect(find.byKey(DevToolsLayer.selectionKey), findsNothing);
    });

    test('the source map maps a node to its document path [DEV-030]', () {
      final bytes = fbs.SourceMapObjectBuilder(
        nodes: [
          fbs.LocationObjectBuilder(
            section: fbs.UuidObjectBuilder(hi: 0x01c0c4506c007000, lo: 1),
            index: 4,
            pointer: '/pages/home/root/children/0',
          ),
        ],
      ).toBytes();
      final lookup = sourceMapLookup(Uint8List.fromList(bytes));
      const section = '01c0c450-6c00-7000-0000-000000000001';
      expect(lookup(section, 4), '/pages/home/root/children/0');
      expect(lookup(section, 5), isNull);
      expect(lookup('other', 4), isNull);
    });
  });

  group('overlays', () {
    testWidgets('each overlay toggles on and off [DEV-034]', (tester) async {
      await tester.pumpWidget(_page(host, capture));
      final cases = {
        'DEV_OVERLAY_FRAME_CHART': DevToolsLayer.frameChartKey,
        'DEV_OVERLAY_REBUILD_COUNTERS': DevToolsLayer.rebuildCountersKey,
        'DEV_OVERLAY_TOUCH_TARGETS': DevToolsLayer.touchTargetsKey,
        'DEV_OVERLAY_CONTRAST': DevToolsLayer.contrastKey,
      };
      for (final MapEntry(key: overlay, value: key) in cases.entries) {
        expect(find.byKey(key), findsNothing, reason: overlay);
        expect(
          await send('toggleOverlay', {'overlay': overlay, 'enabled': true}),
          {'status': _ok},
        );
        await tester.pump();
        expect(find.byKey(key), findsOneWidget, reason: overlay);
        await send('toggleOverlay', {'overlay': overlay, 'enabled': false});
        await tester.pump();
        expect(find.byKey(key), findsNothing, reason: overlay);
      }
      expect(host.overlays, isEmpty);
    });

    test('an overlay is named by its enum name or number [DEV-034]', () async {
      expect(await send('toggleOverlay', {'overlay': 3, 'enabled': true}), {
        'status': _ok,
      });
      expect(host.overlays, {DevOverlayKind.touchTargets});
      expect(
        (await send('toggleOverlay', {
          'overlay': 'DEV_OVERLAY_UNSPECIFIED',
          'enabled': true,
        }))['status'],
        _failed,
      );
      expect(
        (await send('toggleOverlay', {'enabled': true}))['status'],
        _failed,
      );
    });

    testWidgets(
      'the touch target highlight finds the small targets [DEV-034]',
      (tester) async {
        await tester.pumpWidget(_page(host, capture));
        expect(smallTargets(host), [_info(1, 'IconButton', interactive: true)]);
      },
    );

    testWidgets('the contrast highlight finds weak text [DEV-034]', (
      tester,
    ) async {
      await tester.pumpWidget(_page(host, capture));
      expect(lowContrast(host), [_info(3, 'Text')]);
      expect(
        contrastRatio(const Color(0xFF000000), const Color(0xFFFFFFFF)),
        closeTo(21, 0.01),
      );
    });

    testWidgets('rebuild counters count the builds of a node [DEV-034]', (
      tester,
    ) async {
      host
        ..built('home/1')
        ..built('home/1')
        ..built('home/2');
      expect(host.rebuilds('home/1'), 2);
      expect(host.rebuilds('home/2'), 1);
      expect(host.rebuilds('home/3'), 0);
      await send('toggleOverlay', {
        'overlay': 'DEV_OVERLAY_REBUILD_COUNTERS',
        'enabled': true,
      });
      await tester.pumpWidget(_page(host, capture));
      expect(find.byKey(DevToolsLayer.rebuildCountersKey), findsOneWidget);
    });

    test('the frame chart keeps the last 120 frames [DEV-034]', () {
      for (var i = 0; i < 130; i++) {
        host.recordFrame(DevFrame(Duration(milliseconds: i), Duration.zero));
      }
      expect(host.frames, hasLength(120));
      expect(host.frames.first.build, const Duration(milliseconds: 10));
      expect(host.frames.last.build, const Duration(milliseconds: 129));
    });
  });

  group('console', () {
    test('evaluates against the page state [DEV-032]', () async {
      final r = await send('evaluatePxl', {'expression': 'n + 3'});
      expect(r['status'], _ok);
      expect(_decode(r['result']! as String), {'value': 10});
    });

    test('refuses writes [DEV-032]', () async {
      for (final write in ['n = 4', 'page.count = page.count + 1']) {
        final r = await send('evaluatePxl', {'expression': write});
        expect(r['status'], _refused, reason: write);
        expect((r['error']! as Map)['code'], 'PLX-9206');
      }
      final comparison = await send('evaluatePxl', {
        'expression': 'n == 7 && "a=b" != "c"',
      });
      expect(comparison['status'], _ok);
    });

    group('with the program the server compiled', () {
      late Map<String, DevCommandHandler> served;

      setUp(() {
        served = devExtensions(
          host: host,
          sink: sink,
          roots: () => {'n': 7},
          captureKey: capture,
          network: network,
        );
      });

      Future<Map<String, Object?>> serve(Map<String, Object?> body) =>
          served['evaluatePxl']!.handle(_command('evaluatePxl', body));

      test('evaluates the program bytes against the state [DEV-032]', () async {
        final r = await serve({'expression': 'n + 3', 'program': _addProgram});
        expect(r['status'], _ok);
        expect(_decode(r['result']! as String), {'value': 10});
      });

      test('refuses writes even with a program [DEV-032]', () async {
        final r = await serve({'expression': 'n = 4', 'program': _addProgram});
        expect(r['status'], _refused);
        expect((r['error']! as Map)['code'], 'PLX-9206');
      });

      test('refuses a program that does not verify [DEV-032]', () async {
        final r = await serve({
          'expression': 'n + 3',
          'program': base64.encode([1, 2, 3]),
        });
        expect(r['status'], _failed);
        expect((r['error']! as Map)['code'], 'PLX-2001');
      });

      test(
        'without a program or a compiler it is unsupported [DEV-032]',
        () async {
          final r = await serve({'expression': 'n + 3'});
          expect(r['status'], _failed);
          expect((r['error']! as Map)['code'], 'PLX-9232');
        },
      );
    });

    test('reports errors with their code [DEV-032]', () async {
      final divide = await send('evaluatePxl', {'expression': 'n / (n - 7)'});
      expect(divide['status'], _failed);
      expect(
        (divide['error']! as Map)['message']! as String,
        contains('divisionByZero'),
      );
      final bad = await send('evaluatePxl', {'expression': 'bad'});
      expect(bad['status'], _failed);
      expect((bad['error']! as Map)['code'], 'PLX-2001');
      expect((await send('evaluatePxl', {}))['status'], _failed);
    });
  });

  group('screenshot', () {
    testWidgets('emits the PNG of the screen [DEV-033]', (tester) async {
      tester.view
        ..physicalSize = const Size(200, 300)
        ..devicePixelRatio = 2;
      addTearDown(tester.view.reset);
      await tester.pumpWidget(_page(host, capture));

      final result = await tester.runAsync(() => send('captureScreenshot', {}));
      expect(result, {'status': _ok});
      final shot = sink.events.single['screenshot']! as Map<String, Object?>;
      final png = base64.decode(shot['png']! as String);
      expect(png.sublist(0, 8), [
        0x89,
        0x50,
        0x4e,
        0x47,
        0x0d,
        0x0a,
        0x1a,
        0x0a,
      ]);
      final ihdr = ByteData.sublistView(png, 16, 24);
      expect(ihdr.getUint32(0), 200);
      expect(ihdr.getUint32(4), 300);
      expect(shot['width'], 200);
      expect(shot['height'], 300);
      expect(shot['pixelRatio'], 2.0);
      expect(shot['platformViewsNotCaptured'], false);
    });

    testWidgets('says when a platform view is on screen [DEV-033]', (
      tester,
    ) async {
      await tester.pumpWidget(_page(host, capture));
      platformViews = true;
      await tester.runAsync(() => send('captureScreenshot', {}));
      expect(
        (sink.events.single['screenshot']!
            as Map<String, Object?>)['platformViewsNotCaptured'],
        true,
      );
    });

    testWidgets('finds no platform view in an ordinary tree [DEV-033]', (
      tester,
    ) async {
      await tester.pumpWidget(_page(host, capture));
      final boundary = capture.currentContext!.findRenderObject()!;
      expect(hasPlatformView(boundary), isFalse);
    });

    test('fails without a screen [DEV-033]', () async {
      expect((await send('captureScreenshot', {}))['status'], _failed);
    });

    test('encodes a valid PNG [DEV-033]', () {
      final png = encodePng(
        2,
        1,
        Uint8List.fromList([1, 2, 3, 255, 4, 5, 6, 255]),
      );
      expect(png.sublist(png.length - 12), [
        0,
        0,
        0,
        0,
        0x49,
        0x45,
        0x4e,
        0x44,
        0xae,
        0x42,
        0x60,
        0x82,
      ]);
    });
  });

  group('network', () {
    late List<Duration> waits;
    late int sent;
    late http.Client client;

    setUp(() {
      waits = [];
      sent = 0;
      network = DevNetworkSimulation(
        random: Random(7),
        delay: (d) async => waits.add(d),
      );
      tools = devExtensions(
        host: host,
        sink: sink,
        compile: (_, _) => Program.decode(base64.decode(_addProgram)),
        roots: () => const {},
        captureKey: capture,
        network: network,
      );
      client = network.wrap(
        http_testing.MockClient.streaming((request, body) async {
          sent++;
          return http.StreamedResponse(
            Stream.value(Uint8List(1000)),
            200,
            contentLength: 1000,
          );
        }),
      );
    });

    Future<void> use(String id) async => expect(
      await send('networkProfile', {'profileId': id}),
      {'status': _ok},
    );

    test('offline fails every request at once [DEV-031]', () async {
      await use('offline');
      await expectLater(
        client.get(Uri.parse('https://example.test/')),
        throwsA(isA<http.ClientException>()),
      );
      expect(sent, 0);
      expect(waits, isEmpty);
    });

    test(
      'latency delays the request and bandwidth the body [DEV-031]',
      () async {
        await use('high_latency');
        final response = await client.get(Uri.parse('https://example.test/'));
        expect(response.bodyBytes, hasLength(1000));
        expect(waits, hasLength(2));
        expect(
          waits.first.inMilliseconds,
          inInclusiveRange(600, 1000),
          reason: '800 ms with 200 ms of jitter either way',
        );
        // 1000 bytes at 5000 kbit/s.
        expect(waits.last, const Duration(milliseconds: 1, microseconds: 600));
      },
    );

    test('a latency profile waits on the injected delay [DEV-031]', () async {
      var elapsed = Duration.zero;
      final slow = DevNetworkSimulation(
        random: Random(0),
        delay: (d) async => elapsed += d,
      )..profile = networkProfiles.firstWhere((p) => p.id == 'high_latency');
      final c = slow.wrap(
        http_testing.MockClient((_) async => http.Response('', 200)),
      );
      await c.get(Uri.parse('https://example.test/'));
      expect(elapsed, greaterThan(const Duration(milliseconds: 599)));
    });

    test('loss is deterministic under a seeded random [DEV-031]', () async {
      Future<List<bool>> run() async {
        final net = DevNetworkSimulation(
          random: Random(42),
          delay: (_) async {},
        )..profile = networkProfiles.firstWhere((p) => p.id == 'packet_loss');
        final c = net.wrap(
          http_testing.MockClient((_) async => http.Response('', 200)),
        );
        final outcomes = <bool>[];
        for (var i = 0; i < 60; i++) {
          try {
            await c.get(Uri.parse('https://example.test/'));
            outcomes.add(true);
          } on http.ClientException {
            outcomes.add(false);
          }
        }
        return outcomes;
      }

      final first = await run();
      expect(await run(), first);
      expect(first, contains(false));
      expect(first.where((ok) => ok).length, greaterThan(40));
    });

    test('good_wifi and an empty id reset the simulation [DEV-031]', () async {
      for (final reset in ['good_wifi', '']) {
        await use('offline');
        expect(network.profile?.id, 'offline');
        await use(reset);
        expect(network.profile, isNull);
        final response = await client.get(Uri.parse('https://example.test/'));
        expect(response.statusCode, 200);
        expect(waits, isEmpty);
      }
    });

    test('an unknown profile fails [DEV-031]', () async {
      final r = await send('networkProfile', {'profileId': 'dial_up'});
      expect(r['status'], _failed);
      expect(network.profile, isNull);
    });
  });
}
