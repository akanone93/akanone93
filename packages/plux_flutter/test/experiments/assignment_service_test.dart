// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/experiments/assignment.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/experiments/experiment_service.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';
import 'package:plux_flutter/src/state/persistence.dart';

ControlExperiment cta({
  String salt = 'cta-1',
  List<(String, int)> weights = const [('control', 50), ('bold', 50)],
  Map<String, Object?> boldProps = const {'label': 'Buy now'},
  TargetingRule? rule,
  DateTime? endsAt,
}) => ControlExperiment(
  key: 'cta',
  salt: salt,
  variants: [
    for (final (n, w) in weights)
      ControlVariant(
        name: n,
        weight: w,
        props: n == 'control' ? const {'label': 'Buy'} : boldProps,
      ),
  ],
  rule: rule,
  endsAt: endsAt,
);

ControlSnapshot snapshot(
  List<ControlExperiment> experiments, {
  ControlHoldout? holdout,
}) => ControlSnapshot(
  version: 1,
  expiresAt: DateTime.utc(2030),
  experiments: experiments,
  holdout: holdout,
);

/// A device the tests move: its consent, user, control document and
/// persisted state, with the events it recorded.
final class Device {
  Device({
    this.consent = true,
    this.control,
    this.user,
    this.install = 'install-1',
    StatePersistence? persistence,
    DateTime? now,
  }) : persistence = persistence ?? StatePersistence.inMemory(),
       now = now ?? DateTime.utc(2026, 11);

  bool consent;
  ControlSnapshot? control;
  String? user;
  String install;
  DateTime now;
  final StatePersistence persistence;
  final List<(String, Map<String, Object?>)> events = [];

  /// A new session on this device: nothing but the persisted state carries
  /// over.
  ExperimentService session() => ExperimentService(
    snapshot: () => control,
    consent: () => consent,
    facts: () => const {'os': 'android'},
    userId: () => user,
    installId: () => install,
    persistence: persistence,
    record: (name, {fields = const {}}) => events.add((name, fields)),
    clock: () => now,
  );
}

