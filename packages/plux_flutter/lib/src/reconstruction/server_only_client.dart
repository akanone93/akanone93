// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The HTTP client of reconstruction mode (SEC-142, ADR-0082): requests to
/// the Plux server go through, every other request is refused before it
/// leaves the device, so a reconstructed page cannot reach a customer's
/// system or any third party.
library;

import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/errors/plux_exception.dart';

/// Wraps [inner] so that only requests to the origin of [server] are sent.
final class ServerOnlyClient extends http.BaseClient {
  /// Creates the client for [server], the Plux server's base URL.
  ServerOnlyClient(this._inner, this.server);

  final http.Client _inner;

  /// The Plux server.
  final Uri server;

  /// Whether [url] is on the Plux server's origin.
  bool allows(Uri url) =>
      url.scheme == server.scheme &&
      url.host == server.host &&
      url.port == server.port;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) {
    if (!allows(request.url)) {
      // Only the host: the rest of the address may carry user data
      // (SEC-092).
      return Future.error(
        PluxException(
          PluxErrorCode.actionsNotAvailable,
          'reconstruction mode makes no request to ${request.url.host} '
          '(SEC-142)',
        ),
      );
    }
    return _inner.send(request);
  }

  @override
  void close() => _inner.close();
}
