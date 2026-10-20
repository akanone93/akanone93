// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The control document as the UI isolate applies it (SYN-060, REL-060,
/// REL-070): the kill switches in force and the mandatory update, if one is
/// pending. The runtime replaces it as documents arrive; pages and runs
/// ask it at the moment they decide, so a change takes effect without a
/// restart.
library;

import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';

/// The control state in force.
final class LiveControl {
  /// Creates the state.
  const LiveControl({this.snapshot, this.pendingUpdate});

  /// No document yet: nothing is switched off, no update is pending.
  static const none = LiveControl();

  /// The newest verified document, or null. Its switches stay in force
  /// whether or not it has expired.
  final ControlSnapshot? snapshot;

  /// The mandatory update that must be active before a Plux page shows, or
  /// null (REL-070).
  final MandatoryUpdate? pendingUpdate;

  /// Whether a kill switch turns off the [kind] named [target].
  bool isOff(SwitchKind kind, String target) =>
      snapshot?.isOff(kind, target) ?? false;

  /// The error a run, load or call of a switched-off [kind] fails with:
  /// `PLX-4020` for a plugin or page (their fallback shows, RT-022) and the
  /// scope's own code for an action graph, data source or function.
  static PluxException disabled(SwitchKind kind, String target) =>
      PluxException(
        switch (kind) {
          SwitchKind.plugin || SwitchKind.page => PluxErrorCode.pluginDisabled,
          SwitchKind.actionGraph => PluxErrorCode.actionGraphSwitchedOff,
          SwitchKind.dataSource => PluxErrorCode.dataSourceSwitchedOff,
          SwitchKind.function => PluxErrorCode.functionSwitchedOff,
        },
        'the ${kind.name} $target is switched off',
        details: {'scope': kind.name, 'target': target, 'reason': 'killSwitch'},
      );
}
