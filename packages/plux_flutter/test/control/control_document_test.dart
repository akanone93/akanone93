// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/control/control_reader.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/update_metadata.dart';

import 'control_support.dart';

const _limits = VerifierLimits(maxDepth: 64, maxVisits: 1000000);

void main() {
  late ControlFixture control;
  setUp(() async => control = await ControlFixture.create());

  MetadataVerifier verifier({
    RootMetadata? root,
    DateTime? now,
    int floor = 0,
  }) => MetadataVerifier(
    root: root ?? control.root,
    now: now ?? controlNow,
    floor: MetadataFloor(control: floor),
  );

  Future<PluxException> refused(Uint8List file, {MetadataVerifier? by}) async {
    try {
      await (by ?? verifier()).control(file, controlScope);
    } on PluxException catch (e) {
      return e;
    }
    fail('the control document was accepted');
  }

  Future<ControlEnvelope> accepted(Uint8List file) =>
      verifier().control(file, controlScope);

  group('verification before parsing', () {
    test('accepts a document signed by the control role [SYN-060]', () async {
      final doc = control.document(
        switches: [killSwitch(fbs.SwitchScope.Plugin, 'loans')],
      );
      final envelope = await accepted(await control.file(doc));
      expect(envelope.version, 2);
      expect(envelope.channel, 'stable');
      expect(envelope.document, doc);
    });

    test(
      'refuses a tampered signed part with PLX-3100 [SYN-060] [SEC-052]',
      () async {
        final file = await control.file(
          control.document(),
          afterSigning: (signed) => signed['version'] = 99,
        );
        final e = await refused(file);
        expect(e.code, PluxErrorCode.controlSignatureInvalid);
        expect(metadataFault(e), MetadataFault.threshold);
      },
    );

    test(
      'refuses a document signed by another key with PLX-3100 [SYN-060]',
      () async {
        final other = await ControlFixture.create();
        final file = await control.file(control.document(), signer: other);
        expect(
          (await refused(file)).code,
          PluxErrorCode.controlSignatureInvalid,
        );
      },
    );

    test('refuses a key of another role with PLX-3100 [SYN-060]', () async {
      final asTargets = rootFromKeys([
        control.trusted(),
        control.trusted('targets'),
      ]);
      final onlyTargets = rootFromKeys([control.trusted('targets')]);
      final file = await control.file(control.document());
      expect(
        (await refused(file, by: verifier(root: onlyTargets))).code,
        PluxErrorCode.controlSignatureInvalid,
        reason: 'a root that delegates no control role accepts none',
      );
      expect(
        (await verifier(root: asTargets).control(file, controlScope)).version,
        2,
      );
    });

    test('refuses an expired document with PLX-3101 [SYN-060]', () async {
      final expires = controlNow.subtract(const Duration(seconds: 1));
      final file = await control.file(
        control.document(expires: expires),
        expires: expires,
      );
      expect((await refused(file)).code, PluxErrorCode.controlDocumentExpired);
    });

    test('refuses an older version with PLX-3102, and accepts the same one '
        '[SYN-060]', () async {
      final file = await control.file(control.document(version: 4), version: 4);
      expect(
        (await refused(file, by: verifier(floor: 5))).code,
        PluxErrorCode.controlRollbackRejected,
      );
      final same = await verifier(floor: 4).control(file, controlScope);
      expect(same.version, 4);
    });

    test(
      'refuses a document for another channel with PLX-3100 [SYN-060]',
      () async {
        final file = await control.file(
          control.document(channel: 'beta'),
          channel: 'beta',
        );
        expect(
          (await refused(file)).code,
          PluxErrorCode.controlSignatureInvalid,
        );
      },
    );

    test(
      'checks the signature before it looks at the fields [SEC-052]',
      () async {
        // The signed part is malformed (no document) and unsigned: the
        // failure is the signature, not the format.
        final file = await control.file(
          control.document(),
          signer: await ControlFixture.create(),
          afterSigning: (signed) => signed.remove('document'),
        );
        final e = await refused(file);
        expect(metadataFault(e), MetadataFault.threshold);
      },
    );
  });

  group('reading the document', () {
    Future<ControlSnapshot> read(Uint8List doc) async {
      final envelope = await accepted(await control.file(doc));
      return readControlDocument(envelope, _limits);
    }

    test('reads switches, the mandatory update and the rollback [REL-060] '
        '[REL-070]', () async {
      final snapshot = await read(
        control.document(
          switches: [
            killSwitch(fbs.SwitchScope.Plugin, 'loans'),
            killSwitch(fbs.SwitchScope.Page, 'p-1'),
            killSwitch(fbs.SwitchScope.ActionGraph, 'g-1'),
            killSwitch(fbs.SwitchScope.DataSource, 'd-1'),
            killSwitch(fbs.SwitchScope.$Function, 'orders.total'),
          ],
          mandatory: fbs.MandatoryUpdateObjectBuilder(
            releaseId: 'rel-9',
            releaseSequence: 9,
            messageKey: 'update.now',
          ),
          rollback: fbs.ForcedRollbackObjectBuilder(
            releaseId: 'rel-3',
            releaseSequence: 3,
          ),
        ),
      );
      for (final (kind, target) in [
        (SwitchKind.plugin, 'loans'),
        (SwitchKind.page, 'p-1'),
        (SwitchKind.actionGraph, 'g-1'),
        (SwitchKind.dataSource, 'd-1'),
        (SwitchKind.function, 'orders.total'),
      ]) {
        expect(snapshot.isOff(kind, target), isTrue, reason: '$kind');
      }
      expect(snapshot.isOff(SwitchKind.plugin, 'p-1'), isFalse);
      expect(snapshot.mandatory!.releaseSequence, 9);
      expect(snapshot.mandatory!.messageKey, 'update.now');
      expect(snapshot.rollback!.releaseSequence, 3);
    });

    test(
      'a switch with a targeting rule applies to everyone [REL-060]',
      () async {
        final snapshot = await read(
          control.document(
            switches: [killSwitch(fbs.SwitchScope.Plugin, 'x', withRule: true)],
          ),
        );
        expect(snapshot.isOff(SwitchKind.plugin, 'x'), isTrue);
      },
    );

    test('survives its stored form [SYN-060]', () async {
      final snapshot = await read(
        control.document(
          switches: [killSwitch(fbs.SwitchScope.Plugin, 'loans')],
          rollback: fbs.ForcedRollbackObjectBuilder(
            releaseId: 'r',
            releaseSequence: 3,
          ),
        ),
      );
      final back = ControlSnapshot.fromJson(snapshot.toJson())!;
      expect(back.version, snapshot.version);
      expect(back.expiresAt, snapshot.expiresAt);
      expect(back.isOff(SwitchKind.plugin, 'loans'), isTrue);
      expect(back.rollback!.releaseSequence, 3);
      expect(back.switchesChangedFrom(null), 1);
      expect(back.switchesChangedFrom(snapshot), 0);
    });

    test('refuses a signed buffer that is not FlatBuffers with PLX-3100 '
        '[SEC-052]', () async {
      final garbage = Uint8List.fromList(List.filled(64, 0xff));
      final envelope = await accepted(await control.file(garbage));
      expect(
        () => readControlDocument(envelope, _limits),
        throwsA(
          isA<PluxException>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.controlSignatureInvalid,
          ),
        ),
      );
    });

    test('refuses a truncated buffer that has the right identifier '
        '[SEC-052]', () async {
      final whole = control.document(
        switches: [killSwitch(fbs.SwitchScope.Plugin, 'loans')],
      );
      final cut = Uint8List.sublistView(whole, 0, whole.length ~/ 2);
      final envelope = await accepted(await control.file(cut));
      expect(
        () => readControlDocument(envelope, _limits),
        throwsA(isA<PluxException>()),
      );
    });

    test('refuses a document that disagrees with its signed header '
        '[SEC-052]', () async {
      final doc = control.document(version: 7);
      final envelope = await accepted(await control.file(doc));
      expect(
        () => readControlDocument(envelope, _limits),
        throwsA(
          isA<PluxException>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.controlSignatureInvalid,
          ),
        ),
      );
    });

    test('the generated layout accepts what the generated builder '
        'writes [SEC-052] [SYN-060]', () async {
      final doc = control.document(
        switches: [
          for (var i = 0; i < 50; i++)
            killSwitch(fbs.SwitchScope.Plugin, 'plugin$i', withRule: i.isEven),
        ],
        mandatory: fbs.MandatoryUpdateObjectBuilder(
          releaseId: 'r',
          releaseSequence: 1,
          messageKey: 'k',
        ),
        rollback: fbs.ForcedRollbackObjectBuilder(
          releaseId: 'r',
          releaseSequence: 1,
        ),
      );
      final snapshot = await read(doc);
      expect(snapshot.switches, hasLength(50));
    });

    test('the generated layout checks the tables a hand-written one left '
        'out: an experiment without its key is refused [SEC-052] '
        '[SYN-060]', () async {
      final doc = control.document(
        experiments: [
          fbs.ExperimentAllocationObjectBuilder(
            scope: fbs.ExperimentScope.Prop,
            salt: 's',
          ),
        ],
      );
      final envelope = await accepted(await control.file(doc));
      expect(
        () => readControlDocument(envelope, _limits),
        throwsA(
          isA<PluxException>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.controlSignatureInvalid,
          ),
        ),
      );
    });
  });
}
