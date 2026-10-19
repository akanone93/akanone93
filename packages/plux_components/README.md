<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# plux_components

The widgets most apps do not use, as an optional [Plux](../../README.md) widget package
(`RT-060`, [ADR-0071](../../docs/adr/0071-catalogue-completion.md)). `plux_flutter` keeps
within its size budget (`RT-061`) because these widgets, and the parts of the Flutter
framework only they need, are in a build only when the host registers this package.

## Use

```dart
dependencies:
  plux_components: ^0.1.0
```

```dart
final config = PluxConfig(
  // …
  widgetPackages: [PluxComponents()],
);
```

A node whose widget is in the package, on a host that did not register it, renders the node
fallback and reports `PLX-4403`. Publishing warns when a targeted host build lacks the package
(`PLX-1233`), and `plux init` adds and registers it when the project shows one of its widgets.

## Widgets

| Group | Widgets |
|---|---|
| Layer 2, forms and lists | `ConsentCheckbox`, `SearchField`, `FilterBar`, `SortSheet`, `InfiniteList`, `WizardForm`, `OnboardingPager`, `SettingsList`, `ProfileHeader`, `StatusTimeline` |
| Layer 2, commerce, campaigns, ratings, uploads | `LiveEtaCard`, `ProductCard`, `QuantityStepper`, `Carousel`, `PromoBanner`, `CountdownTimer`, `RatingBar`, `RatingInput`, `ImageGallery`, `FileUploader` |
| Layer 2, money and locale | `AmountInput`, `MoneyText`, `MaskedText`, `PhoneInput`, `IbanInput`, `CountryPicker`, `CurrencyPicker`, `PriceTag`, `CartSummary`, `TransactionListItem`, `AccountCard`, `PaymentCard` |
| Layer 1, scrolling and display | `Table`, `ReorderableListView`, `NestedScrollView`, `DataTable`, `Stepper` |
| Layer 1, search, dates and navigation | `SearchBar`, `SearchAnchor`, `Autocomplete`, `CalendarDatePicker`, `NavigationRail` |
| Layer 1, Cupertino and interaction | `CupertinoPicker`, `CupertinoDatePicker`, `CupertinoContextMenu`, `Draggable`, `DragTarget`, `InteractiveViewer` |

Every widget, with its props and events, is in the [widget reference](../../docs/reference/widgets.md).
`FileUploader` picks files through `plux_media`, which the host registers in
`PluxConfig.devicePackages`.

The package uses only `plux_flutter`'s public API. The permanent IDs of its widgets are in
`PluxComponentIds`; a test compares them with the descriptors in `schema/widgets`.
