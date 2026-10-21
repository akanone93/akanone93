// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:math' show Random;

import 'package:flutter/painting.dart' show Locale;
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/experiments/experiment_service.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';
import 'package:plux_flutter/src/experiments/targeting_facts.dart';
import 'package:plux_flutter/src/state/persistence.dart';
import 'package:plux_flutter/src/telemetry/events.dart';
import 'package:plux_flutter/src/telemetry/recorder.dart';

TargetingRule rule(Map<String, Object?> json) => TargetingRule.fromJson(json)!;

Map<String, Object?> cond(String attribute, String operator, Object value) => {
  'attribute': attribute,
  'operator': operator,
  'value': value,
};

ControlSnapshot snapshot(List<ControlFlag> flags) =>
    ControlSnapshot(version: 1, expiresAt: DateTime.utc(2030), flags: flags);

/// A rule that holds on every device the tests describe.
final Map<String, Object?> anyone = cond('channel', 'neq', 'none');

/// A flag whose type follows its [defaultValue], with the [segments] (a rule
/// and a value each) in precedence order.
ControlFlag flag(
  String key,
  Object? defaultValue, [
  List<(Map<String, Object?>, Object?)> segments = const [],
]) => ControlFlag(
  key: key,
  type: switch (defaultValue) {
    bool() => FlagType.boolean,
    num() => FlagType.number,
    String() => FlagType.string,
    _ => FlagType.json,
  },
  defaultValue: defaultValue,
  segments: [
    for (final (i, (r, v)) in segments.indexed)
      FlagSegment(segment: 'segment$i', rule: rule(r), value: v),
  ],
);

