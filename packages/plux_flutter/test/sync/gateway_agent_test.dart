// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/src/data/client.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/gateway.dart';
import 'package:plux_flutter/src/data/spec.dart';
import 'package:plux_flutter/src/data/transport.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/attestation.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/sca.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/device_auth.dart';
import 'package:plux_flutter/src/sync/gateway_agent.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';

import 'fake_device.dart';

const _challengePath = '/plux.v1.DeviceService/CreateRegistrationChallenge';
const _registerPath = '/plux.v1.DeviceService/RegisterAttestedDevice';
const _refreshPath = '/plux.v1.TokenService/RefreshDeviceToken';
const _callPath = '/plux.v1.GatewayService/Call';
const _exchangePath = '/plux.v1.TokenService/ExchangeUserToken';
const _scaPath = '/plux.v1.ScaService/CreateTransactionChallenge';

final _registration = Uint8List.fromList([for (var i = 1; i <= 32; i++) i]);
final _scaChallenge = Uint8List.fromList([for (var i = 50; i < 66; i++) i]);

String _b64url(List<int> bytes) => base64Url.encode(bytes).replaceAll('=', '');

final class _Seen {
  _Seen(this.path, this.headers, this.body);
  final String path;
  final Map<String, String> headers;
  final Map<String, Object?> body;

  Map<String, Object?> get proof => jsonDecode(
    utf8.decode(
      base64Url.decode(base64Url.normalize(headers['dpop']!.split('.')[1])),
    ),
  ) as Map<String, Object?>;
}

http.Response _json(Object body, {int status = 200}) =>
    http.Response(jsonEncode(body), status);

http.Response _plx(int status, String code, String plx) =>
    _json({'code': code, 'message': '$plx: refused'}, status: status);

final class _Rig {
  _Rig({bool sca = false}) {
    client = MockClient((req) async {
      final text = utf8.decode(req.bodyBytes);
      seen.add(
        _Seen(
          req.url.path,
          req.headers,
          text.isEmpty ? {} : jsonDecode(text) as Map<String, Object?>,
        ),
      );
      final queued = script[req.url.path];
      if (queued != null && queued.isNotEmpty) return queued.removeAt(0)();
      return _default(req.url.path);
    });
    api = PluxApiClient(client, Uri.parse('https://plux.example/'));
    auth = DeviceAuth(
      api: api,
      keys: keys,
      attestation: attestation,
      credentials: credentials,
      appId: 'app_1',
      environment: 'production',
      device: DeviceInfo(
        platform: 'android',
        osVersion: '14',
        runtimeVersion: '0.1.0',
        hostBuild: '7',
      ),
      enrolSca: sca,
      clock: () => now,
    );
    agent = GatewayAgent(api: api, auth: auth, clock: () => now);
  }

  late final http.Client client;
  late final PluxApiClient api;
  late final DeviceAuth auth;
  late final GatewayAgent agent;
  final List<_Seen> seen = [];
  final Map<String, List<http.Response Function()>> script = {};
  final keys = FakeDeviceKeys();
  final attestation = FakeAttestation();
  final credentials = MemoryCredentialStore();
  var now = DateTime.utc(2026, 10, 10, 12);
  var exchanges = 0;
  var devices = 0;
  var tokens = 0;

  http.Response _default(String path) => switch (path) {
    _challengePath => _json({
      'challenge': base64.encode(_registration),
      'expiresAt': '2100-01-01T00:00:00Z',
    }),
    _registerPath => _json({
      'device': {'id': 'dev-${++devices}'},
    }),
    _refreshPath => _json({
      'accessToken': 'tok-${++tokens}',
      'expiresAt': now.add(const Duration(minutes: 15)).toIso8601String(),
      'tokenType': 'DPoP',
    }),
    _callPath => _json({
      'status': 200,
      'headers': {'Content-Type': 'application/json'},
      'body': base64.encode(utf8.encode('{"ok":true}')),
    }),
    _exchangePath => _json({
      'accessToken': 'utok-${++exchanges}',
      'issuedTokenType': 'urn:ietf:params:oauth:token-type:access_token',
      'expiresAt': now.add(const Duration(minutes: 5)).toIso8601String(),
    }),
    _scaPath => _json({
      'challengeId': 'ch-1',
      'challenge': base64.encode(_scaChallenge),
      'expiresAt': '2100-01-01T00:00:00Z',
    }),
    _ => _json(<String, Object?>{}),
  };

