<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# plux_maps

`MapView` and `OrderTrackerMap` for Plux pages, on
[`flutter_map`](https://pub.dev/packages/flutter_map). Part of the optional
widget packages of `plux_flutter` (ADR-0071).

```dart
PluxConfig(
  widgetPackages: [PluxMaps()],
)
```

Without the package a `MapView` node renders the node fallback and the
runtime reports `PLX-4403`.

## Capability and domains (SEC-080, AST-002)

Tiles are fetched from the page's `tileUrlTemplate` (`{z}`, `{x}`, `{y}`).
The plugin must declare the tile host in its network domains; the URL must be
HTTPS. A tile whose domain is not declared is not fetched, no tile is drawn,
and the runtime reports `PLX-6030` once. Without a template the map draws
markers and lines on an empty background.

**Tiles go through the pinned client (A3).** Every tile is requested with the
client `NodeContext.remoteClient` returns: the renderer's own HTTP client,
with the host's certificate pinning and proxy settings. The package never
opens a connection of its own.

## Attribution

`attribution` is required with `tileUrlTemplate` and is always shown at the
end corner of the map (the right in left-to-right, the left in right-to-left
layouts). Use the text your tile provider requires. The public OpenStreetMap
servers also require an identifying `User-Agent` and are not meant for
production traffic: use a provider whose terms you accept.

## Accessibility

The map is a labelled group (`semanticLabel`). Each marker with a `title` is
a semantics node with that name, and a button when `onMarkerTap` is bound.
The arrival estimate of `OrderTrackerMap` is a live region.

## Events

`onMarkerTap` (the marker `id`), `onMapTap` (`{lat, lng}`) and `onCameraIdle`
(`{center, zoom}`) fire as the descriptors define. The courier marker of
`OrderTrackerMap` glides to each new position, and jumps when the platform
asks for reduced motion.

## Not included

`showUserLocation` is not drawn yet: it needs the `location` device API
through a seam the widget builders do not have.
