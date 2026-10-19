<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Changelog

## 0.1.0

- First version. The Layer 2 components of the catalogue catch-up that most apps do not use
  (forms and lists, commerce, campaigns, ratings, uploads, and the money and locale
  components) and the Layer 1 widgets that pull large parts of the Flutter framework into a
  build (`Table`, `ReorderableListView`, `NestedScrollView`, `DataTable`, `Stepper`,
  `SearchBar`, `SearchAnchor`, `Autocomplete`, `CalendarDatePicker`, `NavigationRail`,
  `CupertinoPicker`, `CupertinoDatePicker`, `CupertinoContextMenu`, `Draggable`,
  `DragTarget`, `InteractiveViewer`) moved out of `plux_flutter` 0.5.0 to keep its size within
  `RT-061` (`RT-060`). Register `PluxComponents()` in `PluxConfig.widgetPackages`.
