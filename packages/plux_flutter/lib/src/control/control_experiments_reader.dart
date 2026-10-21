// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Reads the targeting rules, flags, experiment allocations and global
/// holdout of a control document that has passed the FlatBuffers verifier
/// (`schema/control.fbs`, ADR-0079) into the plain models the runtime
/// evaluates. A value from a newer server that this runtime cannot read is
/// never guessed at: a rule it cannot evaluate matches no device, and a
/// flag or experiment it cannot read is left out, so the app's default or
/// the control holds.
library;

import 'dart:convert';

import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';

/// How deeply the groups of a rule nest at most; the server encodes no more.
const int _maxRuleDepth = 16;

/// The largest number of seconds after the epoch that a [DateTime] holds.
const int _maxSeconds = 8640000000000;

/// The rule [r] as the runtime evaluates it, or null when it cannot: an
/// operator or value type this runtime does not know, an operand that does
/// not read as its type, an empty group or too deep a nesting.
TargetingRule? readRule(fbs.TargetingRule r, [int depth = 0]) {
  if (depth > _maxRuleDepth) return null;
  try {
    final attribute = r.attribute;
    if (attribute != null && attribute.isNotEmpty) {
      return _condition(attribute, r);
    }
    return _group(r, depth);
  } on StateError {
    return null;
  }
}

TargetingRule? _group(fbs.TargetingRule r, int depth) {
  final all = r.all ?? const <fbs.TargetingRule>[];
  final any = r.any ?? const <fbs.TargetingRule>[];
  if (all.isEmpty == any.isEmpty) return null;
  final members = all.isEmpty ? any : all;
  final rules = [for (final m in members) readRule(m, depth + 1)];
  if (rules.contains(null)) return null;
  return TargetingGroup(rules.cast<TargetingRule>(), any: all.isEmpty);
}

TargetingRule? _condition(String attribute, fbs.TargetingRule r) {
  final op = switch (r.$operator) {
    fbs.RuleOperator.Eq => TargetingOperator.eq,
    fbs.RuleOperator.Neq => TargetingOperator.neq,
    fbs.RuleOperator.In => TargetingOperator.inList,
    fbs.RuleOperator.NotIn => TargetingOperator.notIn,
    fbs.RuleOperator.Lt => TargetingOperator.lt,
    fbs.RuleOperator.Lte => TargetingOperator.lte,
    fbs.RuleOperator.Gt => TargetingOperator.gt,
    fbs.RuleOperator.Gte => TargetingOperator.gte,
    fbs.RuleOperator.Unspecified => null,
  };
  final type = r.valueType;
  if (op == null || type == fbs.RuleValueType.Unspecified) return null;
  final operands = [
    for (final v in r.values ?? const <String>[]) _operand(type, v),
  ];
  final list = op == TargetingOperator.inList || op == TargetingOperator.notIn;
  if (operands.isEmpty || operands.contains(null)) return null;
  if (!list && operands.length != 1) return null;
  return TargetingCondition(
    attribute,
    op,
    list ? operands.cast<Object>() : operands.single!,
  );
}

/// The operand [text] as [type] reads it: the text itself, the shortest
/// decimal text of a finite number, or `true` or `false`; null when it does
/// not read as [type].
Object? _operand(fbs.RuleValueType type, String text) => switch (type) {
  fbs.RuleValueType.String => text,
  fbs.RuleValueType.Number => _number(text),
  fbs.RuleValueType.Bool => switch (text) {
    'true' => true,
    'false' => false,
    _ => null,
  },
  fbs.RuleValueType.Unspecified => null,
};

num? _number(String text) {
  final n = int.tryParse(text) ?? double.tryParse(text);
  return n != null && n.isFinite ? n : null;
}

/// The flag [f], or null when this runtime cannot read it (an unknown type,
/// or a default that is not JSON of its type). A segment with a value that
/// does not fit the type, or with a rule that cannot be evaluated, never
/// applies and is left out.
ControlFlag? readFlag(fbs.FlagValue f) {
  final key = f.key, text = f.defaultValue;
  final FlagType? type;
  try {
    type = _flagType(f.type);
  } on StateError {
    return null;
  }
  if (key == null || text == null || type == null) return null;
  final fallback = _json(text);
  if (fallback == null || !type.accepts(fallback.value)) return null;
  return ControlFlag(
    key: key,
    type: type,
    defaultValue: fallback.value,
    segments: [
      for (final s in f.segments ?? const <fbs.FlagSegmentValue>[])
        if (_segment(s, type) case final FlagSegment known) known,
    ],
  );
}

