// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_maps_google/plux_maps_google.dart';

/// A node whose props are given by permanent ID; enough of [NodeContext]
/// for builders that read props and fire events.
final class FakeNode implements NodeContext {
  FakeNode(this.props, {this.handled = const {}});

  final Map<int, Object?> props;
  final Set<int> handled;
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
  bool handles(int id) => handled.contains(id);

  @override
  void fire(int id, [Object? payload]) => fired.add((id, payload));

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Map<String, Object?> _descriptor(String type) => jsonDecode(
  File('../../schema/widgets/layer2/$type.json').readAsStringSync(),
) as Map<String, Object?>;

Map<String, Object?> _ll(double lat, double lng) => {'lat': lat, 'lng': lng};

final _props = <int, Object?>{
  1: 'Branches in Paris',
  2: _ll(48.85, 2.35),
  3: 12.0,
  4: 3.0,
  5: 18.0,
  6: [
    {'id': 'a', 'position': _ll(48.85, 2.35), 'title': 'Louvre branch'},
    {'id': 'b', 'position': _ll(48.86, 2.36)},
  ],
  7: [
    {
      'points': [_ll(48.85, 2.35), _ll(48.86, 2.36)],
      'width': 3.0,
    },
    {
      'points': [_ll(48.85, 2.35)],
    },
  ],
  8: 'https://tiles.example.com/{z}/{x}/{y}.png',
  9: '© ignored',
  11: false,
};

/// Builds the widget of [id] without rendering it: the Google map is a
/// platform view.
Widget _build(int id, Map<int, Object?> props) =>
    const PluxMapsGoogle().builders[id]!(FakeNode(props));

void main() {
  // Verifies: SCH-000.
  test('the widget IDs are those of the descriptors and of plux_maps', () {
    expect(_descriptor('MapView')['id'], PluxMapIds.mapView);
    expect(_descriptor('OrderTrackerMap')['id'], PluxMapIds.orderTrackerMap);
    expect(const PluxMapsGoogle().name, _descriptor('MapView')['package']);
    expect(const PluxMapsGoogle().builders.keys, [197, 198]);
  });

  test('MapView decodes into a GoogleMap description', () {
    final w = _build(PluxMapIds.mapView, _props) as PluxGoogleMapView;
    expect(w.semanticLabel, 'Branches in Paris');
    expect(w.center, const LatLng(48.85, 2.35));
    expect(w.zoom, 12);
    expect(w.minZoom, 3);
    expect(w.maxZoom, 18);
    expect(w.interactive, isFalse);
    expect(w.markers.map((m) => m.id), ['a', 'b']);
    expect(w.onMarkerTap, isNull);

    final markers = googleMarkers(w.markers);
    expect(markers.map((m) => m.markerId.value), {'a', 'b'});
    final a = markers.firstWhere((m) => m.markerId.value == 'a');
    expect(a.position, const LatLng(48.85, 2.35));
    expect(a.infoWindow.title, 'Louvre branch');

    final lines = googlePolylines(w.polylines, ThemeData().colorScheme);
    expect(lines, hasLength(1));
    expect(lines.single.width, 3);
    expect(lines.single.points, hasLength(2));
  });

  testWidgets('MapView builds a GoogleMap with converted markers', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PluxGoogleMapView(
            semanticLabel: 'Branches',
            center: const LatLng(48.85, 2.35),
            markers: const [
              MapMarkerData(
                id: 'a',
                position: LatLng(48.85, 2.35),
                title: 'Louvre branch',
              ),
            ],
            onMarkerTap: (_) {},
          ),
        ),
      ),
    );
    final map = tester.widget<GoogleMap>(find.byType(GoogleMap));
    expect(map.markers.single.markerId.value, 'a');
    expect(map.markers.single.infoWindow.title, 'Louvre branch');
    expect(map.initialCameraPosition.zoom, 13);
    expect(map.layoutDirection, TextDirection.ltr);
    map.markers.single.onTap!();
  });

  testWidgets('a marker tap fires onMarkerTap with the marker id', (
    tester,
  ) async {
    String? tapped;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PluxGoogleMapView(
            semanticLabel: 'Branches',
            center: const LatLng(48.85, 2.35),
            markers: const [
              MapMarkerData(id: 'a', position: LatLng(48.85, 2.35)),
            ],
            onMarkerTap: (id) => tapped = id,
          ),
        ),
      ),
    );
    tester.widget<GoogleMap>(find.byType(GoogleMap)).markers.single.onTap!();
    expect(tapped, 'a');
  });

  testWidgets('the map is labelled and follows the text direction', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(
      MaterialApp(
        home: Directionality(
          textDirection: TextDirection.rtl,
          child: Scaffold(
            body: PluxGoogleMapView(
              semanticLabel: 'Branches',
              center: const LatLng(48.85, 2.35),
            ),
          ),
        ),
      ),
    );
    expect(find.bySemanticsLabel('Branches'), findsOneWidget);
    expect(
      tester.widget<GoogleMap>(find.byType(GoogleMap)).layoutDirection,
      TextDirection.rtl,
    );
    handle.dispose();
  });

  testWidgets('OrderTrackerMap builds markers and shows the estimate', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PluxGoogleOrderTrackerMap(
            semanticLabel: 'Your order',
            origin: const LatLng(48.85, 2.35),
            destination: const LatLng(48.87, 2.38),
            courier: const LatLng(48.86, 2.36),
            route: const [LatLng(48.85, 2.35), LatLng(48.87, 2.38)],
            etaLabel: 'Arrives in 5 min',
            courierLabel: 'Courier',
          ),
        ),
      ),
    );
    final map = tester.widget<GoogleMap>(find.byType(GoogleMap));
    expect(map.markers.map((m) => m.markerId.value), {
      'origin',
      'destination',
      'courier',
    });
    expect(
      map.markers
          .firstWhere((m) => m.markerId.value == 'courier')
          .infoWindow
          .title,
      'Courier',
    );
    expect(map.polylines, hasLength(1));
    expect(find.text('Arrives in 5 min'), findsOneWidget);
  });

  Widget mapApp({bool interactive = true, TextDirection? dir}) => MaterialApp(
    home: Directionality(
      textDirection: dir ?? TextDirection.ltr,
      child: Scaffold(
        body: PluxGoogleMapView(
          semanticLabel: 'Branches',
          center: const LatLng(48.85, 2.35),
          interactive: interactive,
        ),
      ),
    ),
  );

  // Verifies: A11Y-001, A11Y-004.
  testWidgets('zoom buttons are labelled 48 px targets and replace the '
      'native controls', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(mapApp());
    expect(find.bySemanticsLabel('Zoom in'), findsOneWidget);
    expect(find.bySemanticsLabel('Zoom out'), findsOneWidget);
    expect(
      tester.getSize(find.byTooltip('Zoom in')).shortestSide,
      greaterThanOrEqualTo(48),
    );
    expect(
      tester.widget<GoogleMap>(find.byType(GoogleMap)).zoomControlsEnabled,
      isFalse,
    );
    await tester.tap(find.byTooltip('Zoom in'));
    handle.dispose();
  });

  // Verifies: A11Y-001, I18N-005.
  testWidgets('the zoom buttons sit at the end corner in both directions', (
    tester,
  ) async {
    await tester.pumpWidget(mapApp());
    expect(tester.getCenter(find.byTooltip('Zoom in')).dx, greaterThan(400));
    await tester.pumpWidget(mapApp(dir: TextDirection.rtl));
    expect(tester.getCenter(find.byTooltip('Zoom in')).dx, lessThan(400));
  });

  // Verifies: A11Y-001.
  testWidgets('a map that is not interactive has no zoom buttons', (
    tester,
  ) async {
    await tester.pumpWidget(mapApp(interactive: false));
    expect(find.byTooltip('Zoom in'), findsNothing);
  });
}
