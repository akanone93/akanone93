// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget registry's value types and enums as named types (SCH-010).
library;

import 'package:plux_flutter/src/pxl/types.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';

/// The registry's value types and enums as named types, by name: a declared
/// type's field or a data source may be one, such as a `LatLng` (build log
/// row 143). A value type's fields are typed as data reads them, an
/// optional one nullable.
Map<String, NamedType> registryTypes() {
  final out = <String, NamedType>{
    for (final e in enumDescriptors)
      e.name: NamedType.enumeration(e.name, [...e.values.keys]),
    for (final t in valueTypeDescriptors) t.name: NamedType.object(t.name),
  };
  for (final t in valueTypeDescriptors) {
    final fields = out[t.name]!.fields!;
    for (final MapEntry(:key, :value) in t.types.entries) {
      fields[key] = PxlType.parse(value, (n) => out[n]);
    }
  }
  return out;
}
