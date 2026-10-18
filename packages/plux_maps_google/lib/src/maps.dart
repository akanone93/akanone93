// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `MapView` and `OrderTrackerMap` on `google_maps_flutter`.
library;

import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:plux_maps_google/src/model.dart';
import 'package:plux_maps_google/src/zoom_controls.dart';

/// Called with the id of a tapped marker.
typedef MarkerTapped = void Function(String id);

/// Called with the position of a tap on the map.
typedef MapTapped = void Function(LatLng position);

/// Called when the camera stops moving.
typedef CameraIdle = void Function(LatLng center, double zoom);

/// The Google markers of [markers]: the title is the info window's title,
/// which the platform view exposes to screen readers.
Set<Marker> googleMarkers(List<MapMarkerData> markers, {MarkerTapped? onTap}) =>
    {
      for (final m in markers)
        Marker(
          markerId: MarkerId(m.id),
          position: m.position,
          infoWindow: InfoWindow(title: m.title),
          onTap: onTap == null ? null : () => onTap(m.id),
        ),
    };

/// The Google polylines of [lines].
Set<Polyline> googlePolylines(
  List<MapPolylineData> lines,
  ColorScheme scheme,
) => {
  for (var i = 0; i < lines.length; i++)
    if (lines[i].points.length >= 2)
      Polyline(
        polylineId: PolylineId('line$i'),
        points: lines[i].points,
        width: lines[i].width.round(),
        color: polylineColor(scheme, lines[i].color),
      ),
};

Widget _sized(Widget Function(BuildContext) build) => LayoutBuilder(
  builder: (context, c) => SizedBox(
    height: c.hasBoundedHeight
        ? c.maxHeight
        : (c.hasBoundedWidth ? c.maxWidth * 0.75 : 300.0),
    child: build(context),
  ),
);

/// `MapView`.
final class PluxGoogleMapView extends StatefulWidget {
  /// Creates the map.
  const PluxGoogleMapView({
    required this.semanticLabel,
    required this.center,
    super.key,
    this.zoom = 13,
    this.minZoom = 2,
    this.maxZoom = 19,
    this.markers = const [],
    this.polylines = const [],
    this.interactive = true,
    this.onMarkerTap,
    this.onMapTap,
    this.onCameraIdle,
  });

  /// What the map shows.
  final String semanticLabel;

  /// The centre.
  final LatLng center;

  /// The zoom.
  final double zoom;

  /// The lowest zoom.
  final double minZoom;

  /// The highest zoom.
  final double maxZoom;

  /// The markers.
  final List<MapMarkerData> markers;

  /// The lines.
  final List<MapPolylineData> polylines;

  /// Whether the user can pan and zoom.
  final bool interactive;

  /// A marker was tapped.
  final MarkerTapped? onMarkerTap;

  /// The map was tapped.
  final MapTapped? onMapTap;

  /// The camera stopped moving.
  final CameraIdle? onCameraIdle;

  @override
  State<PluxGoogleMapView> createState() => _PluxGoogleMapViewState();
}

final class _PluxGoogleMapViewState extends State<PluxGoogleMapView> {
  GoogleMapController? _controller;
  late CameraPosition _camera = CameraPosition(
    target: widget.center,
    zoom: widget.zoom,
  );

  @override
  void didUpdateWidget(PluxGoogleMapView old) {
    super.didUpdateWidget(old);
    if (old.center != widget.center || old.zoom != widget.zoom) {
      final c = _controller;
      if (c != null) {
        unawaited(
          c.moveCamera(CameraUpdate.newLatLngZoom(widget.center, widget.zoom)),
        );
      }
    }
  }

  void _zoomBy(double delta) {
    final c = _controller;
    if (c != null) unawaited(c.animateCamera(CameraUpdate.zoomBy(delta)));
  }

  @override
  void dispose() {
    _controller?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final idle = widget.onCameraIdle;
    final maxZoom = math.max(widget.minZoom, widget.maxZoom);
    return _sized(
      (context) => Semantics(
        container: true,
        label: widget.semanticLabel,
        child: Stack(
          fit: StackFit.expand,
          children: [
            GoogleMap(
              initialCameraPosition: _camera,
              minMaxZoomPreference: MinMaxZoomPreference(
                widget.minZoom,
                maxZoom,
              ),
              markers: googleMarkers(widget.markers, onTap: widget.onMarkerTap),
              polylines: googlePolylines(
                widget.polylines,
                Theme.of(context).colorScheme,
              ),
              scrollGesturesEnabled: widget.interactive,
              zoomGesturesEnabled: widget.interactive,
              rotateGesturesEnabled: false,
              tiltGesturesEnabled: false,
              zoomControlsEnabled: false,
              mapToolbarEnabled: false,
              myLocationButtonEnabled: false,
              layoutDirection: Directionality.of(context),
              onMapCreated: (c) => _controller = c,
              onTap: widget.onMapTap,
              onCameraMove: (p) => _camera = p,
              onCameraIdle: idle == null
                  ? null
                  : () => idle(_camera.target, _camera.zoom),
            ),
            if (widget.interactive)
              PositionedDirectional(
                top: 8,
                end: 8,
                child: PluxMapZoomControls(onZoom: _zoomBy),
              ),
          ],
        ),
      ),
    );
  }
}

/// How long the courier marker takes to glide to a new position.
const Duration courierGlide = Duration(milliseconds: 800);

