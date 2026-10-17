// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package: builders that read the nodes of `MapView` and
/// `OrderTrackerMap`. The IDs are those of the descriptors in
/// `schema/widgets/layer2`; a test compares them.
library;

import 'package:flutter/widgets.dart';
import 'package:latlong2/latlong.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_maps/src/maps.dart';
import 'package:plux_maps/src/model.dart';

/// The widget IDs of the package's descriptors.
abstract final class PluxMapIds {
  /// `MapView`.
  static const int mapView = 197;

  /// `OrderTrackerMap`.
  static const int orderTrackerMap = 198;
}

/// The map widgets of Plux pages on `flutter_map`, to register in
/// `PluxConfig.widgetPackages`.
final class PluxMaps implements PluxWidgetPackage {
  /// Creates the package.
  const PluxMaps();

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

/// The tile template and attribution of a node; the attribution is required
/// with a template.
(String?, String?) _tiles(NodeContext c, String widget) {
  final template = c.decode(8, asString);
  final attribution = c.decode(9, asString);
  if (template != null && attribution == null) {
    c.missing('$widget.attribution');
  }
  return (template, attribution);
}

Widget _mapView(NodeContext c) {
  final (template, attribution) = _tiles(c, 'MapView');
  return PluxMapView(
    semanticLabel: c.decode(1, asString) ?? c.missing('MapView.semanticLabel'),
    center: c.decode(2, asLatLng) ?? c.missing('MapView.center'),
    zoom: c.decode(3, asDouble) ?? 13,
    minZoom: c.decode(4, asDouble) ?? 2,
    maxZoom: c.decode(5, asDouble) ?? 19,
    markers: c.decode(6, (d, v) => asList(d, v, asMarker)) ?? const [],
    polylines: c.decode(7, (d, v) => asList(d, v, asPolyline)) ?? const [],
    tileUrlTemplate: template,
    attribution: attribution,
    clientFor: template == null ? null : c.remoteClient,
    interactive: c.decode(11, asBool) ?? true,
    onMarkerTap: c.handles(1) ? (id) => c.fire(1, id) : null,
    onMapTap: c.handles(2) ? (p) => c.fire(2, _latLng(p)) : null,
    onCameraIdle: c.handles(3)
        ? (center, zoom) => c.fire(3, {'center': _latLng(center), 'zoom': zoom})
        : null,
  );
}

Widget _tracker(NodeContext c) {
  final (template, attribution) = _tiles(c, 'OrderTrackerMap');
  return PluxOrderTrackerMap(
    semanticLabel:
        c.decode(1, asString) ?? c.missing('OrderTrackerMap.semanticLabel'),
    origin: c.decode(2, asLatLng) ?? c.missing('OrderTrackerMap.origin'),
    destination:
        c.decode(3, asLatLng) ?? c.missing('OrderTrackerMap.destination'),
    courier: c.decode(4, asLatLng),
    route: c.decode(5, (d, v) => asList(d, v, asLatLng)) ?? const [],
    etaLabel: c.decode(6, asString),
    follow: c.decode(7, asBool) ?? true,
    tileUrlTemplate: template,
    attribution: attribution,
    clientFor: template == null ? null : c.remoteClient,
    originLabel: c.decode(10, asString),
    destinationLabel: c.decode(11, asString),
    courierLabel: c.decode(12, asString),
  );
}
