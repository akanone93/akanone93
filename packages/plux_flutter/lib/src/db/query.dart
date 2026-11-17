// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Typed queries over a collection (DB-006): a filter tree over fields,
/// sort keys, a limit and an offset. Adapters that cannot push a query
/// down evaluate it with [DbQuery.run]; the evaluation is the reference
/// every adapter's results must equal.
library;

import 'dart:convert';

import 'package:plux_flutter/src/db/schema.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';

/// A comparison of a field with a value.
enum DbOp {
  /// Equal; for `json` fields, equal as JSON.
  eq,

  /// Not equal.
  ne,

  /// Less than; scalar fields only.
  lt,

  /// Less than or equal.
  le,

  /// Greater than.
  gt,

  /// Greater than or equal.
  ge,

  /// The field is one of a list of values.
  isIn,

  /// The text field contains a text.
  contains,

  /// The text field starts with a text.
  startsWith,

  /// The field is null; takes no value.
  isNull,

  /// The field is not null; takes no value.
  notNull,
}

/// A condition on records.
sealed class DbFilter {
  const DbFilter();

  /// Reads a filter from its JSON form, as a `database` data source
  /// configures it: `{"field": "done", "op": "eq", "value": false}`,
  /// `{"and": [...]}`, `{"or": [...]}` or `{"not": {...}}`. Throws
  /// `PLX-5206` for a malformed filter.
  factory DbFilter.fromJson(Object? json) {
    if (json is! Map<String, Object?>) throw _bad('a filter is an object');
    if (json['and'] case final List<Object?> l) {
      return DbAnd([for (final f in l) DbFilter.fromJson(f)]);
    }
    if (json['or'] case final List<Object?> l) {
      return DbOr([for (final f in l) DbFilter.fromJson(f)]);
    }
    if (json.containsKey('not')) return DbNot(DbFilter.fromJson(json['not']));
    final field = json['field'], op = json['op'];
    final parsed = DbOp.values.where(
      (o) => o.name == op || (o == DbOp.isIn && op == 'in'),
    );
    if (field is! String || parsed.isEmpty) {
      throw _bad('a filter names a field and one of the operators');
    }
    final param = json['param'];
    if (param != null && param is! String) {
      throw _bad('a comparison names its param with a string');
    }
    return DbCompare(field, parsed.first, json['value'], param as String?);
  }

  /// Whether [record] satisfies the condition.
  bool matches(Map<String, Object?> record);

  /// The filter with the value of every comparison that names a param
  /// taken from [params], the source's params as its scope evaluates them
  /// (DB-006). Throws `PLX-5206` for a param the source does not give.
  DbFilter bind(Map<String, Object?> params);

  /// Checks the condition against [schema]: known fields, operators the
  /// field's kind supports and values of its type. Throws `PLX-5206`.
  void check(DbCollectionSchema schema);

  /// The filter in the JSON form [DbFilter.fromJson] reads.
  Object? toJson();
}

PluxException _bad(String message) =>
    PluxException(PluxErrorCode.dbQueryInvalid, message);

/// A field compared with a value.
final class DbCompare extends DbFilter {
  /// Creates the comparison; with [param], its value is the source's param
  /// of that name, bound when the watch starts.
  const DbCompare(this.field, this.op, [this.value, this.param]);

  /// The field.
  final String field;

  /// The operator.
  final DbOp op;

  /// The value; a list for [DbOp.isIn], absent for the null tests.
  final Object? value;

  /// The name of the source's param that gives the value, or null.
  final String? param;

  @override
  DbFilter bind(Map<String, Object?> params) {
    final p = param;
    if (p == null) return this;
    if (!params.containsKey(p)) {
      throw _bad('the filter names param $p, which the source does not give');
    }
    return DbCompare(field, op, params[p]);
  }

  @override
  bool matches(Map<String, Object?> record) {
    final v = record[field];
    switch (op) {
      case DbOp.isNull:
        return v == null;
      case DbOp.notNull:
        return v != null;
      case DbOp.eq:
        return _equal(v, value);
      case DbOp.ne:
        return !_equal(v, value);
      case DbOp.isIn:
        return (value! as List<Object?>).any((x) => _equal(v, x));
      case DbOp.contains:
        return v is String && v.contains(value! as String);
      case DbOp.startsWith:
        return v is String && v.startsWith(value! as String);
      case DbOp.lt || DbOp.le || DbOp.gt || DbOp.ge:
        if (v == null) return false;
        final c = compareValues(v, value);
        return switch (op) {
          DbOp.lt => c < 0,
          DbOp.le => c <= 0,
          DbOp.gt => c > 0,
          _ => c >= 0,
        };
    }
  }

  @override
  void check(DbCollectionSchema schema) {
    final f = schema.field(field);
    if (f == null) throw _bad('$field is not a field of ${schema.key}');
    final scalar = f.kind != DbFieldKind.json;
    final ordered = {DbOp.lt, DbOp.le, DbOp.gt, DbOp.ge};
    final text = {DbOp.contains, DbOp.startsWith};
    if ((ordered.contains(op) && !scalar) ||
        (text.contains(op) && f.kind != DbFieldKind.string)) {
      throw _bad('${op.name} does not apply to $field (${f.type})');
    }
    if (op == DbOp.isNull || op == DbOp.notNull || param != null) return;
    final values = op == DbOp.isIn
        ? (value is List<Object?>
              ? value! as List<Object?>
              : throw _bad('in takes a list'))
        : [value];
    for (final v in values) {
      final ok = switch (f.kind) {
        DbFieldKind.string => v is String,
        DbFieldKind.integer => v is int,
        DbFieldKind.real => v is num,
        DbFieldKind.boolean => v is bool,
        DbFieldKind.json => true,
      };
      if (!(ok || (v == null && (op == DbOp.eq || op == DbOp.ne)))) {
        throw _bad('the value compared with $field is not a ${f.type}');
      }
    }
  }

