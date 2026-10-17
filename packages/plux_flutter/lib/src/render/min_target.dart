// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widgets whose descriptor sets `minTarget` (A11Y-004): the renderer
/// holds their interactive area to at least [minTargetSize] logical pixels
/// square. The set [minTargetTypes] is generated from the descriptors.
library;

import 'package:plux_flutter/src/schema/registry.g.dart';

/// The smallest interactive area in logical pixels (A11Y-004).
const double minTargetSize = 44;

/// The permanent IDs of the widgets in [minTargetTypes].
final Set<int> minTargetWidgets = {
  for (final d in widgetDescriptors)
    if (minTargetTypes.contains(d.type)) d.id,
};

/// The types of the icon-only buttons: their own tap node has no text to
/// read, so a semantics label on the node names that tap node (A11Y-001).
const Set<String> iconOnlyButtonTypes = {'FloatingActionButton', 'IconButton'};

/// The permanent IDs of the widgets in [iconOnlyButtonTypes].
final Set<int> iconOnlyButtonWidgets = {
  for (final d in widgetDescriptors)
    if (iconOnlyButtonTypes.contains(d.type)) d.id,
};
