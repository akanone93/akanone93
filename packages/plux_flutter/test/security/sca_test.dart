// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/confirm_sheet.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/sca.dart';

import '../sync/fake_device.dart';

final _challenge = Uint8List.fromList([for (var i = 1; i <= 8; i++) i]);

const _summary = TransactionSummary(
  amount: '12.50',
  currency: 'EUR',
  payee: 'Ada "Lovelace"',
);

/// Records the order of the steps and answers as told.
final class _Rig
    implements ChallengeSource, UserAuthSigner, TransactionConfirmer {
  final List<String> steps = [];
  var approve = true;
  var cancelPrompt = false;
  var enrols = true;
  DateTime expires = DateTime.utc(2100);
  Uint8List? signed;
  String? promptSubtitle;
  TransactionSummary? shown;
  List<DisplayedValue> shownValues = const [];
  List<DisplayedValue> serverValues = const [];
  Uint8List? serverSummary;
  Uint8List? challengedSummary;

  @override
  Future<TransactionChallenge> challenge({
    required String dataSource,
    required String operation,
    required Uint8List summaryJcs,
  }) async {
    steps.add('challenge $dataSource.$operation');
    challengedSummary = summaryJcs;
    return TransactionChallenge(
      id: 'ch-1',
      bytes: _challenge,
      expiresAt: expires,
      displayed: serverValues,
      summaryJcs: serverSummary,
    );
  }

  @override
  Future<bool> ensureEnrolled() async {
    steps.add('enrol');
    return enrols;
  }

  @override
  Future<bool> confirm(
    BuildContext context,
    TransactionSummary summary, {
    List<DisplayedValue> displayed = const [],
  }) async {
    steps.add('sheet');
    shown = summary;
    shownValues = displayed;
    return approve;
  }

  @override
  Future<Uint8List> signWithUser(
    String alias,
    Uint8List data, {
    required String title,
    required String subtitle,
  }) async {
    steps.add('prompt');
    if (cancelPrompt) throw const UserAuthenticationCancelled();
    promptSubtitle = subtitle;
    signed = data;
    return Uint8List.fromList(List.filled(64, 9));
  }
}

