// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/src/devtools_api/dev_pairing_link.dart';
import 'package:plux_flutter/src/devtools_api/dev_phone.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart' show MemorySecretStore;

import '../sync/fake_device.dart';

/// The JWK of a DPoP proof's header.
Map<String, Object?> jwkOf(String proof) {
  final header = proof.split('.').first;
  final json = utf8.decode(base64Url.decode(base64Url.normalize(header)));
  return (jsonDecode(json) as Map<String, Object?>)['jwk']!
      as Map<String, Object?>;
}

/// A server that answers the phone's calls from [answers], by procedure, and
/// records each request.
final class FakePhoneServer {
  FakePhoneServer(this.answers);

  /// The answer of each procedure: a body, or an error status and message.
  final Map<String, (int, Map<String, Object?>)> answers;

  /// The requests, in order.
  final List<http.Request> seen = [];

  http.Client client() => MockClient((req) async {
    seen.add(req);
    final method = req.url.pathSegments.last;
    final (status, body) =
        answers[method] ??
        (404, {'code': 'not_found', 'message': 'no such call'});
    return http.Response(
      jsonEncode(body),
      status,
      headers: {'content-type': 'application/json', 'dpop-nonce': 'n1'},
    );
  });
}

final link = DevSignInLink(
  server: Uri.parse('https://plux.example.com'),
  grant: 'g1',
);

