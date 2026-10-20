// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Reads the FlatBuffers document of a verified control envelope
/// (`schema/control.fbs`, ADR-0077). The buffer goes through the
/// FlatBuffers verifier first (SEC-052, ADR-0029) against the layout tables
/// below, and only a buffer that passes is read with the generated reader.
/// Unknown kill switch scopes and unknown fields come from a newer server
/// and are skipped (additive evolution).
library;

import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/errors/plux_exception.dart';
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
  TableLayout('TargetingRule', []),
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
  TableLayout('FlagValue', []),
  // 5
  TableLayout('ExperimentAllocation', []),
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
  ]),
];

const int _documentTable = 6;

PluxException _invalid(String message) => PluxException(
  PluxErrorCode.controlSignatureInvalid,
  'control document: $message',
  details: {'fault': MetadataFault.format},
);

/// Verifies the FlatBuffers [bytes] of [envelope] and reads them: the kill
/// switches, the mandatory update and the forced rollback. The document
/// must say what the signed header says (version, channel, times), and
/// carry at most `control.killSwitches` switches. Throws `PLX-3100` for any
/// buffer that is not a well-formed, agreeing document.
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
/// know (it comes from a newer server) or one that names no target.
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
  return kind == null || target == null ? null : ControlSwitch(kind, target);
}
