// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Targeting rules as the device evaluates them (REL-051, ABT-006, Q10):
/// the same condition and group shape as a segment document
/// (`schema/json/segment.schema.json`), matched against facts about this
/// device. Evaluation is pure and total: a fact the device lacks, values
/// that cannot be compared and rules this runtime cannot read all fail
/// closed, so a flag keeps its default and an experiment its control.
library;

/// What the device knows about itself for targeting: `build`,
/// `runtimeVersion`, `os`, `osVersion`, `deviceClass`, `locale`, `region`,
/// `channel` and `user.<name>`. Values are numbers or strings.
typedef TargetingFacts = Map<String, Object>;

/// A comparison operator of a condition.
enum TargetingOperator {
  /// Equal.
  eq('eq'),

  /// Not equal.
  neq('neq'),

  /// A member of the list.
  inList('in'),

  /// Not a member of the list.
  notIn('notIn'),

  /// Less than.
  lt('lt'),

  /// Less than or equal.
  lte('lte'),

  /// Greater than.
  gt('gt'),

  /// Greater than or equal.
  gte('gte');

  const TargetingOperator(this.wireName);

  /// The name a document uses.
  final String wireName;

  /// The operator named [name] in a document; null for an unknown one.
  static TargetingOperator? parse(Object? name) {
    for (final op in values) {
      if (op.wireName == name) return op;
    }
    return null;
  }
}

/// A rule: one condition or a group of rules.
sealed class TargetingRule {
  const TargetingRule();

  /// Reads the document form; null when it is not a rule this runtime can
  /// evaluate (an unknown operator, an empty group, a malformed value).
  static TargetingRule? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    if (json['never'] == true) return const TargetingNever();
    if (json['all'] case final List<Object?> all) {
      return _group(all, any: false);
    }
    if (json['any'] case final List<Object?> any) {
      return _group(any, any: true);
    }
    final attribute = json['attribute'];
    final op = TargetingOperator.parse(json['operator']);
    final value = json['value'];
    if (attribute is! String || op == null || !_validValue(op, value)) {
      return null;
    }
    return TargetingCondition(attribute, op, value!);
  }

  static TargetingRule? _group(List<Object?> items, {required bool any}) {
    final rules = [for (final i in items) fromJson(i)];
    if (rules.isEmpty || rules.contains(null)) return null;
    return TargetingGroup(rules.cast<TargetingRule>(), any: any);
  }

  static bool _validValue(TargetingOperator op, Object? value) {
    bool scalar(Object? v) => v is String || v is num || v is bool;
    return switch (op) {
      TargetingOperator.inList ||
      TargetingOperator.notIn => value is List<Object?> && value.every(scalar),
      _ => scalar(value),
    };
  }

  /// Whether the device described by [facts] is in the rule.
  bool matches(TargetingFacts facts);

  /// The document form.
  Object toJson();
}

/// A rule that matches no device: what a rule this runtime cannot evaluate
/// becomes, so that its flag segment or experiment is not matched rather
/// than matched wrongly (`schema/control.fbs`, `TargetingRule`).
final class TargetingNever extends TargetingRule {
  /// Creates the rule.
  const TargetingNever();

  @override
  bool matches(TargetingFacts facts) => false;

  @override
  Object toJson() => {'never': true};
}

/// Every rule holds ([any] false) or at least one does ([any] true).
final class TargetingGroup extends TargetingRule {
  /// Creates a group of at least one rule.
  const TargetingGroup(this.rules, {required this.any});

  /// The members.
  final List<TargetingRule> rules;

  /// Whether one member is enough.
  final bool any;

  @override
  bool matches(TargetingFacts facts) => any
      ? rules.any((r) => r.matches(facts))
      : rules.every((r) => r.matches(facts));

  @override
  Object toJson() => {
    any ? 'any' : 'all': [for (final r in rules) r.toJson()],
  };
}

/// One comparison of a device fact with a value.
final class TargetingCondition extends TargetingRule {
  /// Creates a condition; [value] is a list for `in` and `notIn`.
  const TargetingCondition(this.attribute, this.operator, this.value);

  /// The fact compared.
  final String attribute;

  /// How it is compared.
  final TargetingOperator operator;

  /// The scalar, or the list of scalars.
  final Object value;

  @override
  bool matches(TargetingFacts facts) {
    final fact = facts[attribute];
    if (fact == null) return false;
    return switch (operator) {
      TargetingOperator.eq => _test(fact, value, (c) => c == 0),
      TargetingOperator.neq => _test(fact, value, (c) => c != 0),
      TargetingOperator.inList => _contains(fact),
      TargetingOperator.notIn => !_contains(fact),
      TargetingOperator.lt => _test(fact, value, (c) => c < 0),
      TargetingOperator.lte => _test(fact, value, (c) => c <= 0),
      TargetingOperator.gt => _test(fact, value, (c) => c > 0),
      TargetingOperator.gte => _test(fact, value, (c) => c >= 0),
    };
  }

  @override
  Object toJson() => {
    'attribute': attribute,
    'operator': operator.wireName,
    'value': value,
  };

  bool _contains(Object fact) {
    final list = value;
    return list is List<Object?> &&
        list.any((v) => v != null && _test(fact, v, (c) => c == 0));
  }
}

bool _test(Object fact, Object value, bool Function(int order) holds) {
  final c = _compare(fact, value);
  return c != null && holds(c);
}

final _dotted = RegExp(r'^\d+(\.\d+)*$');
final _decimal = RegExp(r'^-?\d+(\.\d+)?$');

/// Orders [a] and [b] as dotted versions when both are (`1.10` after
/// `1.9`), else as numbers, else, for two strings or two equal values,
/// as text; null when they have no order, so every test on them fails.
int? _compare(Object a, Object b) {
  if (a is String &&
      b is String &&
      _dotted.hasMatch(a) &&
      _dotted.hasMatch(b)) {
    return _version(a, b);
  }
  final x = _numeric(a), y = _numeric(b);
  if (x != null && y != null) return x.compareTo(y);
  if (a is String && b is String) return a.compareTo(b).sign;
  return a == b ? 0 : null;
}

num? _numeric(Object v) => switch (v) {
  num() => v,
  String() when _decimal.hasMatch(v) => num.parse(v),
  _ => null,
};

/// The numbers of a dotted version; one too large for an integer counts as
/// the largest.
List<int> _segments(String v) => [
  for (final s in v.split('.')) int.tryParse(s) ?? 0x7fffffffffffffff,
];

int _version(String a, String b) {
  final x = _segments(a), y = _segments(b);
  for (var i = 0; i < x.length || i < y.length; i++) {
    final c = (i < x.length ? x[i] : 0).compareTo(i < y.length ? y[i] : 0);
    if (c != 0) return c;
  }
  return 0;
}
