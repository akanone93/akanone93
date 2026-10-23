// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Verification and hot apply of dev bundles (`DEV-010`, `SEC-052`,
/// `SEC-056`, ADR-0087, ADR-0089).
///
/// A dev bundle is a mini release: a manifest signed with a development key
/// and the bundles it names. [DevOverlayApplier] verifies it under the
/// development keys the host pinned, in the order of every release —
/// manifest signature first, then the fields, then each bundle against the
/// hash the manifest names — before it parses anything beyond a container
/// header (`SEC-052`). It writes the verified bundles to a **session
/// overlay**, a release store of its own at `<storeRoot>/dev-session` with
/// its own sequence space, and never to the production store. Then it opens
/// the overlay release as the active one, so the pages re-render, and resets
/// only the state scopes the compiler marked incompatible.
///
/// Wire form used here, since the frame carries opaque bytes:
///
/// - [DevBundle.manifest] is a UTF-8 JSON object
///   `{"signed": "(base64 of the canonical manifest)", "signatures":
///   [{"keyid", "alg", "sig"}]}`, the pair a release manifest also travels
///   as.
/// - [DevBundle.sections] maps `sha256:` plus hex bundle hashes to whole bundle
///   containers. A bundle the manifest names and the frame leaves out must
///   be one the previous release (the overlay, or the production release)
///   already holds: that is what makes the overlay one over the active
///   release.
///
/// The hashing and writing run on a background isolate (layering rule L-6).
/// Everything here sits behind [kPluxDevtools].
library;

import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:isolate';

import 'package:flutter/foundation.dart';
import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/control/live_control.dart';
import 'package:plux_flutter/src/core/active_release.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';
import 'package:plux_flutter/src/render/values.dart' show uuidString;
import 'package:plux_flutter/src/store/release_record.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/jcs.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

/// The directory of the session overlay inside the store root.
const String devOverlayDirectory = 'dev-session';

/// The largest dev manifest envelope accepted, in bytes.
const int _maxEnvelopeBytes = 2 << 20;

/// Resets the state [scopes] of the page [pageId] to the defaults of the
/// release now active.
typedef DevScopeReset = FutureOr<void> Function(
  String pageId,
  List<String> scopes,
);

/// Verifies dev bundles against the pinned development keys and applies
/// them, hot, to a session overlay (`DEV-010`).
final class DevOverlayApplier implements DevBundleApplier {
  /// Creates an applier over the store at [storeRoot], whose production
  /// release is shown through [active]. [devRootKeys] are the development
  /// keys the host pinned — never the production ones — and sign dev
  /// manifests directly with the `targets` role. [reloadProduction] opens
  /// the production store's active release into [active] again, as the
  /// runtime does after an activation; [resetScopes] resets state.
  DevOverlayApplier({
    required this.storeRoot,
    required this.active,
    required this.app,
    required this.environment,
    required this.channel,
    required this.devRootKeys,
    required this.limits,
    required this.supportsFeature,
    required this.runtimeVersion,
    required this.reloadProduction,
    required this.resetScopes,
    this.report,
    this.live,
    this.facts,
    DateTime Function()? clock,
  }) : _clock = clock ?? DateTime.now {
    if (!kPluxDevtools) {
      throw StateError('the dev channel is not compiled into this build');
    }
  }

  /// The production store's directory.
  final String storeRoot;

  /// The release pages render from, the runtime's.
  final ValueNotifier<ActiveRelease?> active;

  /// The configured app, environment and channel a dev manifest must name.
  final String app;

  /// See [app].
  final String environment;

  /// See [app].
  final String channel;

  /// The development keys the host pinned.
  final List<TrustedKey> devRootKeys;

  /// The verifier's limits from the registry (LIM-001).
  final VerifierLimits limits;

  /// Whether this runtime supports a required feature (BND-008).
  final bool Function(String feature) supportsFeature;

  /// This runtime's version.
  final String runtimeVersion;

