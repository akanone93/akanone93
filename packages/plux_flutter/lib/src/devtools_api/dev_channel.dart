// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The device side of the dev-session channel (DEV-002, DEV-010, DEV-021,
/// ADR-0087): pairs with a session, opens its WebSocket, applies the dev
/// bundles and carries out the commands it sends, and streams the device's
/// events back, resuming after a reconnect.
///
/// The socket, the JSON and the event buffer live on a background isolate
/// (L-6); this library, on the UI isolate, builds the DPoP proofs, calls the
/// [DevBundleApplier] and the [DevCommandHandler], and posts events.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:flutter/foundation.dart';
import 'package:plux_flutter/src/devtools_api/dev_engine.dart';
import 'package:plux_flutter/src/devtools_api/dev_engine_isolate.dart';
import 'package:plux_flutter/src/devtools_api/dev_pairing_link.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/runtime_info.dart';
import 'package:plux_flutter/src/schema/limits.g.dart';
import 'package:plux_flutter/src/sync/api_client.dart';

export 'package:plux_flutter/src/devtools_api/dev_engine.dart'
    show DevEngineConfig, DevEngineHandle, DevEngineHost, DevSocketAuth;
export 'package:plux_flutter/src/devtools_api/dev_engine_isolate.dart'
    show DevEngineStarter;
export 'package:plux_flutter/src/devtools_api/dev_pairing_link.dart'
    show DevPairingCode, DevPairingCredential, DevPairingLink;

/// A paired device's channel to its dev session.
final class DevChannel implements DevEventSink {
  DevChannel._(this.sessionId, this.deviceId, this._engine);

  /// The session this device joined.
  final String sessionId;

  /// The device's identifier inside the session.
  final String deviceId;

  final DevEngineHandle _engine;

  /// Pairs the device with the session of [pairing], a scanned link or a
  /// typed code, and starts the channel.
  ///
  /// Calls `DevSessionService.Pair` with [token] (DPoP-bound), then opens
  /// `wss://<server>/dev/socket` with the returned ticket and a proof of the
  /// device key, and sends the hello first. Applies bundles through
  /// [applier] and carries out commands through [commands]. The limits are
  /// the registry's `devtools.eventBytes`, `devtools.streamBufferEvents` and
  /// `devtools.streamBufferBytes`. [engine] replaces the background
  /// isolate; tests use it. Throws a [StateError] in a build without
  /// devtools (`DEV-050`) and a [FormatException] when the link's server is
  /// not the one [api] talks to; a code pairs against [api]'s server.
  static Future<DevChannel> pair(
    DevPairingCredential pairing, {
    required PluxApiClient api,
    required DeviceToken token,
    required DevBundleApplier applier,
    required DevCommandHandler commands,
    String deviceName = '',
    String platform = '',
    String runtimeVersion = PluxRuntimeInfo.version,
    Uint8List? attestation,
    int eventBytes = 65536,
    int streamBufferEvents = 5000,
    int streamBufferBytes = 8388608,
    DevEngineStarter? engine,
  }) async {
    if (!kPluxDevtools) {
      throw StateError('the dev channel is not part of this build');
    }
    final (server, pairingToken, pairingCode) = switch (pairing) {
      DevPairingLink(:final server, token: final t) => (server, t, ''),
      DevPairingCode(:final value) => (api.endpoint, '', value),
    };
    if (!_sameOrigin(server, api.endpoint)) {
      throw const FormatException(
        'the pairing server is not the server this app talks to',
      );
    }
    final request = _PairRequest(
      pairingToken,
      pairingCode,
      deviceName,
      platform,
      runtimeVersion,
      attestation,
    );
    final paired = await api.devPair(
      token,
      pairingToken: request.token,
      pairingCode: request.code,
      name: request.name,
      platform: request.platform,
      runtimeVersion: request.runtimeVersion,
      attestation: request.attestation,
    );
    final host = _UiHost(
      socketUrl: DevPairingLink.socketUrlOf(server),
      api: api,
      token: token,
      applier: applier,
      commands: commands,
      sessionId: paired.sessionId,
      deviceId: paired.deviceId,
      firstTicket: paired.socketTicket,
    );
    final config = DevEngineConfig(
      eventBytes: eventBytes,
      bufferEvents: streamBufferEvents,
      bufferBytes: streamBufferBytes,
      maxFrameBytes: PluxLimit.bundlePluginSize.defaultValue * 4 ~/ 3 + 65536,
      connectTimeoutMs: PluxLimit.dataRequestTimeout.defaultValue,
      backoffMinMs: PluxLimit.dataStreamBackoffMin.defaultValue,
      backoffMaxMs: PluxLimit.dataStreamBackoffMax.defaultValue,
    );
    final handle = await (engine ?? startDevEngineIsolate)(config, host);
    return DevChannel._(paired.sessionId, paired.deviceId, handle);
  }

