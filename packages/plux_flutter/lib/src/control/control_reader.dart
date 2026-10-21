// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Reads the FlatBuffers document of a verified control envelope
/// (`schema/control.fbs`, ADR-0077). The buffer goes through the
/// FlatBuffers verifier first (SEC-052, ADR-0029) against the layout tables
/// below, and only a buffer that passes is read with the generated reader.
/// Unknown kill switch scopes and unknown fields come from a newer server
/// and are skipped (additive evolution).
library;

import 'package:plux_flutter/src/control/control_experiments_reader.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/schema/limits.g.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/flatbuffers_verifier.dart';
import 'package:plux_flutter/src/verify/update_metadata.dart';

/// The file identifier of a control document.
const String controlIdentifier = 'PLXC';

/// The layout tables of `schema/control.fbs`, in the form the verifier
/// walks, written by hand because the generator emits the bundle sections
/// only. A test builds a document with the generated builder and checks it
/// against these, so a change of the schema fails there.
const List<TableLayout> controlLayouts = [
  // 0
  TableLayout('TargetingRule', [
    FieldLayout('attribute', 0, FieldKind.string, 0, 0, -1, required: false),
    FieldLayout('operator', 1, FieldKind.scalar, 1, 1, -1, required: false),
    FieldLayout('value_type', 2, FieldKind.scalar, 1, 1, -1, required: false),
    FieldLayout('values', 3, FieldKind.vectorString, 0, 0, -1, required: false),
    FieldLayout('all', 4, FieldKind.vectorTable, 0, 0, 0, required: false),
    FieldLayout('any', 5, FieldKind.vectorTable, 0, 0, 0, required: false),
  ]),
  // 1
  TableLayout('KillSwitch', [
    FieldLayout('scope', 0, FieldKind.scalar, 1, 1, -1, required: false),
    FieldLayout('target', 1, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('rule', 2, FieldKind.table, 0, 0, 0, required: false),
  ]),
  // 2
  TableLayout('MandatoryUpdate', [
    FieldLayout('release_id', 0, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout(
      'release_sequence',
      1,
      FieldKind.scalar,
      8,
      8,
      -1,
      required: false,
    ),
    FieldLayout('message_key', 2, FieldKind.string, 0, 0, -1, required: false),
  ]),
  // 3
  TableLayout('ForcedRollback', [
    FieldLayout('release_id', 0, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout(
      'release_sequence',
      1,
      FieldKind.scalar,
      8,
      8,
      -1,
      required: false,
    ),
  ]),
  // 4
  TableLayout('FlagValue', [
    FieldLayout('key', 0, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('type', 1, FieldKind.scalar, 1, 1, -1, required: false),
    FieldLayout('default_value', 2, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('segments', 3, FieldKind.vectorTable, 0, 0, 7, required: false),
  ]),
  // 5
  TableLayout('ExperimentAllocation', [
    FieldLayout('key', 0, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('scope', 1, FieldKind.scalar, 1, 1, -1, required: false),
    FieldLayout('salt', 2, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('variants', 3, FieldKind.vectorTable, 0, 0, 8, required: false),
    FieldLayout('layer', 4, FieldKind.string, 0, 0, -1, required: false),
    FieldLayout('layer_start', 5, FieldKind.scalar, 4, 4, -1, required: false),
    FieldLayout('layer_end', 6, FieldKind.scalar, 4, 4, -1, required: false),
    FieldLayout('holdout_bp', 7, FieldKind.scalar, 4, 4, -1, required: false),
    FieldLayout('segment', 8, FieldKind.table, 0, 0, 0, required: false),
    FieldLayout('starts_at', 9, FieldKind.scalar, 8, 8, -1, required: false),
    FieldLayout('ends_at', 10, FieldKind.scalar, 8, 8, -1, required: false),
  ]),
  // 6
  TableLayout('ControlDocument', [
    FieldLayout('version', 0, FieldKind.scalar, 8, 8, -1, required: false),
    FieldLayout('app_id', 1, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('environment', 2, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('channel', 3, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('issued_at', 4, FieldKind.scalar, 8, 8, -1, required: false),
    FieldLayout('expires_at', 5, FieldKind.scalar, 8, 8, -1, required: false),
    FieldLayout(
      'kill_switches',
      6,
      FieldKind.vectorTable,
      0,
      0,
      1,
      required: false,
    ),
    FieldLayout(
      'mandatory_update',
      7,
      FieldKind.table,
      0,
      0,
      2,
      required: false,
    ),
    FieldLayout(
      'forced_rollback',
      8,
      FieldKind.table,
      0,
      0,
      3,
      required: false,
    ),
    FieldLayout('flags', 9, FieldKind.vectorTable, 0, 0, 4, required: false),
    FieldLayout(
      'experiments',
      10,
      FieldKind.vectorTable,
      0,
      0,
      5,
      required: false,
    ),
    FieldLayout('holdout', 11, FieldKind.table, 0, 0, 9, required: false),
  ]),
  // 7
  TableLayout('FlagSegmentValue', [
    FieldLayout('segment', 0, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('rule', 1, FieldKind.table, 0, 0, 0, required: true),
    FieldLayout('value', 2, FieldKind.string, 0, 0, -1, required: true),
  ]),
  // 8
  TableLayout('AllocationVariant', [
    FieldLayout('name', 0, FieldKind.string, 0, 0, -1, required: true),
    FieldLayout('control', 1, FieldKind.scalar, 1, 1, -1, required: false),
    FieldLayout('weight_bp', 2, FieldKind.scalar, 4, 4, -1, required: false),
  ]),
  // 9
  TableLayout('GlobalHoldout', [
    FieldLayout('basis_points', 0, FieldKind.scalar, 4, 4, -1, required: false),
    FieldLayout('salt', 1, FieldKind.string, 0, 0, -1, required: true),
  ]),
];

const int _documentTable = 6;

PluxException _invalid(String message) => PluxException(
  PluxErrorCode.controlSignatureInvalid,
  'control document: $message',
  details: {'fault': MetadataFault.format},
);

/// Verifies the FlatBuffers [bytes] of [envelope] and reads them: the kill
/// switches, the mandatory update, the forced rollback, the flags, the
/// experiment allocations and the global holdout. The document must say
/// what the signed header says (version, channel, times), and carry at most
/// `control.killSwitches` switches, `flags.perApp` flags and
/// `experiments.activePerApp` experiments. Throws `PLX-3100` for any buffer
/// that is not a well-formed, agreeing document.
ControlSnapshot readControlDocument(
  ControlEnvelope envelope,
  VerifierLimits limits,
) {
  try {
    verifyBuffer(
      envelope.document,
      identifier: controlIdentifier,
      layouts: controlLayouts,
      root: _documentTable,
      maxDepth: limits.maxDepth,
      maxVisits: limits.maxVisits,
    );
  } on PluxException catch (e) {
    throw _invalid(e.message);
  }
  final doc = fbs.ControlDocument(envelope.document);
  _agree(doc, envelope);
  final all = doc.killSwitches ?? const <fbs.KillSwitch>[];
  if (all.length > PluxLimit.controlKillSwitches.max) {
    throw _invalid('${all.length} kill switches');
  }
  final flags = doc.flags ?? const <fbs.FlagValue>[];
  final experiments = doc.experiments ?? const <fbs.ExperimentAllocation>[];
  if (flags.length > PluxLimit.flagsPerApp.max) {
    throw _invalid('${flags.length} flags');
  }
  if (experiments.length > PluxLimit.experimentsActivePerApp.max) {
    throw _invalid('${experiments.length} experiments');
  }
  final mandatory = doc.mandatoryUpdate;
  final rollback = doc.forcedRollback;
  return ControlSnapshot(
    version: doc.version,
    expiresAt: envelope.expires,
    switches: [
      for (final s in all)
        if (_switch(s) case final ControlSwitch known) known,
    ],
    mandatory: mandatory == null
        ? null
        : MandatoryUpdate(
            releaseId: mandatory.releaseId ?? '',
            releaseSequence: mandatory.releaseSequence,
            messageKey: mandatory.messageKey ?? '',
          ),
    rollback: rollback == null
        ? null
        : ForcedRollback(
            releaseId: rollback.releaseId ?? '',
            releaseSequence: rollback.releaseSequence,
          ),
    flags: [
      for (final f in flags)
        if (readFlag(f) case final ControlFlag known) known,
    ],
    experiments: [
      for (final e in experiments)
        if (readExperiment(e) case final ControlExperiment known) known,
    ],
    holdout: readHoldout(doc.holdout),
  );
}

/// Checks that the document repeats what the signed header says.
void _agree(fbs.ControlDocument doc, ControlEnvelope e) {
  final seconds = e.expires.millisecondsSinceEpoch ~/ 1000;
  if (doc.version != e.version ||
      doc.appId != e.app ||
      doc.environment != e.environment ||
      doc.channel != e.channel ||
      doc.issuedAt != e.issuedAt.millisecondsSinceEpoch ~/ 1000 ||
      doc.expiresAt != seconds) {
    throw _invalid('it disagrees with the signed header');
  }
}

/// The switch [s] as a value, or null for a scope this runtime does not
/// know (it comes from a newer server) or one that names no target. A rule
/// that is absent or that this runtime cannot read leaves the switch
/// applying to every device: a kill switch fails safe (REL-060).
ControlSwitch? _switch(fbs.KillSwitch s) {
  final fbs.SwitchScope scope;
  try {
    scope = s.scope;
  } on StateError {
    return null;
  }
  final target = s.target;
  final kind = switch (scope) {
    fbs.SwitchScope.Plugin => SwitchKind.plugin,
    fbs.SwitchScope.Page => SwitchKind.page,
    fbs.SwitchScope.ActionGraph => SwitchKind.actionGraph,
    fbs.SwitchScope.DataSource => SwitchKind.dataSource,
    fbs.SwitchScope.$Function => SwitchKind.function,
    fbs.SwitchScope.Unspecified => null,
  };
  final table = s.rule;
  final rule = table == null ? null : readRule(table);
  return kind == null || target == null
      ? null
      : ControlSwitch(kind, target, rule: rule);
}