  /// Re-opens the production store's active release into [active].
  final Future<void> Function() reloadProduction;

  /// Resets the state scopes of a page.
  final DevScopeReset resetScopes;

  /// Receives problems of the overlay release.
  final void Function(PluxException error)? report;

  /// The control document in force, for the overlay release.
  final LiveControl Function()? live;

  /// The targeting facts, for the overlay release.
  final TargetingFacts Function()? facts;

  final DateTime Function() _clock;

  String get _overlayRoot => '$storeRoot/$devOverlayDirectory';

  ActiveRelease? _production;
  ActiveRelease? _overlay;
  int _highest = 0;
  bool _started = false;
  Future<void> _tail = Future<void>.value();

  @override
  Future<DevApplyResult> apply(DevBundle bundle) {
    final done = Completer<DevApplyResult>();
    _tail = _tail.then((_) async {
      try {
        done.complete(await _apply(bundle));
      } on Object catch (e, s) {
        done.completeError(e, s);
      }
    });
    return done.future;
  }

  Future<DevApplyResult> _apply(DevBundle bundle) async {
    final job = _Job(
      manifest: bundle.manifest,
      sections: bundle.sections,
      keys: devRootKeys,
      app: app,
      environment: environment,
      channel: channel,
      now: _clock(),
      runtimeVersion: runtimeVersion,
      highest: _highest,
      sequence: bundle.sequence,
      limits: limits,
      sourceRoot: _started ? _overlayRoot : storeRoot,
      overlayRoot: _overlayRoot,
      fresh: !_started,
    );
    final _Staged staged;
    try {
      staged = await Isolate.run(() => _verifyAndWrite(job));
    } on PluxException catch (e) {
      return DevApplyResult(error: _refusal(e));
    }
    for (final f in staged.features) {
      if (!supportsFeature(f)) {
        _discardUnreferenced();
        return DevApplyResult(
          error: PluxException(
            PluxErrorCode.unsupportedRequiredFeature,
            'the dev bundle requires $f',
            details: {'feature': f},
          ),
        );
      }
    }
    final overlay = ReleaseStore.open(_overlayRoot)
      ..installBaseline(staged.record)
      ..collectGarbage();
    final ActiveRelease release;
    try {
      release = ActiveRelease.open(
        overlay,
        staged.record,
        limits,
        report: report,
        live: live,
        facts: facts,
      );
    } on PluxException catch (e) {
      return DevApplyResult(error: e);
    }
    _started = true;
    _highest = bundle.sequence;
    final previous = active.value;
    _production ??= previous;
    if (_overlay != null) _overlay!.retire();
    _overlay = release;
    active.value = release;
    final reset = <String>[];
    for (final MapEntry(key: page, value: scopes)
        in bundle.incompatibleScopes.entries) {
      if (scopes.isEmpty) continue;
      await resetScopes(page, scopes);
      reset.addAll([for (final s in scopes) '$page/$s']);
    }
    return DevApplyResult(
      pagesApplied: staged.pages,
      scopesReset: reset,
      error: reset.isEmpty
          ? null
          : PluxException(
              PluxErrorCode.hotApplyRefused,
              'state of ${reset.length} scope(s) was reset: its schema changed',
              details: {'scopes': reset.join(',')},
            ),
    );
  }

  @override
  Future<void> discard() async {
    await _tail;
    final production = _production;
    final overlay = _overlay;
    _production = null;
    _overlay = null;
    _started = false;
    _highest = 0;
    if (overlay != null) await reloadProduction();
    production?.retire();
    final dir = Directory(_overlayRoot);
    if (dir.existsSync()) dir.deleteSync(recursive: true);
  }

  void _discardUnreferenced() {
    if (Directory(_overlayRoot).existsSync()) {
      ReleaseStore.open(_overlayRoot).collectGarbage();
    }
  }

