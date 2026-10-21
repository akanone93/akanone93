// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a verified control document says about feature flags and
/// experiments (ABT-002, ABT-004, ABT-006, ADR-0079), as plain values: the
/// value and segments of each flag, the allocation of each running
/// experiment and the global holdout. The names follow `FlagValue`,
/// `ExperimentAllocation` and `GlobalHoldout` of `schema/control.fbs`. They
/// are made on the sync isolate once the document has been verified and
/// read, kept on disk with the rest of the snapshot, and evaluated on the
/// UI isolate. Nothing here parses FlatBuffers; `toJson` and `fromJson` are
/// the stored form of the snapshot only.
library;

import 'package:plux_flutter/src/experiments/targeting.dart';

/// Traffic is divided into this many buckets (REL-050).
const int bucketCount = 10000;

/// The type of a flag (`FlagType` of `schema/control.fbs`, without the
/// unspecified member).
enum FlagType {
  /// `true` or `false`.
  boolean,

  /// A JSON number.
  number,

  /// A string.
  string,

  /// Any JSON value.
  json;

  /// Whether [value], decoded from JSON, is of this type.
  bool accepts(Object? value) => switch (this) {
    FlagType.boolean => value is bool,
    FlagType.number => value is num,
    FlagType.string => value is String,
    FlagType.json => true,
  };
}

/// The value of a flag for the devices of one segment.
final class FlagSegment {
  /// Creates a segment value.
  const FlagSegment({
    required this.segment,
    required this.rule,
    required this.value,
  });

  /// Reads the stored form; null when it is not one, or its rule cannot be
  /// evaluated (such a segment never matches).
  static FlagSegment? fromJson(Object? json) {
    if (json is! Map<String, Object?> || !json.containsKey('value')) {
      return null;
    }
    final segment = json['segment'],
        rule = TargetingRule.fromJson(json['rule']);
    if (segment is! String || rule == null) return null;
    return FlagSegment(segment: segment, rule: rule, value: json['value']);
  }

  /// The segment's key, for reports.
  final String segment;

  /// Who gets the value.
  final TargetingRule rule;

  /// The value, in JSON form.
  final Object? value;

  /// The stored form.
  Map<String, Object?> toJson() => {
    'segment': segment,
    'rule': rule.toJson(),
    'value': value,
  };
}

/// One feature flag: the value of the first segment whose rule matches the
/// device, otherwise [defaultValue] (ABT-006).
final class ControlFlag {
  /// Creates a flag.
  const ControlFlag({
    required this.key,
    required this.type,
    required this.defaultValue,
    this.segments = const [],
  });

  /// Reads the stored form; null when it is not one.
  static ControlFlag? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final key = json['key'], list = json['segments'];
    final type = FlagType.values.asNameMap()[json['type']];
    if (key is! String || type == null || list is! List<Object?>) return null;
    if (!json.containsKey('defaultValue') ||
        !type.accepts(json['defaultValue'])) {
      return null;
    }
    return ControlFlag(
      key: key,
      type: type,
      defaultValue: json['defaultValue'],
      segments: [
        for (final s in list)
          if (FlagSegment.fromJson(s) case final FlagSegment parsed)
            if (type.accepts(parsed.value)) parsed,
      ],
    );
  }

  /// The flag's key, as `flags.<key>` names it.
  final String key;

  /// The type of its values.
  final FlagType type;

  /// The value for devices in none of the [segments], in JSON form.
  final Object? defaultValue;

  /// The per-segment values in precedence order.
  final List<FlagSegment> segments;

  /// The value for the device described by [facts].
  Object? valueFor(TargetingFacts facts) {
    for (final s in segments) {
      if (s.rule.matches(facts)) return s.value;
    }
    return defaultValue;
  }

  /// The stored form.
  Map<String, Object?> toJson() => {
    'key': key,
    'type': type.name,
    'defaultValue': defaultValue,
    'segments': [for (final s in segments) s.toJson()],
  };
}

/// What an experiment varies (`ExperimentScope` of `schema/control.fbs`,
/// without the unspecified member).
enum ExperimentScope {
  /// A whole page.
  page,

  /// A component subtree, a `Variant` node.
  subtree,

  /// A prop value.
  prop,

  /// A flow.
  flow,

  /// A plugin version.
  pluginVersion,
}

/// One variant of an experiment with its share of the population.
final class ControlVariant {
  /// Creates a variant.
  const ControlVariant({
    required this.name,
    required this.weightBp,
    this.control = false,
    this.props = const {},
  });

