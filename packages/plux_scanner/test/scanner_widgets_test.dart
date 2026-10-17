// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The QrScanner and BarcodeScanner builders on a fake camera session (no
// camera, no engine), and the engine choice of decision B21.
//
// Verifies: RT-060, SEC-080.

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_scanner/plux_scanner.dart';

/// A camera session whose hits and errors the test pushes.
final class FakeSession implements ScanSession {
  FakeSession(this.request, this.engine);

  final ScanRequest request;
  final ScannerEngine engine;
  final StreamController<ScanHit> hitController = StreamController.broadcast();
  final StreamController<String> errorController = StreamController.broadcast();
  final List<bool> torches = [];
  bool disposed = false;

  @override
  Stream<ScanHit> get hits => hitController.stream;

  @override
  Stream<String> get errors => errorController.stream;

  @override
  Widget preview(BuildContext context, Rect? scanWindow) =>
      Text('preview ${scanWindow ?? 'full'}');

  @override
  Future<void> setTorch(bool on) async => torches.add(on);

  @override
  Future<void> dispose() async {
    disposed = true;
    unawaited(hitController.close());
    unawaited(errorController.close());
  }
}

/// A probe that answers with [state] and counts the download requests.
final class FakeProbe implements ScannerProbe {
  FakeProbe(this.state);

  MlKitState state;
  int requests = 0;

  @override
  Future<MlKitState> mlKitState() async => state;

  @override
  Future<void> requestModel() async => requests++;
}

/// A node with props by permanent ID and the events its handlers take.
final class FakeNode implements NodeContext {
  FakeNode(this.context, this.props, {this.events = const {}, this.refusal});

  /// What the runtime answers when the plugin lacks the camera API.
  final Widget? refusal;

  @override
  final BuildContext context;
  final Map<int, Object?> props;
  final Set<int> events;
  final List<(int, Object?)> fired = [];

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
  bool handles(int id) => events.contains(id);

  @override
  void fire(int id, [Object? payload]) => fired.add((id, payload));

