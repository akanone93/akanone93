// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/attestation.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/downloader.dart';
import 'package:plux_flutter/src/sync/sync_event.dart';

import 'fake_device.dart';

void main() {
  test('the installed digest matches the server\'s vector [NFR-006]', () {
    final digest = installedDigest({'loans': 'bb' * 32, '': 'aa' * 32});
    expect(
      digest.map((b) => b.toRadixString(16).padLeft(2, '0')).join(),
      '8f0a15e796b7d9217c248acc882f04d173f350342a0733a60dd1e50d13deedc9',
    );
  });

  test('reports the OS version within what registration accepts, without '
      'kernel build details [SYN-001] [REL-080]', () {
    // Android's kernel string, as Platform.operatingSystemVersion gives it.
    expect(
      deviceOsVersion(
        'Linux 5.10.157-android13-4-00001-g5c7ff5dc7aac-ab10381520 '
        '#1 SMP PREEMPT Fri Jun 23 18:49:14 UTC 2023',
      ),
      'Linux 5.10.157-android13-4-00001-g5c7ff5dc7aac-ab10381520',
    );
    expect(
      deviceOsVersion('Version 18.1 (Build 22B83)'),
      'Version 18.1 (Build 22B83)',
    );
    final long = deviceOsVersion('Linux ${'9' * 100}');
    expect(long.length, maxDeviceField);
    expect(
      deviceOsVersion('Versión 18 ✓'),
      'Versin 18 ',
      reason: 'ASCII only, so the length is also the byte count',
    );
    expect(deviceOsVersion(''), '');
  });

  test('reads Retry-After as seconds or an HTTP date, capped [SYN-010]', () {
    final now = DateTime.utc(2026, 9, 28, 12);
    expect(parseRetryAfter('5', now), const Duration(seconds: 5));
    expect(
      parseRetryAfter('Mon, 28 Sep 2026 12:00:30 GMT', now),
      const Duration(seconds: 30),
    );
    expect(
      parseRetryAfter('Mon, 28 Sep 2026 11:00:00 GMT', now),
      Duration.zero,
    );
    expect(parseRetryAfter('99999', now), const Duration(minutes: 5));
    expect(parseRetryAfter('soon', now), isNull);
    expect(parseRetryAfter('Mon, 28 Foo 2026 12:00:30 GMT', now), isNull);
    expect(parseRetryAfter(null, now), isNull);
    expect(parseRetryAfter(' ', now), isNull);
  });

  test('maps Connect errors and knows which to retry', () async {
    final client = PluxApiClient(
      MockClient((req) async {
        expect(req.headers['connect-protocol-version'], '1');
        expect(
          req.url.path,
          '/base/plux.v1.DeviceService/CreateRegistrationChallenge',
        );
        return http.Response(
          jsonEncode({'code': 'permission_denied', 'message': 'no'}),
          403,
          headers: {'retry-after': '2'},
        );
      }),
      Uri.parse('https://plux.example/base'),
    );
    try {
      await client.registrationChallenge(appId: 'a', environment: 'e');
      fail('no error');
    } on ApiError catch (e) {
      expect(
        [e.status, e.code, e.message, e.retryAfter, e.retryable],
        [403, 'permission_denied', 'no', const Duration(seconds: 2), false],
      );
      final x = e.toException('token');
      expect(x.code, PluxErrorCode.syncFailed);
      expect(x.details['code'], 'permission_denied');
      expect(e.toString(), contains('403'));
    }
    for (final (status, code, retry) in [
      (0, 'unavailable', true),
      (408, 'x', true),
      (429, 'x', true),
      (500, 'x', true),
      (400, 'unavailable', true),
      (400, 'resource_exhausted', true),
      (404, 'not_found', false),
    ]) {
      expect(
        ApiError(status, code, '').retryable,
        retry,
        reason: '$status $code',
      );
    }
    final broken = PluxApiClient(
      MockClient((_) async => throw const SocketException('down')),
      Uri.parse('https://x/'),
    );
    await expectLater(
      broken.registrationChallenge(appId: 'a', environment: 'e'),
      throwsA(isA<ApiError>().having((e) => e.status, 'status', 0)),
    );
    final garbage = PluxApiClient(
      MockClient((_) async => http.Response('<html>', 502)),
      Uri.parse('https://x/'),
    );
    await expectLater(
      garbage.registrationChallenge(appId: 'a', environment: 'e'),
      throwsA(isA<ApiError>().having((e) => e.code, 'code', 'unknown')),
    );
    expect(
      DeviceCredential('d', 'jkt-value').toString(),
      isNot(contains('jkt-value')),
    );
  });

  test(
    'reports installs and reads a manifest without optional fields',
    () async {
      final seen = <String>[];
      final client = PluxApiClient(
        MockClient((req) async {
          seen.add(req.url.path);
          if (req.url.path.endsWith('GetManifest')) {
            return http.Response(
              jsonEncode({
                'etag': '"e"',
                'manifest': {
                  'signed': base64.encode([123, 125]),
                  'appBundle': {'sha256': 'sha256:00', 'size': 5},
                },
              }),
              200,
            );
          }
          return http.Response('{}', 200);
        }),
        Uri.parse('https://x'),
      );
      await client.reportInstalled(fakeToken('t'), 'd', 3);
      final m = await client.manifest(
        token: fakeToken('t'),
        appId: 'a',
        environment: 'e',
        channel: 'c',
        installedSequence: 0,
        installed: const {},
        ifNoneMatch: '',
      );
      expect(m.bundles.single.action, 'full');
      expect(m.bundles.single.stepUrl, isNull);
      expect(m.bundles.single.size, 5);
      expect(m.signatures, isEmpty);
      expect(seen, [
        '/plux.v1.DeviceService/ReportInstalled',
        '/plux.v1.ManifestService/GetManifest',
      ]);
    },
  );

  test('the downloader restarts after a range the server cannot serve and stops on a 404', () async {
    final dir = Directory.systemTemp.createTempSync('plux_dl');
    final part = '${dir.path}/x.part';
    var calls = 0;
    final client = MockClient.streaming((req, _) async {
      calls++;
      if (req.url.path == '/missing') {
        return http.StreamedResponse(const Stream.empty(), 404);
      }
      if (req.url.path == '/elsewhere') {
        return http.StreamedResponse(
          Stream.value([1, 2]),
          206,
          headers: {'content-range': 'bytes 5-6/7'},
        );
      }
      if (req.headers.containsKey('range') && calls == 1) {
        return http.StreamedResponse(const Stream.empty(), 416);
      }
      return http.StreamedResponse(
        Stream.value([1, 2, 3]),
        200,
        contentLength: 3,
      );
    });
    final sleeps = <Duration>[];
    final d = Downloader(
      client,
      sleep: (x) async => sleeps.add(x),
      random: Random(2),
      policy: const RetryPolicy(attempts: 3),
    );
    File(part).writeAsBytesSync([9, 9, 9, 9]);
    await d.fetch(Download(Uri.parse('https://x/object'), part));
    expect(File(part).readAsBytesSync(), [
      1,
      2,
      3,
    ], reason: 'a 416 restarts from zero');
    await expectLater(
      d.fetchAll([
        Download(Uri.parse('https://x/missing'), '${dir.path}/m.part'),
      ]),
      throwsA(isA<DownloadFailed>()),
    );
    File('${dir.path}/e.part').writeAsBytesSync([1]);
    await expectLater(
      d.fetch(Download(Uri.parse('https://x/elsewhere'), '${dir.path}/e.part')),
      throwsA(isA<DownloadFailed>()),
    );
    expect(
      DownloadFailed(Uri.parse('https://x/o'), 'x').toString(),
      contains('failed'),
    );
    dir.deleteSync(recursive: true);
  });

  test('a policy backs off exponentially with full jitter [SYN-010]', () {
    const p = RetryPolicy(
      base: Duration(milliseconds: 100),
      max: Duration(seconds: 1),
    );
    final r = Random(3);
    for (var attempt = 1; attempt < 8; attempt++) {
      final cap = min(1000, 100 * pow(2, attempt - 1));
      expect(p.backoff(attempt, r).inMilliseconds, lessThanOrEqualTo(cap));
    }
  });

  test('describes events and results', () {
    expect(const SyncDownloading(5, 10).progress, 0.5);
    expect(const SyncDownloading(5, 0).progress, 0);
    expect(const SyncChecking().toString(), 'SyncEvent.checking');
    expect(const SyncUpToDate(3).toString(), 'SyncEvent.upToDate(3)');
    expect(const SyncDownloading(1, 2).toString(), contains('1/2'));
    expect(const SyncActivated(4).toString(), 'SyncEvent.activated(4)');
    expect(const SyncRolledBack(4, 3).toString(), contains('4'));
    expect(
      const SyncFailed(PluxException(PluxErrorCode.syncFailed, 'x')).toString(),
      contains('PLX-3050'),
    );
    const r = SyncResult(
      outcome: SyncOutcome.staged,
      duration: Duration(seconds: 1),
      bytes: 30,
      fullBytes: 100,
      pluginsUpdated: 2,
    );
    expect(r.deltaRatio, 0.3);
    expect(
      const SyncResult(
        outcome: SyncOutcome.upToDate,
        duration: Duration.zero,
      ).deltaRatio,
      0,
    );
    expect(r.toTelemetry(), {
      'duration_ms': 1000,
      'bytes': 30,
      'delta_ratio': 0.3,
      'plugins_updated': 2,
      'outcome': 'staged',
    });
    const failed = SyncResult(
      outcome: SyncOutcome.failed,
      duration: Duration(milliseconds: 5),
      error: PluxException(PluxErrorCode.manifestExpired, 'x'),
    );
    expect(
      failed.toTelemetry()['reason'],
      'MANIFEST_EXPIRED',
      reason: 'failures by reason [SYN-015]',
    );
    expect(failed.toTelemetry()['code'], 'PLX-3002');
  });

  // Verifies: SEC-041.
  test(
    'a failed certificate pin is not mistaken for an outage [SEC-041]',
    () async {
      final client = PluxApiClient(
        MockClient(
          (_) async => throw const PluxException(
            PluxErrorCode.certificatePinMismatch,
            'pinned',
            details: {'host': 'plux.example'},
          ),
        ),
        Uri.parse('https://plux.example/'),
      );
      await expectLater(
        client.registrationChallenge(appId: 'a', environment: 'e'),
        throwsA(
          isA<PluxException>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.certificatePinMismatch,
          ),
        ),
      );
    },
  );

  test('[FN-020] invokes a server function with the idempotency key and reads its output or business error', () async {
    final bodies = <Map<String, Object?>>[];
    var business = false;
    final api = PluxApiClient(
      MockClient((request) async {
        expect(request.url.path, '/plux.v1.FunctionService/Invoke');
        expect(request.headers['Authorization'], startsWith('DPoP '));
        expect(request.headers['DPoP'], isNotEmpty);
        bodies.add(jsonDecode(request.body) as Map<String, Object?>);
        return http.Response(
          jsonEncode(
            business
                ? {
                    'businessError': {
                      'code': 'C',
                      'messageKey': 'k',
                      'params': base64.encode(utf8.encode('{"a":1}')),
                    },
                  }
                : {'output': base64.encode(utf8.encode('{"v":2}'))},
          ),
          200,
        );
      }),
      Uri.parse('https://plux.example/'),
    );
    final token = fakeToken('tok');
    final ok = await api.invokeFunction(
      token: token,
      function: 'fee.compute',
      version: 7,
      input: Uint8List.fromList(utf8.encode('{}')),
      idempotencyKey: 'key-1',
    );
    expect(utf8.decode(ok.output!), '{"v":2}');
    expect(bodies.single['function'], 'fee.compute');
    expect(bodies.single['version'], 7);
    expect(bodies.single['idempotencyKey'], 'key-1');
    expect(utf8.decode(base64.decode(bodies.single['input']! as String)), '{}');
    business = true;
    final failed = await api.invokeFunction(
      token: token,
      function: 'fee.compute',
      version: 7,
      input: Uint8List(0),
      idempotencyKey: 'key-1',
    );
    expect(failed.output, isNull);
    expect(failed.code, 'C');
    expect(failed.messageKey, 'k');
    expect(failed.params, {'a': 1});
  });

  test('[FN-020] the evidence of an invocation is bound to the function, '
      'version, input and the proof\'s jti, and a retry with a new proof '
      'asks again', () async {
    final bodies = <Map<String, Object?>>[];
    final proofs = <String>[];
    var calls = 0;
    final api = PluxApiClient(
      MockClient((request) async {
        bodies.add(jsonDecode(request.body) as Map<String, Object?>);
        proofs.add(request.headers['DPoP']!);
        if (calls++ == 0) {
          return http.Response(
            '{}',
            401,
            headers: {
              'www-authenticate': 'DPoP error="use_dpop_nonce"',
              'dpop-nonce': 'n-2',
            },
          );
        }
        return http.Response(jsonEncode({'output': ''}), 200);
      }),
      Uri.parse('https://plux.example/'),
    );
    final input = Uint8List.fromList(utf8.encode('{"amount":"5"}'));
    final bound = <Uint8List>[];
    await api.invokeFunction(
      token: fakeToken('tok'),
      function: 'pay.charge',
      version: 3,
      input: input,
      idempotencyKey: 'k',
      challengeId: 'ch-1',
      signature: Uint8List.fromList(List.filled(64, 7)),
      evidence: (binding) async {
        bound.add(binding);
        return InvocationEvidence(
          assertion: Uint8List.fromList([1, 2, 3]),
          integrityToken: 'token',
        );
      },
    );
    expect(bodies, hasLength(2));
    expect(bound, hasLength(2));
    for (var i = 0; i < 2; i++) {
      final payload = jsonDecode(
        utf8.decode(
          base64Url.decode(base64Url.normalize(proofs[i].split('.')[1])),
        ),
      ) as Map<String, Object?>;
      expect(
        bound[i],
        invocationBinding(
          function: 'pay.charge',
          version: 3,
          input: input,
          jti: payload['jti']! as String,
        ),
      );
      expect(bodies[i]['appAttestAssertion'], base64.encode([1, 2, 3]));
      expect(bodies[i]['playIntegrityToken'], 'token');
      expect(bodies[i]['transactionChallengeId'], 'ch-1');
      expect(
        base64.decode(bodies[i]['transactionSignature']! as String),
        List.filled(64, 7),
      );
    }
    expect(bound[0], isNot(bound[1]), reason: 'a new proof, a new binding');
  });

  test('[FN-020] a function that declares nothing sends no evidence or '
      'transaction', () async {
    late Map<String, Object?> body;
    final api = PluxApiClient(
      MockClient((request) async {
        body = jsonDecode(request.body) as Map<String, Object?>;
        return http.Response(jsonEncode({'output': ''}), 200);
      }),
      Uri.parse('https://plux.example/'),
    );
    await api.invokeFunction(
      token: fakeToken('tok'),
      function: 'fee.compute',
      version: 7,
      input: Uint8List(0),
      idempotencyKey: 'k',
    );
    expect(body.keys, isNot(contains('appAttestAssertion')));
    expect(body.keys, isNot(contains('playIntegrityToken')));
    expect(body.keys, isNot(contains('transactionChallengeId')));
    expect(body.keys, isNot(contains('transactionSignature')));
  });

  test('[SEC-027] a challenge for an invocation names the function, the '
      'version and the hash of the input', () async {
    late Map<String, Object?> body;
    final api = PluxApiClient(
      MockClient((request) async {
        expect(
          request.url.path,
          '/plux.v1.ScaService/CreateTransactionChallenge',
        );
        body = jsonDecode(request.body) as Map<String, Object?>;
        return http.Response(
          jsonEncode({
            'challengeId': 'c1',
            'challenge': base64.encode([1, 2, 3]),
            'expiresAt': '2100-01-01T00:00:00Z',
            'transactionSummaryJcs': base64.encode(utf8.encode('{}')),
          }),
          200,
        );
      }),
      Uri.parse('https://plux.example/'),
    );
    final c = await api.invocationChallenge(
      token: fakeToken('tok'),
      deviceId: 'dev',
      function: 'pay.charge',
      version: 3,
      inputSha256: Uint8List.fromList(List.filled(32, 9)),
      summaryJcs: Uint8List.fromList(utf8.encode('{}')),
    );
    expect(c.id, 'c1');
    expect(body['function'], 'pay.charge');
    expect(body['functionVersion'], 3);
    expect(base64.decode(body['inputSha256']! as String), List.filled(32, 9));
    expect(body.containsKey('dataSource'), isFalse);
    expect(body.containsKey('operation'), isFalse);
  });
}
