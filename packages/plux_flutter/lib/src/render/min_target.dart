// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widgets whose descriptor sets `minTarget` (A11Y-004): the renderer
/// holds their interactive area to at least [minTargetSize] logical pixels
/// square. The generated registry does not carry the descriptor field, so
/// the set is kept here and a test compares it with `schema/widgets`.
library;

import 'package:plux_flutter/src/schema/registry.g.dart';

/// The smallest interactive area in logical pixels (A11Y-004).
const double minTargetSize = 44;

/// The types of the widgets marked `minTarget` in their descriptor.
const Set<String> minTargetTypes = {
  'CupertinoContextMenu',
  'CupertinoDatePicker',
  'CupertinoPicker',
  'CupertinoTabBar',
  'ConsentCheckbox',
  'FileUploader',
  'FilterBar',
  'OnboardingPager',
  'ProductCard',
  'PromoBanner',
  'QuantityStepper',
  'RatingInput',
  'SearchField',
  'SortSheet',
  'WizardForm',
};

/// The permanent IDs of the widgets in [minTargetTypes].
final Set<int> minTargetWidgets = {
  for (final d in widgetDescriptors)
    if (minTargetTypes.contains(d.type)) d.id,
};