  @override
  Widget? deviceRefusal(String api) => api == 'camera' ? refusal : null;

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Map<String, Object?> _descriptor(String type) => jsonDecode(
  File('../../schema/widgets/layer2/$type.json').readAsStringSync(),
) as Map<String, Object?>;

/// Lets events reach the widget and the frame they cause be built.
Future<void> settle(WidgetTester tester) async {
  for (var i = 0; i < 4; i++) {
    await tester.pump();
  }
}

void main() {
  late List<FakeSession> sessions;
  late FakeProbe probe;
  var now = DateTime(2026);

  PluxScannerConfig config({
    ScannerEngine engine = ScannerEngine.platform,
    bool android = true,
    ScanFrameSourceFactory? frames,
    Duration debounce = const Duration(milliseconds: 1500),
  }) => PluxScannerConfig(
    engine: engine,
    probe: probe,
    android: android,
    debounce: debounce,
    frameSource: frames,
    sessionFactory: (request, engine) {
      final s = FakeSession(request, engine);
      sessions.add(s);
      return s;
    },
  );

  setUp(() {
    sessions = [];
    probe = FakeProbe(MlKitState.ready);
    now = DateTime(2026);
  });

  Future<FakeNode> pump(
    WidgetTester tester,
    int widgetId,
    Map<int, Object?> props, {
    Set<int> events = const {1, 2},
    PluxScannerConfig? cfg,
  }) async {
    late FakeNode node;
    final package = PluxScannerWidgets(config: cfg ?? config());
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SizedBox(
            width: 300,
            height: 400,
            child: Builder(
              builder: (context) {
                node = FakeNode(context, props, events: events);
                return package.builders[widgetId]!(node);
              },
            ),
          ),
        ),
      ),
    );
    await settle(tester);
    return node;
  }

  test('the builders use the IDs of the descriptors and the package name', () {
    final package = PluxScannerWidgets();
    expect(package.name, 'plux_scanner');
    expect(package, isA<PluxWidgetPackage>());
    expect(_descriptor('QrScanner')['id'], PluxScannerIds.qrScanner);
    expect(_descriptor('BarcodeScanner')['id'], PluxScannerIds.barcodeScanner);
    expect(_descriptor('QrCodeView')['id'], PluxScannerIds.qrCodeView);
    expect(package.builders.keys, [199, 200, 201]);
    for (final t in ['QrScanner', 'BarcodeScanner', 'QrCodeView']) {
      expect(_descriptor(t)['package'], 'plux_scanner');
    }
  });

  test('the prop IDs of the builders are those of the descriptors', () {
    List<String> names(String type) => [
      for (final p
          in (_descriptor(type)['props']! as List<Object?>)
              .cast<Map<String, Object?>>())
        '${p['id']}:${p['name']}',
    ];
    expect(names('QrScanner'), [
      '1:torch',
      '2:cameraFacing',
      '3:scanWindow',
      '4:continuous',
      '5:overlayHint',
      '6:semanticLabel',
    ]);
    expect(names('BarcodeScanner'), [
      '1:formats',
      '2:torch',
      '3:cameraFacing',
      '4:scanWindow',
      '5:continuous',
      '6:overlayHint',
      '7:semanticLabel',
    ]);
  });

  testWidgets('QrScanner reads QR codes only and fires onDetected', (
    tester,
  ) async {
    final node = await pump(tester, 199, {});
    expect(sessions, hasLength(1));
    expect(sessions.single.request.formats, ['qrCode']);
    expect(sessions.single.engine, ScannerEngine.platform);
    sessions.single.hitController.add(const ScanHit('ticket-42', 'qrCode'));
    await settle(tester);
    expect(node.fired.single.$1, 1);
    expect(node.fired.single.$2, {'value': 'ticket-42', 'format': 'qrCode'});
  });

  testWidgets('BarcodeScanner passes its formats, by ID or by name', (
    tester,
  ) async {
    await pump(tester, 200, {
      1: [10, 'code128'],
      2: true,
      3: 'front',
    });
    final r = sessions.single.request;
    expect(r.formats, ['ean13', 'code128']);
    expect(r.torch, isTrue);
    expect(r.backCamera, isFalse);
  });

  testWidgets('an unknown format or enum value is ignored, not trusted', (
    tester,
  ) async {
    await pump(tester, 200, {
      1: [99],
      3: 7,
    });
    // A list with an undecodable member decodes to nothing: every format.
    expect(sessions.single.request.formats, isEmpty);
    expect(sessions.single.request.backCamera, isTrue);
  });

  testWidgets('the scan window is given in fractions and shown to the engine', (
    tester,
  ) async {
    await pump(tester, 199, {
      3: [0.1, 0.2, 0.5, 0.25],
    });
    expect(sessions.single.request.scanWindow, [0.1, 0.2, 0.5, 0.25]);
    expect(
      find.textContaining('Rect.fromLTRB(30.0, 80.0, 180.0, 180.0)'),
      findsOneWidget,
    );
    await pump(tester, 199, {
      3: [0.8, 0.2, 0.5, 0.25],
    });
    // A window that leaves the preview is dropped.
    expect(sessions.last.request.scanWindow, isNull);
  });

  testWidgets('the hint and the semantic label are shown', (tester) async {
    final handle = tester.ensureSemantics();
    await pump(tester, 199, {5: 'Point at the code', 6: 'Ticket scanner'});
    expect(find.text('Point at the code'), findsOneWidget);
    expect(find.bySemanticsLabel('Ticket scanner'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('the torch button turns the torch on and off', (tester) async {
    await pump(tester, 199, {});
    expect(find.byIcon(Icons.flash_off), findsOneWidget);
    await tester.tap(find.byIcon(Icons.flash_off));
    await tester.pump();
    expect(find.byIcon(Icons.flash_on), findsOneWidget);
    await tester.tap(find.byIcon(Icons.flash_on));
    await tester.pump();
    expect(sessions.single.torches, [true, false]);
  });

  testWidgets('a changed torch prop reaches the running session', (
    tester,
  ) async {
    var torch = false;
    late StateSetter set;
    final package = PluxScannerWidgets(config: config());
    await tester.pumpWidget(
      MaterialApp(
        home: StatefulBuilder(
          builder: (context, setState) {
            set = setState;
            return Scaffold(
              body: package.builders[199]!(FakeNode(context, {1: torch})),
            );
          },
        ),
      ),
    );
    await tester.pump();
    set(() => torch = true);
    await tester.pump();
    expect(sessions, hasLength(1));
    expect(sessions.single.torches, [true]);
  });

  testWidgets('a scanner without continuous reads once', (tester) async {
    final node = await pump(tester, 199, {});
    final s = sessions.single;
    s.hitController
      ..add(const ScanHit('a', 'qrCode'))
      ..add(const ScanHit('b', 'qrCode'));
    await settle(tester);
    expect(node.fired.map((e) => (e.$2! as Map)['value']), ['a']);
  });

  testWidgets('a continuous scanner debounces repeats of one code', (
    tester,
  ) async {
    final hits = <ScanHit>[];
    late FakeSession session;
    final cfg2 = PluxScannerConfig(
      engine: ScannerEngine.platform,
      probe: probe,
      sessionFactory: (r, e) => session = FakeSession(r, e),
    );
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PluxScannerView(
            config: cfg2,
            continuous: true,
            clock: () => now,
            onDetected: hits.add,
          ),
        ),
      ),
    );
    await tester.pump();
    void emit(String v) => session.hitController.add(ScanHit(v, 'qrCode'));

    emit('a');
    await tester.pump();
    now = now.add(const Duration(milliseconds: 400));
    emit('a');
    await tester.pump();
    expect(hits.map((h) => h.value), [
      'a',
    ], reason: 'a repeat inside the debounce');
    emit('b');
    await tester.pump();
    expect(hits.map((h) => h.value), ['a', 'b'], reason: 'another code passes');
    now = now.add(const Duration(milliseconds: 1600));
    emit('b');
    await tester.pump();
    expect(hits.map((h) => h.value), [
      'a',
      'b',
      'b',
    ], reason: 'after the debounce');
  });

  testWidgets('a read is announced in a live region without the value', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    await pump(tester, 199, {6: 'Scanner'});
    expect(find.bySemanticsLabel('Code read'), findsNothing);
    sessions.single.hitController.add(const ScanHit('secret-token', 'qrCode'));
    await settle(tester);
    final live = tester.getSemantics(find.bySemanticsLabel('Code read'));
    expect(live.flagsCollection.isLiveRegion, isTrue);
    expect(find.bySemanticsLabel(RegExp('secret-token')), findsNothing);
    handle.dispose();
  });

  testWidgets('the viewfinder, the hint and the preview are excluded from '
      'the semantics except the label', (tester) async {
    final handle = tester.ensureSemantics();
    await pump(tester, 199, {6: 'Scanner'});
    expect(find.bySemanticsLabel(RegExp('preview')), findsNothing);
    handle.dispose();
  });

  testWidgets('a denied camera shows the error state, fires onError and '
      'retries', (tester) async {
    final node = await pump(tester, 199, {});
    sessions.single.errorController.add(ScanErrorCodes.permissionDenied);
    await settle(tester);
    expect(find.text('The camera permission was denied'), findsOneWidget);
    expect(find.text('Try again'), findsOneWidget);
    expect(node.fired.single.$1, 2);
    expect(node.fired.single.$2, 'permissionDenied');
    expect(sessions.single.disposed, isTrue);

    await tester.tap(find.text('Try again'));
    await settle(tester);
    expect(sessions, hasLength(2));
    expect(find.text('Try again'), findsNothing);
    expect(find.textContaining('preview'), findsOneWidget);
  });

  testWidgets('an error without a handler for onError does not fire', (
    tester,
  ) async {
    final node = await pump(tester, 199, {}, events: const {1});
    sessions.single.errorController.add(ScanErrorCodes.failed);
    await settle(tester);
    expect(find.text('The camera cannot be used'), findsOneWidget);
    expect(node.fired, isEmpty);
  });

  testWidgets(
    'off Android the zxing engine without a frame source is unsupported',
    (tester) async {
      final cfg = PluxScannerConfig(
        engine: ScannerEngine.zxing,
        probe: probe,
        android: false,
      );
      final node = await pump(tester, 199, {}, cfg: cfg);
      expect(
        find.text('Scanning is not available on this device'),
        findsOneWidget,
      );
      expect(node.fired.single.$1, 2);
      expect(node.fired.single.$2, 'unsupported');
    },
  );

  group('engine choice (decision B21)', () {
    test('a configured engine is taken as it is', () async {
      for (final e in [ScannerEngine.platform, ScannerEngine.zxing]) {
        expect(
          await resolveScannerEngine(
            configured: e,
            android: true,
            probe: probe,
          ),
          e,
        );
      }
      expect(probe.requests, 0);
    });

    test('iOS and other platforms use the platform engine', () async {
      probe.state = MlKitState.unavailable;
      expect(
        await resolveScannerEngine(
          configured: ScannerEngine.auto,
          android: false,
          probe: probe,
        ),
        ScannerEngine.platform,
      );
    });

    test('Android uses ML Kit when the model is ready', () async {
      expect(
        await resolveScannerEngine(
          configured: ScannerEngine.auto,
          android: true,
          probe: probe,
        ),
        ScannerEngine.platform,
      );
      expect(probe.requests, 0);
    });

    for (final state in [MlKitState.downloading, MlKitState.unavailable]) {
      test('Android falls back to ZXing while the model is $state, and asks '
          'for it', () async {
        probe.state = state;
        expect(
          await resolveScannerEngine(
            configured: ScannerEngine.auto,
            android: true,
            probe: probe,
          ),
          ScannerEngine.zxing,
        );
        expect(probe.requests, 1);
      });
    }

    testWidgets('a scanner opened while the model is missing gets the '
        'zxing engine, and the next one the platform engine', (tester) async {
      probe.state = MlKitState.unavailable;
      final cfg = config(engine: ScannerEngine.auto);
      await pump(tester, 199, {}, cfg: cfg);
      expect(sessions.single.engine, ScannerEngine.zxing);
      probe.state = MlKitState.ready;
      await pump(tester, 199, {2: 'front'}, cfg: cfg);
      expect(sessions.last.engine, ScannerEngine.platform);
    });

    test(
      'the channel probe reads the Android plugin and fails closed',
      () async {
        TestWidgetsFlutterBinding.ensureInitialized();
        const channel = MethodChannel('dev.plux/scanner');
        final messenger =
            TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
        final calls = <String>[];
        var answer = 'ready';
        messenger.setMockMethodCallHandler(channel, (call) async {
          calls.add(call.method);
          if (answer == 'throw') {
            throw PlatformException(code: 'boom');
          }
          return answer;
        });
        addTearDown(() => messenger.setMockMethodCallHandler(channel, null));
        const p = ChannelScannerProbe();
        expect(await p.mlKitState(), MlKitState.ready);
        answer = 'downloading';
        expect(await p.mlKitState(), MlKitState.downloading);
        answer = 'something else';
        expect(await p.mlKitState(), MlKitState.unavailable);
        answer = 'throw';
        expect(await p.mlKitState(), MlKitState.unavailable);
        await p.requestModel();
        expect(calls.last, 'requestModel');
        messenger.setMockMethodCallHandler(channel, null);
        expect(await p.mlKitState(), MlKitState.unavailable);
        await p.requestModel();
      },
    );
  });

  // Verifies: SEC-080, RT-060.
  testWidgets('a plugin without the camera API gets the refusal and no '
      'session opens', (tester) async {
    final package = PluxScannerWidgets(config: config());
    for (final id in [199, 200]) {
      await tester.pumpWidget(
        MaterialApp(
          home: Builder(
            builder: (context) => package.builders[id]!(
              FakeNode(context, {}, refusal: const Text('refused')),
            ),
          ),
        ),
      );
      await tester.pump();
      expect(find.text('refused'), findsOneWidget);
    }
    expect(sessions, isEmpty);
  });
}
