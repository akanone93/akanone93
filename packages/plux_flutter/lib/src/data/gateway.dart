// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a data request through the Plux gateway is (SEC-030, SEC-031,
/// ADR-0056): the request the data client builds for a source whose route is
/// `plux`, the transport that carries it (the sync isolate, which holds the
/// device's token and DPoP key), and the mapping of the server's refusals to
/// typed data failures.
library;

import 'dart:typed_data';

import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/transport.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/sca.dart';
import 'package:plux_flutter/src/sync/api_client.dart' show ApiError;

/// One request through `GatewayService.Call`.
final class GatewayRequest {
  /// Creates a request.
  const GatewayRequest({
    required this.upstream,
    required this.operation,
    required this.method,
    required this.path,
    required this.headers,
    required this.body,
    required this.maxRequestBytes,
    required this.maxResponseBytes,
    required this.timeout,
    this.plugin = '',
    this.userToken,
    this.confirmation,
  });

  /// The upstream: the data source's name, which the server maps to the
  /// upstream registered for the app and environment.
  final String upstream;

  /// The key of the plugin that makes the call, for the operations the
  /// upstream limits to plugins (SEC-080); empty when there is none.
  final String plugin;

  /// The operation's name; empty for a load of the source itself.
  final String operation;

  /// The HTTP method.
  final String method;

  /// The path and query, relative to the upstream's base URL.
  final String path;

  /// The headers the upstream may receive; the server drops the others.
  final Map<String, String> headers;

  /// The JSON body, encoded, or null.
  final Uint8List? body;

  /// The largest request accepted (`data.requestSize`).
  final int maxRequestBytes;

  /// The largest response accepted (`data.responseSize`).
  final int maxResponseBytes;

  /// How long the exchange may take (`data.requestTimeout`).
  final Duration timeout;

  /// The host's access token to exchange for a Plux user token, or null for
  /// a source that is not `auth` (SEC-026). Never logged.
  final String? userToken;

  /// The confirmation of a transaction operation (SEC-032), or null.
  final Confirmation? confirmation;

  @override
  String toString() => 'GatewayRequest($method $upstream)';
}

/// One request to the gateway's HTTP route, `/gateway/v1/{upstream}/{path}`
/// (SEC-030, ADR-0056): the way a stream, a server-sent-events response or a
/// file transfer reaches an upstream, because its body is not one message.
final class GatewayRouteRequest {
  /// Creates a request.
  const GatewayRouteRequest({
    required this.upstream,
    required this.method,
    required this.path,
    this.operation = '',
    this.plugin = '',
    this.userToken,
    this.confirmation,
  });

  /// The upstream: the data source's name.
  final String upstream;

  /// The key of the plugin that makes the call, for the operations the
  /// upstream limits to plugins (SEC-080); empty when there is none.
  final String plugin;

  /// The HTTP method of the request the route will carry (`GET` for the
  /// handshake of a WebSocket or an event stream); the DPoP proof binds it.
  final String method;

  /// The path and query, relative to the upstream's base URL, beginning
  /// with a slash.
  final String path;

  /// The operation's name; empty for a source's own stream.
  final String operation;

  /// The host's access token to exchange for a Plux user token, or null for
  /// a source that is not `auth` (SEC-026). Never logged.
  final String? userToken;

  /// The confirmation of a transaction (SEC-032), or null.
  final Confirmation? confirmation;

  @override
  String toString() => 'GatewayRouteRequest($method ${Uri.parse(path).path})';
}

/// What a connection to the gateway's HTTP route needs: the URL, with the
/// scheme of the server's endpoint, and the headers that authenticate that
/// one request (`Authorization: DPoP`, the DPoP proof of its method and URL,
/// the user token and the typed fields as `Plux-` headers). A proof is good
/// for one request, so each connection, each reconnection included, asks
/// for its own.
final class GatewayRoute {
  /// Creates the route.
  const GatewayRoute(this.url, this.headers);

  /// The URL of the route for the request.
  final Uri url;

  /// The headers to send with it. They hold credentials: never logged.
  final Map<String, String> headers;

