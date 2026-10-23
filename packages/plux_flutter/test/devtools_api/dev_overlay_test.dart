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
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/render/values.dart' show uuidString;
import 'package:plux_flutter/src/store/release_record.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/jcs.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

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

Map<String, String> _files(String root) => {
  for (final f in Directory(root).listSync(recursive: true).whereType<File>())
    if (!f.path.startsWith('$root/dev-session'))
      f.path.substring(root.length): base64.encode(f.readAsBytesSync()),
};

Matcher _code(PluxErrorCode c) =>
    isA<PluxException>().having((e) => e.code, 'code', c);

void main() {
  late Directory dir;
  late ValueNotifier<ActiveRelease?> active;
  late List<(String, List<String>)> resets;
  late TrustedKey devKey;
  late SimpleKeyPair devPair;
  late Uint8List app;
  late Uint8List loans;

  ActiveRelease openProduction() {
    final store = ReleaseStore.open(dir.path);
    return ActiveRelease.open(
      store,
      store.record(store.pointer.active!)!,
      _limits,
    );
  }

  DevOverlayApplier applier({List<TrustedKey>? keys}) => DevOverlayApplier(
    storeRoot: dir.path,
    active: active,
    app: 'demo',
    environment: 'dev',
    channel: 'main',
    devRootKeys: keys ?? [devKey],
    limits: _limits,
    supportsFeature: (_) => true,
    runtimeVersion: '99.0.0',
    reloadProduction: () async {
      final previous = active.value;
      active.value = openProduction();
      previous?.retire();
    },
    resetScopes: (page, scopes) async => resets.add((page, scopes)),
  );

  setUp(() async {
    dir = Directory.systemTemp.createTempSync('plux_dev_overlay');
    resets = [];
    (devKey, devPair) = await _key('dev-1');
    app = _read('loan-calculator/demo.pxb');
    loans = _read('loan-calculator/loans.pxb');
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
    active = ValueNotifier(openProduction());
  });

  tearDown(() {
    dir.deleteSync(recursive: true);
  });

  test('a valid dev bundle is applied and switches the active release [DEV-010] [SEC-052]', () async {
    final (changed, pageId) = _changedLoans();
    final production = active.value!;
    final result = await applier().apply(
      await _bundle(
        signer: devPair,
        keyId: 'dev-1',
        sequence: 7,
        app: app,
        plugin: changed,
        supply: {changed},
      ),
    );
    expect(result.error, isNull);
    expect(result.pagesApplied, [pageId]);
    expect(result.scopesReset, isEmpty);
    expect(active.value, isNot(same(production)));
    expect(active.value!.sequence, 7);
    expect(active.value!.record.bundles.map((b) => b.hash).toSet(), {
      _hash(app),
      _hash(changed),
    });
    // The overlay holds the release in a store of its own.
    final overlay = ReleaseStore.open('${dir.path}/dev-session');
    expect(overlay.pointer.active, 7);
    expect(ReleaseStore.open(dir.path).pointer.active, 5);
  });

  test('a signature that does not verify is PLX-9230 and writes nothing [DEV-010] [SEC-052]', () async {
    final (changed, _) = _changedLoans();
    final (_, other) = await _key('dev-1');
    final production = active.value;
    final result = await applier().apply(
      await _bundle(
        signer: other,
        keyId: 'dev-1',
        sequence: 7,
        app: app,
        plugin: changed,
        supply: {changed},
      ),
    );
    expect(result.error, _code(PluxErrorCode.devBundleSignatureInvalid));
    expect(result.refused, isTrue);
    expect(active.value, same(production));
    expect(Directory('${dir.path}/dev-session').existsSync(), isFalse);
  });

  test('a bundle that does not match its signed hash is PLX-9230 and writes nothing [SEC-052]', () async {
    final (changed, _) = _changedLoans();
    final bundle = await _bundle(
      signer: devPair,
      keyId: 'dev-1',
      sequence: 7,
      app: app,
      plugin: changed,
      supply: {changed},
    );
    final tampered = Uint8List.fromList(changed);
    final page = BundleContainer.parse(tampered).ofKind(SectionKind.page).first;
    tampered[page.data.offsetInBytes + page.data.length ~/ 2] ^= 1;
    final result = await applier().apply(
      DevBundle(
        sequence: bundle.sequence,
        manifest: bundle.manifest,
        sections: {'sha256:${_hash(changed)}': tampered},
      ),
    );
    expect(result.error, _code(PluxErrorCode.devBundleSignatureInvalid));
    expect(Directory('${dir.path}/dev-session').existsSync(), isFalse);
  });

  test('production root keys do not verify a dev bundle [SEC-056]', () async {
    final (changed, _) = _changedLoans();
    final (production, _) = await _key('prod-1');
    final result = await applier(keys: [production]).apply(
      await _bundle(
        signer: devPair,
        keyId: 'dev-1',
        sequence: 7,
        app: app,
        plugin: changed,
        supply: {changed},
      ),
    );
    expect(result.error, _code(PluxErrorCode.devBundleSignatureInvalid));
    expect(Directory('${dir.path}/dev-session').existsSync(), isFalse);
  });

  test('a manifest without development_session is refused by the dev path [SEC-056]', () async {
    final (changed, _) = _changedLoans();
    final result = await applier().apply(
      await _bundle(
        signer: devPair,
        keyId: 'dev-1',
        sequence: 7,
        app: app,
        plugin: changed,
        supply: {changed},
        session: false,
      ),
    );
    expect(result.error, _code(PluxErrorCode.devBundleSignatureInvalid));
    expect(Directory('${dir.path}/dev-session').existsSync(), isFalse);
  });

  test('production verification refuses a manifest carrying development_session [SEC-056]', () async {
    final (changed, _) = _changedLoans();
    final bundle = await _bundle(
      signer: devPair,
      keyId: 'dev-1',
      sequence: 7,
      app: app,
      plugin: changed,
    );
    final envelope =
        jsonDecode(utf8.decode(bundle.manifest)) as Map<String, Object?>;
    final signed = base64.decode(envelope['signed']! as String);
    final sigs = [
      for (final s
          in (envelope['signatures']! as List<Object?>)
              .cast<Map<String, Object?>>())
        DocumentSignature.fromJson(s),
    ];
    VerificationContext context({required bool development}) =>
        VerificationContext(
          keys: [devKey],
          app: 'demo',
          environment: 'dev',
          channel: 'main',
          now: DateTime.utc(2026, 10, 23, 11),
          highestAccepted: 0,
          runtimeVersion: '99.0.0',
          supportsFeature: (_) => true,
          development: development,
        );
    // Signed and valid, yet a release context refuses it.
    await expectLater(
      verifyManifest(signed, sigs, context(development: false)),
      throwsA(_code(PluxErrorCode.manifestSignatureInvalid)),
    );
    final m = await verifyManifest(signed, sigs, context(development: true));
    expect(m.developmentSession?.sessionId, 's1');
    expect(m.developmentSession?.sequenceSpace, 1);
  });

  test('a replayed or older sequence is refused [SEC-055]', () async {
    final (changed, _) = _changedLoans();
    final a = applier();
    Future<DevApplyResult> push(int sequence) async => a.apply(
      await _bundle(
        signer: devPair,
        keyId: 'dev-1',
        sequence: sequence,
        app: app,
        plugin: changed,
        supply: {changed},
      ),
    );
    expect((await push(7)).error, isNull);
    expect((await push(7)).error, isNotNull);
    expect((await push(6)).error, isNotNull);
    expect((await push(8)).error, isNull);
    expect(active.value!.sequence, 8);
  });

  test('compatible state is kept and an incompatible scope is reset and reported [DEV-010]', () async {
    final (changed, pageId) = _changedLoans();
    final a = applier();
    final first = await a.apply(
      await _bundle(
        signer: devPair,
        keyId: 'dev-1',
        sequence: 7,
        app: app,
        plugin: changed,
        supply: {changed},
      ),
    );
    expect(first.scopesReset, isEmpty);
    expect(first.error, isNull);
    expect(resets, isEmpty);

    final second = await a.apply(
      await _bundle(
        signer: devPair,
        keyId: 'dev-1',
        sequence: 8,
        app: app,
        plugin: changed,
        incompatible: {
          pageId: ['page', 'session'],
        },
      ),
    );
    expect(second.refused, isFalse);
    expect(second.scopesReset, ['$pageId/page', '$pageId/session']);
    expect(second.error, _code(PluxErrorCode.hotApplyRefused));
    expect(resets.single.$1, pageId);
    expect(resets.single.$2, ['page', 'session']);
    // Nothing of the second bundle changed a page.
    expect(second.pagesApplied, isEmpty);
    expect(active.value!.sequence, 8);
  });

  test(
    'discard restores the production release and removes the overlay [DEV-010]',
    () async {
      final (changed, _) = _changedLoans();
      final a = applier();
      await a.apply(
        await _bundle(
          signer: devPair,
          keyId: 'dev-1',
          sequence: 7,
          app: app,
          plugin: changed,
          supply: {changed},
        ),
      );
      expect(active.value!.sequence, 7);
      await a.discard();
      expect(active.value!.sequence, 5);
      expect(Directory('${dir.path}/dev-session').existsSync(), isFalse);
    },
  );

  test(
    'the production store is not touched by apply and discard [SEC-056]',
    () async {
      final (changed, _) = _changedLoans();
      final before = _files(dir.path);
      final a = applier();
      await a.apply(
        await _bundle(
          signer: devPair,
          keyId: 'dev-1',
          sequence: 7,
          app: app,
          plugin: changed,
          supply: {changed},
        ),
      );
      expect(_files(dir.path), before);
      await a.discard();
      expect(_files(dir.path), before);
    },
  );
}
