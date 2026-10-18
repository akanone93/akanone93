// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart' show MapController;
import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_maps/plux_maps.dart';

/// A node whose props are given by permanent ID; enough of [NodeContext]
/// for builders that read props, fire events and fetch tiles.
final class FakeNode implements NodeContext {
  FakeNode(
    this.context,
    this.props, {
    this.handled = const {},
    this.allowed = 'tiles.example.com',
    this.refusal,
  });

  /// What the runtime answers when the plugin lacks the location API.
  final Widget? refusal;

  @override
  final BuildContext context;
  final Map<int, Object?> props;
  final Set<int> handled;
  final String allowed;
  final List<(int, Object?)> fired = [];
  final List<String> asked = [];

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
  bool handles(int id) => handled.contains(id);

  @override
  void fire(int id, [Object? payload]) => fired.add((id, payload));

  @override
  ImageProvider<Object>? remoteImage(String url) {
    asked.add(url);
    return Uri.parse(url).host == allowed ? MemoryImage(transparentTile) : null;
  }

  @override
  Widget? deviceRefusal(String api) => api == 'location' ? refusal : null;

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Map<String, Object?> _descriptor(String type) => jsonDecode(
  File('../../schema/widgets/layer2/$type.json').readAsStringSync(),
) as Map<String, Object?>;

Map<String, Object?> _ll(double lat, double lng) => {'lat': lat, 'lng': lng};

Map<int, Object?> _mapProps({
  bool tiles = true,
  String host = 'tiles.example.com',
}) => {
  1: 'Branches in Paris',
  2: _ll(48.85, 2.35),
  3: 13.0,
  6: [
    {'id': 'a', 'position': _ll(48.85, 2.35), 'title': 'Louvre branch'},
    {'id': 'b', 'position': _ll(48.86, 2.36)},
  ],
  7: [
    {
      'points': [_ll(48.85, 2.35), _ll(48.86, 2.36)],
      'width': 3.0,
    },
  ],
  if (tiles) 8: 'https://$host/{z}/{x}/{y}.png',
  if (tiles) 9: '© Example tiles',
};

Widget _app(Widget child, {TextDirection dir = TextDirection.ltr}) =>
    MaterialApp(
      debugShowCheckedModeBanner: false,
      home: Directionality(
        textDirection: dir,
        child: Scaffold(body: child),
      ),
    );

Future<FakeNode> _pump(
  WidgetTester tester,
  int id,
  Map<int, Object?> props, {
  Set<int> handled = const {},
  String host = 'tiles.example.com',
  TextDirection dir = TextDirection.ltr,
}) async {
  late FakeNode node;
  await tester.pumpWidget(
    _app(
      Builder(
        builder: (context) {
          node = FakeNode(context, props, handled: handled, allowed: host);
          return const PluxMaps().builders[id]!(node);
        },
      ),
      dir: dir,
    ),
  );
  await tester.pump(const Duration(milliseconds: 100));
  return node;
}

void main() {
  // Verifies: SCH-000.
  test('the widget IDs are those of the descriptors', () {
    expect(_descriptor('MapView')['id'], PluxMapIds.mapView);
    expect(_descriptor('OrderTrackerMap')['id'], PluxMapIds.orderTrackerMap);
    expect(const PluxMaps().name, _descriptor('MapView')['package']);
    expect(const PluxMaps().builders.keys, [197, 198]);
  });

  // Verifies: SEC-080, AST-002, LIM-001.
  testWidgets('tiles are the remote images of the runtime, the template '
      'filled per tile', (tester) async {
    final node = await _pump(tester, PluxMapIds.mapView, _mapProps());
    await tester.pump(const Duration(milliseconds: 100));
    expect(node.asked, isNotEmpty);
    expect(
      node.asked.every(
        (u) =>
            RegExp(r'^https://tiles\.example\.com/\d+/\d+/\d+\.png$')
                .hasMatch(u),
      ),
      isTrue,
    );
    expect(find.text('© Example tiles'), findsOneWidget);
  });

  // Verifies: SEC-080, RT-060.
  testWidgets('a blocked tile domain draws no tiles and asks once '
      '(the runtime reports PLX-6030 once)', (tester) async {
    final node = await _pump(
      tester,
      PluxMapIds.mapView,
      _mapProps(host: 'evil.example.net'),
    );
    await tester.pump(const Duration(milliseconds: 100));
    expect(node.asked, hasLength(1));
    expect(find.text('© Example tiles'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('without a template no tile is asked for', (tester) async {
    final node = await _pump(
      tester,
      PluxMapIds.mapView,
      _mapProps(tiles: false),
    );
    expect(node.asked, isEmpty);
    expect(find.byKey(const ValueKey('marker:a')), findsOneWidget);
  });

  testWidgets('a template without attribution is a missing prop', (
    tester,
  ) async {
    final props = _mapProps()..remove(9);
    await tester.pumpWidget(
      _app(
        Builder(
          builder: (context) {
            try {
              const PluxMaps().builders[PluxMapIds.mapView]!(
                FakeNode(context, props),
              );
            } on DecodeError catch (e) {
              return Text(e.message);
            }
            return const Text('built');
          },
        ),
      ),
    );
    expect(find.textContaining('MapView.attribution'), findsOneWidget);
  });

  // Verifies: SEC-041.
  testWidgets('a marker tap fires onMarkerTap with the marker id', (
    tester,
  ) async {
    final node = await _pump(
      tester,
      PluxMapIds.mapView,
      _mapProps(tiles: false),
      handled: {1, 2},
    );
    await tester.tap(find.byKey(const ValueKey('marker:a')));
    await tester.pump(const Duration(milliseconds: 400));
    expect(node.fired, [(1, 'a')]);
  });

  testWidgets('a tap on the map fires onMapTap with the position', (
    tester,
  ) async {
    final node = await _pump(
      tester,
      PluxMapIds.mapView,
      _mapProps(tiles: false),
      handled: {1, 2},
    );
    await tester.tapAt(const Offset(30, 500));
    await tester.pump(const Duration(milliseconds: 400));
    expect(node.fired, hasLength(1));
    expect(node.fired.single.$1, 2);
    final p = node.fired.single.$2! as Map<String, Object?>;
    expect(p['lat'], isA<double>());
    expect(p['lng'], isA<double>());
  });

  testWidgets('a drag fires onCameraIdle with centre and zoom', (tester) async {
    final node = await _pump(
      tester,
      PluxMapIds.mapView,
      _mapProps(tiles: false),
      handled: {3},
    );
    await tester.drag(find.byType(PluxMapView), const Offset(120, 0));
    await tester.pump(const Duration(seconds: 2));
    final idle = node.fired.where((e) => e.$1 == 3);
    expect(idle, isNotEmpty);
    final payload = idle.last.$2! as Map<String, Object?>;
    expect((payload['center']! as Map<String, Object?>)['lng'], isA<double>());
    expect(payload['zoom'], 13.0);
  });

  testWidgets('markers are semantics nodes with their titles', (tester) async {
    final handle = tester.ensureSemantics();
    await _pump(
      tester,
      PluxMapIds.mapView,
      _mapProps(tiles: false),
      handled: {1},
    );
    expect(find.bySemanticsLabel('Louvre branch'), findsOneWidget);
    expect(find.bySemanticsLabel('Branches in Paris'), findsOneWidget);
    final node = tester.getSemantics(find.bySemanticsLabel('Louvre branch'));
    expect(node.flagsCollection.isButton, isTrue);
    handle.dispose();
  });

  testWidgets('the attribution sits at the end corner in both directions', (
    tester,
  ) async {
    await _pump(tester, PluxMapIds.mapView, _mapProps());
    final ltr = tester.getCenter(find.text('© Example tiles')).dx;
    expect(ltr, greaterThan(400));
    await _pump(
      tester,
      PluxMapIds.mapView,
      _mapProps(),
      dir: TextDirection.rtl,
    );
    final rtl = tester.getCenter(find.text('© Example tiles')).dx;
    expect(rtl, lessThan(400));
  });

  testWidgets('a map in an unbounded column gets a height', (tester) async {
    await tester.pumpWidget(
      _app(
        SingleChildScrollView(
          child: PluxMapView(
            semanticLabel: 'Map',
            center: const LatLng(48.85, 2.35),
          ),
        ),
      ),
    );
    expect(tester.getSize(find.byType(PluxMapView)).height, 600);
  });

  testWidgets('OrderTrackerMap shows the markers, labels and the estimate', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    await _pump(tester, PluxMapIds.orderTrackerMap, {
      1: 'Your order',
      2: _ll(48.85, 2.35),
      3: _ll(48.87, 2.38),
      4: _ll(48.86, 2.36),
      5: [_ll(48.85, 2.35), _ll(48.86, 2.36), _ll(48.87, 2.38)],
      6: 'Arrives in 5 min',
      10: 'Restaurant',
      11: 'Your home',
      12: 'Courier',
    });
    expect(find.text('Arrives in 5 min'), findsOneWidget);
    expect(find.bySemanticsLabel('Restaurant'), findsOneWidget);
    expect(find.bySemanticsLabel('Your home'), findsOneWidget);
    expect(find.bySemanticsLabel('Courier'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('the courier marker glides to a new position', (tester) async {
    var courier = _ll(48.855, 2.355);
    late StateSetter set;
    await tester.pumpWidget(
      _app(
        StatefulBuilder(
          builder: (context, setState) {
            set = setState;
            return Builder(
              builder: (context) =>
                  const PluxMaps().builders[PluxMapIds.orderTrackerMap]!(
                    FakeNode(context, {
                      1: 'Your order',
                      2: _ll(48.85, 2.35),
                      3: _ll(48.87, 2.38),
                      4: courier,
                    }),
                  ),
            );
          },
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    final key = find.byKey(const ValueKey('marker:courier'));
    final before = tester.getCenter(key);
    set(() => courier = _ll(48.865, 2.37));
    await tester.pump();
    await tester.pump(courierGlide ~/ 2);
    final mid = tester.getCenter(key);
    await tester.pump(courierGlide);
    final end = tester.getCenter(key);
    expect(mid, isNot(before));
    expect(mid, isNot(end));
    expect(end.dx, greaterThan(before.dx));
  });

  // Verifies: SEC-080, RT-060.
  testWidgets('showing the user needs the location API; a plain map does '
      'not', (tester) async {
    late Widget shown;
    late Widget plain;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          const maps = PluxMaps();
          final refusal = const Text('refused');
          shown = maps.builders[PluxMapIds.mapView]!(
            FakeNode(
              context,
              _mapProps(tiles: false)..[10] = true,
              refusal: refusal,
            ),
          );
          plain = maps.builders[PluxMapIds.mapView]!(
            FakeNode(context, _mapProps(tiles: false), refusal: refusal),
          );
          return const SizedBox();
        },
      ),
    );
    expect(shown, isA<Text>());
    expect(plain, isA<PluxMapView>());
  });

  Future<MapController> pumpCanvas(
    WidgetTester tester, {
    bool interactive = true,
    TextDirection dir = TextDirection.ltr,
  }) async {
    final controller = MapController();
    await tester.pumpWidget(
      _app(
        PluxMapCanvas(
          controller: controller,
          semanticLabel: 'Map',
          center: const LatLng(48.85, 2.35),
          zoom: 10,
          overlays: const [],
          interactive: interactive,
        ),
        dir: dir,
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    return controller;
  }

  // Verifies: A11Y-001, A11Y-004.
  testWidgets('zoom buttons are labelled 48 px targets that zoom the map', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    final controller = await pumpCanvas(tester);
    expect(find.bySemanticsLabel('Zoom in'), findsOneWidget);
    expect(find.bySemanticsLabel('Zoom out'), findsOneWidget);
    expect(
      tester.getSize(find.byTooltip('Zoom in')).shortestSide,
      greaterThanOrEqualTo(48),
    );
    await tester.tap(find.byTooltip('Zoom in'));
    await tester.pump(const Duration(milliseconds: 100));
    expect(controller.camera.zoom, 11);
    await tester.tap(find.byTooltip('Zoom out'));
    await tester.tap(find.byTooltip('Zoom out'));
    await tester.pump(const Duration(milliseconds: 100));
    expect(controller.camera.zoom, 9);
    handle.dispose();
  });

  // Verifies: A11Y-001, I18N-005.
  testWidgets('the zoom buttons sit at the end corner in both directions', (
    tester,
  ) async {
    await pumpCanvas(tester);
    expect(tester.getCenter(find.byTooltip('Zoom in')).dx, greaterThan(400));
    await pumpCanvas(tester, dir: TextDirection.rtl);
    expect(tester.getCenter(find.byTooltip('Zoom in')).dx, lessThan(400));
  });

  // Verifies: A11Y-001.
  testWidgets('a map that is not interactive has no zoom buttons', (
    tester,
  ) async {
    await pumpCanvas(tester, interactive: false);
    expect(find.byTooltip('Zoom in'), findsNothing);
    expect(find.byTooltip('Zoom out'), findsNothing);
  });
}
