// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:cryptography/cryptography.dart' show SimpleKeyPair;
import 'package:cryptography/dart.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/core/active_release.dart';
import 'package:plux_flutter/src/devtools_api/dev_overlay.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/render/values.dart' show uuidString;
import 'package:plux_flutter/src/store/release_record.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/jcs.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

/// The device's share of the edit-to-device latency (NFR-030, DEV-010):
/// [DevOverlayApplier.apply] of a signed dev bundle that changes one page,
/// end to end on the host: manifest signature against the development root,
/// hashes of the bundles, the overlay store's write and the switch of the
/// active release, which re-renders the page. Budget: at most 150 ms at the
/// 95th percentile, of the 500 ms from the save to the changed page. The
/// signature is checked with pure-Dart Ed25519, an upper bound for a phone
/// that verifies natively. `make bench-devapply` runs it; without
/// `PLUX_BENCH_DEVAPPLY=1` it skips.
const _limits = VerifierLimits(maxDepth: 64, maxVisits: 1000000);
const _bundles = '../../schema/testdata/bundles';

final _ed = DartEd25519(sha512: const DartSha512());

Uint8List _read(String name) => File('$_bundles/$name').readAsBytesSync();

String _hash(Uint8List data) => hexEncode(Uint8List.sublistView(data, 16, 48));

String _pageId(Section s) {
  final b = ByteData.sublistView(s.id);
  return uuidString((b.getUint64(0), b.getUint64(8)));
}

/// The loans plugin with its first page replaced by a page of another
/// bundle: the same page ID, other bytes.
(Uint8List, String) _changedLoans() {
  final loans = BundleContainer.parse(_read('loan-calculator/loans.pxb'));
  final other = BundleContainer.parse(_read('state/notes.pxb'));
  final page = loans.ofKind(SectionKind.page).first;
  final donor = other.ofKind(SectionKind.page).first;
  final data = encodeBundle(loans.kind, [
    for (final s in loans.sections)
      identical(s, page) ? Section(s.kind, s.id, s.hash, donor.data) : s,
  ]);
  return (data, _pageId(page));
}

Future<(TrustedKey, SimpleKeyPair)> _key(String id) async {
  final pair = await _ed.newKeyPair();
  final pub = await pair.extractPublicKey();
  return (
    TrustedKey(
      keyId: id,
      algorithm: 'ed25519',
      role: 'targets',
      publicKey: Uint8List.fromList(pub.bytes),
    ),
    pair,
  );
}

Map<String, Object?> _ref(Uint8List data, Map<String, Object?> more) => {
  'hash': 'sha256:${_hash(data)}',
  'size': data.length,
  'requiredFeatures': <String>[],
  'minRuntime': '0.0.0',
  ...more,
};

/// A dev bundle: a manifest signed by [signer] (named [keyId]) and the
/// bundles in [supply].
Future<DevBundle> _bundle({
  required SimpleKeyPair signer,
  required String keyId,
  required int sequence,
  required Uint8List app,
  required Uint8List plugin,
  Set<Uint8List> supply = const {},
  bool session = true,
  Map<String, List<String>> incompatible = const {},
}) async {
  final signed = Uint8List.fromList(
    utf8.encode(
      canonicalJson({
        'type': 'manifest',
        'specVersion': 1,
        'role': 'targets',
        'app': 'demo',
        'environment': 'dev',
        'channel': 'main',
        'releaseSequence': sequence,
        'issuedAt': '2026-10-23T10:00:00Z',
        'expires': '2099-01-01T00:00:00Z',
        'appBundle': _ref(app, {}),
        'plugins': [
          _ref(plugin, {'key': 'loans', 'version': 2}),
        ],
        'control': {'killSwitches': <String>[]},
        if (session)
          'developmentSession': {'sessionId': 's1', 'sequenceSpace': 1},
      }),
    ),
  );
  final sig = await _ed.sign(signed, keyPair: signer);
  final envelope = utf8.encode(
    jsonEncode({
      'signed': base64.encode(signed),
      'signatures': [
        {'keyid': keyId, 'alg': 'ed25519', 'sig': base64.encode(sig.bytes)},
      ],
    }),
  );
  return DevBundle(
    sequence: sequence,
    manifest: Uint8List.fromList(envelope),
    sections: {for (final d in supply) 'sha256:${_hash(d)}': d},
    incompatibleScopes: incompatible,
  );
}

