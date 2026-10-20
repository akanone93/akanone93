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

  /// The error a run, load or call of a switched-off [kind] fails with.
  /// The registry has `PLX-4020` for a plugin turned off by a kill switch
  /// and no code of its own for the other scopes, so it stands for all.
  static PluxException disabled(SwitchKind kind, String target) =>
      PluxException(
        PluxErrorCode.pluginDisabled,
        'the ${kind.name} $target is switched off',
        details: {'scope': kind.name, 'target': target, 'reason': 'killSwitch'},
      );
}
