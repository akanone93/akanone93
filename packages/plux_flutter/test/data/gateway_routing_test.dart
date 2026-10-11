// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/data/client.dart';
import 'package:plux_flutter/src/data/failure.dart';
import 'package:plux_flutter/src/data/gateway.dart';
import 'package:plux_flutter/src/data/services.dart';
import 'package:plux_flutter/src/data/source.dart';
import 'package:plux_flutter/src/data/spec.dart';
import 'package:plux_flutter/src/data/transport.dart';
import 'package:plux_flutter/src/security/sca.dart';
import 'package:plux_flutter/src/store/kv_store.dart';

import 'io_support.dart';

/// Answers gateway calls as told and records them.
final class _Gateway implements GatewayTransport {
  final List<GatewayRequest> calls = [];
  final List<Object> script = [];

  @override
  Future<DataResponse> call(GatewayRequest request) async {
    calls.add(request);
    final next = script.isEmpty
        ? const DataResponse(200, {'ok': true}, 11)
        : script.removeAt(0);
    if (next is DataFailure) throw next;
    return next as DataResponse;
  }
}

/// Authorises transactions as told and records what it was asked.
final class _Authoriser implements TransactionAuthoriser {
  final List<TransactionRequest> asked = [];
  var decline = false;

  @override
  Future<Confirmation> authorise(TransactionRequest request) async {
    asked.add(request);
    if (decline) throw const ScaDeclined();
    return Confirmation(
      challengeId: 'ch-7',
      signature: Uint8List.fromList(List.filled(64, 3)),
      summary: request.summary,
      expiresAt: DateTime.utc(2100),
    );
  }
}

final class _Auth implements PluxAuthDelegate {
  String? current = 'host-token';
  final List<String> refreshed = [];

  @override
  bool get isAuthenticated => current != null;

  @override
  Future<String?> accessToken() async => current;

  @override
  Future<String?> refresh() async {
    current = 'host-token-2';
    refreshed.add(current!);
    return current;
  }

  @override
  void onLogout() {}
}

final class _MemoryKv implements PluxKeyValueStore {
  Map<String, Object?> content = {};

  @override
  Future<Map<String, Object?>> load() async =>
      jsonDecode(jsonEncode(content)) as Map<String, Object?>;

  @override
  Future<void> save(Map<String, Object?> entries) async =>
      content = jsonDecode(jsonEncode(entries)) as Map<String, Object?>;

  @override
  Future<void> wipe() async => content = {};
}

const _caller = DataCaller(pluginKey: 'bank', domains: ['api.bank.example']);

const _mapping = TransactionMapping(
  amount: '/amount',
  currency: '/currency',
  payee: '/to',
);

DataSourceSpec _source({
  DataRoute route = DataRoute.plux,
  bool auth = false,
  TransactionMapping? transaction,
  DataKind kind = DataKind.rest,
  bool offline = false,
}) => DataSourceSpec(
  id: 'ledger-id',
  name: 'ledger',
  kind: kind,
  type: 'int',
  route: route,
  transaction: transaction,
  baseUrls: {'production': 'https://api.bank.example/v1'},
  path: '/balance',
  auth: auth,
  headers: const {'x-app': 'bank'},
  offlineCapable: offline,
  operations: {
    'transfer': OperationSpec(
      name: 'transfer',
      path: '/transfers/{id}',
      auth: auth,
      offlineCapable: offline,
    ),
    'transferFlat': OperationSpec(
      name: 'transferFlat',
      path: '/transfers',
      auth: auth,
    ),
    'read': const OperationSpec(
      name: 'read',
      method: 'GET',
      path: '/transfers',
      auth: false,
    ),
  },
);

