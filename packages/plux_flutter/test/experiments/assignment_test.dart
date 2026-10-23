// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/experiments/assignment.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';

ControlExperiment experiment({
  String key = 'cta',
  String salt = 'cta-1',
  List<(String, int)> weights = const [
    ('control', 5000),
    ('bold', 3000),
    ('short', 2000),
  ],
  String layer = '',
  int layerStart = 0,
  int layerEnd = 0,
  int holdoutBp = 0,
  TargetingRule? segment,
  DateTime? startsAt,
  DateTime? endsAt,
}) => ControlExperiment(
  key: key,
  salt: salt,
  variants: [
    for (final (i, (n, w)) in weights.indexed)
      ControlVariant(name: n, weightBp: w, control: i == 0),
  ],
  layer: layer,
  layerStart: layerStart,
  layerEnd: layerEnd,
  holdoutBp: holdoutBp,
  segment: segment,
  startsAt: startsAt,
  endsAt: endsAt,
);

/// The subjects of the deterministic population every test draws from.
final subjects = [for (var i = 0; i < 20000; i++) 'user-$i'];

void main() {
  group('bucketing', () {
    test('a bucket is the first eight bytes of sha256(salt ‖ subject) '
        'modulo 10000 [ABT-002]', () {
      for (final (salt, subject) in [
        ('cta-1', 'a'),
        ('', 'device'),
        ('s' * 40, 'ü-user'),
        ('x', '42'),
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
  });

  group('weighted pick', () {
    test('the weights split the buckets in order, and a zero weight gets '
        'none [ABT-002]', () {
      final e = experiment(weights: [('a', 5000), ('b', 0), ('c', 5000)]);
      expect(weightedPick(e, 0), 'a');
      expect(weightedPick(e, 4999), 'a');
      expect(weightedPick(e, 5000), 'c');
      expect(weightedPick(e, 9999), 'c');
    });

    test('weights that stop short of the bucket pick nothing [ABT-002]', () {
      expect(
        weightedPick(experiment(weights: [('a', 0), ('b', 0)]), 10),
        isNull,
      );
      expect(weightedPick(experiment(weights: [('a', 5000)]), 5000), isNull);
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
        layerEnd: 4000,
      );
      final second = experiment(
        key: 'b',
        salt: 'b-1',
        layer: 'checkout',
        layerStart: 4000,
        layerEnd: 10000,
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
      final e = experiment(layer: 'solo', layerEnd: 5000);
      final enrolled = subjects.where(
        (s) => drawVariant(e, s, const {}) != null,
      );
      expect(enrolled.length / subjects.length, closeTo(0.5, 0.015));
    });

    test('the global holdout keeps its share out of every experiment, the '
        'same subjects in each [ABT-004]', () {
      const holdout = ControlHoldout(salt: 'holdout-1', basisPoints: 1000);
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

    test('an experiment holdout shows that share the control, whatever the '
        'weights [ABT-004]', () {
      final e = experiment(
        weights: [('control', 0), ('bold', 10000)],
        holdoutBp: 2000,
      );
      final drawn = [for (final s in subjects) drawVariant(e, s, const {})];
      expect(
        drawn.where((v) => v == 'control').length / subjects.length,
        closeTo(0.2, 0.012),
      );
      expect(
        drawn.where((v) => v == 'bold').length / subjects.length,
        closeTo(0.8, 0.012),
      );
      for (final s in subjects.take(500)) {
        final held = bucketOf('cta-1:holdout', s) < 2000;
        expect(drawVariant(e, s, const {}), held ? 'control' : 'bold');
      }
    });

    test('the control is the variant marked as such, not the first '
        '[ABT-004]', () {
      final e = ControlExperiment(
        key: 'k',
        salt: 'k-1',
        variants: const [
          ControlVariant(name: 'wide', weightBp: 0),
          ControlVariant(name: 'base', weightBp: 10000, control: true),
        ],
        holdoutBp: 10000,
      );
      expect(drawVariant(e, 'someone', const {}), 'base');
      expect(e.control.name, 'base');
    });
  });

  group('segment', () {
    test('a subject outside the segment is not in the experiment, one inside '
        'draws as everyone does [ABT-001] [ABT-002]', () {
      const ios = TargetingCondition('os', TargetingOperator.eq, 'ios');
      final e = experiment(segment: ios);
      for (final s in subjects.take(200)) {
        expect(drawVariant(e, s, const {'os': 'android'}), isNull);
        expect(drawVariant(e, s, const {}), isNull);
        expect(
          drawVariant(e, s, const {'os': 'ios'}),
          drawVariant(experiment(), s, const {}),
        );
      }
    });

    test('a segment this runtime cannot evaluate matches no device '
        '[ABT-001]', () {
      final e = experiment(segment: const TargetingNever());
      expect(drawVariant(e, 'a', const {'os': 'ios'}), isNull);
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
