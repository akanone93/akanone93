// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Feature flags and experiment assignment on the device (ABT-002, ABT-004,
/// ABT-006, NAV-009, ADR-0079). The service reads the control document in
/// force at the moment a page, a guard or a `Variant` node asks, so a new
/// document takes effect without a restart. It evaluates targeting and
/// draws variants locally; the only things that leave the device are the
/// `flag_evaluated` and `experiment_exposure` events, under the
/// `experiments` consent, and neither carries a user attribute or an
/// identifier.
library;

import 'dart:math' as math;

import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/experiments/assignment.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';
import 'package:plux_flutter/src/state/persistence.dart';

/// Records a telemetry event; the recorder applies consent and sampling.
typedef ExperimentEventSink = void Function(
  String name, {
  Map<String, Object?> fields,
});

/// The fingerprint a stored assignment carries, which versions its form.
const String _assignmentForm = 'experiment-assignment-1';

/// The props of an experiment as the compiler embedded them in the app
/// bundle: the control's values, which the runtime shows for a device the
/// control document does not enrol, and each variant's (ABT-002).
final class ExperimentDefault {
  /// Creates the default of experiment [key] whose control variant is
  /// [control] and whose props have the values [props] for it, and
  /// [variants] for each variant by name.
  const ExperimentDefault({
    required this.key,
    required this.control,
    required this.props,
    this.variants = const {},
  });

  /// The experiment key, the PXL name in `experiment.<key>`.
  final String key;

  /// The name of the control variant.
  final String control;

  /// The value of each declared prop for the control, by prop name.
  final Map<String, Object?> props;

  /// The value of each declared prop for each variant, by variant name and
  /// prop name; a variant the bundle names no values for is not a key.
  final Map<String, Map<String, Object?>> variants;

  /// The props of [variant]: its own values, or the control's when the
  /// bundle carries none for it.
  Map<String, Object?> propsOf(String variant) => variants[variant] ?? props;
}

/// Evaluates flags and assigns experiments.
final class ExperimentService {
  /// Creates the service.
  ///
  /// [snapshot] answers with the control document in force, [consent] with
  /// whether `PluxConsent.experiments` is given, [facts] with what the
  /// device knows about itself for targeting, [userId] with the signed-in
  /// user's pseudonymous ID if one is set, and [installId] with the ID of
  /// this installation, which is the assignment subject otherwise.
  /// Assignments are kept in [persistence] so they stay sticky.
  ExperimentService({
    required this._snapshot,
    required this._consent,
    required this._facts,
    required this._userId,
    required this._installId,
    required this._persistence,
    required this._record,
    DateTime Function()? clock,
  }) : _clock = clock ?? DateTime.now;

  /// A service for a runtime without a control document: every flag has
  /// its default and every experiment shows its control.
  ExperimentService.none()
    : this(
        snapshot: () => null,
        consent: () => false,
        facts: () => const {},
        userId: () => null,
        installId: () => '',
        persistence: StatePersistence.inMemory(),
        record: _ignore,
      );

  static void _ignore(String name, {Map<String, Object?> fields = const {}}) {}

  final ControlSnapshot? Function() _snapshot;
  final bool Function() _consent;
  final TargetingFacts Function() _facts;
  final String? Function() _userId;
  final String Function() _installId;
  final StatePersistence _persistence;
  final ExperimentEventSink _record;
  final DateTime Function() _clock;

  final Map<String, String?> _drawn = {};
  Object? _drawnFor;
  final Set<String> _exposed = {};
  final Set<String> _evaluated = {};

  // ── Flags (ABT-006) ──────────────────────────────────────────────────────

  /// The flags of [defaults] (the app document's, by key) with the value
  /// the control document gives this device wherever it names the flag and
  /// the value has the default's kind. Returns [defaults] itself when no
  /// flag changes, which is the common case.
  Map<String, Object?> flags(Map<String, Object?> defaults) {
    Map<String, Object?>? out;
    for (final MapEntry(:key, :value) in defaults.entries) {
      final effective = flag(key, value);
      if (!identical(effective, value)) {
        (out ??= {...defaults})[key] = effective;
      }
    }
    return out ?? defaults;
  }

  /// The effective value of flag [key] whose default is [fallback]: what
  /// `flags.<key>` reads, for guards and host accessors alike. A flag the
  /// control document names takes the value of the first segment whose rule
  /// matches this device, else the document's default, provided the value
  /// has the kind of [fallback]; any other flag keeps its default.
  Object? flag(String key, Object? fallback) {
    final controlled = _snapshot()?.flags.where((f) => f.key == key);
    final found = controlled == null || controlled.isEmpty
        ? null
        : _controlledValue(controlled.first, fallback);
    final value = found == null ? fallback : found.value;
    _flagEvaluated(key, value, controlled: found != null);
    return value;
  }

  ({Object? value})? _controlledValue(ControlFlag flag, Object? fallback) {
    final coerced = _coerce(flag.valueFor(_facts()), fallback);
    return coerced.fits ? (value: coerced.value) : null;
  }

