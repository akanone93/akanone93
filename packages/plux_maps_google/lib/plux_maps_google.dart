// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `MapView` and `OrderTrackerMap` for Plux pages on Google Maps
/// (ADR-0071). Register [PluxMapsGoogle] in `PluxConfig.widgetPackages`
/// instead of `plux_maps`; the host app supplies the Google Maps API key.
library;

export 'src/maps.dart'
    show
        PluxGoogleMapView,
        PluxGoogleOrderTrackerMap,
        courierGlide,
        googleMarkers,
        googlePolylines;
export 'src/model.dart'
    show
        MapMarkerData,
        MapPolylineData,
        asLatLng,
        asMarker,
        asPolyline,
        polylineColor;
export 'src/package.dart' show PluxMapIds, PluxMapsGoogle;
