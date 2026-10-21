// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/core/runtime.dart';
import 'package:plux_flutter/src/render/renderer.dart';
import 'package:plux_flutter/src/state/providers.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';
import 'package:plux_flutter/src/verify/jcs.dart';

import '../sync/fake_device.dart';
import '../sync/fake_server.dart';

http.Client _client() => http.Client();

/// Runtime in reconstruction mode, against a fake server that must stay
/// untouched.
void main() {
  final goldens = Goldens.load();
  final widgets = goldens.bundles['widgets/widgets.pxb']!;
  final gallery = goldens.bundles['widgets/gallery.pxb']!;

  late FakePluxServer server;
  late String dir;
  final errors = <PluxException>[];

  setUp(() async {
    server = await FakePluxServer.start();
    dir = Directory.systemTemp.createTempSync('plux_rec').path;
    errors.clear();
  });
  tearDown(() async {
    await Plux.dispose();
    await server.close();
    Directory(dir).deleteSync(recursive: true);
  });

  /// A reconstruction of the widgets app and its gallery plugin, signed by
  /// the fake server's key unless [tweak] or [sign] change that.
  Future<PluxReconstruction> release({
    String id = 'rec_1',
    Map<String, Object?> Function(Map<String, Object?> manifest)? tweak,
    Map<String, Uint8List>? bundles,
    Uint8List Function(Uint8List canonical)? alter,
    bool canonical = true,
  }) async {
    final held = bundles ?? {'': widgets, 'gallery': gallery};
    final manifest = <String, Object?>{
      'type': 'reconstruction',
      'specVersion': 1,
      'id': id,
      'app': FakePluxServer.app,
      'environment': FakePluxServer.environment,
      'channel': 'audit',
      'issuedAt': '2026-10-21T10:00:00Z',
      'moment': '2026-10-20T09:30:15Z',
      'subject': {'kind': 'device', 'deviceId': 'dev_1'},
      'release': {'sequence': 7, 'channel': 'production'},
      'bundles': [
        for (final MapEntry(:key, :value) in held.entries)
          {
            'key': key,
            'version': 7,
            'hash': 'sha256:${Goldens.hashOf(value)}',
            'size': value.length,
          },
      ],
      'control': {
        'version': 0,
        'killSwitches': <Object?>[],
        'mandatoryUpdate': null,
        'forcedRollback': null,
      },
      'experiments': <Object?>[],
      'presentation': {
        'locale': 'en-GB',
        'deviceClass': 'phone',
        'theme': 'dark',
        'textScalePercent': 120,
      },
      'route': 'calendar',
    };
    final shown = tweak == null ? manifest : tweak(manifest);
    final text = canonical
        ? canonicalJson(shown)
        : const JsonEncoder.withIndent(' ').convert(shown);
    var signed = Uint8List.fromList(utf8.encode(text));
    final signature = base64.decode(await server.signHash(signed));
    if (alter != null) signed = alter(signed);
    return PluxReconstruction(
      id: id,
      manifest: signed,
      signatures: [
        PluxReconstructionSignature(
          keyId: server.keyId,
          algorithm: 'ed25519',
          signature: Uint8List.fromList(signature),
        ),
      ],
      bundles: [
        for (final MapEntry(:key, :value) in held.entries)
          PluxReconstructionBundle(key: key, bytes: value),
      ],
    );
  }

  Future<PluxStartup> start(
    WidgetTester tester,
    PluxReconstruction reconstruction, {
    List<PluxPublicKey>? keys,
  }) async {
    final startup = await tester.runAsync(
      () => Plux.initializeWith(
        PluxConfig(
          appId: FakePluxServer.app,
          endpoint: server.endpoint,
          rootKeys:
              keys ??
              [
                PluxPublicKey(
                  keyId: server.keyId,
                  algorithm: 'ed25519',
                  role: 'targets',
                  publicKey: server.publicKey,
                ),
              ],
          storageDirectory: dir,
          httpClient: _client,
          onError: (e, _) => errors.add(e),
          reconstruction: reconstruction,
        ),
        RuntimeOverrides(
          credentials: MemoryCredentialStore.new,
          deviceKeys: FakeDeviceKeys.new,
          attestation: FakeAttestation.new,
          configSecrets: MemorySecretStore.new,
          secrets: MemorySecretStore.new,
          healthyAfter: const Duration(hours: 1),
        ),
      ),
    );
    return startup!;
  }

  PluxRuntime rt() => Plux.container.read(pluxRuntimeProvider)!;

  Future<void> settle(WidgetTester tester) async {
    for (var i = 0; i < 5; i++) {
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 10)),
      );
      await tester.pump();
    }
  }

  Directory bundlesOf(String id) =>
      Directory('$dir/reconstruction/r-$id/objects/bundles');

  group('verifies before it loads [SEC-142] [SEC-052]', () {
    testWidgets('a signed release is installed and its moment is read', (
      tester,
    ) async {
      final startup = await start(tester, await release());
      expect(startup.ready, isTrue);
      expect(startup.sequence, 7);
      final moment = Plux.reconstruction!;
      expect(moment.id, 'rec_1');
      expect(moment.at, DateTime.utc(2026, 10, 20, 9, 30, 15));
      expect(moment.route, 'calendar');
      expect(moment.deviceId, 'dev_1');
      expect(moment.releaseSequence, 7);
      expect(bundlesOf('rec_1').listSync(), hasLength(2));
    });

    final cases = <(String, Future<PluxReconstruction> Function())>[
      (
        'a manifest changed after it was signed',
        () => release(
          alter: (b) => Uint8List.fromList(
            utf8.encode(utf8.decode(b).replaceFirst('calendar', 'calendaX')),
          ),
        ),
      ),
      (
        'a manifest that is not in canonical form',
        () => release(canonical: false),
      ),
      (
        'a manifest of another environment',
        () => release(tweak: (m) => {...m, 'environment': 'staging'}),
      ),
      (
        'a manifest of another app',
        () => release(tweak: (m) => {...m, 'app': 'app_other'}),
      ),
      (
        'a manifest on a channel devices follow',
        () => release(tweak: (m) => {...m, 'channel': 'production'}),
      ),
      (
        'a manifest that is not a reconstruction',
        () => release(tweak: (m) => {...m, 'type': 'manifest'}),
      ),
      (
        'a manifest that lacks the presentation',
        () => release(tweak: (m) => {...m}..remove('presentation')),
      ),
      (
        'a bundle that is not the one the manifest names',
        () async {
          final other = Uint8List.fromList(widgets)..[widgets.length - 1] ^= 1;
          final r = await release();
          return PluxReconstruction(
            id: r.id,
            manifest: r.manifest,
            signatures: r.signatures,
            bundles: [
              PluxReconstructionBundle(key: '', bytes: other),
              r.bundles.last,
            ],
          );
        },
      ),
      (
        'a plugin bundle that is not the one the manifest names',
        () async {
          final other = Uint8List.fromList(gallery)..[gallery.length - 1] ^= 1;
          final r = await release();
          return PluxReconstruction(
            id: r.id,
            manifest: r.manifest,
            signatures: r.signatures,
            bundles: [
              r.bundles.first,
              PluxReconstructionBundle(key: 'gallery', bytes: other),
            ],
          );
        },
      ),
      (
        'a bundle the manifest does not name',
        () async {
          final r = await release();
          return PluxReconstruction(
            id: r.id,
            manifest: r.manifest,
            signatures: r.signatures,
            bundles: [
              ...r.bundles,
              PluxReconstructionBundle(key: 'extra', bytes: widgets),
            ],
          );
        },
      ),
      (
        'a bundle that is missing',
        () async {
          final r = await release();
          return PluxReconstruction(
            id: r.id,
            manifest: r.manifest,
            signatures: r.signatures,
            bundles: [r.bundles.first],
          );
        },
      ),
      (
        'a release without signatures',
        () async {
          final r = await release();
          return PluxReconstruction(
            id: r.id,
            manifest: r.manifest,
            signatures: const [],
            bundles: r.bundles,
          );
        },
      ),
    ];
    for (final (name, make) in cases) {
      testWidgets('refuses $name and installs nothing', (tester) async {
        final r = await tester.runAsync(make);
        final startup = await start(tester, r!);
        expect(startup.ready, isFalse);
        expect(startup.error, isNotNull);
        expect(
          startup.error!.code,
          anyOf(
            PluxErrorCode.manifestSignatureInvalid,
            PluxErrorCode.sectionHashMismatch,
          ),
        );
        expect(Plux.reconstruction, isNull);
        expect(rt().active.value, isNull);
        // Nothing reached the store before the checks passed.
        expect(
          bundlesOf('rec_1').existsSync()
              ? bundlesOf('rec_1').listSync()
              : const <FileSystemEntity>[],
          isEmpty,
        );
      });
    }

    testWidgets('refuses a release signed by a key the app does not trust', (
      tester,
    ) async {
      final startup = await start(
        tester,
        await release(),
        keys: [
          PluxPublicKey(
            keyId: server.keyId,
            algorithm: 'ed25519',
            role: 'targets',
            publicKey: Uint8List(32),
          ),
        ],
      );
      expect(startup.ready, isFalse);
      expect(startup.error!.code, PluxErrorCode.manifestSignatureInvalid);
    });

    testWidgets('refuses a release when no keys are given', (tester) async {
      final startup = await start(tester, await release(), keys: const []);
      expect(startup.ready, isFalse);
      expect(startup.error!.code, PluxErrorCode.manifestSignatureInvalid);
    });

    testWidgets('keeps a hostile ID inside the reconstruction directory', (
      tester,
    ) async {
      final startup = await start(tester, await release(id: '..'));
      expect(startup.ready, isTrue);
      expect(Directory('$dir/reconstruction/r-..').existsSync(), isTrue);
      expect(Directory('$dir/reconstruction').listSync(), hasLength(1));
    });
  });

  group('shows the recorded moment [SEC-142]', () {
    Future<void> show(WidgetTester tester, PluxReconstruction r) async {
      tester.view.physicalSize = const Size(800, 2000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final startup = await start(tester, r);
      expect(startup.ready, isTrue);
      await tester.pumpWidget(
        const MaterialApp(
          debugShowCheckedModeBanner: false,
          home: Scaffold(body: PluxReconstructionView()),
        ),
      );
      await settle(tester);
    }

    testWidgets('opens the recorded route under a banner that names the '
        'moment and cannot be dismissed', (tester) async {
      await show(tester, await release());
      expect(find.text('March 2024'), findsOneWidget);
      final banner = find.byType(PluxReconstructionBanner);
      expect(banner, findsOneWidget);
      expect(
        find.descendant(
          of: banner,
          matching: find.textContaining('2026-10-20 09:30:15 UTC'),
        ),
        findsOneWidget,
      );
      expect(
        find.descendant(of: banner, matching: find.textContaining('release 7')),
        findsOneWidget,
      );
      // Nothing in the banner can be pressed.
      for (final t in [
        IconButton,
        TextButton,
        ElevatedButton,
        InkWell,
        GestureDetector,
        Dismissible,
      ]) {
        expect(
          find.descendant(of: banner, matching: find.byType(t)),
          findsNothing,
          reason: '$t in the banner',
        );
      }
    });

    testWidgets('applies the recorded locale, theme and text scale', (
      tester,
    ) async {
      await show(tester, await release());
      final env = Plux.container.read(environmentProvider);
      expect(env.locale, const Locale('en', 'GB'));
      expect(env.themeMode, ThemeMode.dark);
      final inside = tester.element(find.text('March 2024'));
      expect(MediaQuery.textScalerOf(inside).scale(10), closeTo(12, 1e-9));
    });

    testWidgets('lays a phone out at phone width on a wide screen', (
      tester,
    ) async {
      await show(tester, await release());
      final inside = tester.element(find.text('March 2024'));
      expect(MediaQuery.sizeOf(inside).width, 390);
    });

    testWidgets('lays a tablet out at tablet width on a narrow screen', (
      tester,
    ) async {
      tester.view.physicalSize = const Size(400, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final r = await release(
        tweak: (m) => {
          ...m,
          'presentation': {
            'locale': 'en-GB',
            'deviceClass': 'tablet',
            'textScalePercent': 100,
          },
        },
      );
      expect((await start(tester, r)).ready, isTrue);
      await tester.pumpWidget(
        const MaterialApp(
          debugShowCheckedModeBanner: false,
          home: Scaffold(body: PluxReconstructionView()),
        ),
      );
      await settle(tester);
      final inside = tester.element(find.text('March 2024'));
      expect(MediaQuery.sizeOf(inside).width, 834);
      expect(
        Plux.container.read(environmentProvider).themeMode,
        ThemeMode.system,
      );
    });

    testWidgets('says so when the device recorded no page', (tester) async {
      tester.view.physicalSize = const Size(800, 2000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final r = await release(tweak: (m) => {...m}..remove('route'));
      expect((await start(tester, r)).ready, isTrue);
      await tester.pumpWidget(
        const MaterialApp(home: Scaffold(body: PluxReconstructionView())),
      );
      expect(
        find.text('No page was recorded for this moment.'),
        findsOneWidget,
      );
      expect(find.byType(PluxReconstructionBanner), findsNothing);
    });

    testWidgets('says so when no reconstruction is open', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(home: Scaffold(body: PluxReconstructionView())),
      );
      expect(find.text('No reconstruction is open.'), findsOneWidget);
    });
  });

  group('reaches nothing beyond the page [SEC-142]', () {
    testWidgets('syncs nothing, checks no control document and sends no '
        'telemetry', (tester) async {
      await start(tester, await release());
      final runtime = rt();
      final result = await tester.runAsync(runtime.sync);
      expect(result!.outcome, SyncOutcome.upToDate);
      expect(result.sequence, 7);
      await tester.runAsync(() async {
        await runtime.checkControl();
        await runtime.flushTelemetry();
        expect(await runtime.handleSilentPush({'plux': 'control'}), isFalse);
      });
      expect(server.requests, isEmpty);
    });

    testWidgets('gives the page no data layer and gates its actions', (
      tester,
    ) async {
      await start(tester, await release());
      final renderer = rt().renderer! as PluxRenderer;
      expect(renderer.data, isNull);
      expect(renderer.actions!.reconstruction, isNotNull);
    });

    testWidgets('removes its store when the runtime is disposed', (
      tester,
    ) async {
      await start(tester, await release());
      expect(Directory('$dir/reconstruction/r-rec_1').existsSync(), isTrue);
      await tester.runAsync(Plux.dispose);
      expect(Directory('$dir/reconstruction/r-rec_1').existsSync(), isFalse);
    });

    testWidgets('starts afresh when the same release is opened again', (
      tester,
    ) async {
      await start(tester, await release());
      await tester.runAsync(Plux.dispose);
      Directory('$dir/reconstruction/r-rec_1').createSync(recursive: true);
      File('$dir/reconstruction/r-rec_1/stale').writeAsStringSync('x');
      final startup = await start(tester, await release());
      expect(startup.ready, isTrue);
      expect(File('$dir/reconstruction/r-rec_1/stale').existsSync(), isFalse);
    });
  });
}
