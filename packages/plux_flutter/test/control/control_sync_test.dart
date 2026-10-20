// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/store/control_store.dart';
import 'package:plux_flutter/src/store/metadata_state.dart';
import 'package:plux_flutter/src/store/release_record.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/control_sync.dart';
import 'package:plux_flutter/src/sync/metadata_sync.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

import '../sync/fake_device.dart';
import 'control_support.dart';

/// What the fake server holds and has been asked.
final class _Server {
  Uint8List? file;
  String etag = '"v1"';
  final List<http.Request> asked = [];
  bool down = false;

  Future<http.Response> handle(http.Request req) async {
    if (down) throw const SocketException('down');
    if (req.url.path.endsWith('GetRootKeys')) {
      return http.Response(
        jsonEncode({'keys': <Object?>[], 'roots': <Object?>[]}),
        200,
      );
    }
    asked.add(req);
    final held = (jsonDecode(req.body) as Map<String, Object?>)['ifNoneMatch'];
    if (held == etag) {
      return http.Response(
        jsonEncode({'notModified': true, 'etag': etag}),
        200,
      );
    }
    return http.Response(
      jsonEncode({'etag': etag, 'signed': base64.encode(file!)}),
      200,
    );
  }
}

void main() {
  late Directory dir;
  late ControlFixture control;
  late ReleaseStore releases;
  late _Server server;
  late DateTime now;

  setUp(() async {
    dir = Directory.systemTemp.createTempSync('plux_control');
    control = await ControlFixture.create();
    releases = ReleaseStore.open(dir.path);
    server = _Server();
    now = controlNow;
  });
  tearDown(() => dir.deleteSync(recursive: true));

  ControlSync sync({List<TrustedKey>? keys}) {
    final api = PluxApiClient(
      MockClient(server.handle),
      Uri.parse('https://plux.test/'),
    );
    return ControlSync(
      api: api,
      metadata: MetadataSync(
        api: api,
        store: MetadataStore(dir.path),
        keys: keys ?? [control.trusted()],
        appId: controlScope.app,
        environment: controlScope.environment,
        production: false,
        clock: () => now,
      ),
      releases: releases,
      documents: ControlDocumentStore(dir.path),
      limits: const VerifierLimits(maxDepth: 64, maxVisits: 1000000),
      channel: controlScope.channel,
      token: () async => fakeToken('access'),
      clock: () => now,
    );
  }

  Future<void> publish(
    int version, {
    List<fbs.KillSwitchObjectBuilder> switches = const [],
    fbs.ForcedRollbackObjectBuilder? rollback,
    fbs.MandatoryUpdateObjectBuilder? mandatory,
    DateTime? expires,
    ControlFixture? signer,
  }) async {
    server.etag = '"v$version"';
    server.file = await control.file(
      control.document(
        version: version,
        expires: expires,
        switches: switches,
        rollback: rollback,
        mandatory: mandatory,
      ),
      version: version,
      expires: expires,
      signer: signer,
    );
  }

  fbs.KillSwitchObjectBuilder plugin(String key) =>
      killSwitch(fbs.SwitchScope.Plugin, key);

  void retain(int sequence) {
    final bytes = utf8.encode('app $sequence');
    final hash = sha256.convert(bytes).toString();
    releases.writeObject(ObjectKind.bundles, hash, Uint8List.fromList(bytes));
    releases
      ..stage(
        ReleaseRecord(
          sequence: sequence,
          source: ReleaseSource.sync,
          bundles: [RecordBundle(key: '', version: 1, hash: hash)],
        ),
      )
      ..activate();
  }

  test('fetches with DPoP and If-None-Match, and keeps the verified document '
      '[SYN-060]', () async {
    await publish(2, switches: [plugin('loans')]);
    final check = sync();
    final first = await check.check();
    expect(first.fresh, isTrue);
    expect(first.error, isNull);
    expect(first.snapshot!.isOff(SwitchKind.plugin, 'loans'), isTrue);
    final request = server.asked.single;
    expect(request.headers['Authorization'], 'DPoP access');
    expect(request.headers['DPoP'], isNotEmpty);
    expect(jsonDecode(request.body), {'channel': 'stable', 'ifNoneMatch': ''});

    final second = await check.check();
    expect(
      jsonDecode(server.asked.last.body),
      containsPair('ifNoneMatch', '"v2"'),
    );
    expect(second.fresh, isFalse, reason: '304: the document held is in force');
    expect(second.snapshot!.version, 2);
    expect(second.snapshot!.isOff(SwitchKind.plugin, 'loans'), isTrue);
  });

  test('a new version replaces the switches, and raises the floor '
      '[SYN-060]', () async {
    await publish(2, switches: [plugin('loans')]);
    final check = sync();
    await check.check();
    await publish(3, switches: [plugin('cards')]);
    final outcome = await check.check();
    expect(outcome.fresh, isTrue);
    expect(outcome.snapshot!.isOff(SwitchKind.plugin, 'loans'), isFalse);
    expect(outcome.snapshot!.isOff(SwitchKind.plugin, 'cards'), isTrue);
    expect(MetadataStore(dir.path).read().floor.control, 3);
  });

  test('refuses an older version with PLX-3102 and keeps the newer '
      'switches [SYN-060]', () async {
    await publish(5, switches: [plugin('loans')]);
    final check = sync();
    await check.check();
    await publish(4);
    final outcome = await check.check();
    expect(outcome.error!.code, PluxErrorCode.controlRollbackRejected);
    expect(outcome.fresh, isFalse);
    expect(outcome.snapshot!.version, 5);
    expect(outcome.snapshot!.isOff(SwitchKind.plugin, 'loans'), isTrue);
  });

  test('refuses a tampered document with PLX-3100 and stores nothing '
      '[SYN-060] [SEC-052]', () async {
    await publish(2, switches: [plugin('loans')]);
    server.file = await control.file(
      control.document(switches: [plugin('loans')]),
      afterSigning: (signed) => signed['version'] = 9,
    );
    final outcome = await sync().check();
    expect(outcome.error!.code, PluxErrorCode.controlSignatureInvalid);
    expect(outcome.snapshot, isNull);
    expect(ControlDocumentStore(dir.path).read(), isNull);
    expect(MetadataStore(dir.path).read().floor.control, 0);
  });

  test('refuses a document of another role or key with PLX-3100 '
      '[SYN-060]', () async {
    await publish(2, signer: await ControlFixture.create());
    final other = await sync().check();
    expect(other.error!.code, PluxErrorCode.controlSignatureInvalid);

    await publish(2);
    final undelegated = await sync(keys: [control.trusted('targets')]).check();
    expect(undelegated.error!.code, PluxErrorCode.controlSignatureInvalid);
  });

  test('an expired document from the server is refused with PLX-3101, and '
      'the switches held stay in force [SYN-060]', () async {
    await publish(2, switches: [plugin('loans')]);
    final check = sync();
    await check.check();
    now = controlNow.add(const Duration(hours: 30));
    await publish(
      3,
      switches: [plugin('cards')],
      expires: controlNow.add(const Duration(hours: 1)),
    );
    final outcome = await check.check();
    expect(outcome.error!.code, PluxErrorCode.controlDocumentExpired);
    expect(outcome.expired, isTrue);
    expect(outcome.snapshot!.version, 2);
    expect(outcome.snapshot!.isOff(SwitchKind.plugin, 'loans'), isTrue);
  });

  test('past its expiry, with no server, the last switches stay in force '
      '[SYN-060]', () async {
    await publish(2, switches: [plugin('loans')]);
    final check = sync();
    await check.check();
    server.down = true;
    now = controlNow.add(const Duration(days: 3));
    final outcome = await sync().check();
    expect(outcome.error, isNull, reason: 'offline is not a control failure');
    expect(outcome.expired, isTrue);
    expect(outcome.snapshot!.isOff(SwitchKind.plugin, 'loans'), isTrue);
  });

  test(
    'before the first document the device has no switches [SYN-060]',
    () async {
      server.down = true;
      final outcome = await sync().check();
      expect(outcome.snapshot, isNull);
      expect(outcome.error, isNull);
    },
  );

  group('forced rollback', () {
    test('activates the retained release once [REL-060]', () async {
      retain(1);
      retain(2);
      expect(releases.pointer.active, 2);
      await publish(
        2,
        rollback: fbs.ForcedRollbackObjectBuilder(
          releaseId: 'rel-1',
          releaseSequence: 1,
        ),
      );
      final check = sync();
      final outcome = await check.check();
      expect(outcome.rolledBackTo, 1);
      expect(outcome.error, isNull);
      expect(releases.pointer.active, 1);
      expect(releases.pointer.rejected, 2);

      // The same document is not acted on again.
      retain(3);
      final again = await check.check();
      expect(again.rolledBackTo, isNull);
      expect(releases.pointer.active, 3);
    });

    test('a release the device no longer holds is refused, once '
        '[REL-060]', () async {
      retain(2);
      await publish(
        2,
        rollback: fbs.ForcedRollbackObjectBuilder(
          releaseId: 'rel-1',
          releaseSequence: 1,
        ),
      );
      final check = sync();
      final outcome = await check.check();
      expect(outcome.rolledBackTo, isNull);
      expect(outcome.error!.code, PluxErrorCode.rollbackRejected);
      expect(releases.pointer.active, 2);
      expect((await check.check()).error, isNull);
    });

    test('the release already active needs no rollback [REL-060]', () async {
      retain(1);
      await publish(
        2,
        rollback: fbs.ForcedRollbackObjectBuilder(
          releaseId: 'rel-1',
          releaseSequence: 1,
        ),
      );
      final outcome = await sync().check();
      expect(outcome.rolledBackTo, isNull);
      expect(outcome.error, isNull);
    });
  });

  test(
    'keeps the mandatory update for the runtime to apply [REL-070]',
    () async {
      await publish(
        2,
        mandatory: fbs.MandatoryUpdateObjectBuilder(
          releaseId: 'rel-9',
          releaseSequence: 9,
          messageKey: 'update.now',
        ),
      );
      final outcome = await sync().check();
      expect(outcome.snapshot!.mandatory!.releaseSequence, 9);
      expect(outcome.snapshot!.mandatory!.messageKey, 'update.now');
    },
  );
}
