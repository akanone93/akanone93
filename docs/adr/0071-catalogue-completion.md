# 0071. Catalogue completion: the widget catch-up, widget packages and `catalogue-check`

- **Status:** Proposed
- **Date:** 2026-10-16
- **Requirements:** `WGT-001`, `WGT-003`, `WGT-020`, `RT-060`, `A11Y-001`, `ANI-005`

## Context and problem

98 Appendix C widgets tagged P3–P5 have no descriptor: 50 Layer 1 and 48 Layer 2 (P8 plan §2.3).
Appendix C rows carry no requirement IDs, plans were built from IDs, and `make trace` checks only
IDs. The maintainer decided to build all of them in P8 (D14). Fourteen live in optional packages,
some needing new dependencies (A1).

## Decision

- **Layer 1:** each mirrors its Flutter class (coverage table, `WGT-003`); Flutter controllers map to
  state as the coverage table already records ("controller→state"): `TabBar` and `TabBarView`
  share an `index` bound to a state key; `ReorderableListView` writes its bound list; menus and
  `Autocomplete` take items from a list binding or data source; `Draggable` and `DragTarget` carry a
  typed payload; `RefreshIndicator` completes when its graph ends. The `Animated*` widgets coexist
  with node-level implicit animation (`ANI-001`); `Hero` takes the node-level `hero` tags;
  `Timeline` plays a page timeline over its subtree; the `*Transition` widgets bind to a timeline
  track; all follow reduce motion (`ANI-007`). `DataScope` binds one data source with `loading`,
  `empty`, `error` and `success` slots; `NativeSlot` names a catalogue slot (the slot name as a node
  type keeps working).
- **Layer 2 (`WGT-020`):** built from Layer 1, the theme and runtime services; a descriptor, widget
  tests, goldens in light, dark, RTL and 200% text, and a screen-reader script each.
- **Packages:** a package contributes builders through `PluxWidgetPackage` in
  `PluxConfig.widgetPackages`; its descriptors stay in `schema/widgets/layer2` with `package`; a
  node whose package is missing renders the node fallback and reports `PLX-4403`; publishing warns
  when a targeted host build lacks it (`PLX-1233`). Placement (P8 plan §2.6, A1): `plux_charts`
  (in house), `plux_maps` (`flutter_map`) and `plux_maps_google` (`google_maps_flutter`),
  `plux_player` (`video_player`, for video and audio), `plux_markdown` (`markdown`),
  `plux_scanner` (`QrScanner`, `BarcodeScanner` on `mobile_scanner`; `QrCodeView` on `qr`),
  `plux_media` (`SignaturePad`, in house), `plux_lottie` and `plux_rive` (catalogue widgets
  replacing the native slots, ADR-0050 Revision). `plux_scanner` also completes P6's B21: ML Kit
  unbundled through Play services, vendored ZXing-C++ as the fallback, measured per ABI.
- **IDs:** allocated per workstream in P8 plan §2.7.
- **`catalogue-check`** (`tools/cmd/catalogue`): fails when an Appendix C widget or Appendix D action
  at or before the current phase lacks a descriptor, a builder or a handler, or disagrees on phase.
  Report-only until the catch-up ends, then part of `make check`.

## Consequences

- **Positive:** the catalogue the spec promises exists, and cannot silently fall behind again.
- **Negative:** about a hundred builders grow the core package; size is measured each milestone and
  heavy components can move to optional packages.
- **Follow-up:** K1–K3.

## Revision (2026-10-18, P8: the size split, `plux_components`)

**Context.** The catch-up put every Layer 2 component and the Layer 1 widgets of K1 into
`plux_flutter` (the plan's §2.4 named the size risk). Measured in CI, the core then added
9,835,006 B to the arm64 APK, 11,242,334 B to the armeabi-v7a APK (over `RT-061`'s 10 MiB) and
4,175,235 B to the iOS IPA, and `QA-007` failed each on growth beyond 20 %. The Linux release
build of the size app (`flutter build linux --release --analyze-size`) tracks the Android
numbers closely and was used to choose what moves.

**Decision.** A new optional package, `plux_components`, builds the widgets that most apps do not
use. It follows ADR-0051's placement rule (a measurable size cost most apps would not pay) and
registers like `plux_charts`, through `PluxConfig.widgetPackages`:

- the Layer 2 components of K2a and K2b (`ConsentCheckbox` … `PaymentCard`, `FileUploader`
  included, which picks through `plux_media`), about 0.23 MB of the Linux AOT code;
- sixteen Layer 1 widgets whose Flutter implementation is large and rarely used, about 0.78 MB:
  `Table`, `ReorderableListView`, `NestedScrollView`, `DataTable`, `Stepper`, `SearchBar`,
  `SearchAnchor`, `Autocomplete`, `CalendarDatePicker`, `NavigationRail`, `CupertinoPicker`,
  `CupertinoDatePicker`, `CupertinoContextMenu`, `Draggable`, `DragTarget` and
  `InteractiveViewer`.

Their descriptors name the package; the compiler's optional-package table (PLX-1233) is checked
against the descriptors by a test; `plux init` registers the widget packages a project shows.
`plux_flutter` exports what a widget package needs, nothing more: four members of `NodeContext`,
the decoders and value types the builders read.

**Consequences.** An app that uses one of these widgets adds one dependency and one line of
configuration, and `plux init` does both. A node whose package is missing renders the fallback
and reports `PLX-4403`, as for the other packages. `plux_flutter`'s tests register the package
in their harness (a development dependency), so the core keeps its gallery and accessibility
tests; the package runs its own.