  /// A verification failure is `PLX-9230`; a lack of storage or of a
  /// feature keeps its own code.
  static PluxException _refusal(PluxException e) =>
      e.code == PluxErrorCode.diskQuotaExceeded ||
          e.code == PluxErrorCode.unsupportedRequiredFeature
      ? e
      : PluxException(
          PluxErrorCode.devBundleSignatureInvalid,
          e.message,
          details: {...e.details, 'cause': e.code.id},
        );
}

/// What the background isolate needs; every field can be sent.
final class _Job {
  const _Job({
    required this.manifest,
    required this.sections,
    required this.keys,
    required this.app,
    required this.environment,
    required this.channel,
    required this.now,
    required this.runtimeVersion,
    required this.highest,
    required this.sequence,
    required this.limits,
    required this.sourceRoot,
    required this.overlayRoot,
    required this.fresh,
  });

  final Uint8List manifest;
  final Map<String, Uint8List> sections;
  final List<TrustedKey> keys;
  final String app;
  final String environment;
  final String channel;
  final DateTime now;
  final String runtimeVersion;
  final int highest;
  final int sequence;
  final VerifierLimits limits;
  final String sourceRoot;
  final String overlayRoot;
  final bool fresh;
}

/// What the isolate verified and stored.
final class _Staged {
  const _Staged(this.record, this.features, this.pages);

  final ReleaseRecord record;
  final Set<String> features;
  final List<String> pages;
}

PluxException _invalid(String message) =>
    PluxException(PluxErrorCode.manifestSignatureInvalid, message);

