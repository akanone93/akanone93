// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';
import 'dart:math';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/secure/otp.dart';
import 'package:plux_flutter/src/secure/pin_layout.dart';
import 'package:plux_flutter/src/secure/platform.dart';
import 'package:plux_flutter/src/secure/widgets.dart';

Widget _app(Widget child) => MaterialApp(
  home: Scaffold(body: Center(child: child)),
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late List<MethodCall> platformCalls;

  setUp(() {
    platformCalls = [];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (call) async {
          platformCalls.add(call);
          if (call.method == 'Clipboard.getData') {
            return <String, Object?>{'text': 'pasted'};
          }
          return null;
        });
    addTearDown(
      () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(SystemChannels.platform, null),
    );
  });

  group('SecureTextField', () {
    Future<(TextEditingController, List<String>)> show(
      WidgetTester tester, {
      bool obscure = true,
      VoidCallback? onActivity,
    }) async {
      final controller = TextEditingController();
      addTearDown(controller.dispose);
      final changes = <String>[];
      await tester.pumpWidget(
        _app(
          SecureTextFieldView(
            controller: controller,
            label: 'Password',
            obscure: obscure,
            onChanged: changes.add,
            onActivity: onActivity,
          ),
        ),
      );
      return (controller, changes);
    }

    testWidgets(
      'has no suggestions, autocorrect, keyboard learning or clipboard menu, and is obscured [SEC-090]',
      (tester) async {
        await show(tester);
        final field = tester.widget<TextField>(find.byType(TextField));
        expect(field.obscureText, isTrue);
        expect(field.enableSuggestions, isFalse);
        expect(field.autocorrect, isFalse);
        // Android's IME_FLAG_NO_PERSONALIZED_LEARNING.
        expect(field.enableIMEPersonalizedLearning, isFalse);
        expect(field.enableInteractiveSelection, isFalse);
        expect(field.keyboardType, TextInputType.visiblePassword);
        expect(field.smartDashesType, SmartDashesType.disabled);
        expect(field.smartQuotesType, SmartQuotesType.disabled);
        expect(field.autofillHints, isEmpty);
        expect(
          field.contextMenuBuilder!(
            tester.element(find.byType(TextField)),
            tester.state<EditableTextState>(find.byType(EditableText)),
          ),
          isA<SizedBox>(),
        );
      },
    );

    testWidgets(
      'a long press opens no copy or paste menu, and the paste shortcut does nothing [SEC-090]',
      (tester) async {
        final (controller, _) = await show(tester);
        await tester.enterText(find.byType(TextField), 'secret');
        await tester.longPress(find.byType(TextField));
        await tester.pump();
        expect(find.text('Paste'), findsNothing);
        expect(find.text('Copy'), findsNothing);
        expect(find.text('Select all'), findsNothing);

        controller.clear();
        await tester.tap(find.byType(TextField));
        await tester.pump();
        await tester.sendKeyDownEvent(LogicalKeyboardKey.control);
        await tester.sendKeyEvent(LogicalKeyboardKey.keyV);
        await tester.sendKeyUpEvent(LogicalKeyboardKey.control);
        await tester.pump();
        expect(controller.text, isEmpty);
        expect(
          platformCalls.where((c) => c.method == 'Clipboard.getData'),
          isEmpty,
        );
      },
    );

    testWidgets(
      'a plain TextField takes the same paste, so the test above can fail [SEC-090]',
      (tester) async {
        final controller = TextEditingController();
        addTearDown(controller.dispose);
        await tester.pumpWidget(_app(TextField(controller: controller)));
        await tester.tap(find.byType(TextField));
        await tester.pump();
        await tester.sendKeyDownEvent(LogicalKeyboardKey.control);
        await tester.sendKeyEvent(LogicalKeyboardKey.keyV);
        await tester.sendKeyUpEvent(LogicalKeyboardKey.control);
        await tester.pump();
        await tester.pump();
        expect(controller.text, 'pasted');
      },
    );

    testWidgets(
      'characters show only after the user unmasks, and hide again on losing focus [SEC-092]',
      (tester) async {
        final (_, changes) = await show(tester);
        bool obscured() =>
            tester.widget<TextField>(find.byType(TextField)).obscureText;

        await tester.tap(find.byType(TextField));
        await tester.enterText(find.byType(TextField), 'abc');
        expect(changes, ['abc']);
        expect(obscured(), isTrue);

        await tester.tap(find.byTooltip('Show'));
        await tester.pump();
        expect(obscured(), isFalse);
        // Unmasked text keeps the hardened keyboard.
        expect(
          tester.widget<TextField>(find.byType(TextField)).enableSuggestions,
          isFalse,
        );

        FocusManager.instance.primaryFocus?.unfocus();
        await tester.pump();
        expect(obscured(), isTrue);
        expect(find.byTooltip('Show'), findsOneWidget);

        await tester.tap(find.byTooltip('Show'));
        await tester.pump();
        await tester.tap(find.byTooltip('Hide'));
        await tester.pump();
        expect(obscured(), isTrue);
      },
    );

    testWidgets(
      'a field that does not obscure has nothing to unmask [SEC-092]',
      (tester) async {
        await show(tester, obscure: false);
        expect(find.byTooltip('Show'), findsNothing);
        expect(
          tester.widget<TextField>(find.byType(TextField)).obscureText,
          isFalse,
        );
      },
    );

    testWidgets(
      'every edit counts as activity for the inactivity lock [SEC-094]',
      (tester) async {
        var activity = 0;
        await show(tester, onActivity: () => activity++);
        await tester.enterText(find.byType(TextField), 'a');
        await tester.enterText(find.byType(TextField), 'ab');
        expect(activity, 2);
      },
    );
  });

  group('SecurePinPad', () {
    test('the layout is a permutation that a seed fixes, and a seed changes [SEC-090]', () {
      expect(pinDigitOrder(), [1, 2, 3, 4, 5, 6, 7, 8, 9, 0]);
      final a = pinDigitOrder(Random(42));
      expect(a, pinDigitOrder(Random(42)));
      expect([...a]..sort(), [0, 1, 2, 3, 4, 5, 6, 7, 8, 9]);
      final others = {
        for (var s = 0; s < 20; s++) pinDigitOrder(Random(s)).join(),
      };
      expect(others.length, greaterThan(15));
      expect(others, isNot(contains('1234567890')));
    });

    List<String> keys(WidgetTester tester) => [
      for (final t in tester.widgetList<Text>(
        find.descendant(
          of: find.byType(SecurePinPadView),
          matching: find.byType(Text),
        ),
      ))
        t.data!,
    ];

    testWidgets(
      'a randomised pad lays the digits out from the seeded generator, again after each entry [SEC-090]',
      (tester) async {
        final seeds = <int>[5, 6];
        final pins = <String>[];
        await tester.pumpWidget(
          _app(
            SecurePinPadView(
              length: 4,
              randomize: true,
              random: () => Random(seeds.removeAt(0)),
              onCompleted: pins.add,
            ),
          ),
        );
        expect(keys(tester).take(10).toList(), [
          for (final d in pinDigitOrder(Random(5))) '$d',
        ]);
        for (final d in pinDigitOrder(Random(5)).take(4)) {
          await tester.tap(find.text('$d'));
          await tester.pump();
        }
        expect(pins, [pinDigitOrder(Random(5)).take(4).join()]);
        expect(keys(tester).take(10).toList(), [
          for (final d in pinDigitOrder(Random(6))) '$d',
        ]);
      },
    );

    testWidgets(
      'the fixed pad reads 1 to 9 then 0, completes with the PIN and clears for the next try [SEC-090]',
      (tester) async {
        final pins = <String>[];
        await tester.pumpWidget(
          _app(SecurePinPadView(length: 4, onCompleted: pins.add)),
        );
        expect(keys(tester).take(10).toList(), [
          '1', '2', '3', '4', '5', '6', '7', '8', '9', '0', //
        ]);
        for (final d in ['2', '4', '6', '8']) {
          await tester.tap(find.text(d));
        }
        expect(pins, ['2468']);
        for (final d in ['1', '1', '0']) {
          await tester.tap(find.text(d));
        }
        await tester.tap(find.text('⌫'));
        await tester.tap(find.text('9'));
        await tester.tap(find.text('9'));
        expect(pins, ['2468', '1199']);
      },
    );

    testWidgets(
      'a key press only vibrates: no ripple, highlight or animation [SEC-090]',
      (tester) async {
        await tester.pumpWidget(
          _app(SecurePinPadView(length: 4, onCompleted: (_) {})),
        );
        final pad = find.byType(SecurePinPadView);
        expect(
          find.descendant(
            of: pad,
            matching: find.byWidgetPredicate(
              (w) =>
                  w is InkResponse ||
                  w is ImplicitlyAnimatedWidget ||
                  w is AnimatedWidget,
            ),
          ),
          findsNothing,
        );
        final before = keys(tester);
        final gesture = await tester.startGesture(
          tester.getCenter(find.text('5')),
        );
        await tester.pump(const Duration(milliseconds: 50));
        expect(tester.hasRunningAnimations, isFalse);
        expect(keys(tester), before);
        await gesture.up();
        await tester.pump();
        final haptics = platformCalls.where(
          (c) => c.method == 'HapticFeedback.vibrate',
        );
        expect(haptics.map((c) => c.arguments), [
          'HapticFeedbackType.selectionClick',
        ]);
      },
    );

    testWidgets('haptics can be switched off [SEC-090]', (tester) async {
      await tester.pumpWidget(
        _app(SecurePinPadView(length: 4, haptics: false, onCompleted: (_) {})),
      );
      await tester.tap(find.text('1'));
      expect(
        platformCalls.where((c) => c.method == 'HapticFeedback.vibrate'),
        isEmpty,
      );
    });

    testWidgets(
      'keys are buttons with accessible names, and the digits entered are never announced [SEC-090]',
      (tester) async {
        final handle = tester.ensureSemantics();
        final pins = <String>[];
        await tester.pumpWidget(
          _app(SecurePinPadView(length: 4, onCompleted: pins.add)),
        );
        expect(find.bySemanticsLabel('7'), findsOneWidget);
        expect(find.bySemanticsLabel('Delete'), findsOneWidget);
        expect(
          tester.getSemantics(find.bySemanticsLabel('7')),
          matchesSemantics(label: '7', isButton: true, hasTapAction: true),
        );
        await tester.tap(find.text('7'));
        await tester.tap(find.text('3'));
        await tester.pump();
        final status = tester.getSemantics(find.bySemanticsLabel('PIN'));
        expect(status.value, '2 of 4 digits entered');
        expect(status.label, isNot(contains('7')));
        // The last digit completes the PIN and clears the pad, so a screen
        // reader never hears "4 of 4"; Plux Bank's iOS accessibility pass
        // checks the pad by this count.
        await tester.tap(find.text('1'));
        await tester.pump();
        expect(
          tester.getSemantics(find.bySemanticsLabel('PIN')).value,
          '3 of 4 digits entered',
        );
        await tester.tap(find.text('9'));
        await tester.pump();
        expect(pins, ['7319']);
        expect(
          tester.getSemantics(find.bySemanticsLabel('PIN')).value,
          '0 of 4 digits entered',
        );
        handle.dispose();
      },
    );
  });

  group('OtpInput', () {
    test('a code is the digits or letters of its alphabet, grouped or not [SEC-090]', () {
      const numeric = OtpFormat(length: 6);
      expect(numeric.normalize('123456'), '123456');
      expect(numeric.normalize('123 456'), '123456');
      expect(numeric.normalize('123-456'), '123456');
      expect(numeric.normalize('12'), '12');
      expect(numeric.normalize('1234567'), isNull);
      expect(numeric.normalize('12a456'), isNull);
      expect(numeric.normalize('١٢٣٤٥٦'), isNull, reason: 'ASCII digits only');
      const alnum = OtpFormat(length: 6, charset: OtpCharset.alphanumeric);
      expect(alnum.normalize('aB3dE9'), 'aB3dE9');
      expect(alnum.normalize('aB3d_9'), isNull);
      expect(OtpCharset.named('alphanumeric'), OtpCharset.alphanumeric);
      expect(OtpCharset.named(null), OtpCharset.numeric);
    });

    Future<(TextEditingController, List<String>)> show(
      WidgetTester tester, {
      OtpFormat format = const OtpFormat(length: 6),
      bool autofill = true,
    }) async {
      final controller = TextEditingController();
      addTearDown(controller.dispose);
      final done = <String>[];
      await tester.pumpWidget(
        _app(
          OtpInputView(
            controller: controller,
            format: format,
            autofill: autofill,
            onCompleted: done.add,
          ),
        ),
      );
      return (controller, done);
    }

    testWidgets('asks the platform for one-time-code autofill [SEC-090]', (
      tester,
    ) async {
      await show(tester);
      var field = tester.widget<TextField>(find.byType(TextField));
      expect(field.autofillHints, [AutofillHints.oneTimeCode]);
      expect(field.keyboardType, TextInputType.number);
      await show(
        tester,
        autofill: false,
        format: const OtpFormat(length: 8, charset: OtpCharset.alphanumeric),
      );
      field = tester.widget<TextField>(find.byType(TextField));
      expect(field.autofillHints, isNull);
      expect(field.keyboardType, TextInputType.visiblePassword);
    });

    testWidgets(
      'typing the last character completes the code, once per completion [SEC-090]',
      (tester) async {
        final (controller, done) = await show(tester);
        for (var n = 1; n <= 5; n++) {
          await tester.enterText(
            find.byType(TextField),
            '12345'.substring(0, n),
          );
        }
        expect(done, isEmpty);
        expect(
          tester.testTextInput.log.map((c) => c.method),
          isNot(contains('TextInput.finishAutofillContext')),
        );
        await tester.enterText(find.byType(TextField), '123456');
        expect(done, ['123456']);
        // Completing the code ends the autofill context, which on iOS takes
        // the keyboard away (the engine resigns the field's first
        // responder); Plux Bank's iOS accessibility pass sends no Return
        // after the code for this reason.
        expect(
          tester.testTextInput.log.map((c) => c.method),
          contains('TextInput.finishAutofillContext'),
        );
        // Typing past the end is dropped.
        await tester.enterText(find.byType(TextField), '1234567');
        expect(controller.text, '123456');
        expect(done, ['123456']);
        await tester.enterText(find.byType(TextField), '12345');
        await tester.enterText(find.byType(TextField), '123459');
        expect(done, ['123456', '123459']);
      },
    );

    testWidgets(
      'pasted or autofilled text is accepted whole, or refused whole [SEC-090]',
      (tester) async {
        final (controller, done) = await show(tester);
        await tester.enterText(find.byType(TextField), '123 456');
        expect(controller.text, '123456');
        expect(done, ['123456']);

        await tester.enterText(find.byType(TextField), '');
        await tester.enterText(find.byType(TextField), '12ab56');
        await tester.pump();
        expect(controller.text, isEmpty);
        expect(find.text('That is not a valid code.'), findsOneWidget);

        await tester.enterText(find.byType(TextField), '12345678');
        expect(controller.text, isEmpty);
        expect(done, ['123456']);

        // The next valid edit clears the notice.
        await tester.enterText(find.byType(TextField), '1');
        await tester.pump();
        expect(find.text('That is not a valid code.'), findsNothing);
      },
    );

    testWidgets(
      'an alphanumeric code takes letters and digits and nothing else [SEC-090]',
      (tester) async {
        final (controller, done) = await show(
          tester,
          format: const OtpFormat(length: 6, charset: OtpCharset.alphanumeric),
        );
        await tester.enterText(find.byType(TextField), 'aB3d_9');
        expect(controller.text, isEmpty);
        await tester.enterText(find.byType(TextField), 'aB3dE9');
        expect(controller.text, 'aB3dE9');
        expect(done, ['aB3dE9']);
      },
    );
  });

  group('BiometricButton', () {
    testWidgets(
      'fires onSuccess when the user confirms, onFailure otherwise [SEC-090]',
      (tester) async {
        final events = <String>[];
        var outcome = BiometricOutcome.success;
        await tester.pumpWidget(
          _app(
            BiometricButtonView(
              label: 'Confirm',
              ask: () async => outcome,
              onSuccess: () => events.add('success'),
              onFailure: () => events.add('failure'),
            ),
          ),
        );
        await tester.tap(find.text('Confirm'));
        await tester.pump();
        for (final o in [
          BiometricOutcome.cancelled,
          BiometricOutcome.failed,
          BiometricOutcome.unavailable,
        ]) {
          outcome = o;
          await tester.tap(find.text('Confirm'));
          await tester.pump();
        }
        expect(events, ['success', 'failure', 'failure', 'failure']);
      },
    );

    testWidgets(
      'a press while the prompt is open does nothing, and no prompt means a disabled button [SEC-090]',
      (tester) async {
        final answer = Completer<BiometricOutcome>();
        var asked = 0;
        final events = <String>[];
        await tester.pumpWidget(
          _app(
            BiometricButtonView(
              label: 'Confirm',
              ask: () {
                asked++;
                return answer.future;
              },
              onSuccess: () => events.add('success'),
            ),
          ),
        );
        await tester.tap(find.text('Confirm'));
        await tester.pump();
        await tester.tap(find.text('Confirm'), warnIfMissed: false);
        await tester.pump();
        expect(asked, 1);
        answer.complete(BiometricOutcome.success);
        await tester.pump();
        expect(events, ['success']);

        await tester.pumpWidget(
          _app(const BiometricButtonView(label: 'Confirm', ask: null)),
        );
        expect(
          tester
              .widget<ButtonStyleButton>(find.bySubtype<ButtonStyleButton>())
              .onPressed,
          isNull,
        );
      },
    );
  });
}
