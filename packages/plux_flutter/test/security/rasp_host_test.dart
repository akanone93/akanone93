// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/core/runtime.dart';
import 'package:plux_flutter/src/state/providers.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';

import '../sync/fake_device.dart';
import '../sync/fake_server.dart';

/// Keeps the host it is attached to.
final class _Module implements PluxSecurityModule {
  PluxSecurityHost? host;
  var attached = 0;
  var detached = 0;

  @override
  String get name => 'plux_security_test';

  @override
  void attach(PluxSecurityHost host) {
    this.host = host;
    attached++;
  }

  @override
  void detach() => detached++;
}

http.Client _client() => http.Client();

Future<Uint8List?> Function(String) _reader(Map<String, Uint8List> files) =>
    (p) async => files[p];

Map<String, Object?> _fieldsOf(Map<String, Object?> e) =>
    jsonDecode(utf8.decode(base64.decode(e['fields']! as String)))
        as Map<String, Object?>;

void main() {
  final goldens = Goldens.load();
  late FakePluxServer server;
  late String dir;
  late _Module module;

  setUp(() async {
    server = await FakePluxServer.start();
    dir = Directory.systemTemp.createTempSync('plux_rasp').path;
    module = _Module();
  });
  tearDown(() async {
    await Plux.dispose();
    await server.close();
    Directory(dir).deleteSync(recursive: true);
  });

  Future<void> start(WidgetTester tester) async {
    await tester.runAsync(() async {
      server.release = null;
      server.assuranceLevel = 'AL3';
      await Plux.initializeWith(
        PluxConfig(
          appId: FakePluxServer.app,
          endpoint: server.endpoint,
          rootKeys: [
            PluxPublicKey(
              keyId: server.keyId,
              algorithm: 'ed25519',
              role: 'targets',
              publicKey: server.publicKey,
            ),
          ],
          storageDirectory: dir,
          httpClient: _client,
          securityModules: [module],
        ),
        RuntimeOverrides(
          credentials: MemoryCredentialStore.new,
          configSecrets: MemorySecretStore.new,
          deviceKeys: FakeDeviceKeys.new,
          attestation: FakeAttestation.new,
          baseline: _reader(
            await server.baseline(
              5,
              goldens.bundles['loan-calculator/demo.pxb']!,
              {'loans': goldens.bundles['loan-calculator/loans.pxb']!},
            ),
          ),
          healthyAfter: const Duration(hours: 1),
        ),
      );
      await Plux.sync();
    });
  }

  PluxRuntime rt() => Plux.container.read(pluxRuntimeProvider)!;

  testWidgets('a registered module is attached at initialize and detached at '
      'dispose [SEC-070]', (tester) async {
    await start(tester);
    expect(module.attached, 1);
    expect(module.host, isNotNull);
    expect(module.host!.settings.value, rt().settings.value);
    await tester.runAsync(Plux.dispose);
    expect(module.detached, 1);
  });

  testWidgets('a detection is sent at once as a necessary rasp_detection '
      'event with codes only [SEC-071]', (tester) async {
    await start(tester);
    module.host!.reportDetection(
      kind: 'root',
      severity: 'high',
      response: 'degrade',
      indicators: ['su_binary', 'test_keys'],
    );
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 300)),
    );
    final sent = server.events.where((e) => e['name'] == 'rasp_detection');
    expect(sent, hasLength(1));
    expect(_fieldsOf(sent.single), {
      'indicators': 'su_binary,test_keys',
      'kind': 'root',
      'response': 'degrade',
      'severity': 'high',
    });
  });

  testWidgets('limiting assurance lowers the level at once and no later '
      'token lifts it above the cap [SEC-071] [SEC-007]', (tester) async {
    await start(tester);
    expect(rt().assurance.value, 3);
    module.host!.limitAssurance(2);
    expect(rt().assurance.value, 2);
    await tester.runAsync(Plux.sync);
    expect(
      rt().assurance.value,
      2,
      reason: 'the server still names AL3; the cap holds',
    );
    module.host!.limitAssurance(3);
    await tester.runAsync(Plux.sync);
    expect(rt().assurance.value, 3, reason: 'without the cap the token rules');
  });

  testWidgets('requesting re-attestation offers the server fresh evidence '
      '[SEC-006]', (tester) async {
    await start(tester);
    expect(server.reattestations, 0);
    await tester.runAsync(() => module.host!.requestReattestation());
    expect(server.reattestations, 1);
  });

  testWidgets('after dispose the host does nothing [SEC-071]', (tester) async {
    await start(tester);
    final host = module.host!;
    await tester.runAsync(Plux.dispose);
    host.reportDetection(kind: 'root', severity: 'high', response: 'report');
    host.limitAssurance(0);
    await tester.runAsync(host.requestReattestation);
    expect(server.reattestations, 0);
  });
}