  List<String> get paths => [for (final s in seen) s.path];
  List<_Seen> at(String path) => [
    for (final s in seen)
      if (s.path == path) s,
  ];
}

GatewayRequest _request({
  String upstream = 'ledger',
  String? userToken,
  Confirmation? confirmation,
  int maxResponse = 1024,
  Duration timeout = const Duration(seconds: 5),
}) => GatewayRequest(
  upstream: upstream,
  operation: 'transfer',
  method: 'POST',
  path: '/transfers?x=1',
  headers: const {'content-type': 'application/json'},
  body: Uint8List.fromList(utf8.encode('{"a":1}')),
  maxRequestBytes: 1024,
  maxResponseBytes: maxResponse,
  timeout: timeout,
  userToken: userToken,
  confirmation: confirmation,
);

void main() {
  group('the call', () {
    test('carries the device token and a DPoP proof bound to the gateway route and the token [SEC-020] [SEC-030]', () async {
      final rig = _Rig();
      final res = await rig.agent.call(_request());
      expect(res.status, 200);
      expect(res.json, {'ok': true});
      expect(res.bytes, 11);

      final call = rig.at(_callPath).single;
      expect(call.headers['authorization'], 'DPoP tok-1');
      expect(call.headers, isNot(contains('plux-user-token')));
      final claims = call.proof;
      expect(claims['htm'], 'POST');
      expect(claims['htu'], 'https://plux.example$_callPath');
      expect(
        claims['ath'],
        _b64url(sha256.convert(ascii.encode('tok-1')).bytes),
      );
      expect(call.body['upstream'], 'ledger');
      expect(call.body['operation'], 'transfer');
      expect(call.body['method'], 'POST');
      expect(call.body['path'], '/transfers?x=1');
      expect(call.body['headers'], {'content-type': 'application/json'});
      expect(
        utf8.decode(base64.decode(call.body['body']! as String)),
        '{"a":1}',
      );
      expect(call.body, isNot(contains('transactionChallengeId')));
      expect(call.body, isNot(contains('transactionSignature')));
    });

    test(
      'a confirmation travels as the challenge ID and the signature [SEC-032]',
      () async {
        final rig = _Rig();
        final signature = Uint8List.fromList(List.filled(64, 7));
        await rig.agent.call(
          _request(
            confirmation: Confirmation(
              challengeId: 'ch-9',
              signature: signature,
              summary: const TransactionSummary(
                amount: '1',
                currency: 'EUR',
                payee: 'Ada',
              ),
              expiresAt: DateTime.utc(2100),
            ),
          ),
        );
        final body = rig.at(_callPath).single.body;
        expect(body['transactionChallengeId'], 'ch-9');
        expect(
          base64.decode(body['transactionSignature']! as String),
          signature,
        );
      },
    );

    test('a response over the limit or not JSON is a typed failure [LIM-004] [DAT-004]', () async {
      final rig = _Rig();
      await expectLater(
        rig.agent.call(_request(maxResponse: 5)),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.dataSizeExceeded,
          ),
        ),
      );
      rig.script[_callPath] = [
        () => _json({
          'status': 200,
          'body': base64.encode(utf8.encode('not json')),
        }),
      ];
      await expectLater(
        rig.agent.call(_request()),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.dataMappingFailed,
          ),
        ),
      );
    });

    test('an upstream error status and its empty body come back as they are [DAT-001]', () async {
      final rig = _Rig();
      rig.script[_callPath] = [
        () => _json({'status': 503}),
      ];
      final res = await rig.agent.call(_request());
      expect((res.status, res.json, res.bytes), (503, null, 0));
    });

    test('a token the server no longer accepts is replaced once, then the call repeats [SEC-021]', () async {
      final rig = _Rig();
      rig.script[_callPath] = [
        () => _json({
          'code': 'unauthenticated',
          'message': 'expired',
        }, status: 401),
      ];
      await rig.agent.call(_request());
      expect(rig.at(_callPath), hasLength(2));
      expect(rig.at(_callPath).last.headers['authorization'], 'DPoP tok-2');
    });

    test('the server\'s nonce is used on the repeat [SEC-024]', () async {
      final rig = _Rig();
      rig.script[_callPath] = [
        () => http.Response(
          jsonEncode({'code': 'unauthenticated', 'message': 'nonce'}),
          401,
          headers: {
            'dpop-nonce': 'n-1',
            'www-authenticate': 'DPoP error="use_dpop_nonce"',
          },
        ),
      ];
      await rig.agent.call(_request());
      expect(rig.at(_callPath).last.proof['nonce'], 'n-1');
      expect(rig.at(_callPath), hasLength(2));
      expect(rig.at(_refreshPath), hasLength(1), reason: 'same token');
    });

    test('the call times out with data.requestTimeout [LIM-004]', () async {
      final rig = _Rig();
      final agent = GatewayAgent(
        api: PluxApiClient(
          MockClient((req) async {
            if (req.url.path == _callPath) {
              await Future<void>.delayed(const Duration(seconds: 2));
            }
            return rig.client.send(req).then(http.Response.fromStream);
          }),
          Uri.parse('https://plux.example/'),
        ),
        auth: rig.auth,
      );
      await expectLater(
        agent.call(_request(timeout: const Duration(milliseconds: 50))),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.dataRequestTimeout,
          ),
        ),
      );
    });

    test(
      'the server\'s registered refusals keep their codes [SEC-031] [SEC-030]',
      () async {
        final cases = {
          6060: PluxErrorCode.gatewayUpstreamUnknown,
          6061: PluxErrorCode.gatewayDestinationRefused,
          6062: PluxErrorCode.gatewayUpstreamFailed,
          6063: PluxErrorCode.directDataSourceRefused,
          6064: PluxErrorCode.gatewayRateLimited,
          6070: PluxErrorCode.scaSignatureInvalid,
          6071: PluxErrorCode.scaChallengeInvalid,
          6072: PluxErrorCode.scaDynamicLinkingMismatch,
          6073: PluxErrorCode.transactionRequiresGateway,
          6002: PluxErrorCode.assuranceInsufficient,
          6006: PluxErrorCode.deviceRevoked,
        };
        for (final MapEntry(:key, :value) in cases.entries) {
          final rig = _Rig();
          rig.script[_callPath] = [
            () => _plx(403, 'permission_denied', 'PLX-$key'),
          ];
          await expectLater(
            rig.agent.call(_request()),
            throwsA(isA<DataFailure>().having((f) => f.code, 'code', value)),
          );
        }
      },
    );

    test('a refusal for assurance or a revoked device lowers the level at once [SEC-007]', () async {
      final rig = _Rig();
      await rig.auth.token();
      rig.script[_refreshPath] = [
        () => _json({
          'accessToken': 'tok-x',
          'expiresAt': '2100-01-01T00:00:00Z',
          'assuranceLevel': 'AL2',
        }),
      ];
      rig.auth.forgetToken();
      await rig.auth.token();
      expect(rig.auth.assurance, 2);
      rig.script[_callPath] = [
        () => _plx(403, 'permission_denied', 'PLX-6002'),
      ];
      await expectLater(
        rig.agent.call(_request()),
        throwsA(isA<DataFailure>()),
      );
      expect(rig.auth.assurance, 0);
    });

    test('a network failure or an unanswered call is a network failure, not an http one [DAT-001]', () async {
      final rig = _Rig();
      rig.script[_callPath] = [
        () => _json({'code': 'unavailable', 'message': 'down'}, status: 503),
      ];
      await expectLater(
        rig.agent.call(_request()),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.dataNetworkFailed,
          ),
        ),
      );
    });
  });

  group('the user token', () {
    test('the host token is exchanged for the upstream\'s audience and the Plux token, not the host\'s, goes in Plux-User-Token [SEC-026]', () async {
      final rig = _Rig();
      await rig.agent.call(_request(userToken: 'host-jwt'));
      final x = rig.at(_exchangePath).single;
      expect(x.body, {
        'subjectToken': 'host-jwt',
        'subjectTokenType': 'urn:ietf:params:oauth:token-type:access_token',
        'audience': 'ledger',
      });
      expect(x.headers['authorization'], 'DPoP tok-1');
      expect(x.proof['htu'], 'https://plux.example$_exchangePath');
      final call = rig.at(_callPath).single;
      expect(call.headers['plux-user-token'], 'utok-1');
      expect(jsonEncode(call.body), isNot(contains('host-jwt')));
      expect(jsonEncode(call.body), isNot(contains('utok-1')));
      expect(call.headers.values.join(), isNot(contains('host-jwt')));
      // The exchange comes first.
      expect(
        rig.paths.indexOf(_exchangePath),
        lessThan(rig.paths.indexOf(_callPath)),
      );
    });

    test('is kept per upstream until shortly before it expires, and a different host token exchanges again [SEC-026]', () async {
      final rig = _Rig();
      await rig.agent.call(_request(userToken: 'host-jwt'));
      await rig.agent.call(_request(userToken: 'host-jwt'));
      expect(rig.at(_exchangePath), hasLength(1));
      await rig.agent.call(_request(userToken: 'host-jwt', upstream: 'cards'));
      expect(rig.at(_exchangePath), hasLength(2));
      expect(rig.at(_exchangePath).last.body['audience'], 'cards');
      await rig.agent.call(_request(userToken: 'other-user'));
      expect(rig.at(_exchangePath), hasLength(3));
      expect(rig.at(_callPath).last.headers['plux-user-token'], 'utok-3');

      rig.now = rig.now.add(const Duration(minutes: 5));
      await rig.agent.call(_request(userToken: 'other-user'));
      expect(rig.at(_exchangePath), hasLength(4), reason: 'near expiry');
    });

    test('a source that is not auth sends none, and an invalid host token is PLX-6016 [SEC-026]', () async {
      final rig = _Rig();
      await rig.agent.call(_request());
      expect(rig.at(_exchangePath), isEmpty);
      rig.script[_exchangePath] = [
        () => _plx(401, 'unauthenticated', 'PLX-6016'),
      ];
      await expectLater(
        rig.agent.call(_request(userToken: 'bad')),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.userTokenInvalid,
          ),
        ),
      );
      expect(
        rig.at(_callPath),
        hasLength(1),
        reason: 'nothing reached the upstream',
      );
    });
  });

  group('enrolment', () {
    test('registration creates the SCA key for the registration challenge and registers it with its own evidence [SEC-027]', () async {
      final rig = _Rig(sca: true)
        ..keys.chain = [
          Uint8List.fromList([1, 2]),
        ]
        ..attestation.evidence = AndroidEvidence(
          keyAttestationChain: [
            Uint8List.fromList([1, 2]),
          ],
          playIntegrityToken: 'integrity',
        );
      await rig.auth.token();
      final sca = rig.keys.stored[scaKeyAlias('app_1', 'production')]!;
      expect(sca.purpose, KeyPurpose.sca);
      expect(rig.keys.challenges, [_registration, _registration]);
      final body = rig.at(_registerPath).single.body;
      expect(
        jsonDecode(
          utf8.decode(base64.decode(body['scaPublicKeyJwk']! as String)),
        ),
        sca.publicKey.jwk,
      );
      final evidence = (body['scaEvidence']! as Map)['android']! as Map;
      expect(evidence['playIntegrityToken'], 'integrity');
      expect(evidence['keyAttestationChain'], isNotEmpty);
      expect(body['dpopPublicKeyJwk'], isNot(body['scaPublicKeyJwk']));
    });

    test(
      'a device that cannot make the key registers without it [SEC-027]',
      () async {
        final rig = _Rig(sca: true);
        final keys = _NoScaKeys(rig.keys);
        final auth = DeviceAuth(
          api: rig.api,
          keys: keys,
          attestation: rig.attestation,
          credentials: rig.credentials,
          appId: 'app_1',
          environment: 'production',
          device: const DeviceInfo(
            platform: 'android',
            osVersion: '14',
            runtimeVersion: '0.1.0',
            hostBuild: '7',
          ),
          enrolSca: true,
        );
        await auth.token();
        final body = rig.at(_registerPath).single.body;
        expect(body, isNot(contains('scaPublicKeyJwk')));
        expect(body, isNot(contains('scaEvidence')));
        expect(await auth.ensureSca(), isFalse);
        expect(
          await auth.ensureSca(),
          isFalse,
          reason: 'tried once per launch',
        );
        expect(rig.at(_registerPath), hasLength(2));
      },
    );

    test('without the flag no key is made; ensureSca registers again with one, once [SEC-027]', () async {
      final rig = _Rig();
      await rig.auth.token();
      expect(rig.keys.stored.keys.where((k) => k.contains('sca')), isEmpty);
      expect(
        rig.at(_registerPath).single.body,
        isNot(contains('scaPublicKeyJwk')),
      );

      expect(await rig.auth.ensureSca(), isTrue);
      expect(rig.at(_registerPath), hasLength(2));
      expect(rig.at(_registerPath).last.body, contains('scaPublicKeyJwk'));
      expect(await rig.auth.ensureSca(), isTrue);
      expect(rig.at(_registerPath), hasLength(2), reason: 'it has the key now');
    });
  });

  group('a transaction, from the challenge to the call', () {
    testWidgets(
      'runs challenge, sheet, system prompt, signature and then the gateway call, in that order [SEC-027] [SEC-028] [SEC-032]',
      (tester) async {
        final rig = _Rig(sca: true);
        final order = <String>[];
        final signer = _Signer(order);
        late BuildContext screen;
        final sca = ScaService(
          challenges: _Challenges(rig, order),
          keys: rig.keys,
          signer: signer,
          confirmer: _Confirmer(order),
          alias: scaKeyAlias('app_1', 'production'),
          context: () => screen,
        );
        await tester.pumpWidget(
          MaterialApp(
            home: Builder(
              builder: (c) {
                screen = c;
                return const SizedBox();
              },
            ),
          ),
        );
        await rig.auth.token();
        final client = DataClient(
          transport: _Unused(),
          auth: AuthSession(() => null),
          limits: () => DataLimits.of(const {}),
          environment: 'production',
          record: (name, {fields = const {}, route = '', pluginKey = ''}) {},
          report: (_) {},
          gateway: _AgentGateway(rig, order),
          authoriser: sca,
        );
        const mapping = TransactionMapping(
          amount: '/amount',
          currency: '/currency',
          payee: '/to',
        );
        final spec = DataSourceSpec(
          id: 'id',
          name: 'bank',
          kind: DataKind.rest,
          type: 'int',
          transaction: mapping,
          operations: const {
            'transfer': OperationSpec(
              name: 'transfer',
              path: '/t',
              auth: false,
            ),
          },
        );
        order.clear();
        rig.seen.clear();
        await client.call(
          spec,
          spec.operations['transfer']!,
          const DataCaller(pluginKey: 'bank', domains: []),
          {'amount': '12.50', 'currency': 'EUR', 'to': 'Ada'},
        );
        expect(order, ['challenge', 'sheet', 'prompt', 'call']);
        expect(rig.paths, [_scaPath, _callPath]);

        final summary = utf8.encode(
          '{"amount":"12.50","currency":"EUR","payee":"Ada"}',
        );
        final challenge = rig.at(_scaPath).single.body;
        expect(challenge['deviceId'], 'dev-1');
        expect(challenge['dataSource'], 'bank');
        expect(challenge['operation'], 'transfer');
        expect(
          base64.decode(challenge['transactionSummaryJcs']! as String),
          summary,
        );
        expect(signer.signed, [..._scaChallenge, ...summary]);

        final call = rig.at(_callPath).single.body;
        expect(call['transactionChallengeId'], 'ch-1');
        expect(
          base64.decode(call['transactionSignature']! as String),
          Uint8List.fromList(List.filled(64, 4)),
        );
        expect(call['body'], isNotNull);
        expect(
          utf8.decode(base64.decode(call['body']! as String)),
          '{"amount":"12.50","currency":"EUR","to":"Ada"}',
        );
      },
    );
  });
}

