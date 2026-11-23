// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:cryptography/dart.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/src/devtools_api/dev_pairing_link.dart';
import 'package:plux_flutter/src/devtools_api/dev_phone.dart';
import 'package:plux_flutter/src/security/device_keys.dart' show KeyPurpose;
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

final _ed = DartEd25519(sha512: const DartSha512());

/// The organisation's development root of the tests, and another key.
late SimpleKeyPair root;
late SimpleKeyPair stranger;
late Uint8List rootKey;
late DevSignInLink link;

/// The certificate [signer] gives an app's development key (ADR-0109).
Future<Map<String, Object?>> certificate({
  SimpleKeyPair? signer,
  String appId = 'a1',
  String environment = 'development',
  String environmentType = 'development',
  String keyId = 'k-dev',
}) async {
  final statement = utf8.encode(
    jsonEncode({
      'algorithm': 'ed25519',
      'appId': appId,
      'environment': environment,
      'environmentId': 'e1',
      'environmentType': environmentType,
      'keyId': keyId,
      'organizationId': 'o1',
      'publicKey': base64.encode(List<int>.filled(32, 7)),
      'role': 'targets',
      'type': 'plux.dev-key-certificate',
      'version': 1,
    }),
  );
  final sig = await _ed.sign(statement, keyPair: signer ?? root);
  return {
    'statement': base64.encode(statement),
    'signature': base64.encode(sig.bytes),
  };
}

void main() {
  late FakeDeviceKeys keys;
  late MemorySecretStore secrets;

  setUpAll(() async {
    root = await _ed.newKeyPairFromSeed(List<int>.filled(32, 1));
    stranger = await _ed.newKeyPairFromSeed(List<int>.filled(32, 2));
    rootKey = Uint8List.fromList((await root.extractPublicKey()).bytes);
    link = DevSignInLink(
      server: Uri.parse('https://plux.example.com'),
      grant: 'g1',
      developmentRoot: rootKey,
    );
  });

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
            'environment': 'development',
            'developmentKey': await certificate(),
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
    final joined = await phone.completeJoin(join, 58);
    expect(joined.pairingLink, 'plux-dev://pair?server=x&token=t');
    // The app's key, certified by the root the phone pinned (ADR-0109), is
    // trusted for this app and kept for it.
    expect(joined.developmentKey.keyId, 'k-dev');
    expect(joined.developmentKey.role, 'targets');
    expect((await phone.trustedKeyOf('a1'))?.keyId, 'k-dev');
    expect(await phone.trustedKeyOf('a2'), isNull);
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
      appId: 'a1',
      pairingLink: 'plux-dev://pair?server=x&token=t2',
      developmentKey: joined.developmentKey,
    );
    expect(again.needsNumber, isFalse);
    expect(
      (await phone.completeJoin(again, 0)).pairingLink,
      'plux-dev://pair?server=x&token=t2',
    );
  });

  group(
    'DEV-003 an app opens only with a key the root certifies (ADR-0109)',
    () {
      Future<DevPhone> joinWith(Map<String, Object?> key) async {
        final server = signedInServer(
          more: {
            'RequestJoin': (
              200,
              {
                'requestId': 'r1',
                'sessionId': 's1',
                'environment': 'development',
                'options': [31, 58, 64],
              },
            ),
            'CompleteJoin': (
              200,
              {
                'pairingLink': 'plux-dev://pair?server=x&token=t',
                'environment': 'development',
                'developmentKey': key,
              },
            ),
          },
        );
        return (await DevPhone.claim(
          link,
          client: server.client,
          name: 'Pixel 8',
          platform: 'android',
          keys: keys,
          secrets: secrets,
        )).complete(42);
      }

      for (final (name, key) in [
        ('signed by another key', () => certificate(signer: stranger)),
        ('for another app', () => certificate(appId: 'a2')),
        ('for production', () => certificate(environmentType: 'production')),
        ('for another environment', () => certificate(environment: 'staging')),
      ]) {
        test('refuses a key $name', () async {
          final phone = await joinWith(await key());
          final join = await phone.requestJoin('a1');
          await expectLater(
            phone.completeJoin(join, 58),
            throwsA(
              isA<DevPhoneException>().having(
                (e) => e.failure,
                'failure',
                DevPhoneFailure.untrusted,
              ),
            ),
          );
          expect(await phone.trustedKeyOf('a1'), isNull);
        });
      }

      test('a rejoin without a number checks the key it carries', () async {
        final server = signedInServer(
          more: {
            'RequestJoin': (
              200,
              {
                'sessionId': 's1',
                'environment': 'development',
                'pairingLink': 'plux-dev://pair?server=x&token=t',
                'developmentKey': await certificate(),
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
        expect(join.needsNumber, isFalse);
        expect(join.developmentKey?.keyId, 'k-dev');
        expect(
          (await phone.completeJoin(join, 0)).developmentKey.keyId,
          'k-dev',
        );
      });

      test('a phone connected before it had a root connects again', () async {
        await secrets.write(
          DevPhone.secretName,
          jsonEncode({
            'server': 'https://plux.example.com',
            'token': 'plux_phn_secret',
            'phoneId': 'p1',
          }),
        );
        await keys.create(DevPhone.keyAlias, KeyPurpose.dpop);
        final phone = await DevPhone.restore(
          client: signedInServer().client,
          keys: keys,
          secrets: secrets,
        );
        expect(phone?.account.developmentRoot, isNull);
        await expectLater(
          phone!.requestJoin('a1'),
          throwsA(
            isA<DevPhoneException>().having(
              (e) => e.failure,
              'failure',
              DevPhoneFailure.reconnect,
            ),
          ),
        );
      });
    },
  );

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
    late String rootText;
    setUpAll(() => rootText = base64Url.encode(rootKey).replaceAll('=', ''));

    test('holds an https server, a grant and the development root', () {
      final l = DevSignInLink.parse(
        'plux-dev://signin?server=https%3A%2F%2Fplux.example.com%2Fp&grant=abc&root=$rootText',
      );
      expect(l.server.toString(), 'https://plux.example.com/p');
      expect(l.grant, 'abc');
      expect(l.developmentRoot, rootKey);
      expect(l.toString(), isNot(contains('abc')));
      expect(DevSignInLink.matches('plux-dev://signin?x'), isTrue);
      expect(DevSignInLink.matches('plux-dev://pair?x'), isFalse);
    });

    test('refuses plain http except on a loopback address in a debug build', () {
      final insecure =
          'plux-dev://signin?server=http%3A%2F%2Fplux.example.com&grant=abc&root=$rootText';
      expect(
        () => DevSignInLink.parse(insecure, allowInsecureLoopback: true),
        throwsFormatException,
      );
      final loopback =
          'plux-dev://signin?server=http%3A%2F%2F127.0.0.1%3A8080&grant=abc&root=$rootText';
      expect(
        DevSignInLink.parse(loopback, allowInsecureLoopback: true).server.port,
        8080,
      );
      expect(
        () => DevSignInLink.parse(loopback, allowInsecureLoopback: false),
        throwsFormatException,
      );
    });

    test('refuses a link without a development root, or with a bad one', () {
      expect(
        () => DevSignInLink.parse(
          'plux-dev://signin?server=https%3A%2F%2Fa.b&grant=g',
        ),
        throwsFormatException,
      );
      expect(
        () => DevSignInLink.parse(
          'plux-dev://signin?server=https%3A%2F%2Fa.b&grant=g&root=AAAA',
        ),
        throwsFormatException,
      );
    });

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