  @override
  Object? toJson() => {
    'field': field,
    'op': op == DbOp.isIn ? 'in' : op.name,
    if (param != null)
      'param': param
    else if (op != DbOp.isNull && op != DbOp.notNull)
      'value': value,
  };
}

/// All of the filters.
final class DbAnd extends DbFilter {
  /// Creates the conjunction.
  const DbAnd(this.filters);

  /// The filters.
  final List<DbFilter> filters;

  @override
  bool matches(Map<String, Object?> record) =>
      filters.every((f) => f.matches(record));

  @override
  DbFilter bind(Map<String, Object?> params) =>
      DbAnd([for (final f in filters) f.bind(params)]);

  @override
  void check(DbCollectionSchema schema) {
    for (final f in filters) {
      f.check(schema);
    }
  }

  @override
  Object? toJson() => {
    'and': [for (final f in filters) f.toJson()],
  };
}

/// Any of the filters.
final class DbOr extends DbFilter {
  /// Creates the disjunction.
  const DbOr(this.filters);

  /// The filters.
  final List<DbFilter> filters;

  @override
  bool matches(Map<String, Object?> record) =>
      filters.any((f) => f.matches(record));

  @override
  DbFilter bind(Map<String, Object?> params) =>
      DbOr([for (final f in filters) f.bind(params)]);

  @override
  void check(DbCollectionSchema schema) {
    for (final f in filters) {
      f.check(schema);
    }
  }

  @override
  Object? toJson() => {
    'or': [for (final f in filters) f.toJson()],
  };
}

/// The negation of a filter.
final class DbNot extends DbFilter {
  /// Creates the negation.
  const DbNot(this.filter);

  /// The negated filter.
  final DbFilter filter;

  @override
  bool matches(Map<String, Object?> record) => !filter.matches(record);

  @override
  DbFilter bind(Map<String, Object?> params) => DbNot(filter.bind(params));

  @override
  void check(DbCollectionSchema schema) => filter.check(schema);

  @override
  Object? toJson() => {'not': filter.toJson()};
}

/// A sort key.
final class DbSort {
  /// Creates a sort key.
  const DbSort(this.field, {this.descending = false});

  /// The field.
  final String field;

  /// Whether larger values come first.
  final bool descending;
}

/// A query: records matching [filter], ordered by [sort], from [offset],
/// at most [limit]. Without a sort, records come in key order, so results
/// are deterministic.
final class DbQuery {
  /// Creates a query.
  const DbQuery({
    this.filter,
    this.sort = const [],
    this.limit,
    this.offset = 0,
  });

  /// The condition, or null for every record.
  final DbFilter? filter;

  /// The sort keys, most significant first.
  final List<DbSort> sort;

  /// The most records returned, or null for all.
  final int? limit;

  /// The matching records skipped.
  final int offset;

  /// Checks the query against [schema]; throws `PLX-5206`.
  void check(DbCollectionSchema schema) {
    if (limit != null && limit! < 0) throw _bad('the limit is negative');
    if (offset < 0) throw _bad('the offset is negative');
    filter?.check(schema);
    for (final s in sort) {
      final f = schema.field(s.field);
      if (f == null) throw _bad('${s.field} is not a field of ${schema.key}');
      if (f.kind == DbFieldKind.json) {
        throw _bad('${s.field} (${f.type}) cannot be sorted');
      }
    }
  }

  /// Evaluates the query over [rows], which hold the collection: the
  /// reference semantics. Null sorts before any value; ties break by key.
  List<Map<String, Object?>> run(
    DbCollectionSchema schema,
    Iterable<Map<String, Object?>> rows,
  ) {
    var out = [
      for (final r in rows)
        if (filter?.matches(r) ?? true) r,
    ];
    out.sort((a, b) {
      for (final s in sort) {
        final c = compareValues(a[s.field], b[s.field]);
        if (c != 0) return s.descending ? -c : c;
      }
      return schema.keyOf(a).compareTo(schema.keyOf(b));
    });
    if (offset > 0) out = out.skip(offset).toList();
    if (limit != null) out = out.take(limit!).toList();
    return out;
  }
}

/// Orders two field values: null first, then numbers, texts and booleans
/// (false first) by their own order.
int compareValues(Object? a, Object? b) {
  if (a == null || b == null) {
    return a == null ? (b == null ? 0 : -1) : 1;
  }
  if (a is num && b is num) return a.compareTo(b);
  if (a is String && b is String) return a.compareTo(b);
  if (a is bool && b is bool) return a == b ? 0 : (a ? 1 : -1);
  return jsonEncode(a).compareTo(jsonEncode(b));
}

bool _equal(Object? a, Object? b) {
  if (a == null || b == null) return a == null && b == null;
  if (a is num && b is num) return a == b;
  if (a is String || a is bool) return a == b;
  return jsonEncode(a) == jsonEncode(b);
}
