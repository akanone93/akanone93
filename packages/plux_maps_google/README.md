<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# plux_maps_google

`MapView` and `OrderTrackerMap` for Plux pages on Google Maps
([`google_maps_flutter`](https://pub.dev/packages/google_maps_flutter)). It
provides the same widget IDs as `plux_maps`: register one of the two, never
both.

```dart
PluxConfig(
  widgetPackages: [PluxMapsGoogle()],
)
```

## API key

The host app supplies the Google Maps API key; plugins never see it.

- Android: the `com.google.android.geo.API_KEY` meta-data entry in
  `AndroidManifest.xml`.
- iOS: `GMSServices.provideAPIKey(...)` in `AppDelegate`.

Restrict the key to the app's package name or bundle ID in the Google Cloud
console.

## Differences from plux_maps

- `tileUrlTemplate` and `attribution` are ignored: Google draws its own tiles
  and its own logo and legal notices, which must stay visible.
- Google tiles are fetched by the Google Maps SDK, not through the runtime's
  pinned client, so the plugin's declared network domains and certificate pins
  do not apply to them (A3 holds for `plux_maps` only). Choose this package
  only where that is acceptable.
- Markers use Google's pins; `icon` is ignored. A marker's `title` is its info
  window title, which the platform exposes to screen readers.
- `showUserLocation` is not drawn yet.

## Terms

Using Google Maps is subject to the
[Google Maps Platform Terms of Service](https://cloud.google.com/maps-platform/terms).
The host app is the licensee: it enables billing, accepts the terms and shows
the required attributions.