  /// Reads the stored form; null when it is not one.
  static ControlVariant? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final name = json['name'], weight = json['weightBp'];
    final control = json['control'], props = json['props'];
    if (name is! String || weight is! int || weight < 0) return null;
    if (control is! bool || (props != null && props is! Map<String, Object?>)) {
      return null;
    }
    return ControlVariant(
      name: name,
      weightBp: weight,
      control: control,
      props: (props as Map<String, Object?>?) ?? const {},
    );
  }

  /// The variant's name, as in the compiled release.
  final String name;

  /// Whether this is the control.
  final bool control;

  /// Its share in basis points of [bucketCount].
  final int weightBp;

  /// The values of the experiment's props for this variant, in JSON form
  /// (`experiment.<key>.<prop>` in PXL). The control document carries none:
  /// `AllocationVariant` has no such field, so a document read from the wire
  /// leaves this empty.
  final Map<String, Object?> props;

  /// The stored form.
  Map<String, Object?> toJson() => {
    'name': name,
    'control': control,
    'weightBp': weightBp,
    'props': props,
  };
}

/// The allocation of one running experiment.
final class ControlExperiment {
  /// Creates an experiment.
  const ControlExperiment({
    required this.key,
    required this.salt,
    required this.variants,
    this.scope = ExperimentScope.subtree,
    this.layer = '',
    this.layerStart = 0,
    this.layerEnd = 0,
    this.holdoutBp = 0,
    this.segment,
    this.startsAt,
    this.endsAt,
  });

  /// Reads the stored form; null when it is not one, or it cannot be run
  /// (no variants, not exactly one control): such an experiment shows its
  /// control.
  static ControlExperiment? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final key = json['key'], salt = json['salt'], list = json['variants'];
    final scope = ExperimentScope.values.asNameMap()[json['scope']];
    if (key is! String || salt is! String || scope == null) return null;
    if (list is! List<Object?>) return null;
    final variants = [for (final v in list) ControlVariant.fromJson(v)];
    if (variants.contains(null) || !hasOneControl(variants.nonNulls)) {
      return null;
    }
    final raw = json['segment'];
    final segment = raw == null ? null : TargetingRule.fromJson(raw);
    final layer = json['layer'];
    final ints = [json['layerStart'], json['layerEnd'], json['holdoutBp']];
    if (raw != null && segment == null) return null;
    if (layer is! String || ints.any((v) => v is! int)) return null;
    return ControlExperiment(
      key: key,
      scope: scope,
      salt: salt,
      variants: variants.cast<ControlVariant>(),
      layer: layer,
      layerStart: ints[0]! as int,
      layerEnd: ints[1]! as int,
      holdoutBp: ints[2]! as int,
      segment: segment,
      startsAt: _time(json['startsAt']),
      endsAt: _time(json['endsAt']),
    );
  }

  /// Whether exactly one of [variants] is the control, and there is one.
  static bool hasOneControl(Iterable<ControlVariant> variants) =>
      variants.where((v) => v.control).length == 1;

  static DateTime? _time(Object? ms) =>
      ms is int ? DateTime.fromMillisecondsSinceEpoch(ms, isUtc: true) : null;

  /// The experiment's key, as `experiment.<key>` and the Variant node name
  /// it.
  final String key;

  /// What it varies.
  final ExperimentScope scope;

  /// The salt of its assignment hash; a new salt re-draws every device.
  final String salt;

  /// The control and the variants, in the order of the experiment document.
  final List<ControlVariant> variants;

  /// The layer the experiment shares traffic in; empty when it is in none
  /// (ABT-004). Experiments of one layer own disjoint ranges.
  final String layer;

  /// The first bucket of the layer the experiment owns; zero without a
  /// layer.
  final int layerStart;

  /// One past the last bucket it owns; zero without a layer.
  final int layerEnd;

  /// The share, in basis points, of the experiment's population that is
  /// shown the control and kept out of the variants.
  final int holdoutBp;

  /// Who is eligible, or null for everyone.
  final TargetingRule? segment;

  /// When the experiment starts, or null for already.
  final DateTime? startsAt;

  /// When it ends, or null for never.
  final DateTime? endsAt;

  /// The control variant.
  ControlVariant get control =>
      variants.firstWhere((v) => v.control, orElse: () => variants.first);

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
    'scope': scope.name,
    'salt': salt,
    'variants': [for (final v in variants) v.toJson()],
    'layer': layer,
    'layerStart': layerStart,
    'layerEnd': layerEnd,
    'holdoutBp': holdoutBp,
    'segment': segment?.toJson(),
    'startsAt': startsAt?.millisecondsSinceEpoch,
    'endsAt': endsAt?.millisecondsSinceEpoch,
  };
}

/// The global holdout (ABT-004): the share of subjects kept out of every
/// experiment and shown every control.
final class ControlHoldout {
  /// Creates a holdout of [basisPoints] of [bucketCount].
  const ControlHoldout({required this.salt, required this.basisPoints});

  /// Reads the stored form; null when it is not one.
  static ControlHoldout? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final salt = json['salt'], basisPoints = json['basisPoints'];
    if (salt is! String || basisPoints is! int) return null;
    return ControlHoldout(salt: salt, basisPoints: basisPoints);
  }

  /// The salt of its hash.
  final String salt;

  /// Its share in basis points.
  final int basisPoints;

  /// The stored form.
  Map<String, Object?> toJson() => {'salt': salt, 'basisPoints': basisPoints};
}
