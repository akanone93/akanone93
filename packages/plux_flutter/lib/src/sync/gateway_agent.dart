// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The device's requests to the Plux gateway (SEC-026, SEC-030, SEC-032,
/// ADR-0056), made on the sync isolate, which holds the device's token and
/// DPoP key (L-6): the gateway call itself, the exchange of the host's user
/// token, and the challenge of a transaction.
///
/// Every call carries the device's access token and a DPoP proof. The user
/// token the exchange returns is scoped to one upstream, kept in memory
/// only until shortly before it expires, and never logged or written to
/// disk; a different host token replaces it at once.
///
/// The agent also holds the device's `live` configuration values (DAT-094),
/// in memory only: every call names their version, a newer epoch on a
/// response refreshes them in the background (DAT-095), and a call the
/// server refuses because a `strict` value changed (DAT-096) is repeated
/// once with the same request, idempotency key included, after a refresh.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/gateway.dart';
import 'package:plux_flutter/src/data/live_config.dart';
import 'package:plux_flutter/src/data/transport.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/sca.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/device_auth.dart';
import 'package:plux_flutter/src/sync/live_config.dart';

/// Sends the gateway calls of one device.
///
/// Failures are [DataFailure]s, which cross the isolate boundary as they
/// are; they carry registered codes and never the server's text or a
/// request's content (SCH-012).
final class GatewayAgent {
  /// Creates the agent over the device's [auth] and the [api] client.
  GatewayAgent({
    required this.api,
    required this.auth,
    DateTime Function()? clock,
  }) : _clock = clock ?? DateTime.now {
    live = LiveConfig(_fetchLive);
  }

  /// The API client.
  final PluxApiClient api;

  /// The device's standing with the server.
  final DeviceAuth auth;

  /// The `live` configuration values, in memory only (DAT-094).
  late final LiveConfig live;

  final DateTime Function() _clock;
  final Map<String, _Exchanged> _users = {};

  /// Sends [r] through the gateway and completes with the upstream's
  /// status and decoded JSON body.
  ///
  /// A token the server no longer accepts is replaced once and the call
  /// repeated: the server refuses a stale token before it looks at a
  /// transaction's challenge, so the challenge is not spent. So is a call
  /// that names a `live` configuration version a `strict` value has
  /// outlived: the values are refreshed and the call repeated once, as it
  /// was (DAT-096); if the repeat is refused too, the failure is reported.
  Future<DataResponse> call(GatewayRequest r) => _call(r).timeout(
    r.timeout,
    onTimeout: () => throw DataFailure(
      ActionErrorKind.timeout,
      PluxErrorCode.dataRequestTimeout,
      'the call of ${r.upstream} took longer than '
      '${r.timeout.inMilliseconds} ms (data.requestTimeout)',
    ),
  );

  /// Authenticates [r] for the gateway's HTTP route (SEC-030): a token and
  /// a proof of this one request, and the exchanged user token. The caller
  /// opens the connection. A token the server no longer accepts shows when
  /// the connection is refused with a 401, and the next route is made with
  /// a replaced token.
  Future<GatewayRoute> route(GatewayRouteRequest r) async {
    final token = await _token();
    final user = r.userToken;
    final userToken = user == null
        ? null
        : await _userToken(token, r.upstream, user);
    final c = r.confirmation;
    final (:url, :headers) = await api.gatewayRoute(
      token: token,
      upstream: r.upstream,
      method: r.method,
      pathAndQuery: r.path,
      operation: r.operation,
      plugin: r.plugin,
      userToken: userToken,
      challengeId: c?.challengeId ?? '',
      signature: c?.signature,
    );
    return GatewayRoute(url, headers);
  }

  Future<DataResponse> _call(GatewayRequest r) async {
    var reauthenticated = false;
    var refreshed = false;
    while (true) {
      final token = await _token();
      final user = r.userToken;
      final userToken = user == null
          ? null
          : await _userToken(token, r.upstream, user);
      final GatewayReply reply;
      try {
        reply = await api.gatewayCall(
          token: token,
          upstream: r.upstream,
          operation: r.operation,
          method: r.method,
          path: r.path,
          headers: r.headers,
          body: r.body,
          plugin: r.plugin,
          userToken: userToken,
          challengeId: r.confirmation?.challengeId ?? '',
          signature: r.confirmation?.signature,
          configVersion: live.version,
          onHeaders: live.noteHeaders,
        );
      } on ApiError catch (e) {
        if (!reauthenticated &&
            e.plxCode == null &&
            e.code == 'unauthenticated') {
          reauthenticated = true;
          auth.forgetToken();
          continue;
        }
        if (!refreshed && ConfigCode.stale.matches(e.plxCode)) {
          refreshed = true;
          await live.refresh(afresh: true);
          continue;
        }
        throw _refused(e, 'the call of ${r.upstream}');
      }
      return _decode(r, reply);
    }
  }