  @override
  String toString() => 'GatewayRoute([redacted])';
}

/// Sends requests through the Plux gateway.
abstract interface class GatewayTransport {
  /// Sends [request] with the device's token and a DPoP proof. Completes
  /// with the upstream's status and decoded JSON body, bounded by the
  /// request's limits; fails with a [DataFailure] when the server refuses
  /// the request, the network fails or the time or size limit is passed.
  Future<DataResponse> call(GatewayRequest request);

  /// Authenticates [request] for the gateway's HTTP route. The caller makes
  /// the connection itself, on the data isolate, so the stream or the file
  /// never passes through the isolate that holds the device's key; only the
  /// proof does. Fails with a [DataFailure] when the device cannot get a
  /// token or the exchange of the user token is refused.
  Future<GatewayRoute> route(GatewayRouteRequest request);
}

/// The data failure for the server's refusal [e] of a gateway call or of the
/// calls around it (the token exchange, a challenge). A refusal carries a
/// registered code in its message; one without is a network failure when no
/// answer arrived and an HTTP failure otherwise. Messages never carry the
/// server's text (it may echo the request, SCH-012).
DataFailure gatewayFailure(ApiError e, String what) {
  final code = _code(e.plxCode);
  if (code != null) {
    return DataFailure(_kind(code), code, '$what was refused: ${code.id}');
  }
  if (e.status == 0 || e.retryable && e.status != 429) {
    return DataFailure(
      ActionErrorKind.network,
      PluxErrorCode.dataNetworkFailed,
      '$what failed: the Plux server did not answer',
    );
  }
  if (e.code == 'unauthenticated') {
    return DataFailure(
      ActionErrorKind.http,
      PluxErrorCode.dataUnauthorised,
      '$what was refused: the device is not authenticated',
      status: 401,
    );
  }
  return DataFailure(
    ActionErrorKind.http,
    PluxErrorCode.dataHttpError,
    '$what failed with ${e.status}',
    status: e.status,
  );
}

PluxErrorCode? _code(int? number) {
  if (number == null) return null;
  for (final c in PluxErrorCode.values) {
    if (c.code == number) return _known.contains(c) || _isConfig(c) ? c : null;
  }
  return null;
}

/// The codes the server answers a gateway, token-exchange or challenge call
/// with that the data layer reports as they are.
const Set<PluxErrorCode> _known = {
  PluxErrorCode.assuranceInsufficient,
  PluxErrorCode.deviceRevoked,
  PluxErrorCode.userTokenInvalid,
  PluxErrorCode.gatewayUpstreamUnknown,
  PluxErrorCode.gatewayDestinationRefused,
  PluxErrorCode.gatewayUpstreamFailed,
  PluxErrorCode.directDataSourceRefused,
  PluxErrorCode.gatewayRateLimited,
  PluxErrorCode.scaSignatureInvalid,
  PluxErrorCode.scaChallengeInvalid,
  PluxErrorCode.scaDynamicLinkingMismatch,
  PluxErrorCode.transactionRequiresGateway,
};

bool _isConfig(PluxErrorCode c) => const {
  PluxErrorCode.boundLocationSetByDevice,
  PluxErrorCode.boundValueChanged,
  PluxErrorCode.liveConfigStale,
  PluxErrorCode.liveConfigUnavailable,
  PluxErrorCode.configStoreUnavailable,
}.contains(c);

ActionErrorKind _kind(PluxErrorCode c) => switch (c) {
  PluxErrorCode.gatewayUpstreamFailed => ActionErrorKind.network,
  PluxErrorCode.gatewayRateLimited => ActionErrorKind.http,
  PluxErrorCode.liveConfigUnavailable ||
  PluxErrorCode.configStoreUnavailable => ActionErrorKind.network,
  PluxErrorCode.userTokenInvalid => ActionErrorKind.http,
  PluxErrorCode.gatewayUpstreamUnknown ||
  PluxErrorCode.gatewayDestinationRefused ||
  PluxErrorCode.assuranceInsufficient ||
  PluxErrorCode.deviceRevoked => ActionErrorKind.custom,
  _ => ActionErrorKind.permission,
};
