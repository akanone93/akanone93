// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package: builders that read the nodes of `MapView` and
/// `OrderTrackerMap` for the Google provider. The IDs are those of the
/// descriptors in `schema/widgets/layer2`; a test compares them.
library;

import 'package:flutter/widgets.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_maps_google/src/maps.dart';
import 'package:plux_maps_google/src/model.dart';

/// The widget IDs of the package's descriptors; those of `plux_maps`.
abstract final class PluxMapIds {
  /// `MapView`.
  static const int mapView = 197;

  /// `OrderTrackerMap`.
  static const int orderTrackerMap = 198;
}

/// The map widgets of Plux pages on Google Maps, to register in
/// `PluxConfig.widgetPackages` instead of `plux_maps` (never both: they
/// provide the same widget IDs).
final class PluxMapsGoogle implements PluxWidgetPackage {
  /// Creates the package.
  const PluxMapsGoogle();

  /// `plux_maps`: the name the descriptors of the map widgets carry in
  /// `package`, which this package provides in place of the OpenStreetMap
  /// one.
  @override
  String get name => 'plux_maps';

  @override
  Map<int, NodeBuilder> get builders => {
    PluxMapIds.mapView: _mapView,
    PluxMapIds.orderTrackerMap: _tracker,
  };
}

Map<String, Object?> _latLng(LatLng p) => {
  'lat': p.latitude,
  'lng': p.longitude,
};

Widget _mapView(NodeContext c) => PluxGoogleMapView(
  semanticLabel: c.decode(1, asString) ?? c.missing('MapView.semanticLabel'),
  center: c.decode(2, asLatLng) ?? c.missing('MapView.center'),
  zoom: c.decode(3, asDouble) ?? 13,
  minZoom: c.decode(4, asDouble) ?? 2,
  maxZoom: c.decode(5, asDouble) ?? 19,
  markers: c.decode(6, (d, v) => asList(d, v, asMarker)) ?? const [],
  polylines: c.decode(7, (d, v) => asList(d, v, asPolyline)) ?? const [],
  interactive: c.decode(11, asBool) ?? true,
  onMarkerTap: c.handles(1) ? (id) => c.fire(1, id) : null,
  onMapTap: c.handles(2) ? (p) => c.fire(2, _latLng(p)) : null,
  onCameraIdle: c.handles(3)
      ? (center, zoom) => c.fire(3, {'center': _latLng(center), 'zoom': zoom})
      : null,
);

Widget _tracker(NodeContext c) => PluxGoogleOrderTrackerMap(
  semanticLabel:
      c.decode(1, asString) ?? c.missing('OrderTrackerMap.semanticLabel'),
  origin: c.decode(2, asLatLng) ?? c.missing('OrderTrackerMap.origin'),
  destination:
      c.decode(3, asLatLng) ?? c.missing('OrderTrackerMap.destination'),
  courier: c.decode(4, asLatLng),
  route: c.decode(5, (d, v) => asList(d, v, asLatLng)) ?? const [],
  etaLabel: c.decode(6, asString),
  follow: c.decode(7, asBool) ?? true,
  originLabel: c.decode(10, asString),
  destinationLabel: c.decode(11, asString),
  courierLabel: c.decode(12, asString),
);
