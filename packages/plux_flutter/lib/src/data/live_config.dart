// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Server-held configuration on the device (DAT-094 to DAT-097): the `live`
/// values the server hands a device over its DPoP-bound channel, the
/// snapshot they arrive in, the codes the server refuses with, and the
/// interface pages read them through.
///
/// Live values are held in memory only. They are never written to disk,
/// logged, traced or put in telemetry, so every type here redacts them in
/// `toString` (DAT-094, SCH-012).
library;

import 'package:plux_flutter/src/errors/plux_exception.dart';

/// The type of a configuration value (DAT-090).
enum LiveConfigType {
  /// A string.
  string,

  /// A whole number.
  integer,

  /// A decimal number, carried as a string so no precision is lost.
  decimal,

  /// A boolean.
  boolean;

  /// Reads the type from the server's name for it, in any case and with or
  /// without an enum prefix such as `CONFIG_VALUE_TYPE_`; null when the name
  /// is none of the four.
  static LiveConfigType? parse(Object? name) {
    if (name is! String) return null;
    final n = name.toLowerCase();
    for (final t in values) {
      if (n == t.name || n.endsWith('_${t.name}')) return t;
    }
    return switch (n) {
      'int' || 'int64' => integer,
      'bool' => boolean,
      _ => null,
    };
  }
}

/// One `live` value of the server's configuration (DAT-090).
final class LiveConfigValue {
  /// Creates a value; [value] is a `String` for a string or a decimal, an
  /// `int` for an integer and a `bool` for a boolean.
  const LiveConfigValue({
    required this.key,
    required this.type,
    required this.value,
    required this.strict,
  });

  /// Reads the value from the server's JSON; throws [FormatException] when
  /// the type is unknown or the value does not read as its type.
  factory LiveConfigValue.fromJson(Map<String, Object?> json) {
    final key = json['key'];
    final type = LiveConfigType.parse(json['type']);
    if (key is! String || key.isEmpty || type == null) {
      throw const FormatException('a configuration value has no key or type');
    }
    final raw = json['value'];
    final Object value;
    switch (type) {
      case LiveConfigType.string || LiveConfigType.decimal:
        if (raw is! String) {
          throw const FormatException('a configuration value is not a string');
        }
        value = raw;
      case LiveConfigType.integer:
        final n = raw is int ? raw : (raw is String ? int.tryParse(raw) : null);
        if (n == null) {
          throw const FormatException(
            'a configuration value is not an integer',
          );
        }
        value = n;
      case LiveConfigType.boolean:
        final b = raw is bool
            ? raw
            : switch (raw) {
                'true' => true,
                'false' => false,
                _ => null,
              };
        if (b == null) {
          throw const FormatException('a configuration value is not a boolean');
        }
        value = b;
    }
    return LiveConfigValue(
      key: key,
      type: type,
      value: value,
      strict: json['strict'] == true,
    );
  }

  /// The key.
  final String key;

  /// The type.
  final LiveConfigType type;

  /// The value; see the constructor.
  final Object value;

  /// Whether the value is `strict` (DAT-096, DAT-097): a feature reading it
  /// must see the current one or fail.
  final bool strict;

  @override
  String toString() => 'LiveConfigValue($key, [redacted])';
}

/// What `ConfigService.GetLiveConfig` returns (DAT-094): the `live` values
/// of an app and environment with their version and epoch.
final class LiveConfigSnapshot {
  /// Creates a snapshot.
  const LiveConfigSnapshot({
    required this.version,
    required this.epoch,
    required this.values,
  });

  /// Reads the snapshot from the server's JSON; throws [FormatException]
  /// when a value does not read.
  factory LiveConfigSnapshot.fromJson(Map<String, Object?> json) {
    final list = json['values'];
    return LiveConfigSnapshot(
      version: _number(json['version']),
      epoch: _number(json['epoch']),
      values: {
        if (list is List<Object?>)
          for (final v in list)
            if (v is Map<String, Object?>)
              LiveConfigValue.fromJson(v).key: LiveConfigValue.fromJson(v),
      },
    );
  }

  /// The version of the values, which every gateway call names
  /// (`Plux-Config-Version`, DAT-096).
  final int version;

  /// The epoch of the app and environment when the snapshot was made
  /// (`Plux-Config-Epoch`, DAT-095).
  final int epoch;

  /// The values by key.
  final Map<String, LiveConfigValue> values;

  @override
  String toString() => 'LiveConfigSnapshot(v$version, [redacted])';
}

int _number(Object? v) => switch (v) {
  int() => v,
  String() => int.tryParse(v) ?? 0,
  _ => 0,
};

/// The header on every gateway call that names the `live` configuration
/// version the device used (DAT-096).
const String pluxConfigVersionHeader = 'Plux-Config-Version';

/// The header on every gateway response that carries the epoch of the app's
/// configuration (DAT-095).
const String pluxConfigEpochHeader = 'Plux-Config-Epoch';

/// A code of the server-held configuration in the error catalogue.
///
/// The catalogue is generated (`make gen`), and the generated enum is read
/// by name, so this works before and after the codes are generated; [assumed]
/// is the number a test server answers with until they are.
final class ConfigCode {
  const ConfigCode._(this.name, this.assumed);

  /// The enum name in [PluxErrorCode].
  final String name;

  /// The number to use while [name] is not in the generated catalogue.
  final int assumed;

  /// The catalogue's entry, or null while it is not generated.
  PluxErrorCode? get registered {
    for (final c in PluxErrorCode.values) {
      if (c.name == name) return c;
    }
    return null;
  }

  /// The `PLX` number.
  int get number => registered?.code ?? assumed;

  /// The code to raise: the catalogue's entry, else the generic data
  /// failure it stands in for.
  PluxErrorCode get raised => registered ?? PluxErrorCode.dataNetworkFailed;

  /// Whether [plx] is this code's number.
  bool matches(int? plx) => plx != null && plx == number;

  /// A gateway call sent a `live` value's version that is older than a
  /// `strict` value's current one (DAT-096): refresh and retry once.
  static const ConfigCode stale = ConfigCode._('liveConfigStale', 6076);

  /// The `live` values cannot be had: offline, or the server cannot answer
  /// (DAT-097).
  static const ConfigCode unavailable = ConfigCode._(
    'liveConfigUnavailable',
    6077,
  );

  /// The server's configuration store is down, so it refuses the call
  /// rather than guess a value (DAT-092).
  static const ConfigCode storeUnavailable = ConfigCode._(
    'configStoreUnavailable',
    6078,
  );

  /// The device set a location the operation binds to a server-held value
  /// (DAT-091).
  static const ConfigCode boundLocationSet = ConfigCode._(
    'boundLocationSetByDevice',
    6074,
  );

  /// A bound value changed between the challenge and the execution
  /// (DAT-093).
  static const ConfigCode boundValueChanged = ConfigCode._(
    'boundValueChangedSinceChallenge',
    6075,
  );

  /// Every code of the server-held configuration.
  static const List<ConfigCode> all = [
    boundLocationSet,
    boundValueChanged,
    stale,
    unavailable,
    storeUnavailable,
  ];
}

/// Where pages read `live` configuration values (DAT-094, DAT-097); the
/// sync isolate holds the values, so a read is a request to it.
abstract interface class ConfigSource {
  /// The value of [key].
  ///
  /// A `strict` value is read from the server each time, and the read fails
  /// closed when it cannot be (DAT-097). Any other value is the last one
  /// this process received when the server cannot be reached. Fails with a
  /// `DataFailure` carrying `liveConfigUnavailable` when there is no value.
  Future<LiveConfigValue> read(String key);
}
