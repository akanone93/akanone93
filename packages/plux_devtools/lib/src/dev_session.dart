// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Pairs the host's own debug build with a dev session (DEV-040): the build
/// gets the Dev app's capabilities (hot reload, commands, events) without
/// being the Dev app. Everything sits behind `kPluxDevtools`; in any other
/// build the controller is inert (`DEV-050`).
library;

import 'package:flutter/foundation.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// Where a [PluxDevSession] stands.
@immutable
sealed class PluxDevSessionState {
  const PluxDevSessionState();
}

/// Not paired.
final class PluxDevIdle extends PluxDevSessionState {
  /// Creates the state.
  const PluxDevIdle();
}

/// The pairing is under way.
final class PluxDevPairingNow extends PluxDevSessionState {
  /// Creates the state.
  const PluxDevPairingNow();
}

/// Paired with the session [sessionId] as the device [deviceId].
final class PluxDevPaired extends PluxDevSessionState {
  /// Creates the state.
  const PluxDevPaired({required this.sessionId, required this.deviceId});

  /// The session this build joined.
  final String sessionId;

  /// This build's identifier inside the session.
  final String deviceId;
}

/// The pairing failed, or the session ended with an error.
final class PluxDevError extends PluxDevSessionState {
  /// Creates the state.
  const PluxDevError(this.code);

  /// A stable code: `invalid_link`, `unavailable`, `pairing_failed`, or a
  /// `PLX-` error code.
  final String code;
}

/// What a pairer returns: the joined session and how to leave it.
typedef PluxDevJoined = ({
  String sessionId,
  String deviceId,
  Future<void> Function() close,
});

/// Pairs with the session of a link or a code and returns the joined
/// session. The default is `Plux.pairDevSession`; tests inject their own.
typedef PluxDevPairer = Future<PluxDevJoined> Function(
  String linkOrCode, {
  required List<PluxPublicKey> devRootKeys,
  required Map<String, DevCommandHandler> extensions,
});

/// Carries the events the device tools raise (an inspected node, a
/// screenshot) to the channel of the pairing in place, and drops them when
/// there is none. Hand it to `devExtensions` as the sink.
final class PluxDevEvents implements DevEventSink {
  DevEventSink? _target;

  @override
  void emit(Map<String, Object?> payload, {String? traceId}) =>
      _target?.emit(payload, traceId: traceId);
}

PluxDevPairer _runtimePairer(PluxDevEvents events) =>
    (
      String linkOrCode, {
      required List<PluxPublicKey> devRootKeys,
      required Map<String, DevCommandHandler> extensions,
    }) async {
      final pairing = await Plux.pairDevSession(
        linkOrCode,
        devRootKeys: devRootKeys,
        extensions: extensions,
      );
      events._target = pairing.channel;
      return (
        sessionId: pairing.sessionId,
        deviceId: pairing.deviceId,
        close: () async {
          events._target = null;
          await pairing.close();
        },
      );
    };

/// The controller of this build's pairing with a dev session.
///
/// ```dart
/// final session = PluxDevSession(devRootKeys: devKeys);
/// await session.pair(scannedOrPastedLink); // or the typed code
/// ```
final class PluxDevSession {
  /// Creates a controller.
  ///
  /// [devRootKeys] are the development roots the host pinned — never the
  /// production ones; dev bundles are verified against them alone.
  /// [extensions] carry out commands other packages own, by the name of the
  /// command's oneof field. [pairer] and [isAvailable] replace the runtime and
  /// `kPluxDevtools`; tests use them.
  PluxDevSession({
    required this.devRootKeys,
    this.extensions = const {},
    @visibleForTesting this._pairer,
    this.isAvailable = kPluxDevtools,
  });

  /// The pinned development roots.
  final List<PluxPublicKey> devRootKeys;

  /// Handlers of further commands, by oneof field name; the host may fill
  /// it before [pair].
  final Map<String, DevCommandHandler> extensions;

  final PluxDevPairer? _pairer;

  /// The sink for the events of the device tools; it reaches the channel
  /// of the pairing in place.
  final PluxDevEvents events = PluxDevEvents();
  final ValueNotifier<PluxDevSessionState> _state = ValueNotifier(
    const PluxDevIdle(),
  );
  PluxDevJoined? _paired;

  /// Whether this build can pair at all: `kPluxDevtools`.
  final bool isAvailable;

  /// The state, for the overlay and the host.
  ValueListenable<PluxDevSessionState> get state => _state;

  /// Pairs with the session of [linkOrCode]: the `plux-dev://pair?...` link
  /// that was scanned or pasted, or the short code `plux dev` prints beside
  /// it (`XXXX-XXXX`), which pairs against the server the runtime talks to.
  /// A pairing in progress or in place is ignored. A failure becomes a
  /// [PluxDevError] state; this never throws.
  Future<void> pair(String linkOrCode) async {
    if (!isAvailable) return;
    if (_state.value is PluxDevPairingNow || _state.value is PluxDevPaired) {
      return;
    }
    try {
      if (DevPairingCode.tryParse(linkOrCode) == null) {
        DevPairingLink.parse(linkOrCode);
      }
    } on FormatException {
      _state.value = const PluxDevError('invalid_link');
      return;
    }
    _state.value = const PluxDevPairingNow();
    try {
      final paired = await (_pairer ?? _runtimePairer(events))(
        linkOrCode,
        devRootKeys: devRootKeys,
        extensions: extensions,
      );
      _paired = paired;
      _state.value = PluxDevPaired(
        sessionId: paired.sessionId,
        deviceId: paired.deviceId,
      );
    } on FormatException {
      _state.value = const PluxDevError('invalid_link');
    } on PluxException catch (e) {
      _state.value = PluxDevError(e.code.id);
    } on StateError {
      _state.value = const PluxDevError('unavailable');
    } on Object {
      _state.value = const PluxDevError('pairing_failed');
    }
  }

  /// Ends the session: closes the channel and discards the overlay, so the
  /// pages render the active release again.
  Future<void> end() async {
    final paired = _paired;
    _paired = null;
    try {
      await paired?.close();
    } finally {
      if (isAvailable) _state.value = const PluxDevIdle();
    }
  }

  /// Ends the session and releases the controller.
  Future<void> dispose() async {
    await end();
    _state.dispose();
  }
}
