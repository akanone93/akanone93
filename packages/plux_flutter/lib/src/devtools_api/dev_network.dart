// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Network simulation for a dev session (`DEV-031`): `networkProfile`
/// selects a profile of `network_profiles.g.dart`, and a client wrapped by
/// [DevNetworkSimulation.wrap] then delays, throttles and drops its
/// requests as the profile says. A host wraps the clients it creates for the
/// UI isolate (`PluxConfig.httpClient`) only under `kPluxDevtools`; the
/// profile applies to what that wrapper carries, not to clients the sync and
/// data isolates create.
library;

import 'dart:async';
import 'dart:math';

import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/devtools_api/dev_nodes.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/devtools_api/network_profiles.g.dart';

/// Waits for [duration]; tests replace it with a recording fake.
typedef DevDelay = Future<void> Function(Duration duration);

/// The profile in force and the clients that apply it.
final class DevNetworkSimulation {
  /// Creates a simulation with no profile. [random] decides jitter and
  /// loss, [delay] waits; both are replaced in tests.
  DevNetworkSimulation({Random? random, DevDelay? delay})
    : _random = random ?? Random(),
      _delay = delay ?? Future<void>.delayed;

  final Random _random;
  final DevDelay _delay;

  /// The profile in force; null for the real network.
  NetworkProfile? profile;

  /// [inner] with the profile applied to each request it sends.
  http.Client wrap(http.Client inner) => _SimulatedClient(inner, this);

  Duration _latency(NetworkProfile p) {
    final jitter = p.jitterMs == 0
        ? 0.0
        : (_random.nextDouble() * 2 - 1) * p.jitterMs;
    final ms = p.latencyMs + jitter;
    return Duration(microseconds: ms <= 0 ? 0 : (ms * 1000).round());
  }

  Duration _transfer(int bytes, int kbitPerSecond) => kbitPerSecond <= 0
      ? Duration.zero
      : Duration(microseconds: (bytes * 8 * 1000 / kbitPerSecond).round());

  bool _drops(NetworkProfile p) =>
      p.lossPercent >= 100 ||
      (p.lossPercent > 0 && _random.nextDouble() * 100 < p.lossPercent);
}

final class _SimulatedClient extends http.BaseClient {
  _SimulatedClient(this._inner, this._net);

  final http.Client _inner;
  final DevNetworkSimulation _net;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final p = _net.profile;
    if (p == null) return _inner.send(request);
    if (_net._drops(p)) {
      throw http.ClientException(
        'SocketException: the simulated network ${p.id} dropped the request',
        request.url,
      );
    }
    await _net._delay(
      _net._latency(p) +
          _net._transfer(request.contentLength ?? 0, p.upKbitPerSecond),
    );
    final response = await _inner.send(request);
    if (p.downKbitPerSecond <= 0) return response;
    return http.StreamedResponse(
      response.stream.asyncMap((chunk) async {
        await _net._delay(_net._transfer(chunk.length, p.downKbitPerSecond));
        return chunk;
      }),
      response.statusCode,
      contentLength: response.contentLength,
      request: response.request,
      headers: response.headers,
      isRedirect: response.isRedirect,
      persistentConnection: response.persistentConnection,
      reasonPhrase: response.reasonPhrase,
    );
  }

  @override
  void close() => _inner.close();
}

/// Carries out `networkProfile`.
final class DevNetworkHandler implements DevCommandHandler {
  /// Creates the handler.
  const DevNetworkHandler({required this.simulation});

  /// The simulation the profile is set on.
  final DevNetworkSimulation simulation;

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    final body = devCommandBody(command, 'networkProfile');
    final id = body?['profileId'] ?? '';
    if (id is! String) {
      return devFailure(devStatusFailed, 'profileId is not text');
    }
    if (id.isEmpty || id == 'good_wifi') {
      simulation.profile = null;
      return {'status': devStatusOk};
    }
    for (final p in networkProfiles) {
      if (p.id == id) {
        simulation.profile = p;
        return {'status': devStatusOk};
      }
    }
    return devFailure(devStatusFailed, 'there is no network profile $id');
  }
}