void main() {
  late _Gateway gateway;
  late _Authoriser authoriser;
  late _Auth auth;
  late List<http.Request> direct;
  late List<PluxException> reports;
  late bool allowDirect;
  late _MemoryKv kv;
  late FakeScheduler sched;

  DataServices make({bool withGateway = true, bool withKv = false}) =>
      DataServices(
        transport: ClientTransport(
          MockClient((r) async {
            direct.add(r);
            return http.Response('{"ok":true}', 200);
          }),
        ),
        authDelegate: () => auth,
        environment: 'production',
        record: (name, {fields = const {}, route = '', pluginKey = ''}) {},
        report: reports.add,
        gateway: withGateway ? gateway : null,
        authoriser: authoriser,
        allowDirect: () => allowDirect,
        outboxStore: withKv ? kv : null,
        random: TopRandom(),
        scheduler: sched.call,
      );

  DataScope scope(DataServices services, DataSourceSpec spec) {
    final s = DataScope(
      services: services,
      own: [
        DataSourceController(
          spec: spec,
          context: services,
          caller: _caller,
          types: const {},
        ),
      ],
      shared: const [],
      roots: () => const {},
    );
    addTearDown(s.dispose);
    return s;
  }

  setUp(() {
    gateway = _Gateway();
    authoriser = _Authoriser();
    auth = _Auth();
    direct = [];
    reports = [];
    allowDirect = true;
    kv = _MemoryKv();
    sched = FakeScheduler();
  });

  group('routing', () {
    test('a plux source goes through the gateway with its path, query and headers, and never direct [SEC-030]', () async {
      final services = make();
      final spec = _source();
      await services.client.load(spec, _caller, {'q': 'a b'});
      expect(direct, isEmpty);
      final r = gateway.calls.single;
      expect(r.upstream, 'ledger');
      expect(r.operation, '');
      expect(r.method, 'GET');
      expect(r.path, '/balance?q=a+b');
      expect(r.headers, {'accept': 'application/json', 'x-app': 'bank'});
      expect(r.headers.keys, isNot(contains('authorization')));
      expect(r.userToken, isNull, reason: 'the source is not auth');
      expect(r.confirmation, isNull);
    });

    test('an operation sends its name, body and the limits of the registry [SEC-030] [LIM-004]', () async {
      final services = make();
      await services.client.call(
        _source(),
        _source().operations['transfer']!,
        _caller,
        {'id': 'a/b', 'note': 'hi'},
      );
      final r = gateway.calls.single;
      expect(r.operation, 'transfer');
      expect(r.method, 'POST');
      expect(r.path, '/transfers/a%2Fb');
      expect(r.headers['content-type'], 'application/json');
      expect(jsonDecode(utf8.decode(r.body!)), {'note': 'hi'});
      expect(r.maxRequestBytes, greaterThan(0));
      expect(r.maxResponseBytes, greaterThan(0));
      expect(r.timeout, greaterThan(Duration.zero));
    });

    test('an auth source hands the host token to the gateway for the exchange and refreshes once on 401 [SEC-026] [HST-010]', () async {
      final services = make();
      gateway.script.add(const DataResponse(401, null, 0));
      final json = await services.client.load(
        _source(auth: true),
        _caller,
        const {},
      );
      expect(json, {'ok': true});
      expect(gateway.calls.map((c) => c.userToken), [
        'host-token',
        'host-token-2',
      ]);
      expect(
        gateway.calls.first.headers.keys,
        isNot(contains('authorization')),
      );
    });

    test('a user token the identity provider rejects is refreshed once like a 401 [SEC-026]', () async {
      final services = make();
      gateway.script.add(
        const DataFailure(
          ActionErrorKind.http,
          PluxErrorCode.userTokenInvalid,
          'rejected',
        ),
      );
      await services.client.load(_source(auth: true), _caller, const {});
      expect(auth.refreshed, ['host-token-2']);
      expect(gateway.calls, hasLength(2));
    });

    test('a signed-out user fails an auth source before anything is sent [HST-010]', () async {
      auth.current = null;
      final services = make();
      await expectLater(
        services.client.load(_source(auth: true), _caller, const {}),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.dataUnauthorised,
          ),
        ),
      );
      expect(gateway.calls, isEmpty);
    });

    test('a direct source goes to the origin, within the plugin\'s domains [DAT-030] [SEC-030]', () async {
      final services = make();
      await services.client.load(
        _source(route: DataRoute.direct),
        _caller,
        const {},
      );
      expect(gateway.calls, isEmpty);
      expect(
        direct.single.url.toString(),
        'https://api.bank.example/v1/balance',
      );
    });

    test('a profile that refuses direct calls refuses a direct source at run time, reports it, and sends nothing [SEC-030]', () async {
      allowDirect = false;
      final services = make();
      await expectLater(
        services.client.load(
          _source(route: DataRoute.direct),
          _caller,
          const {},
        ),
        throwsA(
          isA<DataFailure>()
              .having(
                (f) => f.code,
                'code',
                PluxErrorCode.directDataSourceRefused,
              )
              .having((f) => f.kind, 'kind', ActionErrorKind.permission),
        ),
      );
      expect(direct, isEmpty);
      expect(gateway.calls, isEmpty);
      expect(reports.single.code, PluxErrorCode.directDataSourceRefused);
      expect(reports.single.details, containsPair('source', 'ledger'));
      // The gateway is still open to plux sources under that profile.
      await services.client.load(_source(), _caller, const {});
      expect(gateway.calls, hasLength(1));
    });

    test('a plux source without a gateway fails and never falls back to direct [SEC-030]', () async {
      final services = make(withGateway: false);
      await expectLater(
        services.client.load(_source(), _caller, const {}),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.dataSourceUnavailable,
          ),
        ),
      );
      expect(direct, isEmpty);
    });

    test('streams and file transfers of a plux source are refused, not sent direct [SEC-030]', () async {
      final services = make();
      await expectLater(
        services.client.streamRequest(
          _source(kind: DataKind.sse),
          _caller,
          const {},
        ),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.dataSourceUnavailable,
          ),
        ),
      );
      expect(direct, isEmpty);
    });

    test('a gateway failure keeps its registered code [SEC-031]', () async {
      final services = make();
      gateway.script.add(
        const DataFailure(
          ActionErrorKind.custom,
          PluxErrorCode.gatewayUpstreamUnknown,
          'unknown upstream',
        ),
      );
      await expectLater(
        services.client.load(_source(), _caller, const {}),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.gatewayUpstreamUnknown,
          ),
        ),
      );
    });

    test(
      'an upstream error status is an http failure with its status [DAT-001]',
      () async {
        final services = make();
        gateway.script.add(const DataResponse(503, null, 0));
        await expectLater(
          services.client.load(_source(), _caller, const {}),
          throwsA(isA<DataFailure>().having((f) => f.status, 'status', 503)),
        );
      },
    );
  });

  group('the outbox', () {
    test('replays through the gateway, each attempt with the same idempotency key [DAT-020] [SEC-030]', () async {
      final services = make(withKv: true);
      addTearDown(services.close);
      final spec = _source(offline: true);
      final s = scope(services, spec);
      services.setNetworkAvailable(false);
      expect(
        await s.callOperation('ledger.transfer', {'id': '1', 'x': 2}),
        isNull,
      );
      expect(services.outbox!.length, 1);
      expect(gateway.calls, isEmpty, reason: 'nothing leaves while offline');

      gateway.script.add(
        const DataFailure(
          ActionErrorKind.network,
          PluxErrorCode.dataNetworkFailed,
          'down',
        ),
      );
      services.setNetworkAvailable(true);
      await eventually(() => gateway.calls.isNotEmpty, 'the first replay');
      await eventually(() => sched.active.isNotEmpty, 'the backoff');
      sched.active.first.fire();
      await eventually(() => services.outbox!.length == 0, 'the second replay');
      final keys = [
        for (final c in gateway.calls)
          c.headers['Idempotency-Key'] ?? c.headers['idempotency-key'],
      ];
      expect(keys, hasLength(2));
      expect(keys[0], isNotNull);
      expect(keys[1], keys[0]);
      expect(direct, isEmpty);
    });

    test(
      'a transaction is never queued: its confirmation cannot wait [SEC-027]',
      () async {
        final services = make(withKv: true);
        addTearDown(services.close);
        final spec = _source(transaction: _mapping, offline: true);
        final s = scope(services, spec);
        services.setNetworkAvailable(false);
        gateway.script.add(
          const DataFailure(
            ActionErrorKind.network,
            PluxErrorCode.dataNetworkFailed,
            'down',
          ),
        );
        await expectLater(
          s.callOperation('ledger.transferFlat', {
            'amount': '5.00',
            'currency': 'EUR',
            'to': 'Ada',
          }),
          throwsA(isA<ActionError>()),
        );
        expect(services.outbox!.length, 0);
      },
    );
  });

  group('transactions', () {
    final input = {'amount': '12.50', 'currency': 'EUR', 'to': 'Ada'};

    test('the user confirms what the request body carries, and the confirmation travels with the call [SEC-027] [SEC-028] [SEC-032]', () async {
      final services = make();
      final spec = _source(transaction: _mapping);
      await services.client.call(
        spec,
        spec.operations['transferFlat']!,
        _caller,
        input,
      );
      final asked = authoriser.asked.single;
      expect(asked.dataSource, 'ledger');
      expect(asked.operation, 'transferFlat');
      expect(
        utf8.decode(asked.summary.jcs),
        '{"amount":"12.50","currency":"EUR","payee":"Ada"}',
      );
      final r = gateway.calls.single;
      expect(r.confirmation!.challengeId, 'ch-7');
      expect(r.confirmation!.signature, hasLength(64));
      expect(jsonDecode(utf8.decode(r.body!)), input);
    });

    test(
      'a read of a source with a mapping asks for nothing [SEC-028]',
      () async {
        final services = make();
        final spec = _source(transaction: _mapping);
        await services.client.call(
          spec,
          spec.operations['read']!,
          _caller,
          const {},
        );
        expect(authoriser.asked, isEmpty);
        expect(gateway.calls.single.confirmation, isNull);
      },
    );

    test('declining sends nothing and ends cancelled [SEC-027]', () async {
      authoriser.decline = true;
      final services = make();
      final spec = _source(transaction: _mapping);
      await expectLater(
        services.client.call(
          spec,
          spec.operations['transferFlat']!,
          _caller,
          input,
        ),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.kind,
            'kind',
            ActionErrorKind.cancelled,
          ),
        ),
      );
      expect(gateway.calls, isEmpty);
    });

    test('a mapped field that is not in the body (a path placeholder took it) fails before anything is asked [SEC-028]', () async {
      final services = make();
      final spec = _source(
        transaction: const TransactionMapping(
          amount: '/amount',
          currency: '/currency',
          payee: '/id',
        ),
      );
      await expectLater(
        services.client.call(spec, spec.operations['transfer']!, _caller, {
          ...input,
          'id': 'Ada',
        }),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.scaDynamicLinkingMismatch,
          ),
        ),
      );
      expect(gateway.calls, isEmpty);
    });

    test(
      'a refusal of the authoriser keeps its code and sends nothing [SEC-028]',
      () async {
        final services = DataServices(
          transport: ClientTransport(http.Client()),
          authDelegate: () => auth,
          environment: 'production',
          record: (name, {fields = const {}, route = '', pluginKey = ''}) {},
          report: reports.add,
          gateway: gateway,
          authoriser: _Refusing(),
        );
        final spec = _source(transaction: _mapping);
        await expectLater(
          services.client.call(
            spec,
            spec.operations['transferFlat']!,
            _caller,
            input,
          ),
          throwsA(
            isA<DataFailure>().having(
              (f) => f.code,
              'code',
              PluxErrorCode.scaDynamicLinkingMismatch,
            ),
          ),
        );
        expect(gateway.calls, isEmpty);
      },
    );

    test('a transaction on a direct source is refused: only the gateway carries it [SEC-032]', () async {
      final services = make();
      final spec = _source(route: DataRoute.direct, transaction: _mapping);
      await expectLater(
        services.client.call(
          spec,
          spec.operations['transferFlat']!,
          _caller,
          input,
        ),
        throwsA(
          isA<DataFailure>().having(
            (f) => f.code,
            'code',
            PluxErrorCode.transactionRequiresGateway,
          ),
        ),
      );
      expect(direct, isEmpty);
      expect(authoriser.asked, isEmpty);
    });

    test('the server\'s SCA refusals keep their codes and the call is not repeated [SEC-027] [SEC-028]', () async {
      for (final code in [
        PluxErrorCode.scaSignatureInvalid,
        PluxErrorCode.scaChallengeInvalid,
        PluxErrorCode.scaDynamicLinkingMismatch,
        PluxErrorCode.transactionRequiresGateway,
      ]) {
        gateway.calls.clear();
        gateway.script.add(DataFailure(ActionErrorKind.permission, code, 'no'));
        final services = make();
        final spec = _source(transaction: _mapping, auth: true);
        await expectLater(
          services.client.call(
            spec,
            spec.operations['transferFlat']!,
            _caller,
            input,
          ),
          throwsA(isA<DataFailure>().having((f) => f.code, 'code', code)),
        );
        expect(gateway.calls, hasLength(1));
      }
    });

    test('a 401 on a transaction is not retried: its confirmation is single use [SEC-032]', () async {
      gateway.script.add(const DataResponse(401, null, 0));
      final services = make();
      final spec = _source(transaction: _mapping, auth: true);
      await expectLater(
        services.client.call(
          spec,
          spec.operations['transferFlat']!,
          _caller,
          input,
        ),
        throwsA(isA<DataFailure>().having((f) => f.status, 'status', 401)),
      );
      expect(gateway.calls, hasLength(1));
      expect(auth.refreshed, isEmpty);
    });
  });
}

final class _Refusing implements TransactionAuthoriser {
  @override
  Future<Confirmation> authorise(TransactionRequest request) => Future.error(
    const PluxException(
      PluxErrorCode.scaDynamicLinkingMismatch,
      'not the confirmed transaction',
    ),
  );
}