/// Delegates to [inner] but cannot create SCA keys, as a device without a
/// screen lock.
final class _NoScaKeys implements DeviceKeys {
  _NoScaKeys(this.inner);
  final FakeDeviceKeys inner;

  @override
  Future<DeviceKey> create(
    String alias,
    KeyPurpose purpose, {
    Uint8List? challenge,
    bool strongBox = true,
  }) {
    if (purpose == KeyPurpose.sca) {
      throw const PluxException(PluxErrorCode.deviceKeyUnavailable, 'no lock');
    }
    return inner.create(alias, purpose, challenge: challenge);
  }

  @override
  Future<DeviceKey?> load(String alias, KeyPurpose purpose) =>
      inner.load(alias, purpose);

  @override
  Future<Uint8List> sign(String alias, Uint8List data) =>
      inner.sign(alias, data);

  @override
  Future<Uint8List> agree(String alias, EcPublicKey peer) =>
      inner.agree(alias, peer);

  @override
  Future<void> delete(String alias) => inner.delete(alias);
}

final class _Challenges implements ChallengeSource {
  _Challenges(this.rig, this.order);
  final _Rig rig;
  final List<String> order;

  @override
  Future<TransactionChallenge> challenge({
    required String dataSource,
    required String operation,
    required Uint8List summaryJcs,
  }) {
    order.add('challenge');
    return rig.agent.challenge(
      dataSource: dataSource,
      operation: operation,
      summaryJcs: summaryJcs,
    );
  }

  @override
  Future<bool> ensureEnrolled() => rig.agent.ensureEnrolled();
}

final class _Confirmer implements TransactionConfirmer {
  _Confirmer(this.order);
  final List<String> order;

  @override
  Future<bool> confirm(BuildContext context, TransactionSummary summary) async {
    order.add('sheet');
    return true;
  }
}

final class _Signer implements UserAuthSigner {
  _Signer(this.order);
  final List<String> order;
  Uint8List? signed;

  @override
  Future<Uint8List> signWithUser(
    String alias,
    Uint8List data, {
    required String title,
    required String subtitle,
  }) async {
    order.add('prompt');
    signed = data;
    return Uint8List.fromList(List.filled(64, 4));
  }
}

final class _AgentGateway implements GatewayTransport {
  _AgentGateway(this.rig, this.order);
  final _Rig rig;
  final List<String> order;

  @override
  Future<DataResponse> call(GatewayRequest request) {
    order.add('call');
    return rig.agent.call(request);
  }
}

final class _Unused implements DataTransport {
  @override
  Future<DataResponse> send(DataRequest request) =>
      throw StateError('a transaction never goes direct');

  @override
  Future<void> close() async {}
}
