// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `MapView` and `OrderTrackerMap` on `flutter_map`.
library;

import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';
import 'package:plux_maps/src/model.dart';
import 'package:plux_maps/src/tiles.dart';

/// Called with the id of a tapped marker.
typedef MarkerTapped = void Function(String id);

/// Called with the position of a tap on the map.
typedef MapTapped = void Function(LatLng position);

/// Called when the camera stops moving.
typedef CameraIdle = void Function(LatLng center, double zoom);

Marker _marker(
  MapMarkerData m, {
  required Color color,
  MarkerTapped? onTap,
  LatLng? at,
}) {
  final title = m.title;
  final tap = onTap == null ? null : () => onTap(m.id);
  final body = GestureDetector(
    behavior: HitTestBehavior.opaque,
    onTap: tap,
    child: Center(child: Icon(markerIcon(m.icon), size: 36, color: color)),
  );
  return Marker(
    key: ValueKey('marker:${m.id}'),
    point: at ?? m.position,
    width: kMinInteractiveDimension,
    height: kMinInteractiveDimension,
    alignment: Alignment.topCenter,
    child: title == null
        ? ExcludeSemantics(child: body)
        : Semantics(
            container: true,
            label: title,
            button: tap != null,
            onTap: tap,
            excludeSemantics: true,
            child: body,
          ),
  );
}

/// The map shell both widgets share: the camera, the tile layer through the
/// pinned client, the overlays and the attribution.
final class PluxMapCanvas extends StatefulWidget {
  /// Creates the canvas.
  const PluxMapCanvas({
    required this.controller,
    required this.semanticLabel,
    required this.center,
    required this.zoom,
    required this.overlays,
    super.key,
    this.fit,
    this.minZoom = 2,
    this.maxZoom = 19,
    this.tileUrlTemplate,
    this.attribution,
    this.imageFor,
    this.interactive = true,
    this.corner,
    this.onTap,
    this.onCameraIdle,
    this.onReady,
  });

  /// The camera controller, owned by the caller.
  final MapController controller;

  /// What the map shows, spoken before its markers.
  final String semanticLabel;

  /// The initial centre.
  final LatLng center;

  /// The initial zoom.
  final double zoom;

  /// The points the initial camera fits instead of [center] and [zoom].
  final List<LatLng>? fit;

  /// The lowest zoom.
  final double minZoom;

  /// The highest zoom.
  final double maxZoom;

  /// The tile URL; no tiles without it.
  final String? tileUrlTemplate;

  /// The tile provider's attribution, always shown when given.
  final String? attribution;

  /// Resolves the image of a tile URL.
  final TileImageFor? imageFor;

  /// Whether the user can pan and zoom.
  final bool interactive;

  /// A widget at the start corner (the arrival estimate).
  final Widget? corner;

  /// The layers over the tiles, in order.
  final List<Widget> overlays;

  /// A tap away from a marker.
  final MapTapped? onTap;

  /// The camera stopped moving.
  final CameraIdle? onCameraIdle;

  /// The map has its first camera.
  final VoidCallback? onReady;

  @override
  State<PluxMapCanvas> createState() => _PluxMapCanvasState();
}

final class _PluxMapCanvasState extends State<PluxMapCanvas> {
  PluxTileProvider? _provider;

  PluxTileProvider? _providerFor() {
    final imageFor = widget.imageFor;
    if (widget.tileUrlTemplate == null || imageFor == null) return null;
    return (_provider ??= PluxTileProvider(imageFor))..imageFor = imageFor;
  }

  @override
  void didUpdateWidget(PluxMapCanvas old) {
    super.didUpdateWidget(old);
    if (old.tileUrlTemplate != widget.tileUrlTemplate) _provider = null;
  }

