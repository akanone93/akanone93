// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The value types of the map descriptors (`LatLng`, `MapMarker`,
/// `MapPolyline`), decoded from a node.
library;

import 'package:flutter/material.dart';
import 'package:latlong2/latlong.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// A `MapMarker`.
@immutable
final class MapMarkerData {
  /// Creates a marker.
  const MapMarkerData({
    required this.id,
    required this.position,
    this.title,
    this.icon,
  });

  /// Identifies the marker in `onMarkerTap`.
  final String id;

  /// Where the marker is.
  final LatLng position;

  /// The name spoken by screen readers.
  final String? title;

  /// The built-in icon name; the default pin when absent or unknown.
  final String? icon;
}

/// A `MapPolyline`.
@immutable
final class MapPolylineData {
  /// Creates a line.
  const MapPolylineData({required this.points, this.color, this.width = 4});

  /// The points, in order.
  final List<LatLng> points;

  /// A theme colour token; the primary colour when absent.
  final String? color;

  /// The width in logical pixels.
  final double width;
}

/// A `LatLng`; null when a field is missing or out of range.
LatLng? asLatLng(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final lat = asDouble(d, f.get(1, 'lat'));
  final lng = asDouble(d, f.get(2, 'lng'));
  if (lat == null || lng == null) return null;
  if (!(lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180)) return null;
  return LatLng(lat, lng);
}

/// A `MapMarker`.
MapMarkerData? asMarker(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final id = asString(d, f.get(1, 'id'));
  final position = asLatLng(d, f.get(2, 'position'));
  if (id == null || position == null) return null;
  return MapMarkerData(
    id: id,
    position: position,
    title: asString(d, f.get(3, 'title')),
    icon: asString(d, f.get(4, 'icon')),
  );
}

/// A `MapPolyline`.
MapPolylineData? asPolyline(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final points = asList(d, f.get(1, 'points'), asLatLng);
  if (points == null) return null;
  final width = asDouble(d, f.get(3, 'width')) ?? 4;
  return MapPolylineData(
    points: points,
    color: asString(d, f.get(2, 'color')),
    width: width.isFinite && width > 0 ? width : 4,
  );
}

/// The theme colour named by [token]; the primary colour when absent or
/// unknown.
Color polylineColor(ColorScheme scheme, String? token) => switch (token) {
  'secondary' => scheme.secondary,
  'tertiary' => scheme.tertiary,
  'error' => scheme.error,
  'onSurface' => scheme.onSurface,
  'outline' => scheme.outline,
  _ => scheme.primary,
};

/// The icon of the built-in marker named [name].
IconData markerIcon(String? name) => switch (name) {
  'store' => Icons.storefront,
  'home' => Icons.home,
  'person' => Icons.person_pin_circle,
  'flag' => Icons.flag,
  'delivery' => Icons.delivery_dining,
  _ => Icons.location_pin,
};
