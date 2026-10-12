// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Verifies: SEC-050, SEC-051, QA-008.

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/verify/jcs.dart';
import 'package:plux_flutter/src/verify/manifest.dart';
import 'package:plux_flutter/src/verify/update_metadata.dart';

// Tamper and rollback attacks against the update metadata chain, run over
// the documents the server's own code signed (update_metadata_vectors.json):
// the same vectors as update_metadata_test.dart, assembled the way an
// attacker with a network position would.

final Map<String, Object?> _v = jsonDecode(
  File('test/verify/update_metadata_vectors.json').readAsStringSync(),
) as Map<String, Object?>;

Uint8List _bytes(String s) => Uint8List.fromList(utf8.encode(s));
Uint8List _doc(String name) => _bytes(_v[name]! as String);
final DateTime _now = DateTime.parse(_v['now']! as String);

/// One chain of the vectors: its three files.
typedef _Chain = ({
  Uint8List timestamp,
  Uint8List snapshot,
  Uint8List signed,
  List<DocumentSignature> sigs,
});

_Chain _chain(String name) {
  final c = _v[name]! as Map<String, Object?>;
  return (
    timestamp: _bytes(c['timestamp']! as String),
    snapshot: _bytes(c['snapshot']! as String),
    signed: base64.decode(c['manifestSigned']! as String),
    sigs: [
      for (final s
          in (c['manifestSignatures']! as List<Object?>)
              .cast<Map<String, Object?>>())
        DocumentSignature.fromJson(s),
    ],
  );
}

/// [doc] with the first character of its first signature changed.
Uint8List _flipped(Uint8List doc) {
  final j = jsonDecode(utf8.decode(doc)) as Map<String, Object?>;
  final first =
      (j['signatures']! as List<Object?>).first! as Map<String, Object?>;
  final sig = first['sig']! as String;
  first['sig'] = (sig.startsWith('A') ? 'B' : 'A') + sig.substring(1);
  return _bytes(canonicalJson(j));
}

/// [doc] without any signature.
Uint8List _unsigned(Uint8List doc) {
  final j = jsonDecode(utf8.decode(doc)) as Map<String, Object?>;
  j['signatures'] = <Object?>[];
  return _bytes(canonicalJson(j));
}

Matcher _fails(String fault, PluxErrorCode code) => throwsA(
  isA<PluxException>()
      .having((e) => e.details['fault'], 'fault', fault)
      .having((e) => e.code, 'code', code),
);