void main() {
  late FakeDeviceKeys keys;
  late MemorySecretStore secrets;

  setUp(() {
    keys = FakeDeviceKeys();
    secrets = MemorySecretStore();
  });

  FakePhoneServer signedInServer({
    Map<String, (int, Map<String, Object?>)> more = const {},
  }) => FakePhoneServer({
    'ClaimPhoneSignIn': (
      200,
      {
        'options': [12, 42, 77],
        'expiresAt': '2026-11-11T12:01:00Z',
        'userDisplay': 'Ada',
        'organizationName': 'Acme',
      },
    ),
    'CompletePhoneSignIn': (
      200,
      {
        'token': 'plux_phn_secret',
        'phoneId': 'p1',
        'userDisplay': 'Ada',
        'organizationName': 'Acme',
      },
    ),
    ...more,
  });

  test('DEV-002 a phone signs in with the number Studio shows, with its own key, and keeps its account', () async {
    final server = signedInServer(
      more: {
        'ListPhoneApps': (
          200,
          {
            'apps': [
              {'id': 'a1', 'name': 'Hello', 'live': true},
              {'id': 'a2', 'name': 'Bank'},
            ],
          },
        ),
      },
    );
    final claim = await DevPhone.claim(
      link,
      client: server.client,
      name: 'Pixel 8',
      platform: 'android',
      keys: keys,
      secrets: secrets,
    );
    expect(claim.options, [12, 42, 77]);
    expect(claim.userDisplay, 'Ada');
    expect(keys.stored.keys, [DevPhone.keyAlias]);
    final claimed = server.seen.single;
    expect(claimed.headers['authorization'], isNull);
    expect(jsonDecode(claimed.body), {
      'grant': 'g1',
      'name': 'Pixel 8',
      'platform': 'android',
    });
    final phone = await claim.complete(42);
    expect(jsonDecode(server.seen[1].body), {'grant': 'g1', 'number': 42});
    expect(phone.account.phoneId, 'p1');
    // The token is kept by the secret store, which the platform encrypts.
    expect(secrets.values[DevPhone.secretName], contains('plux_phn_secret'));

    // Restored, it calls with its token and a proof of the same key.
    final restored = await DevPhone.restore(
      client: server.client,
      keys: keys,
      secrets: secrets,
    );
    expect(restored, isNotNull);
    final apps = await restored!.apps();
    expect(apps.map((a) => (a.id, a.name, a.live)), [
      ('a1', 'Hello', true),
      ('a2', 'Bank', false),
    ]);
    final call = server.seen.last;
    expect(call.headers['authorization'], 'DPoP plux_phn_secret');
    expect(jwkOf(call.headers['dpop']!), jwkOf(claimed.headers['dpop']!));
  });

  test('DEV-002 a wrong number ends the sign-in and keeps nothing', () async {
    final server = signedInServer(
      more: {
        'CompletePhoneSignIn': (
          403,
          {
            'code': 'permission_denied',
            'message': 'PLX-9210: that is not the number Studio showed',
          },
        ),
      },
    );
    final claim = await DevPhone.claim(
      link,
      client: server.client,
      name: 'Pixel 8',
      platform: 'android',
      keys: keys,
      secrets: secrets,
    );
    await expectLater(
      claim.complete(12),
      throwsA(
        isA<DevPhoneException>().having(
          (e) => e.failure,
          'failure',
          DevPhoneFailure.wrongNumber,
        ),
      ),
    );
    expect(secrets.values, isEmpty);
  });

  test(
    'DEV-002 a phone whose sign-in lapsed or was revoked is signed out',
    () async {
      final server = signedInServer(
        more: {
          'ListPhoneApps': (
            401,
            {
              'code': 'unauthenticated',
              'message': 'the credential is not valid; sign in again',
            },
          ),
        },
      );
      final claim = await DevPhone.claim(
        link,
        client: server.client,
        name: 'Pixel 8',
        platform: 'android',
        keys: keys,
        secrets: secrets,
      );
      final phone = await claim.complete(42);
      await expectLater(
        phone.apps(),
        throwsA(
          isA<DevPhoneException>().having(
            (e) => e.failure,
            'failure',
            DevPhoneFailure.signedOut,
          ),
        ),
      );
      expect(secrets.values, isEmpty);
      expect(
        await DevPhone.restore(
          client: server.client,
          keys: keys,
          secrets: secrets,
        ),
        isNull,
      );
    },
  );

  test('DEV-002 an account whose key is gone is forgotten', () async {
    final server = signedInServer();
    final claim = await DevPhone.claim(
      link,
      client: server.client,
      name: 'Pixel 8',
      platform: 'android',
      keys: keys,
      secrets: secrets,
    );
    await claim.complete(42);
    await keys.delete(DevPhone.keyAlias);
    expect(
      await DevPhone.restore(
        client: server.client,
        keys: keys,
        secrets: secrets,
      ),
      isNull,
    );
    expect(secrets.values, isEmpty);
  });

  test('DEV-002 joining asks for a number, or pairs at once when the phone joined before', () async {
    final server = signedInServer(
      more: {
        'RequestJoin': (
          200,
          {
            'requestId': 'r1',
            'sessionId': 's1',
            'environment': 'development',
            'options': [31, 58, 64],
            'expiresAt': '2026-11-11T12:01:00Z',
          },
        ),
        'CompleteJoin': (
          200,
          {
            'pairingLink': 'plux-dev://pair?server=x&token=t',
            'sessionId': 's1',
          },
        ),
      },
    );
    final phone = await (await DevPhone.claim(
      link,
      client: server.client,
      name: 'Pixel 8',
      platform: 'android',
      keys: keys,
      secrets: secrets,
    )).complete(42);
    final join = await phone.requestJoin('a1');
    expect(join.needsNumber, isTrue);
    expect(join.options, [31, 58, 64]);
    expect(join.environment, 'development');
    expect(
      await phone.completeJoin(join, 58),
      'plux-dev://pair?server=x&token=t',
    );
    expect(jsonDecode(server.seen.last.body), {
      'requestId': 'r1',
      'number': 58,
    });

    final again = DevJoin(
      requestId: '',
      sessionId: 's1',
      environment: 'development',
      options: const [],
      expiresAt: null,
      pairingLink: 'plux-dev://pair?server=x&token=t2',
    );
    expect(again.needsNumber, isFalse);
    expect(
      await phone.completeJoin(again, 0),
      'plux-dev://pair?server=x&token=t2',
    );
  });

  test('DEV-002 without a Studio session open, joining says so', () async {
    final server = signedInServer(
      more: {
        'RequestJoin': (
          412,
          {
            'code': 'failed_precondition',
            'message': 'PLX-9211: open the app in Studio, then choose it on the phone again',
          },
        ),
      },
    );
    final phone = await (await DevPhone.claim(
      link,
      client: server.client,
      name: 'Pixel 8',
      platform: 'android',
      keys: keys,
      secrets: secrets,
    )).complete(42);
    await expectLater(
      phone.requestJoin('a1'),
      throwsA(
        isA<DevPhoneException>().having(
          (e) => e.failure,
          'failure',
          DevPhoneFailure.noLiveSession,
        ),
      ),
    );
  });

  group('DEV-002 the sign-in link', () {
    test('holds an https server and a grant', () {
      final l = DevSignInLink.parse(
        'plux-dev://signin?server=https%3A%2F%2Fplux.example.com%2Fp&grant=abc',
      );
      expect(l.server.toString(), 'https://plux.example.com/p');
      expect(l.grant, 'abc');
      expect(l.toString(), isNot(contains('abc')));
      expect(DevSignInLink.matches('plux-dev://signin?x'), isTrue);
      expect(DevSignInLink.matches('plux-dev://pair?x'), isFalse);
    });

    test(
      'refuses plain http except on a loopback address in a debug build',
      () {
        const insecure =
            'plux-dev://signin?server=http%3A%2F%2Fplux.example.com&grant=abc';
        expect(
          () => DevSignInLink.parse(insecure, allowInsecureLoopback: true),
          throwsFormatException,
        );
        const loopback =
            'plux-dev://signin?server=http%3A%2F%2F127.0.0.1%3A8080&grant=abc';
        expect(
          DevSignInLink.parse(
            loopback,
            allowInsecureLoopback: true,
          ).server.port,
          8080,
        );
        expect(
          () => DevSignInLink.parse(loopback, allowInsecureLoopback: false),
          throwsFormatException,
        );
      },
    );

    test('refuses a pairing link, a missing grant and other schemes', () {
      expect(
        () => DevSignInLink.parse(
          'plux-dev://pair?server=https%3A%2F%2Fa.b&token=t',
        ),
        throwsFormatException,
      );
      expect(
        () => DevSignInLink.parse('plux-dev://signin?server=https%3A%2F%2Fa.b'),
        throwsFormatException,
      );
      expect(
        () => DevSignInLink.parse('https://a.b/signin?grant=g'),
        throwsFormatException,
      );
    });
  });
}
