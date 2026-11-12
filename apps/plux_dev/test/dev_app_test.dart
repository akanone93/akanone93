// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:isolate';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_dev/dev.dart';
import 'package:plux_devtools/plux_devtools.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// Whether [http] and [socket] can be sent to and called on another isolate,
/// as the runtime's sync and data workers do. A top-level function, so that
/// the closure it sends captures nothing else.
Future<bool> _crossIsolates(
  Object? Function() http,
  HttpClient Function() socket,
) => Isolate.run(() {
  http();
  socket().close(force: true);
  return true;
});

const String _keys =
    '[{"keyId":"d1","algorithm":"ed25519","role":"root","publicKey":'
    '"0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}]';

const _link =
    'plux-dev://pair?server=https%3A%2F%2Fplux.example.org&token=a.e30.c';

/// A signed-in phone with canned answers that records its calls.
final class FakePhone implements SignedInPhone {
  FakePhone({
    this.list = const [
      DevPhoneApp(id: 'app_1', name: 'Shop', live: true),
      DevPhoneApp(id: 'app_2', name: 'Bank', live: false),
    ],
    this.needsNumber = true,
  });

  List<DevPhoneApp> list;
  bool needsNumber;
  DevPhoneException? joinError;
  DevPhoneException? appsError;
  final List<String> calls = [];

  @override
  final DevPhoneAccount account = DevPhoneAccount(
    server: Uri.parse('https://plux.example.org'),
    phoneId: 'p1',
    userDisplay: 'Ada',
    organizationName: 'Acme',
  );

  @override
  Future<List<DevPhoneApp>> apps() async {
    calls.add('apps');
    if (appsError case final e?) throw e;
    return list;
  }

  @override
  Future<DevJoin> requestJoin(String appId) async {
    calls.add('join:$appId');
    if (joinError case final e?) throw e;
    return DevJoin(
      requestId: needsNumber ? 'r1' : '',
      sessionId: 's1',
      environment: 'development',
      options: needsNumber ? const [12, 42, 77] : const [],
      expiresAt: null,
      pairingLink: needsNumber ? null : _link,
    );
  }

  @override
  Future<String> completeJoin(DevJoin join, int number) async {
    calls.add('pick:$number');
    if (number != 42) {
      throw const DevPhoneException(DevPhoneFailure.wrongNumber, 'PLX-9210');
    }
    return _link;
  }

  @override
  Future<void> signOut() async => calls.add('signOut');
}

final class FakeClaim implements PhoneSignIn {
  FakeClaim(this.phone);

  final FakePhone phone;
  int? picked;

  @override
  List<int> get options => const [12, 42, 77];

  @override
  DateTime? get expiresAt => null;

  @override
  String get userDisplay => 'Ada';

  @override
  String get organizationName => 'Acme';

  @override
  Future<SignedInPhone> complete(int number) async {
    picked = number;
    if (number != 42) {
      throw const DevPhoneException(DevPhoneFailure.wrongNumber, 'PLX-9210');
    }
    return phone;
  }
}

final class FakeDirectory implements PhoneDirectory {
  FakeDirectory({this.stored, this.claimed});

  SignedInPhone? stored;
  FakeClaim? claimed;
  final List<String> links = [];

  @override
  Future<SignedInPhone?> restore() async => stored;

  @override
  Future<PhoneSignIn> claim(DevSignInLink link) async {
    links.add(link.grant);
    return claimed!;
  }
}

final class _Diagnostics implements PluxDiagnostics {
  @override
  final ValueNotifier<List<PluxDiagnostic>> log = ValueNotifier([
    PluxDiagnostic(
      at: DateTime(2026, 11, 12, 9, 30),
      error: const PluxException(
        PluxErrorCode.stateWriteTypeMismatch,
        'bad value',
      ),
    ),
  ]);
  @override
  final ValueNotifier<PluxReleaseInfo?> release = ValueNotifier(
    const PluxReleaseInfo(sequence: 7, plugins: []),
  );
  @override
  final ValueNotifier<SyncEvent?> syncStatus = ValueNotifier(
    const SyncActivated(7),
  );
  @override
  final ValueNotifier<List<PluxActionTrace>> actionTraces = ValueNotifier(
    const [],
  );
}

