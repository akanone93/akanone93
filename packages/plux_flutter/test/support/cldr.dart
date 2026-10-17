// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:plux_flutter/src/l10n/l10n.dart';

const _cldr = '../../schema/l10n/cldr';

PluxLocaleIndex? _index;
List<String> _locales = const [];
final _loaded = <String, PluxLocaleData>{};

/// The data of the best locale for [tag], read from schema/l10n/cldr. Tests
/// load locale data from disk; the runtime receives it in the bundle (L4).
PluxLocaleData loadCldr(String tag) {
  if (_index == null) {
    final idx = jsonDecode(
      File('$_cldr/index.json').readAsStringSync(),
    ) as Map<String, Object?>;
    _index = PluxLocaleIndex.fromJson(idx);
    _locales = (idx['locales']! as List<Object?>).cast<String>();
  }
  final id = _index!.resolve(tag, _locales, '');
  return _loaded.putIfAbsent(id, () {
    final json = jsonDecode(File('$_cldr/$id.json').readAsStringSync());
    return PluxLocaleData.fromJson(json as Map<String, Object?>);
  });
}
