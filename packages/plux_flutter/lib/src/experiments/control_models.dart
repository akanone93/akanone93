// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a verified control document says about feature flags and
/// experiments (ABT-002, ABT-004, ABT-006, ADR-0079), as plain values: the
/// per-segment values of each flag, the allocation of each running
/// experiment and the global holdout. They are made on the sync isolate
/// once the document has been verified and read, kept on disk with the rest
/// of the snapshot, and evaluated on the UI isolate. Nothing here parses
/// FlatBuffers.
library;

import 'package:plux_flutter/src/experiments/targeting.dart';

/// Traffic is divided into this many buckets (REL-050).
const int bucketCount = 10000;

/// One value of a flag and the devices it applies to.
final class FlagCase {
  /// Creates a case; a null [rule] applies to every device.
  const FlagCase({required this.rule, required this.value});

  /// Reads the stored form; null when it is not one, or its rule cannot be
  /// evaluated (the flag then keeps its default).
  static FlagCase? fromJson(Object? json) {
    if (json is! Map<String, Object?> || !json.containsKey('value')) {
      return null;
    }
    final raw = json['rule'];
    final rule = raw == null ? null : TargetingRule.fromJson(raw);
    if (raw != null && rule == null) return null;
    return FlagCase(rule: rule, value: json['value']);
  }

  /// Who gets the value.
  final TargetingRule? rule;

  /// The value, in JSON form.
  final Object? value;

  /// The stored form.
  Map<String, Object?> toJson() => {'rule': rule?.toJson(), 'value': value};
}

/// The per-segment values of one flag; the first case whose rule matches
/// wins (ABT-006).
final class ControlFlag {
  /// Creates a flag.
  const ControlFlag({required this.key, required this.cases});

  /// Reads the stored form; null when it is not one.
  static ControlFlag? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final key = json['key'], cases = json['cases'];
    if (key is! String || cases is! List<Object?>) return null;
    return ControlFlag(
      key: key,
      cases: [
        for (final c in cases)
          if (FlagCase.fromJson(c) case final FlagCase parsed) parsed,
      ],
    );
  }

  /// The flag's key, as `flags.<key>` names it.
  final String key;

  /// The cases, in order.
  final List<FlagCase> cases;

  /// The stored form.
  Map<String, Object?> toJson() => {
    'key': key,
    'cases': [for (final c in cases) c.toJson()],
  };
}

/// One variant of an experiment.
final class ControlVariant {
  /// Creates a variant.
  const ControlVariant({
    required this.name,
    required this.weight,
    this.props = const {},
  });

  /// Reads the stored form; null when it is not one.
  static ControlVariant? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final name = json['name'], weight = json['weight'];
    final props = json['props'];
    if (name is! String || weight is! int || weight < 0) return null;
    if (props != null && props is! Map<String, Object?>) return null;
    return ControlVariant(
      name: name,
      weight: weight,
      props: (props as Map<String, Object?>?) ?? const {},
    );
  }

  /// The variant's name.
  final String name;

  /// Its share of the allocation, relative to the other variants'.
  final int weight;

  /// The values of the experiment's props for this variant, in JSON form
  /// (`experiment.<key>.<prop>` in PXL).
  final Map<String, Object?> props;

  /// The stored form.
  Map<String, Object?> toJson() => {
    'name': name,
    'weight': weight,
    'props': props,
  };
}

/// The allocation of one running experiment.
final class ControlExperiment {
  /// Creates an experiment. [variants] lists the control first.
  const ControlExperiment({
    required this.key,
    required this.salt,
    required this.variants,
    this.layer,
    this.rangeStart = 0,
    this.rangeEnd = bucketCount,
    this.holdout = 0,
    this.rule,
    this.startsAt,
    this.endsAt,
  });

  /// Reads the stored form; null when it is not one, or it cannot be run
  /// (no variants, no weight, a rule this runtime cannot evaluate): such an
  /// experiment shows its control.
  static ControlExperiment? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final key = json['key'], salt = json['salt'], list = json['variants'];
    if (key is! String || salt is! String || list is! List<Object?>) {
      return null;
    }
    final variants = [for (final v in list) ControlVariant.fromJson(v)];
    if (variants.isEmpty || variants.contains(null)) return null;
    final raw = json['rule'];
    final rule = raw == null ? null : TargetingRule.fromJson(raw);
    final start = json['rangeStart'] ?? 0,
        end = json['rangeEnd'] ?? bucketCount;
    final holdout = json['holdout'] ?? 0;
    final layer = json['layer'];
    if (raw != null && rule == null) return null;
    if (start is! int || end is! int || holdout is! int) return null;
    if (layer != null && layer is! String) return null;
    return ControlExperiment(
      key: key,
      salt: salt,
      variants: variants.cast<ControlVariant>(),
      layer: layer as String?,
      rangeStart: start,
      rangeEnd: end,
      holdout: holdout,
      rule: rule,
      startsAt: _time(json['startsAt']),
      endsAt: _time(json['endsAt']),
    );
  }

  static DateTime? _time(Object? ms) =>
      ms is int ? DateTime.fromMillisecondsSinceEpoch(ms, isUtc: true) : null;

  /// The experiment's key, as `experiment.<key>` and the Variant node name
  /// it.
  final String key;

  /// The salt of its assignment hash; a new salt re-draws every device.
  final String salt;

  /// The variants, the control first (`arms[0]` of the Variant node).
  final List<ControlVariant> variants;

  /// The layer the experiment shares traffic in, or null for the whole
  /// population (ABT-004). Experiments of one layer own disjoint ranges.
  final String? layer;

  /// The first bucket of the layer (or population) the experiment owns.
  final int rangeStart;

  /// One past the last bucket it owns.
  final int rangeEnd;

  /// The share, in buckets of [bucketCount], kept out of this experiment.
  final int holdout;

  /// Who is eligible, or null for everyone.
  final TargetingRule? rule;

  /// When the experiment starts, or null for already.
  final DateTime? startsAt;

  /// When it ends, or null for never.
  final DateTime? endsAt;

  /// The control variant.
  ControlVariant get control => variants.first;

  /// The variant named [name], or null.
  ControlVariant? variant(String name) {
    for (final v in variants) {
      if (v.name == name) return v;
    }
    return null;
  }

  /// The stored form.
  Map<String, Object?> toJson() => {
    'key': key,
    'salt': salt,
    'variants': [for (final v in variants) v.toJson()],
    'layer': layer,
    'rangeStart': rangeStart,
    'rangeEnd': rangeEnd,
    'holdout': holdout,
    'rule': rule?.toJson(),
    'startsAt': startsAt?.millisecondsSinceEpoch,
    'endsAt': endsAt?.millisecondsSinceEpoch,
  };
}

/// The global holdout (ABT-004): the share of subjects kept out of every
/// experiment and shown every control.
final class ControlHoldout {
  /// Creates a holdout of [size] buckets of [bucketCount].
  const ControlHoldout({required this.salt, required this.size});

  /// Reads the stored form; null when it is not one.
  static ControlHoldout? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final salt = json['salt'], size = json['size'];
    if (salt is! String || size is! int) return null;
    return ControlHoldout(salt: salt, size: size);
  }

  /// The salt of its hash.
  final String salt;

  /// How many buckets it holds.
  final int size;

  /// The stored form.
  Map<String, Object?> toJson() => {'salt': salt, 'size': size};
}
