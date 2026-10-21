// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Reads the FlatBuffers document of a verified control envelope
/// (`schema/control.fbs`, ADR-0077). The buffer goes through the
/// FlatBuffers verifier first (SEC-052, ADR-0029) against the layout tables
/// of `control_layout.g.dart`, which `make gen` writes from the schema
/// (SYN-060), and only a buffer that passes is read with the generated
/// reader.
/// Unknown kill switch scopes and unknown fields come from a newer server
/// and are skipped (additive evolution).
library;

import 'package:plux_flutter/src/control/control_experiments_reader.dart';
import 'package:plux_flutter/src/control/control_layout.g.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/schema/limits.g.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/flatbuffers_verifier.dart';
import 'package:plux_flutter/src/verify/update_metadata.dart';

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
      root: controlRootLayout,
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
