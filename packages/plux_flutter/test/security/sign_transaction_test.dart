// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/pxl/decimal.dart';
import 'package:plux_flutter/src/pxl/values.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/handlers.dart';
import 'package:plux_flutter/src/security/sca.dart';

import '../data/io_support.dart';
import '../sync/fake_device.dart';

final class _Rig
    implements ChallengeSource, UserAuthSigner, TransactionConfirmer {
  var approve = true;
  final List<TransactionSummary> shown = [];
  final List<String> challenged = [];

  @override
  Future<TransactionChallenge> challenge({
    required String dataSource,
    required String operation,
    required Uint8List summaryJcs,
  }) async {
    challenged.add('$dataSource.$operation');
    return TransactionChallenge(
      id: 'ch-3',
      bytes: Uint8List(8),
      expiresAt: DateTime.utc(2100),
    );
  }

  @override
  Future<bool> ensureEnrolled() async => true;

  @override
  Future<bool> confirm(BuildContext context, TransactionSummary s) async {
    shown.add(s);
    return approve;
  }

  @override
  Future<Uint8List> signWithUser(
    String alias,
    Uint8List data, {
    required String title,
    required String subtitle,
  }) async => Uint8List(64);
}

void main() {
  late _Rig rig;
  late ScaService sca;
  late BuildContext screen;

  Future<void> pumpScreen(WidgetTester tester) => tester.pumpWidget(
    MaterialApp(
      home: Builder(
        builder: (c) {
          screen = c;
          return const SizedBox();
        },
      ),
    ),
  );

  StepContext context({
    Iterable<String> wired = const [],
    bool service = true,
  }) {
    return StepContext(
      navigator: NoNavigator(),
      emit: (_, _) {},
      nativeActions: const NoNativeActions(),
      services: {if (service) ScaService: sca},
    ).forStep(wired);
  }

  Map<String, Object?> inputs({Object? amount, Object? payee, Object? tx}) => {
    'transaction': tx ?? 'bank.transfer',
    'amount': amount ?? Money(Decimal.tryParse('12.50')!, 'EUR'),
    'payee': payee ?? 'Ada',
  };

  final handler = securityHandlers['signTransaction']!;

  setUp(() {
    rig = _Rig();
    final keys = FakeDeviceKeys();
    keys.stored['k'] = DeviceKey(
      alias: 'k',
      purpose: KeyPurpose.sca,
      publicKey: EcPublicKey(Uint8List(32), Uint8List(32)),
      storage: KeyStorage.tee,
    );
    sca = ScaService(
      challenges: rig,
      keys: keys,
      signer: rig,
      confirmer: rig,
      alias: 'k',
      context: () => screen,
    );
  });

  test(
    'is a handler of the engine table that waits for the user [SEC-027]',
    () {
      expect(builtInHandlers['signTransaction'], same(handler));
      expect(handler.waitsForUser, isTrue);
    },
  );

  testWidgets(
    'confirms the money and the payee as given and outputs the challenge ID [SEC-027] [SEC-028]',
    (tester) async {
      await pumpScreen(tester);
      final r = await handler.run(context(), inputs()) as StepDone;
      expect(r.output, 'ch-3');
      expect(r.branch, isNull);
      expect(rig.challenged, ['bank.transfer']);
      expect(
        (
          rig.shown.single.amount,
          rig.shown.single.currency,
          rig.shown.single.payee,
        ),
        ('12.50', 'EUR', 'Ada'),
      );
    },
  );

  testWidgets('takes the amount in its JSON form too [SEC-028]', (
    tester,
  ) async {
    await pumpScreen(tester);
    await handler.run(
      context(),
      inputs(amount: {'amount': '3.00', 'currency': 'USD'}),
    );
    expect(rig.shown.single.amount, '3.00');
  });

  testWidgets(
    'a decline takes the declined branch when wired and fails the step otherwise [SEC-027]',
    (tester) async {
      await pumpScreen(tester);
      rig.approve = false;
      final r =
          await handler.run(context(wired: ['declined']), inputs()) as StepDone;
      expect((r.output, r.branch), (null, 'declined'));
      await expectLater(
        Future.value(handler.run(context(), inputs())),
        throwsA(
          isA<ActionError>()
              .having((e) => e.kind, 'kind', ActionErrorKind.cancelled)
              .having((e) => e.code, 'code', PluxErrorCode.actionCancelled),
        ),
      );
    },
  );

  test(
    'bad inputs fail as validation errors before anything is asked [SEC-027]',
    () async {
      for (final bad in [
        inputs(tx: 'bank'),
        inputs(tx: 'bank.'),
        inputs(payee: ''),
        inputs(amount: 12),
      ]) {
        await expectLater(
          Future.value(handler.run(context(), bad)),
          throwsA(
            isA<ActionError>().having(
              (e) => e.kind,
              'kind',
              ActionErrorKind.validation,
            ),
          ),
        );
      }
      expect(rig.challenged, isEmpty);
    },
  );

  testWidgets(
    'a runtime without the service fails with PLX-4010 semantics, not silently [SEC-027]',
    (tester) async {
      await pumpScreen(tester);
      await expectLater(
        Future.value(handler.run(context(service: false), inputs())),
        throwsA(
          isA<ActionError>().having(
            (e) => e.code,
            'code',
            PluxErrorCode.actionsNotAvailable,
          ),
        ),
      );
    },
  );
}