/// `OrderTrackerMap`.
final class PluxGoogleOrderTrackerMap extends StatefulWidget {
  /// Creates the tracker.
  const PluxGoogleOrderTrackerMap({
    required this.semanticLabel,
    required this.origin,
    required this.destination,
    super.key,
    this.courier,
    this.route = const [],
    this.etaLabel,
    this.follow = true,
    this.originLabel,
    this.destinationLabel,
    this.courierLabel,
  });

  /// What the map shows.
  final String semanticLabel;

  /// Where the order starts.
  final LatLng origin;

  /// Where the order goes.
  final LatLng destination;

  /// The courier's position.
  final LatLng? courier;

  /// The route.
  final List<LatLng> route;

  /// The arrival estimate.
  final String? etaLabel;

  /// Whether to keep the courier in view.
  final bool follow;

  /// The origin marker's spoken name.
  final String? originLabel;

  /// The destination marker's spoken name.
  final String? destinationLabel;

  /// The courier marker's spoken name.
  final String? courierLabel;

  @override
  State<PluxGoogleOrderTrackerMap> createState() =>
      _PluxGoogleOrderTrackerMapState();
}

final class _PluxGoogleOrderTrackerMapState
    extends State<PluxGoogleOrderTrackerMap>
    with SingleTickerProviderStateMixin {
  late final AnimationController _glide = AnimationController(
    vsync: this,
    duration: courierGlide,
  )..addListener(_follow);
  GoogleMapController? _controller;
  LatLng? _from;
  LatLng? _to;

  @override
  void initState() {
    super.initState();
    _to = widget.courier;
  }

  LatLng? get _courier {
    final to = _to;
    final from = _from;
    if (to == null || from == null) return to;
    final t = Curves.easeInOut.transform(_glide.value);
    return LatLng(
      from.latitude + (to.latitude - from.latitude) * t,
      from.longitude + (to.longitude - from.longitude) * t,
    );
  }

  @override
  void didUpdateWidget(PluxGoogleOrderTrackerMap old) {
    super.didUpdateWidget(old);
    final next = widget.courier;
    if (next == _to) return;
    _from = _courier ?? next;
    _to = next;
    if (next == null || MediaQuery.disableAnimationsOf(context)) {
      _from = next;
      _glide.value = 1;
    } else {
      _glide.forward(from: 0);
    }
  }

  Future<void> _follow() async {
    final at = _courier;
    final controller = _controller;
    if (!widget.follow || controller == null || at == null) return;
    final bounds = await controller.getVisibleRegion();
    if (!mounted) return;
    final inside =
        at.latitude >= bounds.southwest.latitude &&
        at.latitude <= bounds.northeast.latitude &&
        at.longitude >= bounds.southwest.longitude &&
        at.longitude <= bounds.northeast.longitude;
    if (!inside) await controller.moveCamera(CameraUpdate.newLatLng(at));
  }

  @override
  void dispose() {
    _glide.dispose();
    _controller?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final eta = widget.etaLabel;
    return _sized(
      (context) => Semantics(
        container: true,
        label: widget.semanticLabel,
        child: Stack(
          fit: StackFit.expand,
          children: [
            AnimatedBuilder(
              animation: _glide,
              builder: (context, _) {
                final at = _courier;
                return GoogleMap(
                  initialCameraPosition: CameraPosition(
                    target: widget.destination,
                    zoom: 13,
                  ),
                  markers: googleMarkers([
                    MapMarkerData(
                      id: 'origin',
                      position: widget.origin,
                      title: widget.originLabel,
                    ),
                    MapMarkerData(
                      id: 'destination',
                      position: widget.destination,
                      title: widget.destinationLabel,
                    ),
                    if (at != null)
                      MapMarkerData(
                        id: 'courier',
                        position: at,
                        title: widget.courierLabel,
                      ),
                  ]),
                  polylines: {
                    if (widget.route.length >= 2)
                      Polyline(
                        polylineId: const PolylineId('route'),
                        points: widget.route,
                        width: 4,
                        color: theme.colorScheme.primary,
                      ),
                  },
                  rotateGesturesEnabled: false,
                  tiltGesturesEnabled: false,
                  mapToolbarEnabled: false,
                  myLocationButtonEnabled: false,
                  layoutDirection: Directionality.of(context),
                  onMapCreated: (c) {
                    _controller = c;
                    unawaited(
                      c.moveCamera(
                        CameraUpdate.newLatLngBounds(
                          _bounds([
                            widget.origin,
                            widget.destination,
                            if (widget.courier != null) widget.courier!,
                          ]),
                          48,
                        ),
                      ),
                    );
                  },
                );
              },
            ),
            PositionedDirectional(
              top: 8,
              end: 8,
              child: PluxMapZoomControls(
                onZoom: (delta) {
                  final c = _controller;
                  if (c != null) {
                    unawaited(c.animateCamera(CameraUpdate.zoomBy(delta)));
                  }
                },
              ),
            ),
            if (eta != null)
              PositionedDirectional(
                top: 8,
                start: 8,
                child: Semantics(
                  liveRegion: true,
                  child: Material(
                    color: theme.colorScheme.surface.withValues(alpha: 0.9),
                    borderRadius: BorderRadius.circular(8),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 10,
                        vertical: 6,
                      ),
                      child: Text(eta, style: theme.textTheme.labelLarge),
                    ),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

LatLngBounds _bounds(List<LatLng> points) {
  var south = points.first.latitude;
  var north = south;
  var west = points.first.longitude;
  var east = west;
  for (final p in points) {
    south = math.min(south, p.latitude);
    north = math.max(north, p.latitude);
    west = math.min(west, p.longitude);
    east = math.max(east, p.longitude);
  }
  return LatLngBounds(
    southwest: LatLng(south, west),
    northeast: LatLng(north, east),
  );
}
