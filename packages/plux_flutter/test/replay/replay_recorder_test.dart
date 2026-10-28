// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:math' as math;

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/core/config.dart';
import 'package:plux_flutter/src/replay/replay_outbox.dart';
import 'package:plux_flutter/src/replay/replay_recorder.dart';

const _bounds = ReplayBounds(events: 5, bytes: 100000, perDay: 2);

class _Rig {
  _Rig({
    PluxConsent consent = const PluxConsent(replay: true),
    this.enabled = true,
    ReplayBounds bounds = _bounds,
  }) {
    recorder = ReplayRecorder(
      post: posted.add,
      withdraw: () => withdrawn++,
      enabled: () => enabled,
      bounds: () => bounds,
      sampling: () => 1,
      consent: consent,
      random: math.Random(1),
      clock: () => now,
    );
  }

  late final ReplayRecorder recorder;
  final List<Map<String, Object?>> posted = [];
  int withdrawn = 0;
  bool enabled;
  DateTime now = DateTime.utc(2026, 10, 28, 12);
}

void main() {
  // Verifies: ANL-050, SEC-161.
  group('consent gating', () {
    test('nothing is recorded without the replay consent', () {
      final r = _Rig(consent: const PluxConsent(analytics: true));
      r.recorder.start();
      r.recorder.screen('/home', 'home');
      r.recorder.finish();
      expect(r.recorder.recording, isFalse);
      expect(r.posted, isEmpty);
    });

    test('nothing is recorded when the security profile forbids it', () {
      final r = _Rig(enabled: false)..recorder.start();
      expect(r.recorder.recording, isFalse);
    });

    test(
      'withdrawing the consent discards the replay and asks for a purge',
      () {
        final r = _Rig()..recorder.start();
        r.recorder.screen('/home', 'home');
        r.recorder.consent = PluxConsent.necessaryOnly;
        expect(r.recorder.recording, isFalse);
        r.recorder.finish();
        expect(r.posted, isEmpty);
        expect(r.withdrawn, 1);
      },
    );

    test('a replay carries the consent it was recorded under', () {
      final r = _Rig()..recorder.start();
      r.recorder.screen('/home', 'home');
      r.now = r.now.add(const Duration(seconds: 2));
      r.recorder.finish();
      expect(r.posted, hasLength(1));
      expect(r.posted.single['consent'], {'replay': true});
      expect(r.posted.single['durationMs'], 2000);
    });
  });

  // Verifies: SCH-012, ANL-050.
  group('redaction', () {
    test('sensitive state and text inputs leave no value', () {
      final r = _Rig(
        bounds: const ReplayBounds(events: 50, bytes: 100000, perDay: 2),
      )..recorder.start();
      r.recorder
        ..state('count', 3)
        ..state('agreed', true)
        ..state('label', 'Ada Lovelace')
        ..state('password', 'hunter2')
        ..state('note', 'hunter2', sensitive: true)
        ..state('account', 4111111111111111)
        ..layout(const [
          ReplayNode(
            path: '0/1',
            widget: 'Column',
            x: 0,
            y: 0,
            width: 10,
            height: 10,
          ),
          ReplayNode(
            path: '0/2',
            widget: 'TextField',
            x: 0,
            y: 0,
            width: 10,
            height: 10,
          ),
          ReplayNode(
            path: '0/2/0',
            widget: 'Text',
            x: 0,
            y: 0,
            width: 10,
            height: 10,
          ),
        ])
        ..tap('0/2', 0.5, 0.5)
        ..finish();
      final replay = r.posted.single;
      final json = jsonEncode(replay);
      expect(json, isNot(contains('hunter2')));
      expect(json, isNot(contains('Ada Lovelace')));
      expect(json, isNot(contains('4111111111111111')));
      final events = (replay['events']! as List).cast<Map<String, Object?>>();
      final states = {
        for (final e in events.where((e) => e['kind'] == 'state')) e['name']: e,
      };
      expect(
        states.keys,
        unorderedEquals(['count', 'agreed', 'label', 'account']),
      );
      expect(states['count']!['value'], '3');
      expect(states['label']!.containsKey('value'), isFalse);
      expect(states['account']!.containsKey('value'), isFalse);
      final nodes =
          (events.firstWhere((e) => e['kind'] == 'layout')['nodes']! as List)
              .cast<Map<String, Object?>>();
      expect(nodes.map((n) => n['masked'] ?? false), [false, true, true]);
      expect(events.where((e) => e['kind'] == 'input'), hasLength(1));
    });
  });

  // Verifies: ANL-050, LIM-001.
  group('bounds', () {
    test(
      'a replay stops at replay.eventsPerReplay and is marked truncated',
      () {
        final r = _Rig()..recorder.start();
        for (var i = 0; i < 9; i++) {
          r.recorder.tap('0/$i', 0, 0);
        }
        expect(r.recorder.recording, isFalse);
        expect(r.posted.single['truncated'], isTrue);
        expect((r.posted.single['events']! as List).length, 5);
      },
    );

    test('a replay stops at replay.bytesPerReplay', () {
      final r = _Rig(
        bounds: const ReplayBounds(events: 100, bytes: 150, perDay: 2),
      )..recorder.start();
      r.recorder
        ..tap('0/1', 0, 0)
        ..tap('0/2', 0, 0)
        ..tap('0/3', 0, 0);
      expect(r.posted.single['truncated'], isTrue);
      expect((r.posted.single['events']! as List).length, lessThan(3));
    });

    test('no more than replay.perDevicePerDay replays start in a day', () {
      final r = _Rig();
      for (var i = 0; i < 3; i++) {
        r.recorder.start();
        r.recorder.screen('/home', 'home');
        r.recorder.finish();
      }
      expect(r.posted, hasLength(2));
      r.now = r.now.add(const Duration(days: 1, minutes: 1));
      r.recorder.start();
      expect(r.recorder.recording, isTrue);
    });
  });

  group('outbox', () {
    test('keeps at most the bound, oldest dropped first, and purges', () async {
      final dir = Directory.systemTemp.createTempSync('plux_replay');
      addTearDown(() => dir.deleteSync(recursive: true));
      final box = ReplayOutbox(dir.path);
      for (var i = 0; i < 3; i++) {
        box.append({'id': '00000000-0000-4000-8000-00000000000$i'}, 2);
        await Future<void>.delayed(const Duration(milliseconds: 20));
      }
      expect(box.length, 2);
      box.purge();
      expect(box.length, 0);
    });
  });
}