FlagType? _flagType(fbs.FlagType type) => switch (type) {
  fbs.FlagType.Bool => FlagType.boolean,
  fbs.FlagType.Number => FlagType.number,
  fbs.FlagType.String => FlagType.string,
  fbs.FlagType.Json => FlagType.json,
  fbs.FlagType.Unspecified => null,
};

FlagSegment? _segment(fbs.FlagSegmentValue s, FlagType type) {
  final name = s.segment, table = s.rule, text = s.value;
  if (name == null || table == null || text == null) return null;
  final rule = readRule(table), value = _json(text);
  if (rule == null || value == null || !type.accepts(value.value)) {
    return null;
  }
  return FlagSegment(segment: name, rule: rule, value: value.value);
}

/// [text] decoded as JSON, wrapped so that the JSON value `null` differs
/// from text that is not JSON (null).
({Object? value})? _json(String text) {
  try {
    return (value: jsonDecode(text));
  } on FormatException {
    return null;
  }
}

/// The experiment [e], or null when this runtime cannot run it: an unknown
/// scope, no variants, not exactly one control, a layer range that is not
/// inside the buckets or a window outside the dates a device holds. A
/// segment this runtime cannot evaluate matches no device, so the
/// experiment stays and shows its control.
ControlExperiment? readExperiment(fbs.ExperimentAllocation e) {
  final key = e.key, salt = e.salt;
  final ExperimentScope? scope;
  try {
    scope = _scope(e.scope);
  } on StateError {
    return null;
  }
  final listed = e.variants ?? const <fbs.AllocationVariant>[];
  final variants = [
    for (final v in listed)
      if (v.name case final String name)
        ControlVariant(name: name, weightBp: v.weightBp, control: v.control),
  ];
  final layer = e.layer ?? '';
  final inLayer = layer.isNotEmpty;
  final badRange =
      inLayer && (e.layerStart > e.layerEnd || e.layerEnd > bucketCount);
  final badTime =
      e.startsAt.abs() > _maxSeconds || e.endsAt.abs() > _maxSeconds;
  if (key == null || salt == null || scope == null || badRange || badTime) {
    return null;
  }
  if (variants.length != listed.length) return null;
  if (!ControlExperiment.hasOneControl(variants)) return null;
  final segment = e.segment;
  return ControlExperiment(
    key: key,
    scope: scope,
    salt: salt,
    variants: variants,
    layer: layer,
    layerStart: inLayer ? e.layerStart : 0,
    layerEnd: inLayer ? e.layerEnd : 0,
    holdoutBp: e.holdoutBp,
    segment: segment == null
        ? null
        : readRule(segment) ?? const TargetingNever(),
    startsAt: _seconds(e.startsAt),
    endsAt: _seconds(e.endsAt),
  );
}

ExperimentScope? _scope(fbs.ExperimentScope scope) => switch (scope) {
  fbs.ExperimentScope.Page => ExperimentScope.page,
  fbs.ExperimentScope.Subtree => ExperimentScope.subtree,
  fbs.ExperimentScope.Prop => ExperimentScope.prop,
  fbs.ExperimentScope.Flow => ExperimentScope.flow,
  fbs.ExperimentScope.PluginVersion => ExperimentScope.pluginVersion,
  fbs.ExperimentScope.Unspecified => null,
};

/// The instant [seconds] after the epoch; null for zero, which the schema
/// reads as unbounded.
DateTime? _seconds(int seconds) => seconds == 0
    ? null
    : DateTime.fromMillisecondsSinceEpoch(seconds * 1000, isUtc: true);

/// The global holdout [h], or null when there is none or it has no salt.
ControlHoldout? readHoldout(fbs.GlobalHoldout? h) {
  final salt = h?.salt;
  return h == null || salt == null
      ? null
      : ControlHoldout(salt: salt, basisPoints: h.basisPoints);
}
