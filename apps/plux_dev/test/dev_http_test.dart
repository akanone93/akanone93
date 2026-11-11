// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev_http.dart';

// The runtime's test certificate: generated for each run, so that no
// private key is committed (test code only, as the router adapters share the
// runtime's delegate suite).
import '../../../packages/plux_flutter/test/support/test_certificate.dart';

void main() {
  late TestCertificate cert;
  late HttpServer server;

  setUpAll(() async => cert = await TestCertificate.generate());
  setUp(() async {
    server = await HttpServer.bindSecure('localhost', 0, cert.serverContext());
    server.listen((r) async {
      r.response.write('ok');
      await r.response.close();
    });
  });
  tearDown(() => server.close(force: true));

  Future<String> get(SecurityContext context) async {
    final client = HttpClient(context: context);
    addTearDown(client.close);
    final request = await client.getUrl(
      Uri.parse('https://localhost:${server.port}/'),
    );
    final response = await request.close();
    return response.transform(const SystemEncoding().decoder).join();
  }

  test(
    'DEV-001 the development authority the build carries is trusted',
    () async {
      expect(await get(devSecurityContext(cert.pem)), 'ok');
    },
  );

  test('DEV-001 the authority is trusted when its PEM arrives on one line, '
      'as the build settings file carries it', () async {
    // As scripts/dev-app-defines.sh writes it: no line break, no space.
    final oneLine = cert.pem.replaceAll(RegExp(r'\s'), '');
    expect(oneLine, startsWith('-----BEGINCERTIFICATE-----'));
    expect(await get(devSecurityContext(oneLine)), 'ok');
  });

  test('DEV-001 without one, only the system\'s authorities are', () async {
    await expectLater(
      get(devSecurityContext('')),
      throwsA(isA<HandshakeException>()),
    );
  });

  test('DEV-001 an authority that is not a PEM certificate fails at start, '
      'naming the define', () {
    expect(
      () => devSecurityContext('not a certificate'),
      throwsA(
        isA<FormatException>().having(
          (e) => e.message,
          'message',
          contains(devCertificateAuthorityDefine),
        ),
      ),
    );
  });

  test('DEV-001 a build without an authority starts', () {
    expect(checkDevCertificateAuthority, returnsNormally);
  });
}
