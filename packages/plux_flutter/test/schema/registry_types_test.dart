// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/pxl/types.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';
import 'package:plux_flutter/src/schema/registry_types.dart';

void main() {
  final registry = registryTypes();

  test('every registry value type and enum is a named type [SCH-010]', () {
    for (final t in valueTypeDescriptors) {
      expect(registry[t.name]?.fields?.keys, unorderedEquals(t.fields.keys));
    }
    for (final e in enumDescriptors) {
      expect(registry[e.name]?.members, e.values.keys.toList());
    }
    final marker = registry['MapMarker']!.fields!;
    expect(marker['position']?.named?.name, 'LatLng');
    expect(marker['title']?.nullable, isTrue);
    expect(registry['LatLng']!.fields!['lat']?.kind, PxlKind.double);
  });

  // Build log row 143: a declared type may have a field of a registry type,
  // and a data source may be of one; the runtime reads them as data.
  test('a declared type with a LatLng field reads its JSON [SCH-010]', () {
    final all = {...registry, 'AidStation': NamedType.object('AidStation')};
    all['AidStation']!.fields!
      ..['name'] = PxlType.parse('string', (n) => all[n])
      ..['at'] = PxlType.parse('LatLng', (n) => all[n])
      ..['align'] = PxlType.parse('TextAlign?', (n) => all[n]);
    final station = PxlType.parse('AidStation', (n) => all[n]);
    expect(
      fromJson(station, {
        'name': 'Km 21',
        'at': {'lat': 52.5, 'lng': 13},
        'align': 'center',
      }),
      {
        'align': 'center',
        'at': {'lat': 52.5, 'lng': 13.0},
        'name': 'Km 21',
      },
    );
    final markers = PxlType.parse('list<MapMarker>', (n) => all[n]);
    expect(
      fromJson(markers, [
        {
          'id': 'a',
          'position': {'lat': 1, 'lng': 2},
        },
      ]),
      [
        {
          'icon': null,
          'id': 'a',
          'position': {'lat': 1.0, 'lng': 2.0},
          'title': null,
        },
      ],
    );
    expect(
      () => fromJson(station, {
        'name': 'x',
        'at': {'lat': 1},
      }),
      throwsFormatException,
    );
  });
}
