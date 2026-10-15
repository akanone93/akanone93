// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart' show PluxFunctionContext;
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/functions/invoke_function.dart';
import 'package:plux_flutter/src/security/confirm_sheet.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/sca.dart';

import '../data/io_support.dart';
import '../sync/fake_device.dart';

Uint8List _bytes(String s) => Uint8List.fromList(utf8.encode(s));

/// What the server was called with.
typedef _Call = ({
  String function,
  int version,
  bool attest,
  String challengeId,
  Uint8List? signature,
});

final class _Server implements FunctionServer {
  final List<_Call> calls = [];

  @override
  Future<FunctionServerReply> invoke({
    required String function,
    required int version,
    required Uint8List input,
    required String idempotencyKey,
    String? userToken,
    bool attest = false,
    String challengeId = '',
    Uint8List? signature,
  }) async {
    calls.add((
      function: function,
      version: version,
      attest: attest,
      challengeId: challengeId,
      signature: signature,
    ));
    return FunctionServerOutput(_bytes('{"ok":true}'));
  }
}

/// The challenge source, the confirmer and the signer of one confirmation.
final class _Rig
    implements
        ChallengeSource,
        InvocationChallenges,
        InvocationConfirmer,
        UserAuthSigner {
  var approve = true;

  /// How the server answers: the summary a challenge is bound to, as a
  /// function of the request; null follows the request.
  Uint8List Function(String function, int version, Uint8List hash)? bind;
  final List<InvocationRequest> shown = [];
  final List<({String function, int version, Uint8List input})> asked = [];
  final List<Uint8List> signed = [];
  Uint8List challengeBytes = Uint8List.fromList(List.generate(8, (i) => i));

  @override
  Future<TransactionChallenge> invocationChallenge({
    required String function,
    required int version,
    required Uint8List input,
    required Uint8List summaryJcs,
  }) async {
    asked.add((function: function, version: version, input: input));
    final hash = Uint8List.fromList(sha256.convert(input).bytes);
    final summary =
        bind?.call(function, version, hash) ??
        _bytes(
          '{"function":"$function",'
          '"inputSha256":"${base64Url.encode(hash).replaceAll('=', '')}",'
          '"version":"$version"}',
        );
    final challenge = TransactionChallenge(
      id: 'ch-fn',
      bytes: challengeBytes,
      expiresAt: DateTime.utc(2100),
      summaryJcs: summary,
    );
    checkInvocationSummary(challenge, function, version, hash);
    return challenge;
  }

  @override
  Future<TransactionChallenge> challenge({
    required String dataSource,
    required String operation,
    required Uint8List summaryJcs,
  }) => throw UnimplementedError();

  @override
  Future<bool> ensureEnrolled() async => true;

  @override
  Future<bool> confirmInvocation(
    BuildContext context,
    InvocationRequest request,
  ) async {
    shown.add(request);
    return approve;
  }

  @override
  Future<Uint8List> signWithUser(
    String alias,
    Uint8List data, {
    required String title,
    required String subtitle,
  }) async {
    signed.add(data);
    return Uint8List.fromList(List.filled(64, 3));
  }
}