/// Verifies the dev bundle in the order of ADR-0029, then writes the
/// overlay's objects. The overlay is touched only after everything has
/// verified.
Future<_Staged> _verifyAndWrite(_Job job) async {
  final features = <String>{};
  bool collect(String f) {
    features.add(f);
    return true;
  }

  final (signed, signatures) = _envelope(job.manifest);
  final manifest = await verifyManifest(
    signed,
    signatures,
    VerificationContext(
      keys: job.keys,
      app: job.app,
      environment: job.environment,
      channel: job.channel,
      now: job.now,
      highestAccepted: job.highest + 1,
      runtimeVersion: job.runtimeVersion,
      supportsFeature: collect,
      development: true,
    ),
  );
  if (manifest.releaseSequence != job.sequence) {
    throw _invalid(
      'the manifest is for sequence ${manifest.releaseSequence}, '
      'the frame for ${job.sequence}',
    );
  }
  final named = <String, (String, int, BundleRef)>{
    manifest.appBundle.hashRef: ('', 0, manifest.appBundle),
  };
  for (final p in manifest.plugins) {
    named[p.bundle.hashRef] = (p.key, p.version, p.bundle);
  }
  if (named.length != 1 + manifest.plugins.length) {
    throw _invalid('the manifest names a bundle twice');
  }
  for (final ref in job.sections.keys) {
    if (!named.containsKey(ref)) {
      throw const PluxException(
        PluxErrorCode.bundleMalformed,
        'the frame carries a bundle the manifest does not name',
      );
    }
  }

  final source = ReleaseStore.open(job.sourceRoot);
  final sourceActive = source.pointer.active;
  final sourceRecord = sourceActive == null
      ? null
      : source.record(sourceActive);
  final held = {
    for (final b in sourceRecord?.bundles ?? const <RecordBundle>[])
      'sha256:${b.hash}': b,
  };

  final bundles = <RecordBundle>[];
  final contents = <String, Uint8List>{};
  final pageHashes = <String, String>{};
  for (final MapEntry(key: ref, value: (key, version, b)) in named.entries) {
    final supplied = job.sections[ref];
    final Uint8List data;
    if (supplied != null) {
      data = supplied;
    } else if (held.containsKey(ref)) {
      data = File(source.objectPath(ObjectKind.bundles, held[ref]!.hash))
          .readAsBytesSync();
    } else {
      throw PluxException(
        PluxErrorCode.bundleMalformed,
        'bundle $ref is neither in the frame nor in the previous release',
      );
    }
    if (data.length != b.size) {
      throw PluxException(
        PluxErrorCode.bundleMalformed,
        'bundle $ref is ${data.length} bytes, not the ${b.size} the manifest '
        'names',
      );
    }
    final container = verifyBundle(
      data,
      b.hash,
      limits: job.limits,
      supportsFeature: collect,
    );
    contents[hexEncode(b.hash)] = data;
    bundles.add(
      RecordBundle(
        key: key,
        version: version,
        hash: hexEncode(b.hash),
        features:
            fbs.Meta(container.ofKind(SectionKind.meta).single.data)
                .requiredFeatures ??
            const [],
      ),
    );
    for (final s in container.ofKind(SectionKind.page)) {
      pageHashes[_pageId(s)] = hexEncode(s.hash);
    }
  }

  final before = <String, String>{};
  for (final b in sourceRecord?.bundles ?? const <RecordBundle>[]) {
    final file = File(source.objectPath(ObjectKind.bundles, b.hash));
    if (!file.existsSync()) continue;
    final c = BundleContainer.parse(file.readAsBytesSync());
    for (final s in c.ofKind(SectionKind.page)) {
      before[_pageId(s)] = hexEncode(s.hash);
    }
  }
  final pages = [
    for (final MapEntry(key: id, value: hash) in pageHashes.entries)
      if (before[id] != hash) id,
  ]..sort();

  // Everything verified: only now is the overlay touched.
  if (job.fresh && Directory(job.overlayRoot).existsSync()) {
    Directory(job.overlayRoot).deleteSync(recursive: true);
  }
  final overlay = ReleaseStore.open(job.overlayRoot);
  for (final MapEntry(key: hash, value: data) in contents.entries) {
    if (!overlay.hasObject(ObjectKind.bundles, hash)) {
      overlay.writeObject(ObjectKind.bundles, hash, data);
    }
  }
  final assets = sourceRecord?.assets ?? const <String>[];
  if (job.sourceRoot != job.overlayRoot) {
    for (final a in assets) {
      if (overlay.hasObject(ObjectKind.assets, a)) continue;
      final from = File(source.objectPath(ObjectKind.assets, a));
      if (from.existsSync()) {
        overlay.writeObject(ObjectKind.assets, a, from.readAsBytesSync());
      }
    }
  }
  return _Staged(
    ReleaseRecord(
      sequence: job.sequence,
      source: ReleaseSource.sync,
      bundles: bundles,
      assets: assets,
      signed: signed,
      signatures: [
        for (final s in signatures)
          {
            'keyid': s.keyId,
            'alg': s.algorithm,
            'sig': base64.encode(s.signature),
          },
      ],
      killSwitches: manifest.control.killSwitches,
      appKillSwitch: manifest.control.appKillSwitch,
      message: manifest.control.message,
    ),
    features,
    pages,
  );
}

String _pageId(Section s) {
  final b = ByteData.sublistView(s.id);
  return uuidString((b.getUint64(0), b.getUint64(8)));
}

/// The signed bytes and the signatures of the manifest envelope. Nothing
/// but the envelope is read before the signature has been checked.
(Uint8List, List<DocumentSignature>) _envelope(Uint8List manifest) {
  if (manifest.isEmpty || manifest.length > _maxEnvelopeBytes) {
    throw _invalid('the dev manifest is empty or too large');
  }
  try {
    final json = jsonDecode(utf8.decode(manifest)) as Map<String, Object?>;
    final signed = base64.decode(json['signed']! as String);
    final signatures = [
      for (final s
          in (json['signatures']! as List<Object?>)
              .cast<Map<String, Object?>>())
        DocumentSignature.fromJson(s),
    ];
    if (!isCanonicalJson(signed)) {
      throw _invalid('the signed manifest is not in canonical form');
    }
    return (signed, signatures);
  } on FormatException {
    throw _invalid('the dev manifest is not a signed envelope');
  } on TypeError {
    throw _invalid('the dev manifest is not a signed envelope');
  }
}