  /// Fetches the `live` values for [LiveConfig]; a token the server no
  /// longer accepts is replaced once. A failure to reach the server is the
  /// code of unavailable configuration (DAT-097).
  Future<LiveConfigSnapshot> _fetchLive() async {
    for (var attempt = 0; ; attempt++) {
      final token = await _token();
      try {
        return await api.getLiveConfig(
          token,
          appId: auth.appId,
          environment: auth.environment,
        );
      } on ApiError catch (e) {
        if (attempt == 0 && e.plxCode == null && e.code == 'unauthenticated') {
          auth.forgetToken();
          continue;
        }
        final f = _refused(e, 'the live configuration');
        if (f.kind != ActionErrorKind.network) throw f;
        throw DataFailure(
          ActionErrorKind.network,
          ConfigCode.unavailable.raised,
          'the live configuration could not be fetched: the Plux server did '
          'not answer',
        );
      }
    }
  }

  DataResponse _decode(GatewayRequest r, GatewayReply reply) {
    final bytes = reply.body.length;
    if (bytes > r.maxResponseBytes) {
      throw DataFailure(
        ActionErrorKind.validation,
        PluxErrorCode.dataSizeExceeded,
        'the response of ${r.upstream} exceeds data.responseSize = '
        '${r.maxResponseBytes} bytes',
      );
    }
    final ok = reply.status >= 200 && reply.status < 300;
    if (!ok || bytes == 0) return DataResponse(reply.status, null, bytes);
    try {
      return DataResponse(
        reply.status,
        jsonDecode(utf8.decode(reply.body)),
        bytes,
      );
    } on FormatException {
      throw DataFailure.mapping('the response of ${r.upstream} is not JSON');
    }
  }

  /// The Plux user token for [upstream], exchanged from the host's
  /// [subject] token on first use and again when it is near its expiry or
  /// the host's token changed (SEC-026).
  Future<String> _userToken(
    DeviceToken token,
    String upstream,
    String subject,
  ) async {
    final digest = sha256.convert(utf8.encode(subject)).toString();
    final held = _users[upstream];
    if (held != null &&
        held.subject == digest &&
        held.token.expiresAt
            .subtract(DeviceAuth.refreshMargin)
            .isAfter(_clock())) {
      return held.token.value;
    }
    _users.remove(upstream);
    try {
      final t = await api.exchangeUserToken(
        token: token,
        subjectToken: subject,
        audience: upstream,
        now: _clock(),
      );
      _users[upstream] = _Exchanged(digest, t);
      return t.value;
    } on ApiError catch (e) {
      throw _refused(e, 'the exchange of the user token');
    }
  }

  /// A challenge for the transaction [summaryJcs] of [dataSource] and
  /// [operation] (SEC-027).
  Future<TransactionChallenge> challenge({
    required String dataSource,
    required String operation,
    required Uint8List summaryJcs,
  }) async {
    final token = await _token();
    try {
      return await api.transactionChallenge(
        token: token,
        deviceId: await auth.deviceId(),
        dataSource: dataSource,
        operation: operation,
        summaryJcs: summaryJcs,
      );
    } on ApiError catch (e) {
      throw _refused(e, 'the transaction challenge of $dataSource');
    }
  }

  /// Whether the device has an SCA key registered with the server,
  /// registering it again with one when it has none.
  Future<bool> ensureEnrolled() async {
    try {
      return await auth.ensureSca();
    } on ApiError catch (e) {
      throw _refused(e, 'the registration with a transaction key');
    } on PluxException catch (e) {
      throw DataFailure(ActionErrorKind.custom, e.code, e.message);
    }
  }

  Future<DeviceToken> _token() async {
    try {
      return await auth.token();
    } on ApiError catch (e) {
      throw _refused(e, 'the device token');
    } on PluxException catch (e) {
      throw DataFailure(ActionErrorKind.custom, e.code, e.message);
    }
  }

  /// The data failure of the server's refusal [e]; a refusal for the
  /// assurance level or a revoked device also lowers the level the device
  /// shows at once, as a sync does (SEC-007).
  DataFailure _refused(ApiError e, String what) {
    final f = gatewayFailure(e, what);
    switch (f.code) {
      case PluxErrorCode.assuranceInsufficient || PluxErrorCode.deviceRevoked:
        auth.lowerAssurance();
      case PluxErrorCode.userTokenInvalid:
        _users.clear();
      default:
        break;
    }
    return f;
  }
}

final class _Exchanged {
  const _Exchanged(this.subject, this.token);

  /// SHA-256 of the host's token the exchange was made for, so a token of
  /// another user never reuses it.
  final String subject;

  final UserToken token;
}
