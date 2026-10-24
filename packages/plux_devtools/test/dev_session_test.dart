// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_devtools/plux_devtools.dart';
import 'package:plux_flutter/plux_flutter.dart';

const _link = 'plux-dev://pair?server=https%3A%2F%2Fplux.example&token=abc';

final class _Fake {
  final links = <String>[];
  var closed = 0;
  Object? failure;
  Map<String, DevCommandHandler>? extensions;
  List<PluxPublicKey>? keys;

  Future<PluxDevJoined> pair(
    String link, {
    required List<PluxPublicKey> devRootKeys,
    required Map<String, DevCommandHandler> extensions,
  }) async {
    links.add(link);
    keys = devRootKeys;
    this.extensions = extensions;
    final f = failure;
    if (f != null) throw f; // ignore: only_throw_errors
    return (sessionId: 's-1', deviceId: 'd-1', close: () async => closed++);
  }
}

final class _Handler implements DevCommandHandler {
  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async =>
      const {};
}

void main() {
  late _Fake fake;
  setUp(() => fake = _Fake());

  PluxDevSession session({bool enabled = true}) => PluxDevSession(
    devRootKeys: const [],
    extensions: {'custom': _Handler()},
    pairer: fake.pair,
    isAvailable: enabled,
  );

  test('pairs with a valid link and reaches paired [DEV-040]', () async {
    final s = session();
    final states = <Type>[];
    s.state.addListener(() => states.add(s.state.value.runtimeType));
    await s.pair(_link);
    expect(states, [PluxDevPairingNow, PluxDevPaired]);
    final paired = s.state.value as PluxDevPaired;
    expect(paired.sessionId, 's-1');
    expect(paired.deviceId, 'd-1');
    expect(fake.links, [_link]);
    expect(fake.extensions, contains('custom'));
    expect(fake.keys, isEmpty);
  });

  test('pairs with a typed code [DEV-040] [DEV-002]', () async {
    final s = session();
    await s.pair('7kq2-m9xd');
    expect(s.state.value, isA<PluxDevPaired>());
    expect(fake.links, ['7kq2-m9xd']);
  });

  test(
    'text that is neither a link nor a code pairs nothing [DEV-002]',
    () async {
      final s = session();
      await s.pair('7KQ2-M9X');
      expect((s.state.value as PluxDevError).code, 'invalid_link');
      expect(fake.links, isEmpty);
    },
  );

  test(
    'an invalid link is an error state and pairs nothing [DEV-040]',
    () async {
      final s = session();
      await s.pair('https://example.com/not-a-link');
      expect((s.state.value as PluxDevError).code, 'invalid_link');
      expect(fake.links, isEmpty);
    },
  );

  test('a failed pairing is an error state with its code [DEV-040]', () async {
    fake.failure = const PluxException(
      PluxErrorCode.hotApplyRefused,
      'refused',
    );
    final s = session();
    await s.pair(_link);
    expect(
      (s.state.value as PluxDevError).code,
      PluxErrorCode.hotApplyRefused.id,
    );
    fake.failure = StateError('no channel');
    await s.pair(_link);
    expect((s.state.value as PluxDevError).code, 'unavailable');
  });

  test('end() closes the channel and discards the overlay [DEV-040]', () async {
    final s = session();
    await s.pair(_link);
    await s.end();
    expect(fake.closed, 1);
    expect(s.state.value, isA<PluxDevIdle>());
    await s.end();
    expect(fake.closed, 1, reason: 'ending twice closes once');
  });

  test('a second pair while paired is ignored [DEV-040]', () async {
    final s = session();
    await s.pair(_link);
    await s.pair(_link);
    expect(fake.links, hasLength(1));
  });

  test('is inert in a build without devtools [DEV-040] [DEV-050]', () async {
    final s = session(enabled: false);
    expect(s.isAvailable, isFalse);
    await s.pair(_link);
    expect(s.state.value, isA<PluxDevIdle>());
    expect(fake.links, isEmpty);
    await s.end();
    expect(fake.closed, 0);
  });

  testWidgets(
    'the overlay pairs from a pasted link and ends the session [DEV-040]',
    (tester) async {
      final s = session();
      await tester.pumpWidget(
        MaterialApp(
          home: const Scaffold(body: Text('host')),
          builder: (context, child) =>
              PluxDevtools(diagnostics: null, session: s, child: child!),
        ),
      );
      await tester.tap(find.text('Plux –'));
      await tester.pump();
      await tester.tap(find.text('session'));
      await tester.pump();
      expect(find.text('Not paired'), findsOneWidget);
      expect(find.text('Scan QR code'), findsNothing);
      await tester.tap(find.text('Pair with dev session'));
      await tester.pump();
      await tester.enterText(find.byType(TextField), _link);
      await tester.tap(find.text('Pair'));
      await tester.pumpAndSettle();
      expect(find.textContaining('Paired with session s-1'), findsOneWidget);
      await tester.tap(find.text('End dev session'));
      await tester.pumpAndSettle();
      expect(fake.closed, 1);
      expect(find.text('Not paired'), findsOneWidget);
    },
  );

  testWidgets(
    'the overlay offers a scan when the host provides one [DEV-040]',
    (tester) async {
      final s = session();
      await tester.pumpWidget(
        MaterialApp(
          home: const Scaffold(body: Text('host')),
          builder: (context, child) => PluxDevtools(
            session: s,
            scan: (context) async => _link,
            child: child!,
          ),
        ),
      );
      await tester.tap(find.text('Plux –'));
      await tester.pump();
      await tester.tap(find.text('session'));
      await tester.pump();
      await tester.tap(find.text('Scan QR code'));
      await tester.pumpAndSettle();
      expect(s.state.value, isA<PluxDevPaired>());
    },
  );
}
