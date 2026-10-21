// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_viewer/viewer.dart';

/// The server calls of the viewer [SEC-142].
void main() {
  final server = Uri.parse('https://plux.example.org/');
  late List<http.BaseRequest> sent;
  late Map<String, http.Response Function(Map<String, Object?> body)> routes;

  ViewerApi api({String? token = 't0ken'}) => ViewerApi(
    server,
    token: token,
    client: MockClient.streaming((request, body) async {
      sent.add(request);
      final bytes = await body.toBytes();
      final path = request.url.path;
      final route = routes[path];
      final res = route == null
          ? http.Response('{"code":"not_found","message":"no route"}', 404)
          : route(
              bytes.isEmpty
                  ? const {}
                  : jsonDecode(utf8.decode(bytes)) as Map<String, Object?>,
            );
      return http.StreamedResponse(
        Stream.value(res.bodyBytes),
        res.statusCode,
        headers: res.headers,
      );
    }),
  );

  http.Response json(Object body, [int status = 200]) =>
      http.Response(jsonEncode(body), status);

  setUp(() {
    sent = [];
    routes = {};
  });

  group('every call is a Connect JSON call [SEC-142]', () {
    test(
      'lists the apps and their reconstructions with the bearer token',
      () async {
        routes['/plux.v1.AppService/ListApps'] = (_) => json({
          'apps': [
            {'id': 'app_1', 'name': 'Bank'},
          ],
        });
        routes['/plux.v1.AuditService/ListReconstructions'] = (b) => json({
          'reconstructions': [
            {
              'id': 'rec_1',
              'appId': b['appId'],
              'deviceId': 'dev_1',
              'moment': '2026-10-20T09:30:15Z',
              // int64 travels as a string in proto JSON.
              'releaseSequence': '7',
              'state': 'signed',
              'createdAt': '2026-10-21T10:00:00Z',
            },
          ],
        });
        final c = api();
        final apps = await c.listApps();
        final items = await c.listReconstructions(apps.single.id);
        expect(apps.single.name, 'Bank');
        expect(items.single.releaseSequence, 7);
        expect(items.single.moment, DateTime.utc(2026, 10, 20, 9, 30, 15));
        expect(items.single.openable, isTrue);
        expect(items.single.appId, 'app_1');
        for (final r in sent) {
          expect(r.headers['Authorization'], 'Bearer t0ken');
          expect(r.headers['Content-Type'], 'application/json');
          expect(r.headers['Connect-Protocol-Version'], '1');
          expect(r.method, 'POST');
        }
      },
    );

    test('opens a reconstruction and decodes its bytes', () async {
      routes['/plux.v1.AuditService/GetReconstruction'] = (b) => json({
        'reconstruction': {'id': b['id']},
        'manifest': base64.encode(utf8.encode('{"a":1}')),
        'signatures': [
          {
            'keyId': 'k1',
            'algorithm': 'ed25519',
            'signature': base64.encode([1, 2, 3]),
          },
        ],
        'bundles': [
          {'key': '', 'sha256': 'sha256:ab', 'size': '4', 'url': '/v1/o/ab'},
          {
            'key': 'p',
            'sha256': 'sha256:cd',
            'size': '5',
            'url': 'https://cdn.example/cd',
          },
        ],
      });
      final r = await api().getReconstruction('rec_1');
      expect(utf8.decode(r.manifest), '{"a":1}');
      expect(r.signatures.single.keyId, 'k1');
      expect(r.signatures.single.signature, [1, 2, 3]);
      expect(r.bundles.map((b) => (b.key, b.size, b.url)), [
        ('', 4, '/v1/o/ab'),
        ('p', 5, 'https://cdn.example/cd'),
      ]);
    });

    test('asks for the environment keys and keeps the targets keys', () async {
      routes['/plux.v1.ManifestService/GetRootKeys'] = (b) {
        expect(b['appId'], 'app_1');
        expect(b['environment'], 'production');
        return json({
          'keys': [
            for (final role in ['targets', 'root', 'targets'])
              {
                'keyId': 'k-$role',
                'algorithm': role == 'root' ? 'ed25519' : 'ed25519',
                'role': role,
                'publicKey': base64.encode(Uint8List(32)),
              },
            {
              'keyId': 'old',
              'algorithm': 'rsa',
              'role': 'targets',
              'publicKey': '',
            },
          ],
        });
      };
      final keys = await api().targetKeys('app_1', 'production');
      expect(keys.map((k) => k.keyId), ['k-targets', 'k-targets']);
      expect(keys.every((k) => k.role == 'targets'), isTrue);
    });

    test('maps an error response to its message', () async {
      routes['/plux.v1.AppService/ListApps'] = (_) => json({
        'code': 'permission_denied',
        'message': 'PLX-8030: permission denied',
      }, 403);
      await expectLater(
        api().listApps(),
        throwsA(
          isA<ViewerApiException>()
              .having((e) => e.status, 'status', 403)
              .having((e) => e.code, 'code', 'permission_denied')
              .having((e) => e.message, 'message', contains('PLX-8030'))
              .having((e) => e.unauthenticated, 'unauthenticated', isFalse),
        ),
      );
    });

    test('tells a refused token from other failures', () async {
      routes['/plux.v1.AppService/ListApps'] = (_) =>
          json({'code': 'unauthenticated', 'message': 'expired'}, 401);
      await expectLater(
        api().listApps(),
        throwsA(
          isA<ViewerApiException>().having(
            (e) => e.unauthenticated,
            'unauthenticated',
            isTrue,
          ),
        ),
      );
    });

    test('refuses to call without a token, sending nothing', () async {
      await expectLater(
        api(token: null).listApps(),
        throwsA(isA<ViewerApiException>()),
      );
      expect(sent, isEmpty);
    });

    test('a server that cannot be reached is an error, not a crash', () async {
      final c = ViewerApi(
        server,
        token: 't',
        client: MockClient((_) async => throw http.ClientException('down')),
      );
      await expectLater(c.listApps(), throwsA(isA<ViewerApiException>()));
    });
  });

  group('sign-in calls carry no credential [SEC-142]', () {
    test(
      'starts a device authorization for the viewer\'s scopes only',
      () async {
        routes['/plux.v1.IdentityService/StartDeviceAuthorization'] = (b) {
          expect(b['client'], 'plux-viewer');
          expect(b['scopes'], ['reconstruction.view', 'app.read']);
          return json({
            'deviceCode': 'dc',
            'userCode': 'ABCD-EFGH',
            'verificationUri': 'https://plux.example.org/device',
            'verificationUriComplete':
                'https://plux.example.org/device?code=ABCD-EFGH',
            'intervalSeconds': 5,
            'expiresAt': '2026-10-21T10:15:00Z',
          });
        };
        final g = await api(token: 'ignored').startDeviceAuthorization();
        expect(g.userCode, 'ABCD-EFGH');
        expect(g.interval, const Duration(seconds: 5));
        expect(g.expiresAt, DateTime.utc(2026, 10, 21, 10, 15));
        expect(sent.single.headers.containsKey('Authorization'), isFalse);
      },
    );

    test('reads a poll in each state', () async {
      for (final (status, expected) in [
        ('pending', PollStatus.pending),
        ('slow_down', PollStatus.slowDown),
        ('denied', PollStatus.denied),
        ('approved', PollStatus.approved),
        ('something new', PollStatus.pending),
      ]) {
        routes['/plux.v1.IdentityService/PollDeviceAuthorization'] = (_) =>
            json({'status': status, 'secret': 'plux_secret'});
        final p = await api(token: null).pollDeviceAuthorization('dc');
        expect(p.status, expected, reason: status);
        expect(
          p.token,
          expected == PollStatus.approved ? 'plux_secret' : isNull,
          reason: status,
        );
      }
      expect(
        sent.every((r) => !r.headers.containsKey('Authorization')),
        isTrue,
      );
    });
  });

  group(
    'bundles are downloaded without the token and within their size [SEC-142]',
    () {
      BundleInfo bundle({int size = 4, String url = '/v1/o/ab'}) =>
          BundleInfo(key: 'p', hash: 'sha256:ab', size: size, url: url);

      test('downloads a relative address from the server', () async {
        routes['/v1/o/ab'] = (_) => http.Response.bytes([1, 2, 3, 4], 200);
        expect(await api().downloadBundle(bundle()), [1, 2, 3, 4]);
        expect(sent.single.url, Uri.parse('https://plux.example.org/v1/o/ab'));
        expect(sent.single.headers.containsKey('Authorization'), isFalse);
        expect(sent.single.method, 'GET');
      });

      test('follows an address on another host over https', () async {
        routes['/cd'] = (_) => http.Response.bytes([9], 200);
        final b = await api().downloadBundle(
          bundle(size: 1, url: 'https://cdn.example/cd'),
        );
        expect(b, [9]);
        expect(sent.single.url.host, 'cdn.example');
        expect(sent.single.headers.containsKey('Authorization'), isFalse);
      });

      test('refuses an address over http on an https server', () async {
        await expectLater(
          api().downloadBundle(bundle(url: 'http://cdn.example/cd')),
          throwsA(isA<ViewerApiException>()),
        );
        expect(sent, isEmpty);
      });

      test('stops at more bytes than declared', () async {
        routes['/v1/o/ab'] = (_) => http.Response.bytes(List.filled(5, 1), 200);
        await expectLater(
          api().downloadBundle(bundle()),
          throwsA(
            isA<ViewerApiException>().having(
              (e) => e.message,
              'message',
              contains('larger'),
            ),
          ),
        );
      });

      test(
        'refuses a size beyond the registry\'s bound before fetching',
        () async {
          await expectLater(
            api().downloadBundle(bundle(size: 1 << 40)),
            throwsA(isA<ViewerApiException>()),
          );
          await expectLater(
            api().downloadBundle(bundle(size: -1)),
            throwsA(isA<ViewerApiException>()),
          );
          expect(sent, isEmpty);
        },
      );

      test('reports a failed download', () async {
        routes['/v1/o/ab'] = (_) => http.Response('', 404);
        await expectLater(
          api().downloadBundle(bundle()),
          throwsA(
            isA<ViewerApiException>().having((e) => e.status, 'status', 404),
          ),
        );
      });
    },
  );
}
