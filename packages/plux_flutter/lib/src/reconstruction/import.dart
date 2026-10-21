// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Verifies a reconstruction release and installs it into a release store
/// (SEC-142, SEC-051, SEC-052, ADR-0029, ADR-0082). The `targets` keys come
/// from the signed root chain the release carries, walked from the root keys
/// the host pinned, as in every sync; nothing the server says about keys is
/// trusted on its own. The order is the one of every release: the manifest's
/// canonical form and signature first, then its
/// fields, then each bundle against the hash the manifest names, its
/// structure, section hashes and the FlatBuffers verifier. Nothing is parsed
/// beyond the container header of a bundle, or beyond the signed bytes of
/// the manifest, before the step that vouches for it has passed. It runs on
/// the sync isolate (layering rule L-6).
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/reconstruction/reconstruction.dart';
import 'package:plux_flutter/src/store/release_record.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/sync/metadata_sync.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/jcs.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

/// The channel every reconstruction release is on; no device is served it.
const String reconstructionChannel = 'audit';

/// The largest reconstruction manifest accepted, in bytes.
const int _maxManifestBytes = 1 << 20;

/// Verifies [reconstruction] under the root [keys] the host pinned and
/// installs it into [store] as the active release, with no trial: it is
/// read-only and never replaced by a sync. [now] is the time the root chain
/// is checked at. Returns the moment its manifest describes. Throws
/// [PluxException] when anything fails verification, and when [keys] is
/// empty: a reconstruction is never shown unverified.
Future<PluxReconstructionMoment> importReconstruction({
  required PluxReconstruction reconstruction,
  required ReleaseStore store,
  required List<TrustedKey> keys,
  required DateTime now,
  required String appId,
  required String environment,
  required VerifierLimits limits,
  required bool Function(String feature) supportsFeature,
}) async {
  if (keys.isEmpty) {
    throw _invalid('no root key is pinned to verify the reconstruction with');
  }
  final targets = await verifiedTargetsKeys(
    pinned: keys,
    chain: reconstruction.roots,
    now: now,
  );
  final manifest = await _verifiedManifest(reconstruction, targets);
  if (manifest['app'] != appId ||
      manifest['environment'] != environment ||
      manifest['channel'] != reconstructionChannel ||
      manifest['id'] != reconstruction.id) {
    throw _invalid(
      'the reconstruction is for ${manifest['app']}/${manifest['environment']}'
      '/${manifest['channel']}',
    );
  }
  final moment = PluxReconstructionMoment.fromManifest(manifest);
  final bundles = _verifiedBundles(
    manifest,
    reconstruction,
    store,
    limits,
    supportsFeature,
  );
  store.installBaseline(
    ReleaseRecord(
      sequence: moment.releaseSequence,
      source: ReleaseSource.baseline,
      bundles: bundles,
    ),
  );
  store.collectGarbage();
  return moment;
}

/// The manifest's fields, read only after its canonical form and one
/// signature under a `targets` key have verified.
Future<Map<String, Object?>> _verifiedManifest(
  PluxReconstruction reconstruction,
  List<TrustedKey> keys,
) async {
  final signed = reconstruction.manifest;
  if (signed.isEmpty || signed.length > _maxManifestBytes) {
    throw _invalid('the reconstruction manifest is empty or too large');
  }
  if (!isCanonicalJson(signed)) {
    throw _invalid('the reconstruction manifest is not in canonical form');
  }
  final signatures = [
    for (final s in reconstruction.signatures)
      DocumentSignature(
        keyId: s.keyId,
        algorithm: s.algorithm,
        signature: s.signature,
      ),
  ];
  if (!await verifyDocumentSignature(signed, signatures, keys)) {
    throw _invalid(
      'no signature of the reconstruction verifies under a targets key',
    );
  }
  try {
    final json = jsonDecode(utf8.decode(signed)) as Map<String, Object?>;
    if (json['type'] != 'reconstruction' || json['specVersion'] != 1) {
      throw _invalid('not a version 1 reconstruction manifest');
    }
    return json;
  } on FormatException {
    throw _invalid('the reconstruction manifest is not JSON');
  } on TypeError {
    throw _invalid('the reconstruction manifest is not an object');
  }
}

/// Checks every bundle the manifest names, and no other, against its hash
/// and the verifier, stores it, and returns the release's bundles.
List<RecordBundle> _verifiedBundles(
  Map<String, Object?> manifest,
  PluxReconstruction reconstruction,
  ReleaseStore store,
  VerifierLimits limits,
  bool Function(String feature) supportsFeature,
) {
  final named = _namedBundles(manifest);
  final supplied = {for (final b in reconstruction.bundles) b.key: b.bytes};
  if (supplied.length != reconstruction.bundles.length ||
      supplied.length != named.length ||
      !supplied.keys.every(named.containsKey)) {
    throw _invalid('the bundles supplied are not the ones the manifest names');
  }
  // Every bundle is checked before the first is stored, so that a release
  // with one bad bundle leaves nothing behind.
  final out = <RecordBundle>[];
  for (final MapEntry(:key, value: ref) in named.entries) {
    final data = supplied[key]!;
    if (data.length != ref.size) {
      throw PluxException(
        PluxErrorCode.bundleMalformed,
        'bundle "$key" is ${data.length} bytes, not the ${ref.size} the '
        'manifest names',
      );
    }
    final container = verifyBundle(
      data,
      ref.hash,
      limits: limits,
      supportsFeature: supportsFeature,
    );
    out.add(
      RecordBundle(
        key: key,
        version: ref.version,
        hash: hexEncode(ref.hash),
        features:
            fbs.Meta(container.ofKind(SectionKind.meta).single.data)
                .requiredFeatures ??
            const [],
      ),
    );
  }
  if (out.where((b) => b.isApp).length != 1) {
    throw const PluxException(
      PluxErrorCode.bundleMalformed,
      'the reconstruction has no app bundle',
    );
  }
  for (final bundle in out) {
    if (!store.hasObject(ObjectKind.bundles, bundle.hash)) {
      store.writeObject(ObjectKind.bundles, bundle.hash, supplied[bundle.key]!);
    }
  }
  return out;
}

typedef _Named = ({Uint8List hash, int size, int version});

/// The bundles of the verified manifest by plugin key.
Map<String, _Named> _namedBundles(Map<String, Object?> manifest) {
  try {
    final out = <String, _Named>{};
    for (final b
        in (manifest['bundles']! as List<Object?>)
            .cast<Map<String, Object?>>()) {
      final ref = b['hash']! as String;
      if (!ref.startsWith('sha256:') || ref.length != 7 + 64) {
        throw _invalid('bundle hash $ref is not sha256:<64 hex digits>');
      }
      final key = b['key']! as String;
      final entry = (
        hash: hexDecode(ref.substring(7)),
        size: b['size']! as int,
        version: b['version']! as int,
      );
      if (out.containsKey(key)) {
        throw _invalid('the manifest names bundle "$key" twice');
      }
      out[key] = entry;
    }
    return out;
  } on TypeError {
    throw _invalid('a bundle of the reconstruction manifest is malformed');
  } on FormatException {
    throw _invalid('a bundle hash of the reconstruction manifest is not hex');
  }
}

PluxException _invalid(String message) =>
    PluxException(PluxErrorCode.manifestSignatureInvalid, message);