void main() {
  late RootMetadata root1;
  late Map<String, String> production;

  setUp(() async {
    root1 = await loadRoot(_doc('root1'));
    final ids = (_v['ids']! as Map<String, Object?>).values.expand(
      (l) => (l! as List<Object?>).cast<String>(),
    );
    production = {for (final id in ids) id: environmentProduction};
  });

  MetadataVerifier verifier({
    DateTime? now,
    RootMetadata? root,
    MetadataFloor floor = const MetadataFloor(),
  }) => MetadataVerifier(
    root: root ?? root1,
    now: now ?? _now,
    production: true,
    keyEnvironments: production,
    floor: floor,
  );

  /// Runs timestamp, snapshot and manifest in order, as sync does.
  Future<void> run(MetadataVerifier v, _Chain c) async {
    final ts = await v.timestamp(c.timestamp);
    final snap = await v.snapshot(c.snapshot, ts);
    await v.targets(c.signed, c.sigs, snap);
  }

  group('every online role refuses what an attacker can serve', () {
    final threshold = _fails(
      MetadataFault.threshold,
      PluxErrorCode.updateMetadataInvalid,
    );

    test('a flipped signature or none at all: timestamp and snapshot '
        '[SEC-050] [QA-008]', () async {
      final c = _chain('chain1');
      final v = verifier();
      await expectLater(v.timestamp(_flipped(c.timestamp)), threshold);
      await expectLater(v.timestamp(_unsigned(c.timestamp)), threshold);
      final ts = await v.timestamp(c.timestamp);
      // The timestamp hashes the snapshot's exact bytes, so any edit of the
      // file, its signatures included, is the chain breaking first.
      for (final tampered in [_flipped(c.snapshot), _unsigned(c.snapshot)]) {
        await expectLater(
          v.snapshot(tampered, ts),
          _fails(MetadataFault.mismatch, PluxErrorCode.updateMetadataInvalid),
        );
      }
    });

    test('mix and match: only the timestamp, snapshot and manifest of one '
        'chain verify together [SEC-050] [QA-008]', () async {
      final names = ['chain1', 'chain1Targets4'];
      for (final t in names) {
        for (final s in names) {
          for (final m in names) {
            final mixed = (
              timestamp: _chain(t).timestamp,
              snapshot: _chain(s).snapshot,
              signed: _chain(m).signed,
              sigs: _chain(m).sigs,
            );
            final whole = t == s && s == m;
            if (whole) {
              await run(verifier(), mixed);
            } else {
              await expectLater(
                run(verifier(), mixed),
                throwsA(isA<PluxException>()),
                reason: 'timestamp $t, snapshot $s, manifest $m',
              );
            }
          }
        }
      }
    });
  });

  group('freshness', () {
    test('the timestamp is good until the second it names and not after, '
        'so a frozen one cannot be served on [SEC-050] [QA-008]', () async {
      final c = _chain('chain1');
      final signed =
          (jsonDecode(utf8.decode(c.timestamp))
                  as Map<String, Object?>)['signed']!
              as Map<String, Object?>;
      final expires = DateTime.parse(signed['expires']! as String);
      await verifier(now: expires.subtract(const Duration(seconds: 1)))
          .timestamp(c.timestamp);
      for (final late in [
        expires,
        expires.add(const Duration(seconds: 1)),
        expires.add(const Duration(days: 365)),
      ]) {
        await expectLater(
          verifier(now: late).timestamp(c.timestamp),
          _fails(MetadataFault.expired, PluxErrorCode.updateMetadataInvalid),
          reason: '$late',
        );
      }
    });

    test('an older chain replayed after a newer one was trusted is a '
        'rollback in the role that differs [SEC-050] [QA-008]', () async {
      const trusted = MetadataFloor(timestamp: 9, snapshot: 3, targets: 5);
      await expectLater(
        run(verifier(floor: trusted), _chain('chain1Targets4')),
        _fails(MetadataFault.rollback, PluxErrorCode.rollbackRejected),
      );
      // The newer chain over the old floor is the normal update.
      await run(verifier(floor: trusted), _chain('chain1'));
      // The newest chain refuses once its versions are the floor and a
      // newer one has been seen.
      await expectLater(
        run(
          verifier(
            floor: const MetadataFloor(timestamp: 10, snapshot: 4, targets: 6),
          ),
          _chain('chain1'),
        ),
        _fails(MetadataFault.rollback, PluxErrorCode.rollbackRejected),
      );
    });
  });

  group('the root role', () {
    test('a tampered or unsigned root is refused as the first root '
        '[SEC-051] [QA-008]', () async {
      await expectLater(
        loadRoot(_flipped(_doc('root1'))),
        throwsA(isA<PluxException>()),
      );
      await expectLater(
        loadRoot(_unsigned(_doc('root1'))),
        throwsA(isA<PluxException>()),
      );
    });

    test('an old root replayed over a newer trusted one is a rollback '
        '[SEC-051] [QA-008]', () async {
      final root2 = await nextRoot(root1, _doc('root2'));
      await expectLater(
        nextRoot(root2, _doc('root2')),
        _fails(MetadataFault.rollback, PluxErrorCode.rollbackRejected),
      );
      await expectLater(
        nextRoot(root2, _doc('root1')),
        throwsA(isA<PluxException>()),
      );
    });

    test(
      'an unsigned next root fails the threshold [SEC-051] [QA-008]',
      () async {
        await expectLater(
          nextRoot(root1, _unsigned(_doc('root2'))),
          _fails(MetadataFault.threshold, PluxErrorCode.updateMetadataInvalid),
        );
      },
    );
  });
}
