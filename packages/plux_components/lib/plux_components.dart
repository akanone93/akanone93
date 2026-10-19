// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Layer 2 components most apps do not use, as an optional widget
/// package (RT-060, ADR-0071): forms and lists, onboarding, commerce,
/// ratings, uploads, and the money and locale components. Register
/// [PluxComponents] in `PluxConfig.widgetPackages`.
library;

export 'src/components_b.dart' show CountdownClock;
export 'src/ids.dart' show PluxComponentIds;
export 'src/package.dart' show PluxComponents;