/// The Dev app's flow (DEV-001, DEV-002, ADR-0104).
void main() {
  final paired = <String>[];
  final ended = <String>[];

  PluxDevSession fakeSession() => PluxDevSession(
    devRootKeys: const [],
    extensions: <String, DevCommandHandler>{},
    isAvailable: true,
    pairer: (link, {required devRootKeys, required extensions}) async {
      paired.add(link);
      return (
        sessionId: 's1',
        deviceId: 'd1',
        close: () async => ended.add(link),
      );
    },
  );

  setUp(() {
    paired.clear();
    ended.clear();
  });

  Widget app({
    required PhoneDirectory phones,
    Future<Object?> Function(PluxConfig config)? start,
    List<PluxPublicKey>? roots,
    Future<String?> Function(NavigatorState navigator)? scan,
    ValueListenable<String?>? entry,
  }) => DevApp(
    rootKeys: roots ?? parseDevelopmentRootKeys(_keys),
    phones: phones,
    deviceName: 'Pixel 8',
    startRuntime: start ?? (_) async => null,
    stopRuntime: () async {},
    scope: (child) => child,
    entryRoute: () => entry ?? ValueNotifier<String?>(null),
    scan:
        scan ??
        (_) async =>
            'plux-dev://signin?server=https%3A%2F%2Fplux.example.org&grant=g1',
    newSession: fakeSession,
    diagnostics: _Diagnostics.new,
  );

  Future<void> settle(WidgetTester tester) async {
    for (var i = 0; i < 6; i++) {
      await tester.pump(const Duration(milliseconds: 50));
    }
  }

  testWidgets('[DEV-001] without development roots it opens nothing', (
    tester,
  ) async {
    await tester.pumpWidget(app(phones: FakeDirectory(), roots: const []));
    await settle(tester);
    expect(find.textContaining('No development roots configured'), findsOne);
    expect(find.byKey(const Key('scan')), findsNothing);
  });

  testWidgets(
    '[DEV-002] connecting: scan Studio\'s code, pick its number, see the apps',
    (tester) async {
      final phone = FakePhone();
      final claim = FakeClaim(phone);
      final phones = FakeDirectory(claimed: claim);
      await tester.pumpWidget(app(phones: phones));
      await settle(tester);
      expect(find.text('Plux Dev'), findsWidgets);
      await tester.tap(find.byKey(const Key('scan')));
      await settle(tester);
      expect(phones.links, ['g1']);
      expect(find.text('Pick the number shown in Studio'), findsOneWidget);
      expect(find.textContaining('Ada · Acme'), findsOneWidget);
      await tester.tap(find.byKey(const Key('number-42')));
      await settle(tester);
      expect(claim.picked, 42);
      expect(find.text('Your apps'), findsWidgets);
      expect(find.text('Shop'), findsOneWidget);
      expect(find.text('Open in Studio'), findsOneWidget);
      expect(find.text('Not open in Studio'), findsOneWidget);
    },
  );

  testWidgets(
    '[DEV-002] a wrong number does not connect, and says what to do',
    (tester) async {
      final phones = FakeDirectory(claimed: FakeClaim(FakePhone()));
      await tester.pumpWidget(app(phones: phones));
      await settle(tester);
      await tester.tap(find.byKey(const Key('scan')));
      await settle(tester);
      await tester.tap(find.byKey(const Key('number-12')));
      await settle(tester);
      expect(
        find.textContaining("That wasn't the number Studio showed"),
        findsOneWidget,
      );
      expect(find.text('Your apps'), findsNothing);
    },
  );

  testWidgets('[DEV-002] a code that is not a sign-in code is refused', (
    tester,
  ) async {
    final phones = FakeDirectory();
    await tester.pumpWidget(app(phones: phones, scan: (_) async => _link));
    await settle(tester);
    await tester.tap(find.byKey(const Key('scan')));
    await settle(tester);
    expect(phones.links, isEmpty);
    expect(find.textContaining("isn't a Plux Dev code"), findsOneWidget);
  });

  testWidgets(
    '[DEV-001] tapping an app joins with the number and opens it, with the dev button',
    (tester) async {
      final phone = FakePhone();
      PluxConfig? started;
      await tester.pumpWidget(
        app(
          phones: FakeDirectory(stored: phone),
          start: (config) async {
            started = config;
            return null;
          },
        ),
      );
      await settle(tester);
      await tester.tap(find.text('Shop'));
      await settle(tester);
      expect(find.text('Pick the number shown in Studio'), findsOneWidget);
      await tester.tap(find.byKey(const Key('number-42')));
      await settle(tester);
      expect(phone.calls, containsAllInOrder(['join:app_1', 'pick:42']));
      expect(started!.appId, 'app_1');
      expect(started!.environment, 'development');
      expect(
        started!.endpoint.toString(),
        startsWith('https://plux.example.org'),
      );
      expect(paired, [_link]);
      // No page between the tap and the app: it opens at its entry screen as
      // soon as the release names one.
      expect(find.text('Opening Shop…'), findsOneWidget);
      expect(find.byKey(const Key('dev-button')), findsOneWidget);
      // The runtime starts its sync worker with these on another isolate: a
      // factory that closes over the app's state fails there.
      final http = started!.httpClient!;
      final socket = started!.webSocketClient!;
      expect(await tester.runAsync(() => _crossIsolates(http, socket)), isTrue);

      // One sheet holds the actions and the diagnostics, in tabs: no second
      // panel over the app.
      await tester.tap(find.byKey(const Key('dev-button')));
      await settle(tester);
      expect(find.byKey(const Key('reconnect')), findsOneWidget);
      expect(find.byKey(const Key('tab-connection')), findsOneWidget);
      await tester.tap(find.byKey(const Key('tab-problems')));
      await settle(tester);
      expect(find.text('PLX-5301'), findsOneWidget);
      expect(find.text('bad value'), findsOneWidget);
      await tester.tap(find.byKey(const Key('tab-sync')));
      await settle(tester);
      expect(find.text('release 7 active'), findsOneWidget);
      await tester.tap(find.byKey(const Key('leave')));
      await settle(tester);
      expect(ended, [_link]);
      expect(find.text('Your apps'), findsWidgets);
    },
  );

  testWidgets(
    '[DEV-002] a phone out of the session joins again from the sheet, with the number Studio shows',
    (tester) async {
      final phone = FakePhone();
      var starts = 0;
      await tester.pumpWidget(
        app(
          phones: FakeDirectory(stored: phone),
          start: (_) async {
            starts++;
            return null;
          },
        ),
      );
      await settle(tester);
      await tester.tap(find.text('Shop'));
      await settle(tester);
      await tester.tap(find.byKey(const Key('number-42')));
      await settle(tester);
      expect(paired, [_link]);

      // The link is over (Offline): the sheet offers to join again, and the
      // number picker comes up by itself when Studio asks for one.
      await tester.tap(find.byKey(const Key('dev-button')));
      await settle(tester);
      expect(find.text('Join again'), findsOneWidget);
      await tester.tap(find.byKey(const Key('reconnect')));
      await settle(tester);
      expect(find.text('Pick the number shown in Studio'), findsOneWidget);
      await tester.tap(find.byKey(const Key('number-42')));
      await settle(tester);
      expect(phone.calls.where((c) => c == 'join:app_1'), hasLength(2));
      // The old pairing ends and a new one starts; the app keeps running.
      expect(ended, [_link]);
      expect(paired, [_link, _link]);
      expect(starts, 1);
    },
  );

  testWidgets(
    '[DEV-002] joining again while Studio is closed says so, and does not ask for a number',
    (tester) async {
      final phone = FakePhone();
      await tester.pumpWidget(app(phones: FakeDirectory(stored: phone)));
      await settle(tester);
      await tester.tap(find.text('Shop'));
      await settle(tester);
      await tester.tap(find.byKey(const Key('number-42')));
      await settle(tester);
      phone.joinError = const DevPhoneException(
        DevPhoneFailure.noLiveSession,
        'PLX-9211',
      );
      await tester.tap(find.byKey(const Key('dev-button')));
      await settle(tester);
      await tester.tap(find.byKey(const Key('reconnect')));
      await settle(tester);
      expect(find.text('Pick the number shown in Studio'), findsNothing);
      expect(
        find.text('Open Shop in Studio; this phone joins again by itself.'),
        findsOneWidget,
      );
      // It tries again by itself, here after the wait.
      phone.joinError = null;
      phone.needsNumber = false;
      await tester.pump(rejoinInterval);
      await settle(tester);
      expect(paired, [_link, _link]);
    },
  );

  testWidgets(
    '[DEV-002] a phone that joined the session before opens the app without a number',
    (tester) async {
      final phone = FakePhone(needsNumber: false);
      await tester.pumpWidget(app(phones: FakeDirectory(stored: phone)));
      await settle(tester);
      await tester.tap(find.text('Shop'));
      await settle(tester);
      expect(find.text('Pick the number shown in Studio'), findsNothing);
      expect(paired, [_link]);
      expect(find.text('Opening Shop…'), findsOneWidget);
    },
  );

  testWidgets(
    '[DEV-002] without the app open in Studio, the tap says to open it',
    (tester) async {
      final phone = FakePhone()
        ..joinError = const DevPhoneException(
          DevPhoneFailure.noLiveSession,
          'PLX-9211',
        );
      await tester.pumpWidget(app(phones: FakeDirectory(stored: phone)));
      await settle(tester);
      await tester.tap(find.text('Bank'));
      await settle(tester);
      expect(find.textContaining('Open Bank in Studio first'), findsOneWidget);
      expect(paired, isEmpty);
    },
  );

  testWidgets(
    '[DEV-002] a phone whose sign-in lapsed is back at the connect screen',
    (tester) async {
      final phone = FakePhone()
        ..appsError = const DevPhoneException(
          DevPhoneFailure.signedOut,
          'expired',
        );
      await tester.pumpWidget(app(phones: FakeDirectory(stored: phone)));
      await settle(tester);
      expect(find.text('Plux Dev'), findsWidgets);
      expect(find.textContaining('This phone was signed out'), findsOneWidget);
    },
  );

  testWidgets(
    '[DEV-001] a runtime that does not start is reported, and the list stays',
    (tester) async {
      await tester.pumpWidget(
        app(
          phones: FakeDirectory(stored: FakePhone(needsNumber: false)),
          start: (_) async => throw StateError('no keys'),
        ),
      );
      await settle(tester);
      await tester.tap(find.text('Shop'));
      await settle(tester);
      expect(find.textContaining('Shop could not start'), findsOneWidget);
      expect(find.text('Your apps'), findsWidgets);
    },
  );

  test('[DEV-002] a pairing code names the app to open', () {
    // A token's three parts; only the middle one, {"app":"app_9"}, is read.
    final payload = base64Url
        .encode(utf8.encode('{"app":"app_9"}'))
        .replaceAll('=', '');
    String linkOf(String token) =>
        'plux-dev://pair?server=https%3A%2F%2Fa.b&token=$token';
    expect(pairingAppOf(linkOf('h.$payload.s')), 'app_9');
    expect(pairingAppOf(linkOf('nope')), isNull);
    expect(pairingAppOf('not a link'), isNull);
  });
}
