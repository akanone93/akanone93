// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// A device's pairing with a dev session, as `plux_devtools` holds it
/// (`DEV-040`).
library;

import 'package:plux_flutter/src/devtools_api/dev_channel.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';

/// The channel of a paired device and the overlay applier behind it.
final class PluxDevPairing {
  /// Creates the pairing.
  const PluxDevPairing({
    required this.channel,
    required this.applier,
    this.onClose,
  });

  /// The channel to the session.
  final DevChannel channel;

  /// The applier of the session's dev bundles.
  final DevBundleApplier applier;

  /// Runs when the pairing ends, before the channel closes: the runtime
  /// stops its event sources here (DEV-020).
  final void Function()? onClose;

  /// The session this device joined.
  String get sessionId => channel.sessionId;

  /// The device's identifier inside the session.
  String get deviceId => channel.deviceId;

  /// Ends the pairing: closes the channel and discards the overlay, so the
  /// pages render the active release again.
  Future<void> close() async {
    onClose?.call();
    await channel.close();
    await applier.discard();
  }
}