void main() {
  late _Rig rig;
  late _Server server;
  late ScaService sca;
  late BuildContext screen;
  late FakeDeviceKeys keys;

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

  StepContext context({bool withSca = true}) => StepContext(
    navigator: NoNavigator(),
    emit: (_, _) {},
    nativeActions: const NoNativeActions(),
    services: {
      FunctionRuntime: FunctionRuntime(
        resolve: (_, name) => switch (name) {
          'pay.attested' => const ServerFunctionTarget(
            name: 'pay.attested',
            version: 4,
            requiresAttestation: true,
          ),
          'pay.confirm' => const ServerFunctionTarget(
            name: 'pay.confirm',
            version: 2,
            requiresSca: true,
          ),
          'pay.both' => const ServerFunctionTarget(
            name: 'pay.both',
            version: 5,
            requiresAttestation: true,
            requiresSca: true,
          ),
          'fee.plain' => const ServerFunctionTarget(
            name: 'fee.plain',
            version: 1,
          ),
          _ => null,
        },
        deviceEnabled: () => true,
        limits: () => const {},
        context: () => const PluxFunctionContext(
          app: 'a',
          environment: 'production',
          assuranceLevel: 'AL1',
        ),
        kvSnapshot: (_) async => {},
        kvApply: (_, _, _) async {},
        server: server,
      ),
      if (withSca) ScaService: sca,
    },
  ).forStep(const []);

  final handler = functionHandlers['invokeFunction']!;
  Future<StepResult> call(String function, {bool withSca = true}) async =>
      handler.run(context(withSca: withSca), {
        'function': function,
        'input': {'amount': '5'},
      });

  setUp(() {
    rig = _Rig();
    server = _Server();
    keys = FakeDeviceKeys();
    keys.stored['k'] = DeviceKey(
      alias: 'k',
      purpose: KeyPurpose.sca,
      publicKey: EcPublicKey(Uint8List(32), Uint8List(32)),
      storage: KeyStorage.tee,
    );
    sca = ScaService(
      challenges: rig,
      invocations: rig,
      invocationConfirmer: rig,
      keys: keys,
      signer: rig,
      alias: 'k',
      context: () => screen,
      confirmer: _NoTransactions(),
    );
  });

  test('[FN-020] a function that declares nothing is called without '
      'evidence or a transaction', () async {
    await call('fee.plain');
    final c = server.calls.single;
    expect((c.attest, c.challengeId, c.signature), (false, '', null));
    expect(rig.asked, isEmpty);
  });

  test('[FN-020] [SEC-004] a function that declares attestation is called '
      'with evidence asked for', () async {
    await call('pay.attested');
    final c = server.calls.single;
    expect((c.function, c.version, c.attest), ('pay.attested', 4, true));
    expect(c.challengeId, isEmpty);
    expect(rig.asked, isEmpty, reason: 'no transaction was asked for');
  });

  testWidgets('[FN-020] [SEC-027] a function that declares sca is called '
      'with the transaction the user confirmed over its input', (tester) async {
    await pumpScreen(tester);
    await call('pay.confirm');
    expect(rig.asked.single.function, 'pay.confirm');
    expect(rig.asked.single.version, 2);
    expect(utf8.decode(rig.asked.single.input), '{"amount":"5"}');
    expect(rig.shown.single.function, 'pay.confirm');
    expect(utf8.decode(rig.shown.single.input), '{"amount":"5"}');
    final hash = base64Url
        .encode(sha256.convert(_bytes('{"amount":"5"}')).bytes)
        .replaceAll('=', '');
    expect(
      rig.signed.single,
      [
        ...rig.challengeBytes,
        ..._bytes(
          '{"function":"pay.confirm","inputSha256":"$hash","version":"2"}',
        ),
      ],
      reason:
          'the key signs the challenge and the summary that binds the input',
    );
    final c = server.calls.single;
    expect((c.attest, c.challengeId), (false, 'ch-fn'));
    expect(c.signature, List.filled(64, 3));
  });

  testWidgets('[FN-020] a function that declares both is called with both', (
    tester,
  ) async {
    await pumpScreen(tester);
    await call('pay.both');
    final c = server.calls.single;
    expect((c.attest, c.challengeId), (true, 'ch-fn'));
  });

  testWidgets('[SEC-027] a user who declines sends nothing and signs nothing', (
    tester,
  ) async {
    await pumpScreen(tester);
    rig.approve = false;
    await expectLater(
      Future.value(call('pay.confirm')),
      throwsA(
        isA<ActionError>()
            .having((e) => e.kind, 'kind', ActionErrorKind.cancelled)
            .having((e) => e.code, 'code', PluxErrorCode.actionCancelled),
      ),
    );
    expect(rig.signed, isEmpty);
    expect(server.calls, isEmpty);
  });

  testWidgets('[SEC-027] a device without an SCA key fails the step and '
      'sends nothing', (tester) async {
    await pumpScreen(tester);
    keys.stored.clear();
    final noKey = _Rig()..approve = true;
    sca = ScaService(
      challenges: _NoEnrolment(),
      invocations: noKey,
      invocationConfirmer: noKey,
      keys: keys,
      signer: noKey,
      alias: 'k',
      context: () => screen,
      confirmer: _NoTransactions(),
    );
    await expectLater(
      Future.value(call('pay.confirm')),
      throwsA(
        isA<ActionError>().having(
          (e) => e.code,
          'code',
          PluxErrorCode.deviceKeyUnavailable,
        ),
      ),
    );
    expect(server.calls, isEmpty);
    expect(noKey.asked, isEmpty);
  });

  test('[SEC-027] a runtime that cannot confirm fails the step instead of '
      'calling without it', () async {
    await expectLater(
      Future.value(call('pay.confirm', withSca: false)),
      throwsA(
        isA<ActionError>().having(
          (e) => e.code,
          'code',
          PluxErrorCode.actionsNotAvailable,
        ),
      ),
    );
    expect(server.calls, isEmpty);
  });

  testWidgets('[FN-020] the sheet shows the function, the version and the '
      'input, and answers the button pressed', (tester) async {
    await pumpScreen(tester);
    final request = InvocationRequest(
      function: 'pay.confirm',
      version: 2,
      input: _bytes('{"amount":"5"}'),
    );
    final answer = const SheetInvocationConfirmer().confirmInvocation(
      screen,
      request,
    );
    await tester.pumpAndSettle();
    expect(find.text('pay.confirm'), findsOneWidget);
    expect(find.text('2'), findsOneWidget);
    expect(find.text('{"amount":"5"}'), findsOneWidget);
    await tester.tap(find.text('Confirm'));
    await tester.pumpAndSettle();
    expect(await answer, isTrue);

    final declined = const SheetInvocationConfirmer().confirmInvocation(
      screen,
      request,
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Cancel'));
    await tester.pumpAndSettle();
    expect(await declined, isFalse);
  });

  group('[SEC-028] the summary the server binds the challenge to', () {
    final input = _bytes('{"amount":"5"}');
    final hash = Uint8List.fromList(sha256.convert(input).bytes);
    final b64 = base64Url.encode(hash).replaceAll('=', '');

    void expectRefused(String summary) {
      expect(
        () => checkInvocationSummary(
          TransactionChallenge(
            id: 'c',
            bytes: Uint8List(1),
            expiresAt: DateTime.utc(2100),
            summaryJcs: _bytes(summary),
          ),
          'pay.confirm',
          2,
          hash,
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

    test('names exactly the function, the version and the input', () {
      checkInvocationSummary(
        TransactionChallenge(
          id: 'c',
          bytes: Uint8List(1),
          expiresAt: DateTime.utc(2100),
          summaryJcs: _bytes(
            '{"function":"pay.confirm","inputSha256":"$b64","version":"2"}',
          ),
        ),
        'pay.confirm',
        2,
        hash,
      );
    });

    test('is refused for another input, version, function or an extra '
        'member', () {
      final other = base64Url
          .encode(sha256.convert(_bytes('{"amount":"5000"}')).bytes)
          .replaceAll('=', '');
      expectRefused(
        '{"function":"pay.confirm","inputSha256":"$other","version":"2"}',
      );
      expectRefused(
        '{"function":"pay.confirm","inputSha256":"$b64","version":"3"}',
      );
      expectRefused(
        '{"function":"pay.refund","inputSha256":"$b64","version":"2"}',
      );
      expectRefused(
        '{"extra":"x","function":"pay.confirm","inputSha256":"$b64",'
        '"version":"2"}',
      );
      expectRefused('{}');
      expectRefused('nope');
    });

    test('and a server that binds the challenge to another input is not '
        'signed for', () async {
      rig.bind = (f, v, h) =>
          _bytes('{"function":"$f","inputSha256":"AAAA","version":"$v"}');
      expect(
        () => rig.invocationChallenge(
          function: 'pay.confirm',
          version: 2,
          input: input,
          summaryJcs: _bytes('{}'),
        ),
        throwsA(isA<PluxException>()),
      );
    });
  });
}

/// A confirmer for transactions; invocations never use it.
final class _NoTransactions implements TransactionConfirmer {
  @override
  Future<bool> confirm(
    BuildContext context,
    TransactionSummary summary, {
    List<DisplayedValue> displayed = const [],
  }) => throw UnimplementedError();
}

/// A challenge source whose device cannot enrol an SCA key.
final class _NoEnrolment implements ChallengeSource {
  @override
  Future<TransactionChallenge> challenge({
    required String dataSource,
    required String operation,
    required Uint8List summaryJcs,
  }) => throw UnimplementedError();

  @override
  Future<bool> ensureEnrolled() async => false;
}
