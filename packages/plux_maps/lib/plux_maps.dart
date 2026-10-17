// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `MapView` and `OrderTrackerMap` for Plux pages on `flutter_map`
/// (ADR-0071). Register [PluxMaps] in `PluxConfig.widgetPackages`; tiles are
/// fetched through the runtime's pinned client from domains the plugin
/// declares (SEC-080).
library;

export 'src/maps.dart'
    show PluxMapCanvas, PluxMapView, PluxOrderTrackerMap, courierGlide;
export 'src/model.dart'
    show
        MapMarkerData,
        MapPolylineData,
        asLatLng,
        asMarker,
        asPolyline,
        markerIcon,
        polylineColor;
export 'src/package.dart' show PluxMapIds, PluxMaps;
export 'src/tiles.dart' show PluxTileProvider, TileImageFor, transparentTile;
