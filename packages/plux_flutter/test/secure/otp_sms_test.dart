// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/device/types.dart';
import 'package:plux_flutter/src/secure/otp.dart';
import 'package:plux_flutter/src/secure/platform.dart';
import 'package:plux_flutter/src/secure/widgets.dart';

/// A consent platform the test answers by hand.
final class _FakeSms implements SmsConsentPlatform {
  int started = 0;
  int cancelled = 0;
  Completer<String?>? _pending;
  Object? failure;

  @override
  Future<String?> awaitSmsConsent() {
    started++;
    final f = failure;
    if (f != null) return Future.error(f);
    return (_pending = Completer<String?>()).future;
  }

  @override
  Future<void> cancel() async {
    cancelled++;
    final p = _pending;
    if (p != null && !p.isCompleted) p.complete(null);
  }

  void deliver(String? message) => _pending!.complete(message);
}

void main() {
  group('OtpFormat.extract', () {
    const numeric = OtpFormat(length: 6);
    const alnum = OtpFormat(length: 8, charset: OtpCharset.alphanumeric);

    test('[SEC-090] finds a numeric code of the exact length', () {
      expect(numeric.extract('Your code is 482913. Do not share.'), '482913');
    });

    test('[SEC-090] finds an alphanumeric code', () {
      expect(alnum.extract('Code: aB3dE5gH (valid 5 min)'), 'aB3dE5gH');
    });

    test('[SEC-090] skips runs of another length', () {
      expect(numeric.extract('Call 0800123 or 12345 or 1234567'), isNull);
      expect(numeric.extract('Amount 4500, code 112233'), '112233');
    });

    test('[SEC-090] returns the first of several runs', () {
      expect(numeric.extract('111111 then 222222'), '111111');
    });

    test('[SEC-090] returns null when there is no code', () {
      expect(numeric.extract(''), isNull);
      expect(numeric.extract('no digits here'), isNull);
      expect(alnum.extract('short ab12'), isNull);
    });

    test('[SEC-090] a code at the very end of the message is found', () {
      expect(numeric.extract('code 654321'), '654321');
    });
  });

  group('OtpInputView with SMS User Consent', () {
    late _FakeSms sms;
    late TextEditingController controller;
    late List<String> done;

    setUp(() {
      sms = _FakeSms();
      controller = TextEditingController();
      done = [];
      addTearDown(controller.dispose);
    });

    Future<void> show(
      WidgetTester tester, {
      bool autofill = true,
      SmsConsentPlatform? platform,
    }) => tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: OtpInputView(
            controller: controller,
            format: const OtpFormat(length: 6),
            autofill: autofill,
            onCompleted: done.add,
            sms: platform ?? sms,
          ),
        ),
      ),
    );

    testWidgets('[SEC-090] focus starts the wait and the message fills the '
        'field as if typed', (tester) async {
      await show(tester);
      expect(sms.started, 0);

      await tester.tap(find.byType(TextField));
      await tester.pump();
      expect(sms.started, 1);

      sms.deliver('Your code is 482913');
      await tester.pump();
      expect(controller.text, '482913');
      expect(done, ['482913']);
    }, variant: const TargetPlatformVariant({TargetPlatform.android}));

    testWidgets('[SEC-090] a message without a code leaves the field empty', (
      tester,
    ) async {
      await show(tester);
      await tester.tap(find.byType(TextField));
      await tester.pump();
      sms.deliver('hello');
      await tester.pump();
      expect(controller.text, isEmpty);
      expect(done, isEmpty);
    }, variant: const TargetPlatformVariant({TargetPlatform.android}));

    testWidgets('[SEC-090] a declined consent leaves the field empty', (
      tester,
    ) async {
      await show(tester);
      await tester.tap(find.byType(TextField));
      await tester.pump();
      sms.deliver(null);
      await tester.pump();
      expect(controller.text, isEmpty);
      expect(done, isEmpty);
    }, variant: const TargetPlatformVariant({TargetPlatform.android}));

    testWidgets('[SEC-090] a non-empty field does not start the wait', (
      tester,
    ) async {
      controller.text = '12';
      await show(tester);
      await tester.tap(find.byType(TextField));
      await tester.pump();
      expect(sms.started, 0);
    }, variant: const TargetPlatformVariant({TargetPlatform.android}));

    testWidgets('[SEC-090] losing focus cancels the wait', (tester) async {
      await show(tester);
      await tester.tap(find.byType(TextField));
      await tester.pump();
      FocusManager.instance.primaryFocus?.unfocus();
      await tester.pump();
      expect(sms.cancelled, 1);
      expect(controller.text, isEmpty);
    }, variant: const TargetPlatformVariant({TargetPlatform.android}));

    testWidgets('[SEC-090] disposing the field cancels the wait', (
      tester,
    ) async {
      await show(tester);
      await tester.tap(find.byType(TextField));
      await tester.pump();
      await tester.pumpWidget(const SizedBox());
      expect(sms.cancelled, 1);
    }, variant: const TargetPlatformVariant({TargetPlatform.android}));

    testWidgets('[SEC-090] no call is made on other platforms', (tester) async {
      await show(tester);
      await tester.tap(find.byType(TextField));
      await tester.pump();
      expect(sms.started, 0);
    }, variant: const TargetPlatformVariant({TargetPlatform.iOS}));

    testWidgets('[SEC-090] no call is made with autofill off', (tester) async {
      await show(tester, autofill: false);
      await tester.tap(find.byType(TextField));
      await tester.pump();
      expect(sms.started, 0);
    }, variant: const TargetPlatformVariant({TargetPlatform.android}));

    testWidgets('[SEC-090] a failed call leaves the field as it was', (
      tester,
    ) async {
      sms.failure = const PluxDeviceException.unavailable('no play services');
      await show(tester);
      await tester.tap(find.byType(TextField));
      await tester.pump();
      expect(sms.started, 1);
      expect(controller.text, isEmpty);
      await tester.enterText(find.byType(TextField), '123456');
      expect(done, ['123456']);
    }, variant: const TargetPlatformVariant({TargetPlatform.android}));
  });
}
