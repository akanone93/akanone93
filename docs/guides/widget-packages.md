<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Widget Packages Guide

Some widgets live in optional packages, so an app pays only for the ones it uses
(`RT-060`, `RT-061`). This guide lists them, shows how to register them and says what each needs.
Every widget, with its props and events, is in the [widget reference](../reference/widgets.md).
The decision is [ADR-0071](../adr/0071-catalogue-completion.md).

## 1. Register a package

A host app adds the packages it wants to `PluxConfig.widgetPackages`:

```dart
final config = PluxConfig(
  // …
  widgetPackages: [PluxComponents(), PluxCharts(), PluxMarkdown(), PluxMaps()],
);
```

`plux init` adds and registers the widget packages a project shows, the way it does the device
packages (`plux init --packages` names more).

A node whose package is not registered renders the node fallback and reports `PLX-4403`.
Publishing warns when a targeted host build lacks a package (`PLX-1233`). Register
`PluxMapsGoogle` instead of `PluxMaps`, not beside it.

## 2. The packages

| Package | Class | Widgets |
|---|---|---|
| `plux_components` | `PluxComponents` | The Layer 2 components most apps do not use (forms and lists, commerce, campaigns, ratings, `FileUploader`, and the money and locale components) and the Layer 1 widgets that pull large parts of Flutter into a build: `Table`, `DataTable`, `Stepper`, `ReorderableListView`, `NestedScrollView`, `SearchBar`, `SearchAnchor`, `Autocomplete`, `CalendarDatePicker`, `NavigationRail`, `CupertinoPicker`, `CupertinoDatePicker`, `CupertinoContextMenu`, `Draggable`, `DragTarget`, `InteractiveViewer`. The full list is in the [package README](../../packages/plux_components/README.md). |
| `plux_charts` | `PluxCharts` | `BarChart`, `LineChart`, `PieChart`, `Sparkline`. |
| `plux_markdown` | `PluxMarkdown` | `MarkdownView`, `TermsViewer`. |
| `plux_player` | `PluxPlayer` | `VideoPlayer`, `AudioPlayer` (on `video_player`). |
| `plux_maps` | `PluxMaps` | `MapView`, `OrderTrackerMap` (on `flutter_map`). |
| `plux_maps_google` | `PluxMapsGoogle` | The same two widgets on Google Maps. |
| `plux_scanner` | `PluxScannerWidgets` | `QrScanner`, `BarcodeScanner`, `QrCodeView`. |

`plux_media` (`SignaturePad`), `plux_lottie` (`LottieView`) and `plux_rive` (`RiveView`)
work the same way. The package that provides a widget is named on its reference entry.

## 3. What each needs

Widgets that reach outside the app follow the usual rules: deny by default, explicit
grants (`SEC-080`).

- **Components, Charts, Markdown (text), SignaturePad:** nothing beyond registration.
  `FileUploader` picks files through `plux_media` in `PluxConfig.devicePackages`; without it
  the control is disabled and `PLX-5401` is reported. Charts give
  screen readers a text summary per series; `showTable` adds a data table.
- **Markdown images, Lottie and Rive files:** an HTTPS domain the plugin declares; they
  load through the runtime's pinned client. Raw HTML in Markdown is shown as text.
- **Maps:** tiles come from `tileUrlTemplate`, HTTPS on a domain the plugin declares, and
  the tile provider's `attribution` is required and always shown. With `plux_maps_google`,
  the host app supplies the Google Maps API key.
- **Scanner:** the plugin declares the `camera` device API, the app document approves it,
  and the host declares the camera usage description (`NSCameraUsageDescription`) and, on
  Android, the camera permission. On Android the scanners use ML Kit's unbundled model
  through Google Play services and fall back to ZXing-C++, which works offline; one line in
  `android/gradle.properties` opts in to the unbundled model:
  `dev.steenbakker.mobile_scanner.useUnbundled=true`. The action `scanCode` needs the
  separate `PluxScanner` in `PluxConfig.devicePackages`. A scanned value can be sensitive:
  do not log it or put it in a label.
- **Player:** `VideoPlayer` and `AudioPlayer` take an HTTPS address from a declared domain
  and never start by themselves under reduce motion.

## 4. Pinned media

Platform video and audio players cannot use the app's pinned HTTP client (`SEC-041`). The
security setting `pinnedMediaOnly` (off under `standard` and `strict`, on under `maximum`)
closes that gap: with it on, network video and audio are first downloaded through the pinned
client and played from that copy. The largest file the player downloads is
`media.pinnedDownloadSize` (200 MiB by default; see [limits](../reference/limits.md)).
