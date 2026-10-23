// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Mock selection (DAT-080): tests and development builds show a source's
/// declared `loading`, `empty`, `error` or `success` state instead of
/// loading it. Release builds never do: [dataMocksAllowed] is a constant
/// false there, so the selection is compiled out.
library;

import 'dart:collection';

import 'package:flutter/foundation.dart';

/// Whether this build may select mocks: debug and profile builds only.
const bool dataMocksAllowed = !kReleaseMode;

/// The mock states of a data source.
enum DataMockState {
  /// Loading, forever.
  loading,

  /// The declared empty mock, or an empty list.
  empty,

  /// The declared error.
  error,

  /// The source's design-time mock.
  success,
}

/// The selected mock states, by `<plugin>/<source>` or by `<source>` for
/// every plugin.
class DataMocks {
  /// Creates the selection; [allowed] is [dataMocksAllowed] outside tests.
  const DataMocks([this.selected = const {}, this.allowed = dataMocksAllowed]);

  /// The selection.
  final Map<String, DataMockState> selected;

  /// Whether mocks are honoured at all.
  final bool allowed;

  /// The state selected for [source] of [plugin], or null to load it.
  DataMockState? of(String plugin, String source) {
    if (!allowed || selected.isEmpty) return null;
    return selected['$plugin/$source'] ?? selected[source];
  }
}

/// A selection a development session changes while the app runs (`DAT-080`,
/// `DEV-031`). Only builds with `kPluxDevtools` use it.
final class MutableDataMocks extends DataMocks {
  /// Creates an empty selection; [allowed] is [dataMocksAllowed] outside
  /// tests.
  MutableDataMocks({bool allowed = dataMocksAllowed})
    : super(const {}, allowed);

  final Map<String, DataMockState> _selected = {};

  @override
  Map<String, DataMockState> get selected => UnmodifiableMapView(_selected);

  /// Selects [state] for [source], or loads the source for real again when
  /// [state] is null.
  void select(String source, DataMockState? state) {
    if (state == null) {
      _selected.remove(source);
    } else {
      _selected[source] = state;
    }
  }

  /// Replaces the whole selection.
  void replaceAll(Map<String, DataMockState> selection) {
    _selected
      ..clear()
      ..addAll(selection);
  }
}