  void _flagEvaluated(String key, Object? value, {required bool controlled}) {
    final source = controlled ? 'control' : 'default';
    final type = _kind(value);
    if (!_evaluated.add('$key/$source/$type')) return;
    _record(
      'flag_evaluated',
      fields: {'flag': key, 'valueType': type, 'source': source},
    );
  }

  // ── Experiments (ABT-002, ABT-004) ───────────────────────────────────────

  /// The variant of experiment [key] this device is enrolled in, or null:
  /// without `experiments` consent, outside the control document, outside
  /// the schedule, in the global holdout or outside the experiment's
  /// targeting and share. Null means "show the control" and records
  /// nothing.
  ///
  /// A device keeps the variant it was first drawn into (persisted per
  /// experiment and salt) while that variant exists and the experiment
  /// still runs for it; changing weights, targeting or the user ID moves
  /// nobody.
  String? variantOf(String key) {
    if (!_consent()) return null;
    final snapshot = _snapshot();
    final experiment = snapshot?.experiments.where((e) => e.key == key);
    if (snapshot == null || experiment == null || experiment.isEmpty) {
      return null;
    }
    _forgetDrawsIfStale(snapshot);
    return _drawn.putIfAbsent(
      key,
      () => _assign(experiment.first, snapshot.holdout),
    );
  }

  String? _assign(ControlExperiment experiment, ControlHoldout? holdout) {
    if (!isScheduled(experiment, _clock())) return null;
    final subject = _userId() ?? _installId();
    if (isHeldOut(holdout, subject)) return null;
    final stored = _stored(experiment);
    if (stored != null) return stored;
    final drawn = drawVariant(experiment, subject, _facts());
    if (drawn != null) _store(experiment, drawn);
    return drawn;
  }

  String _storeKey(ControlExperiment e) => 'plux.experiment/${e.key}/${e.salt}';

  String? _stored(ControlExperiment experiment) {
    final value = _persistence
        .read(Persistence.persisted, _storeKey(experiment))
        ?.value;
    return value is String && experiment.variant(value) != null ? value : null;
  }

  void _store(ControlExperiment experiment, String variant) =>
      _persistence.write(
        Persistence.persisted,
        _storeKey(experiment),
        _assignmentForm,
        variant,
      );

  /// Forgets the draws of an earlier document, user or consent.
  void _forgetDrawsIfStale(ControlSnapshot snapshot) {
    final stamp = (snapshot, _userId());
    if (_drawnFor == stamp) return;
    _drawn.clear();
    _drawnFor = stamp;
  }

  /// The PXL `experiment` root (Appendix E.2): each running experiment's
  /// key holds the variant name shown on this device, the control's when
  /// the device is not enrolled, and `<key>.<prop>` the value of each prop
  /// of that variant.
  ///
  /// The values come from [defaults], what the bundle was compiled with
  /// (ABT-002): the shown variant's, else the control's. An experiment the
  /// control document does not name (no document, a document without it, or
  /// none in force) shows the control arm, so a binding to
  /// `experiment.<key>.<prop>` always has a value.
  Map<String, Object?> root([
    Map<String, ExperimentDefault> defaults = const {},
  ]) {
    final out = <String, Object?>{};
    for (final d in defaults.values) {
      out[d.key] = d.control;
      for (final p in d.props.entries) {
        out['${d.key}.${p.key}'] = p.value;
      }
    }
    for (final e in _snapshot()?.experiments ?? const <ControlExperiment>[]) {
      final shown = e.variant(variantOf(e.key) ?? '') ?? e.control;
      out[e.key] = shown.name;
      final arm = defaults[e.key]?.propsOf(shown.name) ?? const {};
      for (final p in arm.entries) {
        out['${e.key}.${p.key}'] = p.value;
      }
    }
    return out;
  }

  /// Records that a device enrolled in experiment [key] was shown
  /// [variant]: once per experiment per session, only under consent
  /// (ABT-002). The renderer calls it when the variant's node mounts,
  /// never at assignment.
  void recordExposure(String key, String variant) {
    if (!_consent() || !_exposed.add(key)) return;
    _record(
      'experiment_exposure',
      fields: {'experiment': key, 'variant': variant},
    );
  }
}

/// The JSON kind of [v] as `flag_evaluated` names it.
String _kind(Object? v) => switch (v) {
  bool() => 'bool',
  num() => 'number',
  String() => 'string',
  _ => 'json',
};

/// [value] in the form of the flag whose default is [fallback]: a whole
/// number is accepted for a decimal default and the other way round, a
/// value of another kind does not fit.
({bool fits, Object? value}) _coerce(Object? value, Object? fallback) {
  if (fallback == null) return (fits: true, value: value);
  if (fallback is double && value is int) {
    return (fits: true, value: value.toDouble());
  }
  if (fallback is int && value is double && value == value.truncateToDouble()) {
    return (fits: true, value: value.toInt());
  }
  final same =
      _kind(value) == _kind(fallback) && !(fallback is int && value is double);
  return (fits: same, value: value);
}

/// A random installation ID: 128 bits from a secure generator in hex.
String newInstallId(math.Random random) => [
  for (var i = 0; i < 16; i++)
    random.nextInt(256).toRadixString(16).padLeft(2, '0'),
].join();
