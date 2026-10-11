// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:async';

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/actions/clock.dart';
import 'package:plux_flutter/src/device/types.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/secure/inactivity_lock.dart';
import 'package:plux_flutter/src/secure/platform.dart';
import 'package:plux_flutter/src/secure/secure_screen.dart';
import 'package:plux_flutter/src/secure/secure_ui.dart';
import 'package:plux_flutter/src/security/security_config.dart';

/// Time the test drives: timers fire on [advance].
final class FakeClock extends ActionClock {
  Duration t = Duration.zero;
  final List<_Timer> _timers = [];

  @override
  Duration get now => t;

  @override
  Timer timer(Duration d, void Function() f) {
    final timer = _Timer(t + d, f);
    _timers.add(timer);
    return timer;
  }

  /// Moves time on by [d], firing the timers that fall due.
  void advance(Duration d) {
    final end = t + d;
    for (;;) {
      final due = _timers.where((x) => x.isActive && x.due <= end).toList()
        ..sort((a, b) => a.due.compareTo(b.due));
      if (due.isEmpty) break;
      t = due.first.due;
      due.first.fire();
    }
    t = end;
  }
}

final class _Timer implements Timer {
  _Timer(this.due, this.f);

  final Duration due;
  final void Function() f;
  bool _active = true;

  void fire() {
    _active = false;
    f();
  }

  @override
  void cancel() => _active = false;

  @override
  bool get isActive => _active;

  @override
  int get tick => 0;
}

final class FakeScreen implements SecureScreenPlatform {
  final List<bool> calls = [];
  PluxDeviceException? failure;

  @override
  Future<void> setProtected({required bool enabled}) async {
    calls.add(enabled);
    if (failure case final f?) throw f;
  }
}

final class FakeBiometrics implements BiometricPlatform {
  final List<String> asked = [];
  final List<bool> credential = [];
  BiometricOutcome outcome = BiometricOutcome.failed;
  PluxDeviceException? failure;
  Completer<BiometricOutcome>? hold;

  @override
  Future<BiometricOutcome> authenticate({
    required String reason,
    required bool allowDeviceCredential,
  }) async {
    asked.add(reason);
    credential.add(allowDeviceCredential);
    if (failure case final f?) throw f;
    return hold?.future ?? outcome;
  }
}

SecuritySettings _settings([Map<String, Object?> overrides = const {}]) =>
    SecuritySettings.fromDocument({'overrides': overrides});

