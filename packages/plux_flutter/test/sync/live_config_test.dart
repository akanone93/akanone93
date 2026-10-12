// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/gateway.dart';
import 'package:plux_flutter/src/data/live_config.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/device_auth.dart';
import 'package:plux_flutter/src/sync/gateway_agent.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';
import 'package:plux_flutter/src/sync/sync_worker.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';

import 'fake_device.dart';
import 'fake_server.dart';

const _poolKey = 'payment.poolAccount';
const _pool = 'POOL-ACCOUNT-0042-7731';

Map<String, Object?> _value(
  String key,
  String type,
  String value, {
  bool strict = false,
}) => {'key': key, 'type': type, 'value': value, 'strict': strict};

GatewayRequest _request({String key = 'idem-1'}) => GatewayRequest(
  upstream: 'ledger',
  operation: 'transfer',
  method: 'POST',
  path: '/transfers',
  headers: {'idempotency-key': key},
  body: Uint8List.fromList(utf8.encode('{"a":1}')),
  maxRequestBytes: 1024,
  maxResponseBytes: 1024,
  timeout: const Duration(seconds: 5),
);

/// The agent of a device registered with [server].
Future<GatewayAgent> _agent(FakePluxServer server) async {
  final api = PluxApiClient(http.Client(), server.endpoint);
  final auth = DeviceAuth(
    api: api,
    keys: FakeDeviceKeys(),
    attestation: FakeAttestation(),
    credentials: MemoryCredentialStore(),
    appId: FakePluxServer.app,
    environment: FakePluxServer.environment,
    device: const DeviceInfo(
      platform: 'android',
      osVersion: '14',
      runtimeVersion: '0.1.0',
      hostBuild: '1',
    ),
    enrolSca: false,
  );
  return GatewayAgent(api: api, auth: auth);
}