void main() {
  const warmUp = 10;
  const iterations = 200;
  const budgetMs = 150;

  test(
    'applies a one-page dev bundle within the device budget [NFR-030] [DEV-010]',
    () async {
      final dir = Directory.systemTemp.createTempSync('plux_dev_apply_bench');
      addTearDown(() => dir.deleteSync(recursive: true));
      final (devKey, devPair) = await _key('dev-1');
      final app = _read('loan-calculator/demo.pxb');
      final loans = _read('loan-calculator/loans.pxb');
      final (changed, _) = _changedLoans();

      final store = ReleaseStore.open(dir.path);
      final bundles = <RecordBundle>[];
      for (final (key, data) in [('', app), ('loans', loans)]) {
        store.writeObject(ObjectKind.bundles, _hash(data), data);
        bundles.add(
          RecordBundle(
            key: key,
            version: 1,
            hash: _hash(data),
            features:
                fbs.Meta(
                  BundleContainer.parse(data)
                      .ofKind(SectionKind.meta)
                      .single
                      .data,
                ).requiredFeatures ??
                const [],
          ),
        );
      }
      store.installBaseline(
        ReleaseRecord(
          sequence: 5,
          source: ReleaseSource.baseline,
          bundles: bundles,
        ),
      );
      ActiveRelease openProduction() {
        final s = ReleaseStore.open(dir.path);
        return ActiveRelease.open(s, s.record(s.pointer.active!)!, _limits);
      }

      final active = ValueNotifier<ActiveRelease?>(openProduction());
      final applier = DevOverlayApplier(
        storeRoot: dir.path,
        active: active,
        app: 'demo',
        environment: 'dev',
        channel: 'main',
        devRootKeys: [devKey],
        limits: _limits,
        supportsFeature: (_) => true,
        runtimeVersion: '99.0.0',
        reloadProduction: () async {
          final previous = active.value;
          active.value = openProduction();
          previous?.retire();
        },
        resetScopes: (page, scopes) async {},
      );

      // Each edit alternates the plugin between its two versions, so that
      // every apply changes exactly one page.
      final micros = <int>[];
      final w = Stopwatch();
      for (var i = 0; i < warmUp + iterations; i++) {
        final plugin = i.isEven ? changed : loans;
        final bundle = await _bundle(
          signer: devPair,
          keyId: 'dev-1',
          sequence: 7 + i,
          app: app,
          plugin: plugin,
          supply: {plugin},
        );
        w
          ..reset()
          ..start();
        final result = await applier.apply(bundle);
        w.stop();
        expect(result.error, isNull, reason: 'edit $i');
        expect(result.pagesApplied, hasLength(1), reason: 'edit $i');
        if (i >= warmUp) micros.add(w.elapsedMicroseconds);
      }
      micros.sort();
      double ms(double q) => double.parse(
        (micros[((micros.length - 1) * q).round()] / 1000).toStringAsFixed(3),
      );
      final p95 = ms(0.95);
      // ignore: avoid_print
      print(
        jsonEncode({
          'dev_apply': {
            'requirements': ['NFR-030', 'DEV-010'],
            'warm_up': warmUp,
            'iterations': iterations,
            'p50_ms': ms(0.5),
            'p95_ms': p95,
            'max_ms': ms(1),
            'budget_ms': budgetMs,
          },
        }),
      );
      expect(p95, lessThanOrEqualTo(budgetMs));
    },
    timeout: const Timeout(Duration(minutes: 5)),
    skip: Platform.environment['PLUX_BENCH_DEVAPPLY'] == null
        ? 'set PLUX_BENCH_DEVAPPLY=1, or run make bench-devapply'
        : false,
  );
}
