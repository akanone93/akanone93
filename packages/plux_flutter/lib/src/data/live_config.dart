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
