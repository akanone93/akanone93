// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/experiments/assignment.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';

ControlExperiment experiment({
  String key = 'cta',
  String salt = 'cta-1',
  List<(String, int)> weights = const [
    ('control', 50),
    ('bold', 30),
    ('short', 20),
  ],
  String? layer,
  int rangeStart = 0,
  int rangeEnd = bucketCount,
  int holdout = 0,
  DateTime? startsAt,
  DateTime? endsAt,
}) => ControlExperiment(
  key: key,
  salt: salt,
  variants: [for (final (n, w) in weights) ControlVariant(name: n, weight: w)],
  layer: layer,
  rangeStart: rangeStart,
  rangeEnd: rangeEnd,
  holdout: holdout,
  startsAt: startsAt,
  endsAt: endsAt,
);

/// The subjects of the deterministic population every test draws from.
final subjects = [for (var i = 0; i < 20000; i++) subjectOf('user-$i')];

void main() {
  group('bucketing', () {
    test('a bucket is the first eight bytes of sha256(salt ‖ subject) '
        'modulo 10000 [ABT-002]', () {
      for (final (salt, subject) in [
        ('cta-1', 'a'),
        ('', 'device'),
        ('s' * 40, 'ü-user'),
        ('x', subjectOf('42')),
      ]) {
        final bytes = sha256.convert(utf8.encode(salt + subject)).bytes;
        final expected =
            BigInt.parse(
              bytes
                  .take(8)
                  .map((b) => b.toRadixString(16).padLeft(2, '0'))
                  .join(),
              radix: 16,
            ) %
            BigInt.from(bucketCount);
        expect(bucketOf(salt, subject), expected.toInt());
      }
    });

    test('a subject is the hash of its ID, never the ID [ABT-002]', () {
      expect(subjectOf('user-1'), isNot(contains('user-1')));
      expect(subjectOf('user-1'), hasLength(64));
      expect(subjectOf('user-1'), subjectOf('user-1'));
    });
  });

  group('weighted pick', () {
    test('the weights split the buckets in order, and a zero weight gets '
        'none [ABT-002]', () {
      final e = experiment(weights: [('a', 50), ('b', 0), ('c', 50)]);
      expect(weightedPick(e, 0), 'a');
      expect(weightedPick(e, 4999), 'a');
      expect(weightedPick(e, 5000), 'c');
      expect(weightedPick(e, 9999), 'c');
    });

    test('an experiment with no weight picks nothing [ABT-002]', () {
      expect(
        weightedPick(experiment(weights: [('a', 0), ('b', 0)]), 10),
        isNull,
      );
    });
  });

  group('distribution and stickiness', () {
    test('50/30/20 weights split 20000 subjects within one point of the '
        'weights [ABT-002]', () {
      final e = experiment();
      final counts = <String, int>{};
      for (final s in subjects) {
        counts.update(
          drawVariant(e, s, const {})!,
          (n) => n + 1,
          ifAbsent: () => 1,
        );
      }
      expect(counts['control']! / subjects.length, closeTo(0.5, 0.01));
      expect(counts['bold']! / subjects.length, closeTo(0.3, 0.01));
      expect(counts['short']! / subjects.length, closeTo(0.2, 0.01));
    });

    test('a subject draws the same variant every time, whatever the '
        'device facts [ABT-002]', () {
      final e = experiment();
      for (final s in subjects.take(200)) {
        expect(
          drawVariant(e, s, const {'os': 'ios'}),
          drawVariant(e, s, const {}),
        );
      }
    });

    test('a new salt re-draws subjects independently of the old one '
        '[ABT-002]', () {
      final a = experiment(), b = experiment(salt: 'cta-2');
      final same = subjects
          .where(
            (s) => drawVariant(a, s, const {}) == drawVariant(b, s, const {}),
          )
          .length;
      // Independent draws agree about 0.5^2 + 0.3^2 + 0.2^2 = 38% of the time.
      expect(same / subjects.length, closeTo(0.38, 0.02));
    });
  });

  group('layers and holdouts', () {
    test('experiments of one layer own disjoint ranges, so no subject is in '
        'two of them [ABT-004]', () {
      final first = experiment(
        key: 'a',
        salt: 'a-1',
        layer: 'checkout',
        rangeEnd: 4000,
      );
      final second = experiment(
        key: 'b',
        salt: 'b-1',
        layer: 'checkout',
        rangeStart: 4000,
        rangeEnd: 10000,
      );
      var inFirst = 0, inSecond = 0;
      for (final s in subjects) {
        final a = drawVariant(first, s, const {});
        final b = drawVariant(second, s, const {});
        expect(a != null && b != null, isFalse, reason: 'exclusive');
        expect(a != null || b != null, isTrue, reason: 'the ranges cover all');
        if (a != null) inFirst++;
        if (b != null) inSecond++;
      }
      expect(inFirst / subjects.length, closeTo(0.4, 0.015));
      expect(inSecond / subjects.length, closeTo(0.6, 0.015));
    });

    test('an experiment alone in its layer with a half range enrols half '
        '[ABT-004]', () {
      final e = experiment(layer: 'solo', rangeEnd: 5000);
      final enrolled = subjects.where(
        (s) => drawVariant(e, s, const {}) != null,
      );
      expect(enrolled.length / subjects.length, closeTo(0.5, 0.015));
    });

    test('the global holdout keeps its share out of every experiment, the '
        'same subjects in each [ABT-004]', () {
      const holdout = ControlHoldout(salt: 'holdout-1', size: 1000);
      final held = subjects.where((s) => isHeldOut(holdout, s)).toSet();
      expect(held.length / subjects.length, closeTo(0.1, 0.01));
      expect(isHeldOut(null, subjects.first), isFalse);
      // Held out of experiments with other salts and layers alike: the
      // holdout does not depend on them.
      expect(
        held,
        subjects.where((s) => bucketOf('holdout-1', s) < 1000).toSet(),
      );
    });

    test('an experiment holdout leaves that share on the control without '
        'enrolling it [ABT-004]', () {
      final e = experiment(holdout: 2000);
      final out = subjects.where((s) => drawVariant(e, s, const {}) == null);
      expect(out.length / subjects.length, closeTo(0.2, 0.012));
    });
  });

  group('schedule', () {
    test('an experiment runs from its start until its end [ABT-001]', () {
      final e = experiment(
        startsAt: DateTime.utc(2026, 11),
        endsAt: DateTime.utc(2026, 12),
      );
      expect(isScheduled(e, DateTime.utc(2026, 10, 31)), isFalse);
      expect(isScheduled(e, DateTime.utc(2026, 11)), isTrue);
      expect(isScheduled(e, DateTime.utc(2026, 11, 30)), isTrue);
      expect(isScheduled(e, DateTime.utc(2026, 12)), isFalse);
      expect(isScheduled(experiment(), DateTime.utc(2000)), isTrue);
    });
  });
}