const _lockOn = {'inactivityLock': true, 'inactivityLockTimeout': 120};

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('SecureScreen', () {
    test('protects the window for the first page and frees it after the last [SEC-091]', () async {
      final platform = FakeScreen();
      final screen = SecureScreen(platform, report: (_) {});
      final a = screen.acquire();
      final b = screen.acquire();
      expect(screen.holders, 2);
      expect(platform.calls, [true]);
      a.release();
      a.release();
      expect(screen.holders, 1);
      expect(platform.calls, [true], reason: 'a second page still shows');
      b.release();
      expect(screen.protected, isFalse);
      expect(platform.calls, [true, false]);
      screen.acquire().release();
      expect(platform.calls, [true, false, true, false]);
    });

    test(
      'reports a platform that cannot protect the window, once [SEC-091]',
      () async {
        final platform = FakeScreen()
          ..failure = const PluxDeviceException.unavailable('no window');
        final reports = <PluxException>[];
        final screen = SecureScreen(platform, report: reports.add);
        screen.acquire().release();
        await pumpEventQueue();
        expect(reports.single.code, PluxErrorCode.deviceUnavailable);
        expect(reports.single.message, contains('no window'));
      },
    );

    test('speaks to the runtime channel as FLAG_SECURE on and off [SEC-091] [SEC-093]', () async {
      final calls = <MethodCall>[];
      final messenger =
          TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      messenger.setMockMethodCallHandler(
        const MethodChannel('dev.plux/runtime'),
        (call) async {
          calls.add(call);
          return null;
        },
      );
      addTearDown(
        () => messenger.setMockMethodCallHandler(
          const MethodChannel('dev.plux/runtime'),
          null,
        ),
      );
      final screen = SecureScreen(
        const ChannelSecureScreenPlatform(),
        report: (_) {},
      );
      final a = screen.acquire();
      final b = screen.acquire();
      a.release();
      await pumpEventQueue();
      expect(calls.single.method, 'secureScreen');
      expect(calls.single.arguments, {'enabled': true});
      b.release();
      await pumpEventQueue();
      expect(calls.last.arguments, {'enabled': false});
      expect(calls, hasLength(2));
    });

    test(
      'a failing channel is a device failure, never a crash [SEC-091]',
      () async {
        final messenger =
            TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
        messenger.setMockMethodCallHandler(
          const MethodChannel('dev.plux/runtime'),
          (call) async => throw PlatformException(code: 'PLUX_PLATFORM'),
        );
        addTearDown(
          () => messenger.setMockMethodCallHandler(
            const MethodChannel('dev.plux/runtime'),
            null,
          ),
        );
        await expectLater(
          const ChannelSecureScreenPlatform().setProtected(enabled: true),
          throwsA(isA<PluxDeviceException>()),
        );
        await expectLater(
          const ChannelBiometricPlatform().authenticate(
            reason: 'x',
            allowDeviceCredential: true,
          ),
          throwsA(isA<PluxDeviceException>()),
        );
      },
    );
  });

  group('the biometric channel', () {
    test('maps the platform answer to a typed outcome and never to success by default [SEC-094]', () async {
      final calls = <MethodCall>[];
      var answer = 'success';
      final messenger =
          TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      messenger.setMockMethodCallHandler(
        const MethodChannel('dev.plux/runtime'),
        (call) async {
          calls.add(call);
          return answer;
        },
      );
      addTearDown(
        () => messenger.setMockMethodCallHandler(
          const MethodChannel('dev.plux/runtime'),
          null,
        ),
      );
      const platform = ChannelBiometricPlatform();
      Future<BiometricOutcome> ask() => platform.authenticate(
        reason: 'Confirm',
        allowDeviceCredential: false,
      );
      expect(await ask(), BiometricOutcome.success);
      expect(calls.single.method, 'biometricAuth');
      expect(calls.single.arguments, {
        'reason': 'Confirm',
        'allowDeviceCredential': false,
      });
      for (final (word, outcome) in [
        ('cancelled', BiometricOutcome.cancelled),
        ('failed', BiometricOutcome.failed),
        ('unavailable', BiometricOutcome.unavailable),
        ('anything else', BiometricOutcome.failed),
      ]) {
        answer = word;
        expect(await ask(), outcome, reason: word);
      }
      expect(BiometricOutcome.fromCode(null), BiometricOutcome.failed);
    });
  });

  group('InactivityLock', () {
    late FakeClock clock;
    late ValueNotifier<SecuritySettings> settings;
    late FakeBiometrics bio;
    late InactivityLock lock;

    setUp(() {
      clock = FakeClock();
      settings = ValueNotifier(_settings(_lockOn));
      bio = FakeBiometrics();
      lock = InactivityLock(settings: settings, clock: clock, biometrics: bio);
      addTearDown(lock.dispose);
    });

    test('engages after the configured idle time and not before [SEC-094]', () {
      lock.attach();
      clock.advance(const Duration(seconds: 119));
      expect(lock.locked, isFalse);
      clock.advance(const Duration(seconds: 1));
      expect(lock.locked, isTrue);
    });

    test('the setting is read from the security settings: the profile and the override [SEC-094] [SEC-182]', () {
      expect(
        SecuritySettings.fromDocument(const {'profile': 'standard'})
            .flag(SecuritySetting.inactivityLock),
        isFalse,
      );
      final maximum = SecuritySettings.fromDocument(const {
        'profile': 'maximum',
      });
      expect(maximum.flag(SecuritySetting.inactivityLock), isTrue);
      expect(maximum.number(SecuritySetting.inactivityLockTimeout), 120);
      settings.value = _settings({
        'inactivityLock': true,
        'inactivityLockTimeout': 30,
      });
      lock.attach();
      clock.advance(const Duration(seconds: 30));
      expect(lock.locked, isTrue);
    });

    test('activity starts the idle time again, pointer and key events included [SEC-094]', () {
      lock.attach();
      clock.advance(const Duration(seconds: 100));
      lock.activity();
      clock.advance(const Duration(seconds: 100));
      expect(lock.locked, isFalse);
      GestureBinding.instance.pointerRouter.route(
        const PointerDownEvent(position: Offset(1, 1)),
      );
      clock.advance(const Duration(seconds: 100));
      expect(lock.locked, isFalse);
      clock.advance(const Duration(seconds: 20));
      expect(lock.locked, isTrue);
    });

    test(
      'a locked screen ignores touches until the user confirms [SEC-094]',
      () async {
        lock.attach();
        clock.advance(const Duration(seconds: 120));
        lock.activity();
        GestureBinding.instance.pointerRouter.route(
          const PointerDownEvent(position: Offset(1, 1)),
        );
        expect(lock.locked, isTrue);

        bio.outcome = BiometricOutcome.failed;
        expect(await lock.unlock(reason: 'Unlock'), BiometricOutcome.failed);
        expect(lock.locked, isTrue);
        bio.outcome = BiometricOutcome.cancelled;
        await lock.unlock(reason: 'Unlock');
        expect(lock.locked, isTrue);
        bio.failure = const PluxDeviceException.unavailable('no prompt');
        expect(
          await lock.unlock(reason: 'Unlock'),
          BiometricOutcome.unavailable,
        );
        expect(lock.locked, isTrue, reason: 'it fails closed');

        bio
          ..failure = null
          ..outcome = BiometricOutcome.success;
        expect(await lock.unlock(reason: 'Unlock'), BiometricOutcome.success);
        expect(lock.locked, isFalse);
        expect(
          bio.credential,
          everyElement(isTrue),
          reason: 'biometric or credential',
        );
        clock.advance(const Duration(seconds: 119));
        expect(lock.locked, isFalse);
        clock.advance(const Duration(seconds: 1));
        expect(lock.locked, isTrue);
      },
    );

    test('one prompt at a time [SEC-094]', () async {
      lock.attach();
      clock.advance(const Duration(seconds: 120));
      bio.hold = Completer<BiometricOutcome>();
      final first = lock.unlock(reason: 'Unlock');
      expect(lock.asking, isTrue);
      expect(await lock.unlock(reason: 'Unlock'), BiometricOutcome.cancelled);
      bio.hold!.complete(BiometricOutcome.success);
      expect(await first, BiometricOutcome.success);
      expect(bio.asked, hasLength(1));
    });

    test(
      'engages on return to the foreground when the time passed away [SEC-094]',
      () {
        lock.attach();
        // Timers do not run while the app is in the background.
        clock.t += const Duration(seconds: 200);
        lock.resumed();
        expect(lock.locked, isTrue);
      },
    );

    test('a quick return to the foreground does not lock [SEC-094]', () {
      lock.attach();
      clock.t += const Duration(seconds: 20);
      lock.resumed();
      expect(lock.locked, isFalse);
    });

    test('never engages with the setting off, and unlocks when it is switched off [SEC-094]', () {
      settings.value = _settings({'inactivityLock': false});
      lock.attach();
      clock.advance(const Duration(hours: 5));
      expect(lock.locked, isFalse);
      settings.value = _settings(_lockOn);
      clock.advance(const Duration(seconds: 120));
      expect(lock.locked, isTrue);
      settings.value = _settings({'inactivityLock': false});
      expect(lock.locked, isFalse);
    });

    test('runs only while a secure page shows [SEC-094]', () {
      clock.advance(const Duration(hours: 1));
      expect(lock.locked, isFalse);
      lock.attach();
      lock.attach();
      lock.detach();
      clock.advance(const Duration(seconds: 120));
      expect(lock.locked, isTrue);
      lock.detach();
      expect(lock.locked, isFalse, reason: 'nothing secure is left to hide');
    });
  });

  group('SecurePageGate', () {
    late FakeClock clock;
    late ValueNotifier<SecuritySettings> settings;
    late FakeScreen screen;
    late FakeBiometrics bio;
    late SecureUi ui;
    late List<PluxException> reports;

    setUp(() {
      clock = FakeClock();
      settings = ValueNotifier(_settings(_lockOn));
      screen = FakeScreen();
      bio = FakeBiometrics();
      reports = [];
      ui = SecureUi(
        settings: settings,
        report: reports.add,
        platforms: SecurePlatforms(screen: screen, biometrics: bio),
        clock: clock,
      );
      addTearDown(ui.dispose);
    });

    Widget page({required bool secure, Key? key}) => SecurePageGate(
      key: key,
      ui: ui,
      secure: secure,
      child: const Scaffold(body: TextField()),
    );

    testWidgets(
      'a secure page holds the screen protection while it shows, pages count each other [SEC-091] [SEC-093]',
      (tester) async {
        await tester.pumpWidget(
          MaterialApp(
            home: Column(
              children: [
                Expanded(child: page(secure: true, key: const ValueKey('a'))),
                Expanded(child: page(secure: true, key: const ValueKey('b'))),
              ],
            ),
          ),
        );
        expect(screen.calls, [true]);
        await tester.pumpWidget(
          MaterialApp(
            home: Column(
              children: [
                Expanded(child: page(secure: true, key: const ValueKey('a'))),
              ],
            ),
          ),
        );
        expect(screen.calls, [true], reason: 'the other page still shows');
        await tester.pumpWidget(const MaterialApp(home: SizedBox()));
        expect(screen.calls, [true, false]);
      },
    );

    testWidgets(
      'a page that is not secure leaves the window alone, unless screenshotBlockingDefault is on [SEC-091]',
      (tester) async {
        await tester.pumpWidget(MaterialApp(home: page(secure: false)));
        expect(screen.calls, isEmpty);
        settings.value = _settings({'screenshotBlockingDefault': true});
        await tester.pump();
        expect(screen.calls, [true]);
        settings.value = _settings();
        await tester.pump();
        expect(screen.calls, [true, false]);
        expect(
          find.text('Locked'),
          findsNothing,
          reason: 'only secure pages lock',
        );
      },
    );

    testWidgets('rebuilding a page does not toggle the protection [SEC-091]', (
      tester,
    ) async {
      for (var i = 0; i < 3; i++) {
        await tester.pumpWidget(MaterialApp(home: page(secure: true)));
      }
      expect(screen.calls, [true]);
    });

    testWidgets(
      'after the idle time a secure page shows a lock screen in place of its content; unlocking restores it as it was [SEC-094]',
      (tester) async {
        await tester.pumpWidget(MaterialApp(home: page(secure: true)));
        await tester.enterText(find.byType(TextField), 'draft');
        expect(find.text('Locked'), findsNothing);

        clock.advance(const Duration(seconds: 120));
        await tester.pump();
        expect(find.text('Locked'), findsOneWidget);
        expect(
          find.byType(TextField),
          findsNothing,
          reason: 'hidden, not painted',
        );
        expect(find.byType(TextField, skipOffstage: false), findsOneWidget);
        // The prompt opened by itself and failed; the screen says so.
        await tester.pump();
        expect(bio.asked, ['Unlock to continue']);
        expect(find.text('That did not work. Try again.'), findsOneWidget);

        bio.outcome = BiometricOutcome.success;
        await tester.tap(find.text('Unlock'));
        await tester.pump();
        await tester.pump();
        expect(find.text('Locked'), findsNothing);
        expect(
          find.text('draft'),
          findsOneWidget,
          reason: 'the entry survived',
        );
      },
    );

    testWidgets(
      'a device with no screen lock stays locked and says what to set up [SEC-094]',
      (tester) async {
        bio.outcome = BiometricOutcome.unavailable;
        await tester.pumpWidget(MaterialApp(home: page(secure: true)));
        clock.advance(const Duration(seconds: 120));
        await tester.pump();
        await tester.pump();
        expect(
          find.text('Set up a screen lock on this device to continue.'),
          findsOneWidget,
        );
        expect(find.byType(TextField), findsNothing);
      },
    );

    testWidgets(
      'returning to the foreground after the idle time locks [SEC-094]',
      (tester) async {
        await tester.pumpWidget(MaterialApp(home: page(secure: true)));
        clock.t += const Duration(seconds: 300);
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.resumed,
        );
        await tester.pump();
        expect(find.text('Locked'), findsOneWidget);
      },
    );

    testWidgets(
      'leaving the secure page clears the lock for the next one [SEC-094]',
      (tester) async {
        await tester.pumpWidget(MaterialApp(home: page(secure: true)));
        clock.advance(const Duration(seconds: 120));
        await tester.pump();
        expect(ui.lock.locked, isTrue);
        await tester.pumpWidget(const MaterialApp(home: SizedBox()));
        expect(ui.lock.locked, isFalse);
      },
    );
  });
}
