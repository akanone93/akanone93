// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package: builders of the Layer 2 components most apps do not
/// use and of the Layer 1 widgets that pull large parts of the Flutter
/// framework into a build. The IDs are those of the descriptors in
/// `schema/widgets` (`ids.dart`); a test compares them.
library;

import 'package:plux_components/src/components_a.dart';
import 'package:plux_components/src/components_b.dart';
import 'package:plux_components/src/cupertino.dart';
import 'package:plux_components/src/interaction.dart';
import 'package:plux_components/src/money.dart';
import 'package:plux_components/src/navigation.dart';
import 'package:plux_components/src/scrolling.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// The widgets that are not part of the runtime, to register in
/// `PluxConfig.widgetPackages`.
///
/// Layer 2 components: forms and lists (`ConsentCheckbox`, `SearchField`,
/// `FilterBar`, `SortSheet`, `InfiniteList`, `WizardForm`,
/// `OnboardingPager`, `SettingsList`, `ProfileHeader`, `StatusTimeline`),
/// commerce, campaigns, ratings and uploads (`LiveEtaCard`, `ProductCard`,
/// `QuantityStepper`, `Carousel`, `PromoBanner`, `CountdownTimer`,
/// `RatingBar`, `RatingInput`, `ImageGallery`, `FileUploader`), and the
/// money and locale components (`AmountInput`, `MoneyText`, `MaskedText`,
/// `PhoneInput`, `IbanInput`, `CountryPicker`, `CurrencyPicker`,
/// `PriceTag`, `CartSummary`, `TransactionListItem`, `AccountCard`,
/// `PaymentCard`).
///
/// Layer 1 widgets: `Table`, `ReorderableListView`, `NestedScrollView`,
/// `DataTable`, `Stepper`, `SearchBar`, `SearchAnchor`, `Autocomplete`,
/// `CalendarDatePicker`, `NavigationRail`, `CupertinoPicker`,
/// `CupertinoDatePicker`, `CupertinoContextMenu`, `Draggable`, `DragTarget`
/// and `InteractiveViewer`.
final class PluxComponents implements PluxWidgetPackage {
  /// Creates the package.
  const PluxComponents();

  @override
  String get name => 'plux_components';

  @override
  Map<int, NodeBuilder> get builders => {
    ...componentsABuilders,
    ...componentsBBuilders,
    ...moneyBuilders,
    ...scrollingBuilders,
    ...navigationBuilders,
    ...cupertinoBuilders,
    ...interactionBuilders,
  };
}
