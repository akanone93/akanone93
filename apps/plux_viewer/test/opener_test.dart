// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_viewer/viewer.dart';

/// Opening a reconstruction in the runtime's reconstruction mode [SEC-142].
void main() {
  final server = Uri.parse('https://plux.example.org/');
  final manifest = utf8.encode(
    jsonEncode({'app': 'app_1', 'environment': 'production', 'type': 'x'}),
  );
  late Map<String, http.Response Function()> routes;
  late List<String> requested;
  late List<PluxConfig> started;
  late int stopped;

  ViewerApi api() => ViewerApi(
    server,
    token: 't',
    client: MockClient((request) async {
      requested.add(request.url.path);
      final route = routes[request.url.path];
      return route == null ? http.Response('', 404) : route();
    }),
  );

  ReconstructionOpener opener({String? refusal}) => ReconstructionOpener(
    api(),
    start: (config) async {
      started.add(config);
      return refusal;
    },
    stop: () async => stopped++,
  );

  http.Response json(Object body) => http.Response(jsonEncode(body), 200);

  setUp(() {
    requested = [];
    started = [];
    stopped = 0;
    routes = {
      '/plux.v1.AuditService/GetReconstruction': () => json({
        'manifest': base64.encode(manifest),
        'signatures': [
          {
            'keyId': 'k1',
            'algorithm': 'ed25519',
            'signature': base64.encode(Uint8List(64)),
          },
        ],
        'bundles': [
          {'key': '', 'sha256': 'sha256:aa', 'size': '3', 'url': '/o/app'},
          {'key': 'p', 'sha256': 'sha256:bb', 'size': '2', 'url': '/o/p'},
        ],
      }),
      '/plux.v1.ManifestService/GetRootKeys': () => json({
        'keys': [
          {
            'keyId': 'k1',
            'algorithm': 'ed25519',
            'role': 'targets',
            'publicKey': base64.encode(Uint8List(32)),
          },
          {
            'keyId': 'k0',
            'algorithm': 'ed25519',
            'role': 'root',
            'publicKey': base64.encode(Uint8List(32)),
          },
        ],
      }),
      '/o/app': () => http.Response.bytes([1, 2, 3], 200),
      '/o/p': () => http.Response.bytes([4, 5], 200),
    };
  });

  test('hands the runtime the release, the keys and the bundles', () async {
    final steps = <String>[];
    await opener().open('rec_1', progress: steps.add);
    final c = started.single;
    expect(c.appId, 'app_1');
    expect(c.environment, 'production');
    expect(c.channel, 'audit');
    expect(c.endpoint, server);
    expect(c.baseline, isNull);
    expect(c.rootKeys.map((k) => (k.keyId, k.role)), [('k1', 'targets')]);
    final r = c.reconstruction!;
    expect(r.id, 'rec_1');
    expect(r.manifest, manifest);
    expect(r.signatures.single.keyId, 'k1');
    expect(r.bundles.map((b) => b.key), ['', 'p']);
    expect(r.bundles[0].bytes, [1, 2, 3]);
    expect(r.bundles[1].bytes, [4, 5]);
    expect(steps.first, 'Fetching the reconstruction');
    expect(steps.last, 'Verifying');
  });

  test('trusts nothing it downloads: the runtime does the verifying', () async {
    // The viewer reads two fields of the unverified manifest, to know which
    // keys to ask for, and passes the rest on untouched.
    routes['/plux.v1.AuditService/GetReconstruction'] = () => json({
      'manifest': base64.encode(
        utf8.encode('{"app":"app_1","environment":"e","x":1}'),
      ),
      'bundles': <Object?>[],
    });
    await opener().open('rec_1');
    expect(
      utf8.decode(started.single.reconstruction!.manifest),
      '{"app":"app_1","environment":"e","x":1}',
    );
  });

  test('stops the runtime and reports why it refused the release', () async {
    await expectLater(
      opener(refusal: 'The reconstruction was refused: PLX-3001 bad')
          .open('rec_1'),
      throwsA(
        isA<OpenException>().having(
          (e) => e.message,
          'message',
          contains('PLX-3001'),
        ),
      ),
    );
    expect(stopped, 1);
  });

  test('says when the server has not signed the release yet', () async {
    routes['/plux.v1.AuditService/GetReconstruction'] = () =>
        json({'manifest': '', 'bundles': <Object?>[]});
    await expectLater(
      opener().open('rec_1'),
      throwsA(
        isA<OpenException>().having(
          (e) => e.message,
          'message',
          contains('not signed'),
        ),
      ),
    );
    expect(started, isEmpty);
    expect(requested, ['/plux.v1.AuditService/GetReconstruction']);
  });

  test('refuses a manifest it cannot read, before asking for keys', () async {
    routes['/plux.v1.AuditService/GetReconstruction'] = () => json({
      'manifest': base64.encode(utf8.encode('[]')),
      'bundles': <Object?>[],
    });
    await expectLater(opener().open('rec_1'), throwsA(isA<OpenException>()));
    expect(requested, ['/plux.v1.AuditService/GetReconstruction']);
  });

  test('needs a targets key to verify with', () async {
    routes['/plux.v1.ManifestService/GetRootKeys'] = () =>
        json({'keys': <Object?>[]});
    await expectLater(opener().open('rec_1'), throwsA(isA<OpenException>()));
    expect(started, isEmpty);
  });

  test('reports a server failure in its own words', () async {
    routes['/plux.v1.AuditService/GetReconstruction'] = () => http.Response(
      jsonEncode({'code': 'not_found', 'message': 'PLX-8502: not retained'}),
      404,
    );
    await expectLater(
      opener().open('rec_1'),
      throwsA(
        isA<OpenException>().having(
          (e) => e.message,
          'message',
          'PLX-8502: not retained',
        ),
      ),
    );
  });

  test('asks for a new sign-in when the token is refused', () async {
    routes['/plux.v1.AuditService/GetReconstruction'] = () => http.Response(
      jsonEncode({'code': 'unauthenticated', 'message': 'expired'}),
      401,
    );
    await expectLater(
      opener().open('rec_1'),
      throwsA(
        isA<OpenException>().having(
          (e) => e.message,
          'message',
          contains('Sign in again'),
        ),
      ),
    );
  });

  test('starts nothing when nobody waits any more', () async {
    await expectLater(
      opener().open('rec_1', cancelled: () => true),
      throwsA(isA<OpenException>()),
    );
    expect(started, isEmpty);
  });

  test('stops the runtime it started for somebody who left', () async {
    var left = false;
    final o = ReconstructionOpener(
      api(),
      start: (config) async {
        left = true;
        return null;
      },
      stop: () async => stopped++,
    );
    await expectLater(
      o.open('rec_1', cancelled: () => left),
      throwsA(isA<OpenException>()),
    );
    expect(stopped, 1);
  });

  test('waits for the previous run to end before it opens again', () async {
    final events = <String>[];
    final o = ReconstructionOpener(
      api(),
      start: (config) async {
        events.add('start');
        return null;
      },
      stop: () async {
        await Future<void>.delayed(const Duration(milliseconds: 20));
        events.add('stop');
      },
    );
    final closing = o.close();
    await o.open('rec_1');
    await closing;
    expect(events, ['stop', 'start']);
  });
}
