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
    this.userToken,
    this.confirmation,
  });

  /// The upstream: the data source's name, which the server maps to the
  /// upstream registered for the app and environment.
  final String upstream;

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

/// Sends requests through the Plux gateway.
abstract interface class GatewayTransport {
  /// Sends [request] with the device's token and a DPoP proof. Completes
  /// with the upstream's status and decoded JSON body, bounded by the
  /// request's limits; fails with a [DataFailure] when the server refuses
  /// the request, the network fails or the time or size limit is passed.
  Future<DataResponse> call(GatewayRequest request);
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
    if (c.code == number) return _known.contains(c) ? c : null;
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

ActionErrorKind _kind(PluxErrorCode c) => switch (c) {
  PluxErrorCode.gatewayUpstreamFailed => ActionErrorKind.network,
  PluxErrorCode.gatewayRateLimited => ActionErrorKind.http,
  PluxErrorCode.userTokenInvalid => ActionErrorKind.http,
  PluxErrorCode.gatewayUpstreamUnknown ||
  PluxErrorCode.gatewayDestinationRefused ||
  PluxErrorCode.assuranceInsufficient ||
  PluxErrorCode.deviceRevoked => ActionErrorKind.custom,
  _ => ActionErrorKind.permission,
};