void main() {
  const phone = <String, Object>{
    'build': 120,
    'runtimeVersion': '1.10.0',
    'os': 'android',
    'osVersion': '14',
    'deviceClass': 'phone',
    'locale': 'de-CH',
    'region': 'CH',
    'channel': 'beta',
    'user.tier': 'gold',
  };

  group('targeting rules', () {
    test('every attribute of REL-051 can be compared [ABT-006]', () {
      for (final (json, expected) in <(Map<String, Object?>, bool)>[
        (cond('build', 'gte', 100), true),
        (cond('build', 'gte', 121), false),
        (cond('build', 'lt', 121), true),
        (cond('build', 'eq', 120), true),
        (cond('build', 'neq', 120), false),
        (cond('runtimeVersion', 'gte', '1.9.0'), true),
        (cond('runtimeVersion', 'lt', '1.9.0'), false),
        (cond('runtimeVersion', 'eq', '1.10'), true),
        (cond('os', 'eq', 'android'), true),
        (cond('os', 'in', ['ios', 'android']), true),
        (cond('os', 'notIn', ['android']), false),
        (cond('osVersion', 'gte', 14), true),
        (cond('deviceClass', 'eq', 'tablet'), false),
        (cond('locale', 'eq', 'de-CH'), true),
        (cond('region', 'in', ['CH', 'AT']), true),
        (cond('channel', 'eq', 'beta'), true),
        (cond('channel', 'neq', 'production'), true),
        (cond('user.tier', 'in', ['gold', 'silver']), true),
        (cond('user.tier', 'eq', 'bronze'), false),
      ]) {
        expect(rule(json).matches(phone), expected, reason: '$json');
      }
    });

    test('all and any combine rules, nested [ABT-006]', () {
      final beta = {
        'all': [
          cond('channel', 'eq', 'beta'),
          {
            'any': [
              cond('user.tier', 'in', ['silver']),
              cond('build', 'gte', 100),
            ],
          },
        ],
      };
      expect(rule(beta).matches(phone), isTrue);
      expect(rule(beta).matches({...phone, 'build': 99}), isFalse);
      expect(
        rule(beta).matches({...phone, 'user.tier': 'silver', 'build': 1}),
        isTrue,
      );
      expect(rule(beta).matches({...phone, 'channel': 'production'}), isFalse);
    });

    test('a fact the device lacks, or cannot order, never matches, not even '
        'with neq or notIn [ABT-006]', () {
      expect(rule(cond('region', 'neq', 'US')).matches({'os': 'ios'}), isFalse);
      expect(
        rule(cond('region', 'notIn', ['US'])).matches({'os': 'ios'}),
        isFalse,
      );
      expect(rule(cond('os', 'gte', 5)).matches(phone), isFalse);
      expect(rule(cond('build', 'gte', 'x')).matches(phone), isFalse);
    });

    test('a rule this runtime cannot read is not a rule [ABT-006]', () {
      for (final json in <Object?>[
        cond('os', 'matches', 'a.*'),
        {'attribute': 'os', 'operator': 'eq'},
        {'attribute': 'os', 'operator': 'in', 'value': 'ios'},
        {'all': <Object>[]},
        {
          'all': [
            cond('os', 'eq', 'ios'),
            {'bogus': 1},
          ],
        },
        'os == ios',
        null,
      ]) {
        expect(TargetingRule.fromJson(json), isNull, reason: '$json');
      }
    });

    test('a rule survives its stored form [ABT-006]', () {
      final json = {
        'any': [
          cond('os', 'in', ['ios']),
          cond('build', 'gt', 3),
        ],
      };
      expect(rule(json).toJson(), json);
    });

    test('device facts are the build, runtime, OS, class, locale, region, '
        'channel and the user attributes [REL-051]', () {
      final facts = targetingFacts(
        hostBuild: '42',
        channel: 'beta',
        os: 'ios',
        osVersion: '17.2',
        locale: const Locale('de', 'CH'),
        width: 820,
        user: const PluxUser(id: 'u1', attributes: {'tier': 'gold'}),
      );
      expect(facts, {
        'build': 42,
        'runtimeVersion': PluxRuntimeInfo.version,
        'os': 'ios',
        'osVersion': '17.2',
        'deviceClass': 'tablet',
        'locale': 'de-CH',
        'region': 'CH',
        'channel': 'beta',
        'user.tier': 'gold',
      });
      final bare = targetingFacts(
        hostBuild: '',
        channel: 'production',
        os: 'android',
        locale: const Locale('en'),
        width: 400,
      );
      expect(bare.keys, isNot(containsAll(['build', 'region', 'osVersion'])));
      expect(bare['deviceClass'], 'phone');
    });
  });

  group('flags', () {
    late ControlSnapshot? control;
    late TargetingFacts facts;
    late List<(String, Map<String, Object?>)> events;
    late ExperimentService service;

    setUp(() {
      control = null;
      facts = phone;
      events = [];
      service = ExperimentService(
        snapshot: () => control,
        consent: () => true,
        facts: () => facts,
        userId: () => null,
        installId: () => 'install',
        persistence: StatePersistence.inMemory(),
        record: (name, {fields = const {}}) => events.add((name, fields)),
      );
    });

    test('without a control document a flag has its app default [ABT-006]', () {
      expect(service.flags({'beta': false, 'label': 'Second'}), {
        'beta': false,
        'label': 'Second',
      });
    });

    test('the first segment whose rule matches the device gives the value; '
        'no match gives the default of the document [ABT-006]', () {
      control = snapshot([
        flag('beta', false, [
          (cond('os', 'eq', 'ios'), false),
          (cond('channel', 'eq', 'beta'), true),
          (anyone, false),
        ]),
        flag('label', 'Document label', [
          (cond('os', 'eq', 'ios'), 'iOS label'),
        ]),
      ]);
      expect(service.flags({'beta': false, 'label': 'Second'}), {
        'beta': true,
        'label': 'Document label',
      });
      facts = {...phone, 'os': 'ios'};
      expect(service.flag('label', 'Second'), 'iOS label');
      expect(
        service.flag('beta', true),
        isFalse,
        reason: 'the ios segment is first and wins',
      );
    });

    test('a segment that holds on every device decides before the default '
        '[ABT-006]', () {
      control = snapshot([
        flag('beta', false, [(anyone, true)]),
      ]);
      facts = const {'channel': 'stable'};
      expect(service.flag('beta', false), isTrue);
      facts = const {};
      expect(
        service.flag('beta', true),
        isFalse,
        reason: 'a device without the fact fails the condition',
      );
    });

    test('a value of another kind than the app default is ignored, a whole '
        'number fits a decimal flag [ABT-006]', () {
      control = snapshot([
        flag('beta', 'yes'),
        flag('ratio', 2),
        flag('count', 2.5, [(anyone, 3.0)]),
        flag('whole', 2.5),
        flag('config', {'a': 1}),
      ]);
      expect(service.flag('beta', false), isFalse);
      expect(service.flag('ratio', 1.5), 2.0);
      expect(service.flag('ratio', 1.5), isA<double>());
      expect(service.flag('count', 1), 3);
      expect(service.flag('count', 1), isA<int>());
      expect(service.flag('whole', 1), 1);
      expect(service.flag('config', <String, Object?>{}), {'a': 1});
    });

    test('a flag the app does not declare is not created [ABT-006]', () {
      control = snapshot([flag('ghost', true)]);
      expect(service.flags({'beta': false}), {'beta': false});
    });

    test('a segment with a rule this runtime cannot read, or a value of '
        'another type than the flag, is dropped [ABT-006]', () {
      final stored = ControlFlag.fromJson({
        'key': 'beta',
        'type': 'boolean',
        'defaultValue': false,
        'segments': [
          {'segment': 'a', 'rule': cond('os', 'matches', 'x'), 'value': true},
          {
            'segment': 'b',
            'rule': cond('os', 'eq', 'android'),
            'value': 'not a bool',
          },
        ],
      })!;
      expect(stored.segments, isEmpty);
      control = snapshot([stored]);
      expect(service.flag('beta', true), isFalse);
      expect(
        ControlFlag.fromJson({
          'key': 'beta',
          'type': 'boolean',
          'defaultValue': 'no',
          'segments': <Object?>[],
        }),
        isNull,
        reason: 'a default of another type than the flag is no flag',
      );
    });

    test('a new control document changes the value at once, without a '
        'restart [ABT-006]', () {
      control = snapshot([flag('beta', false)]);
      expect(service.flag('beta', true), isFalse);
      control = snapshot([flag('beta', true)]);
      expect(service.flag('beta', false), isTrue);
      control = null;
      expect(service.flag('beta', false), isFalse);
    });

    test('the snapshot keeps flags and experiments through its stored '
        'form [ABT-006]', () {
      final s = ControlSnapshot(
        version: 3,
        expiresAt: DateTime.utc(2030),
        flags: [
          flag('beta', false, [(cond('os', 'eq', 'ios'), true)]),
        ],
        experiments: const [
          ControlExperiment(
            key: 'cta',
            salt: 's',
            scope: ExperimentScope.prop,
            variants: [
              ControlVariant(name: 'control', weightBp: 10000, control: true),
            ],
            layer: 'checkout',
            layerEnd: 5000,
            holdoutBp: 100,
            segment: TargetingNever(),
          ),
        ],
        holdout: const ControlHoldout(salt: 'h', basisPoints: 500),
      );
      final again = ControlSnapshot.fromJson(s.toJson())!;
      expect(again.toJson(), s.toJson());
      expect(again.flags.single.segments.single.rule, isNotNull);
      expect(again.experiments.single.layer, 'checkout');
      expect(again.experiments.single.segment, isA<TargetingNever>());
      expect(again.holdout!.basisPoints, 500);
      // A document stored before flags existed still reads.
      expect(
        ControlSnapshot.fromJson({
          'version': 1,
          'expiresAt': 0,
          'switches': <Object?>[],
        })!.flags,
        isEmpty,
      );
    });

    test('flag_evaluated names the flag, its value type and its source, '
        'once per session, and never a user attribute [ABT-006]', () {
      control = snapshot([
        flag('beta', false, [(cond('user.tier', 'eq', 'gold'), true)]),
      ]);
      service.flags({'beta': false, 'label': 'x'});
      service.flags({'beta': false, 'label': 'x'});
      expect(events.map((e) => e.$1), everyElement('flag_evaluated'));
      expect(events.map((e) => e.$2), [
        {'flag': 'beta', 'valueType': 'bool', 'source': 'control'},
        {'flag': 'label', 'valueType': 'string', 'source': 'default'},
      ]);
      expect(events.toString(), isNot(contains('gold')));
    });

    test('flag_evaluated is sampled by the telemetry recorder and needs the '
        'experiments consent [ABT-006]', () {
      final lines = <String>[];
      TelemetryRecorder recorder(PluxConsent consent, double rate) =>
          TelemetryRecorder(
            post: lines.addAll,
            withdraw: (_) {},
            consent: consent,
            hostSampling: {'flag_evaluated': rate},
            random: _Fixed(0.4),
            clock: () => DateTime.utc(2026),
          );
      ExperimentService over(TelemetryRecorder r) => ExperimentService(
        snapshot: () => null,
        consent: () => r.consent.experiments,
        facts: () => phone,
        userId: () => null,
        installId: () => 'i',
        persistence: StatePersistence.inMemory(),
        record: (n, {fields = const {}}) => r.record(n, fields: fields),
      );
      expect(
        telemetryCategories['flag_evaluated'],
        TelemetryCategory.experiments,
      );

      over(recorder(const PluxConsent(experiments: true), 0.5)).flag('a', true);
      expect(lines, hasLength(1), reason: 'a 0.4 draw is under a 0.5 rate');
      over(recorder(const PluxConsent(experiments: true), 0.3)).flag('a', true);
      expect(lines, hasLength(1), reason: 'a 0.4 draw is over a 0.3 rate');
      over(recorder(PluxConsent.necessaryOnly, 1)).flag('a', true);
      expect(lines, hasLength(1), reason: 'no consent, no event');
    });
  });
}

final class _Fixed implements Random {
  _Fixed(this.value);

  final double value;

  @override
  double nextDouble() => value;

  @override
  bool nextBool() => false;

  @override
  int nextInt(int max) => 0;
}