/// Waits until [done] holds, or fails.
Future<void> _until(bool Function() done) async {
  for (var i = 0; i < 200 && !done(); i++) {
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
  expect(done(), isTrue, reason: 'it did not happen in time');
}

DataFailure _failure(Object? e) => e! as DataFailure;

void main() {
  late FakePluxServer server;
  late GatewayAgent agent;

  setUp(() async {
    server = await FakePluxServer.start();
    server.gatewayEnabled = true;
    server.liveValues = [
      _value(_poolKey, 'string', _pool),
      _value('limits.daily', 'integer', '5000'),
      _value('fees.rate', 'decimal', '0.0125', strict: true),
      _value('features.fastPay', 'boolean', 'true'),
    ];
    agent = await _agent(server);
  });

  tearDown(() => server.close());

  group('the live values', () {
    test(
      'are fetched over the device channel and typed [DAT-094] [DAT-090]',
      () async {
        final pool = await agent.live.read(_poolKey);
        expect(
          (pool.type, pool.value, pool.strict),
          (LiveConfigType.string, _pool, false),
        );
        expect((await agent.live.read('limits.daily')).value, 5000);
        expect((await agent.live.read('features.fastPay')).value, true);
        final fee = await agent.live.read('fees.rate');
        expect(
          (fee.type, fee.value, fee.strict),
          (LiveConfigType.decimal, '0.0125', true),
        );
        expect(agent.live.version, 1);
        expect(
          server.credentialsSeen.where((c) => c.startsWith('DPoP plux_dat_')),
          isNotEmpty,
          reason: 'the fetch carries the device token and a proof',
        );
      },
    );

    test('a value the server did not send, or one that does not read as its type, is no value [DAT-097]', () async {
      await expectLater(
        agent.live.read('missing'),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            ConfigCode.unavailable.raised,
          ),
        ),
      );
      server.liveValues = [_value('limits.daily', 'integer', 'many')];
      final other = await _agent(server);
      await expectLater(
        other.live.read('limits.daily'),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            ConfigCode.unavailable.raised,
          ),
        ),
      );
    });

    test('never show their content when printed [DAT-094]', () async {
      final v = await agent.live.read(_poolKey);
      expect('$v', isNot(contains(_pool)));
      expect(
        '${LiveConfigSnapshot(version: 1, epoch: 1, values: {_poolKey: v})}',
        isNot(contains(_pool)),
      );
    });

    test(
      'are kept in memory only: a session leaves no trace on disk [DAT-094]',
      () async {
        final root = Directory.systemTemp.createTempSync('plux_live').path;
        final worker = await SyncWorker.start(
          SyncWorkerConfig(
            storeRoot: root,
            endpoint: server.endpoint,
            httpClient: http.Client.new,
            credentials: MemoryCredentialStore.new,
            deviceKeys: FakeDeviceKeys.new,
            attestation: FakeAttestation.new,
            sync: SyncConfig(
              appId: FakePluxServer.app,
              environment: FakePluxServer.environment,
              channel: FakePluxServer.channel,
              keys: [server.trustedKey],
              device: const DeviceInfo(
                platform: 'android',
                osVersion: '14',
                runtimeVersion: '0.1.0',
                hostBuild: '1',
              ),
              supportsFeature: (_) => true,
              verifierLimits: const VerifierLimits(
                maxDepth: 64,
                maxVisits: 1000000,
              ),
            ),
            baseline: (_) async => null,
          ),
        );
        try {
          final v = await worker.read(_poolKey);
          expect(v.value, _pool, reason: 'the read crossed the isolate');
          await worker.call(_request());
          await worker.settle();
          final needle = utf8.encode(_pool);
          for (final e in Directory(root).listSync(recursive: true)) {
            if (e is! File) continue;
            final bytes = e.readAsBytesSync();
            expect(
              _contains(bytes, needle),
              isFalse,
              reason: '${e.path} holds a live value',
            );
          }
          expect(_contains(utf8.encode('x$_pool'), needle), isTrue);
        } finally {
          await worker.close();
          Directory(root).deleteSync(recursive: true);
        }
      },
    );
  });

  group('every gateway call', () {
    test(
      'names the live version it used, 0 before any was fetched [DAT-096]',
      () async {
        await agent.call(_request());
        await agent.live.read(_poolKey);
        server.liveVersion = 3;
        await agent.call(_request());
        expect(server.gatewayVersions, [0, 1]);
        await agent.live.refresh();
        await agent.call(_request());
        expect(server.gatewayVersions, [0, 1, 3]);
      },
    );
  });

  group('a refused version', () {
    test('refreshes the values and repeats the call once, with the same idempotency key [DAT-096]', () async {
      await agent.live.read(_poolKey);
      server
        ..liveVersion = 2
        ..refuseBelowVersion = 2;
      final res = await agent.call(_request(key: 'idem-42'));
      expect(res.status, 200);
      expect(server.gatewayVersions, [1, 2]);
      expect(server.liveFetches, 2);
      final keys = [
        for (final b in server.gatewayBodies)
          (b['headers']! as Map<String, Object?>)['idempotency-key'],
      ];
      expect(keys, ['idem-42', 'idem-42']);
      expect(
        server.gatewayBodies.last['body'],
        server.gatewayBodies.first['body'],
        reason: 'the same request',
      );
    });

    test('is reported when the repeat is refused as well, and nothing is repeated again [DAT-096]', () async {
      await agent.live.read(_poolKey);
      server.refuseBelowVersion = 9;
      await expectLater(
        agent.call(_request()),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            ConfigCode.stale.registered ?? PluxErrorCode.dataHttpError,
          ),
        ),
      );
      expect(server.gatewayVersions, hasLength(2));
      expect(server.liveFetches, 2);
    });

    test('is reported as unavailable when the refresh cannot be made [DAT-096] [DAT-097]', () async {
      await agent.live.read(_poolKey);
      server
        ..refuseBelowVersion = 9
        ..faults['/plux.v1.ConfigService/GetLiveConfig'] = [
          const StatusFault(503),
        ];
      await expectLater(
        agent.call(_request()),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            ConfigCode.unavailable.raised,
          ),
        ),
      );
      expect(server.gatewayVersions, hasLength(1));
    });
  });

  group('a newer epoch', () {
    test('refreshes the values in the background, once [DAT-095]', () async {
      await agent.live.read(_poolKey);
      expect(server.liveFetches, 1);
      server
        ..liveVersion = 2
        ..liveEpoch = 2
        ..liveValues = [_value(_poolKey, 'string', 'POOL-ACCOUNT-NEW')];
      await agent.call(_request());
      await _until(() => agent.live.version == 2);
      expect(server.liveFetches, 2);
      expect((await agent.live.read(_poolKey)).value, 'POOL-ACCOUNT-NEW');
      await agent.call(_request());
      await Future<void>.delayed(const Duration(milliseconds: 50));
      expect(server.liveFetches, 2, reason: 'the epoch is not newer now');
    });

    test(
      'fetches nothing for a device that never read a value [DAT-095]',
      () async {
        server.liveEpoch = 5;
        await agent.call(_request());
        await Future<void>.delayed(const Duration(milliseconds: 50));
        expect(server.liveFetches, 0);
      },
    );

    test(
      'a refresh that fails is not an error of the call [DAT-095]',
      () async {
        await agent.live.read(_poolKey);
        server
          ..liveEpoch = 2
          ..faults['/plux.v1.ConfigService/GetLiveConfig'] = [
            const StatusFault(503),
          ];
        final res = await agent.call(_request());
        expect(res.status, 200);
        await _until(() => server.faults.values.every((q) => q.isEmpty));
      },
    );
  });

  group('offline', () {
    test(
      'a strict value cannot be read, and the read says why [DAT-097]',
      () async {
        expect((await agent.live.read('fees.rate')).value, '0.0125');
        server.faults['/plux.v1.ConfigService/GetLiveConfig'] = [
          const StatusFault(503),
        ];
        Object? error;
        try {
          await agent.live.read('fees.rate');
        } on Object catch (e) {
          error = e;
        }
        final f = _failure(error);
        expect(f.code, ConfigCode.unavailable.raised);
        expect(f.message, isNot(contains('0.0125')));
        expect(f.toActionError().code, f.code);
      },
    );

    test(
      'a strict value is read from the server each time [DAT-097]',
      () async {
        await agent.live.read('fees.rate');
        final before = server.liveFetches;
        await agent.live.read('fees.rate');
        expect(server.liveFetches, before + 1);
        server.liveValues = [
          _value('fees.rate', 'decimal', '0.02', strict: true),
        ];
        server.liveVersion = 2;
        expect((await agent.live.read('fees.rate')).value, '0.02');
      },
    );

    test(
      'any other value is the last one this process received [DAT-097]',
      () async {
        await agent.live.read(_poolKey);
        await server.close();
        expect((await agent.live.read(_poolKey)).value, _pool);
        await expectLater(
          agent.live.read('fees.rate'),
          throwsA(
            isA<DataFailure>().having(
              (f) => f.code,
              'code',
              ConfigCode.unavailable.raised,
            ),
          ),
        );
      },
    );

    test(
      'with no value ever received, nothing can be read [DAT-097]',
      () async {
        await server.close();
        await expectLater(
          agent.live.read(_poolKey),
          throwsA(
            isA<DataFailure>().having(
              (f) => f.code,
              'code',
              ConfigCode.unavailable.raised,
            ),
          ),
        );
      },
    );
  });
}

bool _contains(List<int> haystack, List<int> needle) {
  for (var i = 0; i + needle.length <= haystack.length; i++) {
    var j = 0;
    while (j < needle.length && haystack[i + j] == needle[j]) {
      j++;
    }
    if (j == needle.length) return true;
  }
  return false;
}
