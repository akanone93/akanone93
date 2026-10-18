// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_components/plux_components.dart';
import 'package:plux_components/src/ids.dart';

Map<String, Object?> _descriptor(File f) =>
    jsonDecode(f.readAsStringSync()) as Map<String, Object?>;

/// Every descriptor under `schema/widgets/layer1` and `layer2`.
List<Map<String, Object?>> _descriptors() => [
  for (final layer in ['layer1', 'layer2'])
    for (final f in Directory('../../schema/widgets/$layer').listSync())
      if (f is File && f.path.endsWith('.json')) _descriptor(f),
];

Map<String, int> _ids(Object? members) => {
  for (final m
      in (members as List<Object?>? ?? const []).cast<Map<String, Object?>>())
    m['name']! as String: m['id']! as int,
};

void main() {
  const package = PluxComponents();

  // Verifies: RT-060.
  test('the package is named as its descriptors name it [RT-060]', () {
    expect(package.name, 'plux_components');
    final named = [
      for (final d in _descriptors())
        if (d['package'] == 'plux_components') d['type']! as String,
    ];
    expect(named..sort(), componentIds.keys.toList()..sort());
  });

  // Verifies: RT-060, BND-011.
  test('the IDs are those of the descriptors [RT-060] [BND-011]', () {
    final byType = {for (final d in _descriptors()) d['type']! as String: d};
    for (final MapEntry(key: type, value: ids) in componentIds.entries) {
      final d = byType[type]!;
      expect(ids.id, d['id'], reason: '$type id');
      expect(ids.props, _ids(d['props']), reason: '$type props');
      expect(ids.events, _ids(d['events']), reason: '$type events');
      expect(ids.slots, _ids(d['slots']), reason: '$type slots');
    }
  });

  // Verifies: RT-060.
  test('every widget of the package has a builder [RT-060]', () {
    expect(
      package.builders.keys.toList()..sort(),
      [for (final ids in componentIds.values) ids.id]..sort(),
    );
  });

  test('the ID constants are the widget IDs of the table', () {
    expect(PluxComponentIds.moneyText, componentIds['MoneyText']!.id);
    expect(PluxComponentIds.table, componentIds['Table']!.id);
  });
}