  void _event(MapEvent e) {
    if (e.source == MapEventSource.mapController) return;
    final idle = widget.onCameraIdle;
    if (idle != null &&
        (e is MapEventMoveEnd ||
            e is MapEventFlingAnimationEnd ||
            e is MapEventDoubleTapZoomEnd)) {
      idle(e.camera.center, e.camera.zoom);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final template = widget.tileUrlTemplate;
    final provider = _providerFor();
    final maxZoom = math.max(widget.minZoom, widget.maxZoom);
    final zoom = widget.zoom.clamp(widget.minZoom, maxZoom);
    final fit = widget.fit;
    final attribution = widget.attribution;
    final onTap = widget.onTap;
    final map = FlutterMap(
      mapController: widget.controller,
      options: MapOptions(
        initialCenter: widget.center,
        initialZoom: zoom,
        initialCameraFit: fit == null || fit.isEmpty
            ? null
            : CameraFit.coordinates(
                coordinates: fit,
                padding: const EdgeInsets.all(48),
                maxZoom: maxZoom,
                minZoom: widget.minZoom,
              ),
        minZoom: widget.minZoom,
        maxZoom: maxZoom,
        backgroundColor: theme.colorScheme.surfaceContainerHighest,
        interactionOptions: InteractionOptions(
          flags: widget.interactive
              ? InteractiveFlag.all & ~InteractiveFlag.rotate
              : InteractiveFlag.none,
        ),
        onTap: onTap == null ? null : (_, point) => onTap(point),
        onMapEvent: _event,
        onMapReady: widget.onReady,
      ),
      children: [
        if (template != null && provider != null)
          TileLayer(
            urlTemplate: template,
            tileProvider: provider,
            maxZoom: maxZoom,
            maxNativeZoom: math.min(19, maxZoom.floor()),
            // The subdomain placeholder is not supported: a template names
            // its host, so the domain the plugin declares is the one used.
            subdomains: const [],
            // A blocked or failed tile stays blank; the runtime has already
            // reported a blocked domain (PLX-6030).
            errorTileCallback: (tile, error, stackTrace) {},
          ),
        ...widget.overlays,
      ],
    );
    final style = (theme.textTheme.labelSmall ?? const TextStyle()).copyWith(
      color: theme.colorScheme.onSurface,
    );
    return LayoutBuilder(
      builder: (context, c) {
        final height = c.hasBoundedHeight
            ? c.maxHeight
            : (c.hasBoundedWidth ? c.maxWidth * 0.75 : 300.0);
        return SizedBox(
          height: height,
          child: Semantics(
            container: true,
            explicitChildNodes: true,
            label: widget.semanticLabel,
            child: ClipRect(
              child: Stack(
                fit: StackFit.expand,
                children: [
                  map,
                  if (widget.corner != null)
                    PositionedDirectional(
                      top: 8,
                      start: 8,
                      child: widget.corner!,
                    ),
                  if (attribution != null)
                    PositionedDirectional(
                      bottom: 0,
                      end: 0,
                      child: DecoratedBox(
                        decoration: BoxDecoration(
                          color: theme.colorScheme.surface.withValues(
                            alpha: 0.8,
                          ),
                        ),
                        child: Padding(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 6,
                            vertical: 2,
                          ),
                          child: Text(attribution, style: style),
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ),
        );
      },
    );
  }
}

/// `MapView`.
final class PluxMapView extends StatefulWidget {
  /// Creates the map.
  const PluxMapView({
    required this.semanticLabel,
    required this.center,
    super.key,
    this.zoom = 13,
    this.minZoom = 2,
    this.maxZoom = 19,
    this.markers = const [],
    this.polylines = const [],
    this.tileUrlTemplate,
    this.attribution,
    this.imageFor,
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

  /// The tile URL.
  final String? tileUrlTemplate;

  /// The attribution.
  final String? attribution;

  /// Resolves the image of a tile URL.
  final TileImageFor? imageFor;

  /// Whether the user can pan and zoom.
  final bool interactive;

  /// A marker was tapped.
  final MarkerTapped? onMarkerTap;

  /// The map was tapped.
  final MapTapped? onMapTap;

  /// The camera stopped moving.
  final CameraIdle? onCameraIdle;

  @override
  State<PluxMapView> createState() => _PluxMapViewState();
}

final class _PluxMapViewState extends State<PluxMapView> {
  final MapController _controller = MapController();
  bool _ready = false;

  @override
  void didUpdateWidget(PluxMapView old) {
    super.didUpdateWidget(old);
    if (!_ready) return;
    if (old.center != widget.center || old.zoom != widget.zoom) {
      final cam = _controller.camera;
      if (cam.center != widget.center || cam.zoom != widget.zoom) {
        _controller.move(widget.center, widget.zoom);
      }
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return PluxMapCanvas(
      controller: _controller,
      semanticLabel: widget.semanticLabel,
      center: widget.center,
      zoom: widget.zoom,
      minZoom: widget.minZoom,
      maxZoom: widget.maxZoom,
      tileUrlTemplate: widget.tileUrlTemplate,
      attribution: widget.attribution,
      imageFor: widget.imageFor,
      interactive: widget.interactive,
      onTap: widget.onMapTap,
      onCameraIdle: widget.onCameraIdle,
      onReady: () => _ready = true,
      overlays: [
        if (widget.polylines.isNotEmpty)
          PolylineLayer(
            polylines: [
              for (final p in widget.polylines)
                if (p.points.length >= 2)
                  Polyline(
                    points: p.points,
                    strokeWidth: p.width,
                    color: polylineColor(scheme, p.color),
                  ),
            ],
          ),
        MarkerLayer(
          markers: [
            for (final m in widget.markers)
              _marker(m, color: scheme.error, onTap: widget.onMarkerTap),
          ],
        ),
      ],
    );
  }
}

/// `OrderTrackerMap`.
final class PluxOrderTrackerMap extends StatefulWidget {
  /// Creates the tracker.
  const PluxOrderTrackerMap({
    required this.semanticLabel,
    required this.origin,
    required this.destination,
    super.key,
    this.courier,
    this.route = const [],
    this.etaLabel,
    this.follow = true,
    this.tileUrlTemplate,
    this.attribution,
    this.imageFor,
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

  /// The tile URL.
  final String? tileUrlTemplate;

  /// The attribution.
  final String? attribution;

  /// Resolves the image of a tile URL.
  final TileImageFor? imageFor;

  /// The origin marker's spoken name.
  final String? originLabel;

  /// The destination marker's spoken name.
  final String? destinationLabel;

  /// The courier marker's spoken name.
  final String? courierLabel;

  @override
  State<PluxOrderTrackerMap> createState() => _PluxOrderTrackerMapState();
}

/// How long the courier marker takes to glide to a new position.
const Duration courierGlide = Duration(milliseconds: 800);

final class _PluxOrderTrackerMapState extends State<PluxOrderTrackerMap>
    with SingleTickerProviderStateMixin {
  final MapController _controller = MapController();
  late final AnimationController _glide = AnimationController(
    vsync: this,
    duration: courierGlide,
  )..addListener(_follow);
  LatLng? _from;
  LatLng? _to;
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    _to = widget.courier;
  }

  /// The courier's position at the current point of the glide.
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
  void didUpdateWidget(PluxOrderTrackerMap old) {
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

  void _follow() {
    final at = _courier;
    if (!widget.follow || !_ready || at == null) return;
    final cam = _controller.camera;
    if (!cam.visibleBounds.contains(at)) _controller.move(at, cam.zoom);
  }

  @override
  void dispose() {
    _glide.dispose();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final origin = _marker(
      MapMarkerData(
        id: 'origin',
        position: widget.origin,
        title: widget.originLabel,
        icon: 'store',
      ),
      color: scheme.primary,
    );
    final destination = _marker(
      MapMarkerData(
        id: 'destination',
        position: widget.destination,
        title: widget.destinationLabel,
        icon: 'home',
      ),
      color: scheme.error,
    );
    final eta = widget.etaLabel;
    return PluxMapCanvas(
      controller: _controller,
      semanticLabel: widget.semanticLabel,
      center: widget.destination,
      zoom: 13,
      fit: [
        widget.origin,
        widget.destination,
        if (widget.courier != null) widget.courier!,
      ],
      tileUrlTemplate: widget.tileUrlTemplate,
      attribution: widget.attribution,
      imageFor: widget.imageFor,
      onReady: () => _ready = true,
      corner: eta == null
          ? null
          : Semantics(
              liveRegion: true,
              child: Material(
                color: scheme.surface.withValues(alpha: 0.9),
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
      overlays: [
        if (widget.route.length >= 2)
          PolylineLayer(
            polylines: [
              Polyline(
                points: widget.route,
                strokeWidth: 4,
                color: scheme.primary,
              ),
            ],
          ),
        AnimatedBuilder(
          animation: _glide,
          builder: (context, _) {
            final at = _courier;
            return MarkerLayer(
              markers: [
                origin,
                destination,
                if (at != null)
                  _marker(
                    MapMarkerData(
                      id: 'courier',
                      position: at,
                      title: widget.courierLabel,
                      icon: 'delivery',
                    ),
                    color: scheme.tertiary,
                  ),
              ],
            );
          },
        ),
      ],
    );
  }
}