void main() {
  group('assignment', () {
    test('without the experiments consent everyone gets the control, nothing '
        'is stored and nothing is recorded [ABT-002]', () {
      final d = Device(consent: false, control: snapshot([cta()]));
      final s = d.session();
      expect(s.variantOf('cta'), isNull);
      expect(s.root(), {'cta': 'control', 'cta.label': 'Buy'});
      s.recordExposure('cta', 'bold');
      expect(d.events, isEmpty);
      expect(
        d.persistence.read(Persistence.persisted, 'plux.experiment/cta/cta-1'),
        isNull,
      );
    });

    test('an experiment the control document does not name, or no document, '
        'shows the control and has no root entry [ABT-002]', () {
      final d = Device();
      expect(d.session().variantOf('cta'), isNull);
      expect(d.session().root(), isEmpty);
      d.control = snapshot([cta()]);
      expect(d.session().variantOf('other'), isNull);
    });

    test('the variant is the weighted pick of the subject, the hashed install '
        'ID when no user is set [ABT-002]', () {
      final e = cta();
      for (var i = 0; i < 40; i++) {
        final d = Device(install: 'install-$i', control: snapshot([e]));
        expect(
          d.session().variantOf('cta'),
          drawVariant(e, subjectOf('install-$i'), const {'os': 'android'}),
        );
      }
    });

    test('a user ID is the subject whatever the device: the same user gets '
        'the same variant on two installs [ABT-002]', () {
      final e = cta();
      for (var i = 0; i < 40; i++) {
        final a = Device(
          user: 'user-$i',
          install: 'one',
          control: snapshot([e]),
        );
        final b = Device(
          user: 'user-$i',
          install: 'two',
          control: snapshot([e]),
        );
        expect(a.session().variantOf('cta'), b.session().variantOf('cta'));
        expect(
          a.session().variantOf('cta'),
          drawVariant(e, subjectOf('user-$i'), const {}),
        );
      }
    });

    test('an assignment is sticky across sessions, although the weights, the '
        'targeting and the user change [ABT-002]', () {
      final d = Device(control: snapshot([cta()]));
      final first = d.session().variantOf('cta')!;
      final other = first == 'bold' ? 'control' : 'bold';
      // Every subject now draws the other variant, and nobody is eligible.
      d.control = snapshot([
        cta(
          weights: [(other, 100), (first, 0)],
          rule: const TargetingCondition('os', TargetingOperator.eq, 'ios'),
        ),
      ]);
      d.user = 'signed-in-later';
      expect(d.session().variantOf('cta'), first);
    });

    test('a new salt draws afresh, and the old assignment stays stored '
        'under the old salt [ABT-002]', () {
      final d = Device(install: 'i0', control: snapshot([cta()]));
      final drawn = {
        for (var i = 0; i < 30; i++)
          Device(
            install: 'i$i',
            control: snapshot([cta()]),
          ).session().variantOf('cta'),
      };
      expect(drawn, {'control', 'bold'});
      expect(d.session().variantOf('cta'), isNotNull);
      d.control = snapshot([
        cta(salt: 'cta-2', weights: [('control', 0), ('bold', 100)]),
      ]);
      expect(d.session().variantOf('cta'), 'bold');
      expect(
        d.persistence.read(Persistence.persisted, 'plux.experiment/cta/cta-1'),
        isNotNull,
      );
    });

    test('a stored variant the experiment no longer has is drawn again '
        '[ABT-002]', () {
      final d = Device(control: snapshot([cta()]));
      d.persistence.write(
        Persistence.persisted,
        'plux.experiment/cta/cta-1',
        'experiment-assignment-1',
        'removed',
      );
      expect(d.session().variantOf('cta'), anyOf('control', 'bold'));
    });

    test('the global holdout and the schedule override a stored assignment '
        '[ABT-004]', () {
      final d = Device(control: snapshot([cta()]));
      expect(d.session().variantOf('cta'), isNotNull);
      d.control = snapshot([
        cta(),
      ], holdout: const ControlHoldout(salt: 'h', size: bucketCount));
      expect(d.session().variantOf('cta'), isNull, reason: 'everyone held out');
      d.control = snapshot([cta(endsAt: DateTime.utc(2026, 12))]);
      expect(d.session().variantOf('cta'), isNotNull);
      d.now = DateTime.utc(2026, 12);
      expect(d.session().variantOf('cta'), isNull, reason: 'it ended');
    });

    test('a layered pair never enrols a device in both [ABT-004]', () {
      ControlExperiment layered(String key, int from, int to) =>
          ControlExperiment(
            key: key,
            salt: '$key-1',
            variants: const [
              ControlVariant(name: 'control', weight: 50),
              ControlVariant(name: 'v', weight: 50),
            ],
            layer: 'checkout',
            rangeStart: from,
            rangeEnd: to,
          );
      var both = 0, either = 0;
      for (var i = 0; i < 300; i++) {
        final d = Device(
          install: 'device-$i',
          control: snapshot([layered('a', 0, 5000), layered('b', 5000, 10000)]),
        );
        final s = d.session();
        final a = s.variantOf('a') != null, b = s.variantOf('b') != null;
        if (a && b) both++;
        if (a || b) either++;
      }
      expect(both, 0);
      expect(either, 300);
    });

    test('the experiment root names the control for a device that is not '
        'enrolled and carries the props of the variant shown [ABT-002]', () {
      final d = Device(
        control: snapshot([
          cta(weights: [('control', 0), ('bold', 100)]),
        ]),
      );
      expect(d.session().root(), {'cta': 'bold', 'cta.label': 'Buy now'});
      final out = Device(
        control: snapshot([
          cta(
            rule: const TargetingCondition('os', TargetingOperator.eq, 'ios'),
          ),
        ]),
      );
      expect(out.session().root(), {'cta': 'control', 'cta.label': 'Buy'});
    });
  });

  group('exposure', () {
    test('exposure is never recorded by assigning or reading the root '
        '[ABT-002]', () {
      final d = Device(control: snapshot([cta()]));
      final s = d.session();
      s.variantOf('cta');
      s.root();
      expect(d.events, isEmpty);
    });

    test('recordExposure records the experiment and variant once per '
        'experiment per session, and again in the next session [ABT-002]', () {
      final d = Device(control: snapshot([cta()]));
      final s = d.session();
      s.recordExposure('cta', 'bold');
      s.recordExposure('cta', 'bold');
      expect(d.events.map((e) => e.$1), ['experiment_exposure']);
      expect(d.events.single.$2, {'experiment': 'cta', 'variant': 'bold'});
      d.session().recordExposure('cta', 'bold');
      expect(d.events, hasLength(2));
    });

    test('withdrawing consent stops exposure at once [ABT-002]', () {
      final d = Device(control: snapshot([cta()]));
      final s = d.session();
      d.consent = false;
      s.recordExposure('cta', 'bold');
      expect(d.events, isEmpty);
      expect(s.variantOf('cta'), isNull);
    });
  });
}