  @override
  void emit(Map<String, Object?> payload, {String? traceId}) {
    if (!kPluxDevtools) return;
    _engine.emit(payload, traceId: traceId);
  }

  @override
  String toString() => kPluxDevtools ? 'DevChannel($kDevCanary)' : '';

  /// Closes the socket and stops reconnecting.
  Future<void> close() => _engine.close();
}

bool _sameOrigin(Uri a, Uri b) =>
    a.scheme == b.scheme &&
    a.host.toLowerCase() == b.host.toLowerCase() &&
    a.port == b.port;

final class _PairRequest {
  const _PairRequest(
    this.token,
    this.code,
    this.name,
    this.platform,
    this.runtimeVersion,
    this.attestation,
  );
  final String token;
  final String code;
  final String name;
  final String platform;
  final String runtimeVersion;
  final Uint8List? attestation;
}

/// The UI isolate's side of the channel.
final class _UiHost implements DevEngineHost {
  _UiHost({
    required this.socketUrl,
    required this.api,
    required this.token,
    required this.applier,
    required this.commands,
    required this.sessionId,
    required this.deviceId,
    required this.firstTicket,
  });

  /// The URL of the server's dev socket.
  final Uri socketUrl;
  final PluxApiClient api;
  final DeviceToken token;
  final DevBundleApplier applier;
  final DevCommandHandler commands;
  final String sessionId;
  final String deviceId;

  /// The ticket Pair returned, used by the first connection only.
  String? firstTicket;

  @override
  Future<DevSocketAuth?> auth({required bool first}) async {
    var ticket = firstTicket;
    firstTicket = null;
    if (ticket == null) {
      // A ticket is presented once and the pairing token was consumed, so
      // a reconnect asks for a new ticket (DEV-021).
      try {
        ticket = await api.devRenewSocketTicket(
          token,
          sessionId: sessionId,
          deviceId: deviceId,
        );
      } on ApiError catch (e) {
        final permanent =
            e.status >= 400 &&
            e.status < 500 &&
            e.status != 408 &&
            e.status != 429;
        if (permanent) return null;
        rethrow;
      }
    }
    final url = socketUrl;
    final proof = await token.proofs.proof(method: 'GET', uri: url);
    return DevSocketAuth(url: url, ticket: ticket, proof: proof);
  }

  @override
  Future<List<Map<String, Object?>>> frame(Map<String, Object?> frame) async {
    if (frame['bundle'] case final Map<String, Object?> bundle) {
      return _bundle(bundle);
    }
    if (frame['command'] case final Map<String, Object?> command) {
      return [await _command(command)];
    }
    return const [];
  }

  Future<List<Map<String, Object?>>> _bundle(Map<String, Object?> frame) async {
    final watch = Stopwatch()..start();
    final manifest = _bytes(frame['manifest']);
    final sections = <String, Uint8List>{};
    if (frame['sections'] case final Map<Object?, Object?> raw) {
      for (final e in raw.entries) {
        sections['${e.key}'] = _bytes(e.value);
      }
    }
    final bundle = DevBundle(
      sequence: _int(frame['bundleSequence']),
      manifest: manifest,
      sections: sections,
    );
    final DevApplyResult result;
    try {
      result = await applier.apply(bundle);
    } on PluxException catch (e) {
      return _applied(manifest, watch, DevApplyResult(error: e));
    }
    return _applied(manifest, watch, result);
  }

  List<Map<String, Object?>> _applied(
    Uint8List manifest,
    Stopwatch watch,
    DevApplyResult result,
  ) {
    final error = result.error;
    return [
      {
        'hotApply': {
          'bundleSha256': sha256.convert(manifest).toString(),
          'pagesApplied': result.pagesApplied,
          'scopesReset': result.scopesReset,
          'applyMicros': '${watch.elapsedMicroseconds}',
        },
      },
      if (error != null)
        {
          'error': {
            'error': {
              'code': error.code.id,
              'reason': error.code.reason,
              'message': error.message,
            },
          },
        },
    ];
  }

  Future<Map<String, Object?>> _command(Map<String, Object?> frame) async {
    final id = frame['commandId'] as String? ?? '';
    final command = frame['command'];
    Map<String, Object?> result;
    try {
      result = await commands.handle(
        command is Map<String, Object?> ? command : const {},
      );
    } on PluxException catch (e) {
      result = {
        'status': 'DEV_COMMAND_STATUS_FAILED',
        'error': {
          'code': e.code.id,
          'reason': e.code.reason,
          'message': e.message,
        },
      };
    }
    return {
      'commandResult': {...result, 'commandId': id},
    };
  }

  @override
  Future<void> closed({String? endedCode}) async {
    if (endedCode != null) await applier.discard();
  }
}

Uint8List _bytes(Object? v) => v is String ? base64.decode(v) : Uint8List(0);

int _int(Object? v) => switch (v) {
  int() => v,
  String() => int.tryParse(v) ?? 0,
  _ => 0,
};