void main() {
  group('the summary', () {
    test('is RFC 8785 canonical JSON with sorted members and exact escapes [SEC-028]', () {
      expect(
        utf8.decode(_summary.jcs),
        r'{"amount":"12.50","currency":"EUR","payee":"Ada \"Lovelace\""}',
      );
      const withDescription = TransactionSummary(
        amount: '1',
        currency: 'EUR',
        payee: 'a\\b\n\u0001é',
        description: 'rent',
      );
      expect(
        utf8.decode(withDescription.jcs),
        r'{"amount":"1","currency":"EUR","payee":"a\\b\n\u0001é","summary":"rent"}',
      );
    });

    test('is read from the request body by the document\'s pointers, strings and integers only [SEC-028]', () {
      final body = {
        'to': {'name': 'Ada', 'items/a': 'x~y'},
        'amount': 1250,
        'currency': 'EUR',
      };
      final s = TransactionSummary.read(
        body,
        amount: '/amount',
        currency: '/currency',
        payee: '/to/name',
      );
      expect(s.amount, '1250');
      expect(s.payee, 'Ada');
      expect(readPointer(body, '/to/items~1a'), 'x~y');
      expect(readPointer(body, '/missing/0'), isNull);
      for (final bad in [
        {
          'amount': 1.5,
          'currency': 'EUR',
          'to': {'name': 'Ada'},
        },
        {
          'currency': 'EUR',
          'to': {'name': 'Ada'},
        },
        {
          'amount': '1',
          'currency': '',
          'to': {'name': 'Ada'},
        },
      ]) {
        expect(
          () => TransactionSummary.read(
            bad,
            amount: '/amount',
            currency: '/currency',
            payee: '/to/name',
          ),
          throwsA(
            isA<PluxException>().having(
              (e) => e.code,
              'code',
              PluxErrorCode.scaDynamicLinkingMismatch,
            ),
          ),
        );
      }
    });

    test('leaves a payee the body does not carry to the server, and only a payee [DAT-093]', () {
      final s = TransactionSummary.read(
        {'amount': '5', 'currency': 'EUR'},
        amount: '/amount',
        currency: '/currency',
        payee: '/to',
      );
      expect(s.payee, isNull);
      expect(utf8.decode(s.jcs), '{"amount":"5","currency":"EUR"}');
      expect(
        () => TransactionSummary.read(
          {'amount': '5', 'currency': 'EUR', 'to': 7},
          amount: '/amount',
          currency: '/currency',
          payee: '/to',
        ),
        throwsA(isA<PluxException>()),
      );
    });

    test(
      'the signed input is the challenge followed by the summary [SEC-027]',
      () {
        expect(scaSigningInput(_challenge, _summary.jcs), [
          ..._challenge,
          ..._summary.jcs,
        ]);
      },
    );
  });

  group('ScaService', () {
    late _Rig rig;
    late FakeDeviceKeys keys;
    late ScaService sca;
    late BuildContext screen;

    Future<void> pumpScreen(WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: Builder(
            builder: (c) {
              screen = c;
              return const SizedBox();
            },
          ),
        ),
      );
    }

    setUp(() {
      rig = _Rig();
      keys = FakeDeviceKeys();
      keys.stored['sca'] = DeviceKey(
        alias: 'sca',
        purpose: KeyPurpose.sca,
        publicKey: EcPublicKey(Uint8List(32), Uint8List(32)),
        storage: KeyStorage.tee,
      );
    });

    ScaService make({DateTime Function()? clock}) => sca = ScaService(
      challenges: rig,
      keys: keys,
      signer: rig,
      confirmer: rig,
      alias: 'sca',
      context: () => screen,
      clock: clock,
    );

    const request = TransactionRequest(
      dataSource: 'bank',
      operation: 'transfer',
      summary: _summary,
    );

    const bound = TransactionRequest(
      dataSource: 'bank',
      operation: 'transfer',
      summary: TransactionSummary(
        amount: '12.50',
        currency: 'EUR',
        payee: null,
      ),
    );
    Uint8List merged(String payee) =>
        TransactionSummary(amount: '12.50', currency: 'EUR', payee: payee).jcs;

    testWidgets(
      'shows the values the server resolved and signs the summary it bound them into [DAT-093] [SEC-028]',
      (tester) async {
        await pumpScreen(tester);
        rig
          ..serverValues = const [DisplayedValue('Pool account', 'ACC-7')]
          ..serverSummary = merged('ACC-7');
        final c = await make().confirm(bound);
        expect(
          rig.challengedSummary,
          utf8.encode('{"amount":"12.50","currency":"EUR"}'),
        );
        expect(rig.shown, same(bound.summary));
        expect(rig.shownValues.single.label, 'Pool account');
        expect(rig.shownValues.single.value, 'ACC-7');
        expect(rig.signed, scaSigningInput(_challenge, merged('ACC-7')));
        expect(rig.promptSubtitle, '12.50 EUR to ACC-7');
        expect(c.summary, same(bound.summary));
      },
    );

    for (final (name, values, summary) in [
      (
        'a server value the user is not shown',
        const <DisplayedValue>[],
        '{"amount":"12.50","currency":"EUR","payee":"ACC-7"}',
      ),
      (
        'a shown value the summary does not carry',
        const [DisplayedValue('Pool account', 'ACC-8')],
        '{"amount":"12.50","currency":"EUR","payee":"ACC-7"}',
      ),
      (
        'a member of the device changed',
        const [DisplayedValue('Pool account', 'ACC-7')],
        '{"amount":"99.00","currency":"EUR","payee":"ACC-7"}',
      ),
      (
        'a member beyond the summary\'s',
        const [DisplayedValue('Pool account', 'ACC-7')],
        '{"amount":"12.50","currency":"EUR","fee":"1","payee":"ACC-7"}',
      ),
      (
        'a summary that is not canonical',
        const [DisplayedValue('Pool account', 'ACC-7')],
        '{"currency":"EUR","amount":"12.50","payee":"ACC-7"}',
      ),
      (
        'shown values with no summary',
        const [DisplayedValue('Pool account', 'ACC-7')],
        '',
      ),
    ]) {
      testWidgets('refuses before the sheet $name [DAT-093] [SEC-028]', (
        tester,
      ) async {
        await pumpScreen(tester);
        rig
          ..serverValues = values
          ..serverSummary = utf8.encode(summary);
        await expectLater(
          make().confirm(bound),
          throwsA(
            isA<PluxException>().having(
              (e) => e.code,
              'code',
              PluxErrorCode.scaDynamicLinkingMismatch,
            ),
          ),
        );
        expect(rig.steps, ['challenge bank.transfer']);
        expect(rig.signed, isNull);
      });
    }

    testWidgets(
      'asks for the challenge, shows the sheet, prompts, then signs challenge and summary, in that order [SEC-027] [SEC-028]',
      (tester) async {
        await pumpScreen(tester);
        final c = await make().confirm(request);
        expect(rig.steps, ['challenge bank.transfer', 'sheet', 'prompt']);
        expect(rig.shown, same(_summary));
        expect(rig.challengedSummary, _summary.jcs);
        expect(rig.signed, scaSigningInput(_challenge, _summary.jcs));
        expect(rig.promptSubtitle, '12.50 EUR to Ada "Lovelace"');
        expect(c.challengeId, 'ch-1');
        expect(c.signature, hasLength(64));
      },
    );

    testWidgets(
      'declining on the sheet or at the system prompt signs nothing [SEC-027]',
      (tester) async {
        await pumpScreen(tester);
        rig.approve = false;
        await expectLater(make().confirm(request), throwsA(isA<ScaDeclined>()));
        expect(rig.steps, ['challenge bank.transfer', 'sheet']);
        rig.steps.clear();
        rig
          ..approve = true
          ..cancelPrompt = true;
        await expectLater(sca.confirm(request), throwsA(isA<ScaDeclined>()));
        expect(rig.steps, ['challenge bank.transfer', 'sheet', 'prompt']);
      },
    );

    testWidgets(
      'a device without the key registers again once, and fails closed when it still has none [SEC-027]',
      (tester) async {
        await pumpScreen(tester);
        keys.stored.clear();
        rig.enrols = false;
        await expectLater(
          make().confirm(request),
          throwsA(
            isA<PluxException>().having(
              (e) => e.code,
              'code',
              PluxErrorCode.deviceKeyUnavailable,
            ),
          ),
        );
        expect(rig.steps, ['enrol']);
      },
    );

    test('without a screen the transaction is not signed [SEC-027]', () async {
      final s = ScaService(
        challenges: rig,
        keys: keys,
        signer: rig,
        confirmer: rig,
        alias: 'sca',
        context: () => null,
      );
      await expectLater(s.confirm(request), throwsA(isA<PluxException>()));
      expect(rig.steps, isEmpty);
    });

    testWidgets(
      'a held confirmation is used once by the same transaction and refused for another [SEC-028] [SEC-032]',
      (tester) async {
        await pumpScreen(tester);
        make();
        final held = await sca.confirmAhead(request);
        rig.steps.clear();
        expect(await sca.authorise(request), same(held));
        expect(rig.steps, isEmpty, reason: 'no second prompt');
        // Spent: the next call asks the user again.
        await sca.authorise(request);
        expect(rig.steps, contains('prompt'));

        await sca.confirmAhead(request);
        const other = TransactionRequest(
          dataSource: 'bank',
          operation: 'transfer',
          summary: TransactionSummary(
            amount: '99.00',
            currency: 'EUR',
            payee: 'Mallory',
          ),
        );
        rig.steps.clear();
        await expectLater(
          sca.authorise(other),
          throwsA(
            isA<PluxException>().having(
              (e) => e.code,
              'code',
              PluxErrorCode.scaDynamicLinkingMismatch,
            ),
          ),
        );
        expect(rig.steps, isEmpty, reason: 'nothing asked or signed');
      },
    );

    testWidgets(
      'an expired held confirmation is replaced by a new one [SEC-027]',
      (tester) async {
        await pumpScreen(tester);
        var now = DateTime.utc(2100, 1, 1);
        rig.expires = DateTime.utc(2100, 1, 1, 0, 1);
        make(clock: () => now);
        await sca.confirmAhead(request);
        rig.steps.clear();
        now = DateTime.utc(2100, 1, 1, 0, 2);
        await sca.authorise(request);
        expect(rig.steps, ['challenge bank.transfer', 'sheet', 'prompt']);
      },
    );
  });

  group('the confirmation sheet', () {
    testWidgets(
      'shows the amount, currency and payee exactly as signed, labelled for screen readers [SEC-028]',
      (tester) async {
        final handle = tester.ensureSemantics();
        late BuildContext screen;
        await tester.pumpWidget(
          MaterialApp(
            home: Builder(
              builder: (c) {
                screen = c;
                return const SizedBox();
              },
            ),
          ),
        );
        final result = const SheetTransactionConfirmer().confirm(
          screen,
          _summary,
        );
        await tester.pumpAndSettle();
        expect(find.text('12.50'), findsOneWidget);
        expect(find.text('EUR'), findsOneWidget);
        expect(find.text('Ada "Lovelace"'), findsOneWidget);
        expect(
          tester.getSemantics(find.text('Payee')),
          matchesSemantics(label: 'Payee\nAda "Lovelace"'),
        );
        await tester.tap(find.text('Confirm'));
        await tester.pumpAndSettle();
        expect(await result, isTrue);
        handle.dispose();
      },
    );

    testWidgets(
      'shows the values the server resolved, as it returned them [DAT-093]',
      (tester) async {
        late BuildContext screen;
        await tester.pumpWidget(
          MaterialApp(
            home: Builder(
              builder: (c) {
                screen = c;
                return const SizedBox();
              },
            ),
          ),
        );
        final result = const SheetTransactionConfirmer().confirm(
          screen,
          _summary,
          displayed: const [
            DisplayedValue('Pool account', 'ACC-7'),
            DisplayedValue('Provider', 'Water Co'),
          ],
        );
        await tester.pumpAndSettle();
        expect(find.text('Pool account'), findsOneWidget);
        expect(find.text('ACC-7'), findsOneWidget);
        expect(find.text('Provider'), findsOneWidget);
        expect(find.text('Water Co'), findsOneWidget);
        expect(find.text('Ada "Lovelace"'), findsOneWidget);
        await tester.tap(find.text('Cancel'));
        await tester.pumpAndSettle();
        expect(await result, isFalse);
      },
    );

    testWidgets('cancelling or dismissing the sheet declines [SEC-027]', (
      tester,
    ) async {
      late BuildContext screen;
      await tester.pumpWidget(
        MaterialApp(
          home: Builder(
            builder: (c) {
              screen = c;
              return const SizedBox();
            },
          ),
        ),
      );
      var result = const SheetTransactionConfirmer().confirm(screen, _summary);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(await result, isFalse);

      result = const SheetTransactionConfirmer().confirm(screen, _summary);
      await tester.pumpAndSettle();
      await tester.tapAt(const Offset(5, 5));
      await tester.pumpAndSettle();
      expect(await result, isFalse);
    });
  });
}
