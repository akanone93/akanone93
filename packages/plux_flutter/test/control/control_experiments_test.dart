// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/control/control_reader.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/experiments/assignment.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/experiments/experiment_service.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';
import 'package:plux_flutter/src/state/persistence.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/update_metadata.dart';

import 'control_support.dart';

const _limits = VerifierLimits(maxDepth: 64, maxVisits: 1000000);

fbs.FlagSegmentValueObjectBuilder _segment(
  String key,
  fbs.TargetingRuleObjectBuilder rule,
  String value,
) => fbs.FlagSegmentValueObjectBuilder(segment: key, rule: rule, value: value);

fbs.ExperimentAllocationObjectBuilder _allocation({
  String key = 'cta',
  String salt = 'cta-1',
  fbs.ExperimentScope scope = fbs.ExperimentScope.Subtree,
  List<fbs.AllocationVariantObjectBuilder>? variants,
  String? layer,
  int layerStart = 0,
  int layerEnd = 0,
  int holdoutBp = 0,
  fbs.TargetingRuleObjectBuilder? segment,
  int startsAt = 0,
  int endsAt = 0,
}) => fbs.ExperimentAllocationObjectBuilder(
  key: key,
  scope: scope,
  salt: salt,
  variants:
      variants ??
      [
        allocationVariant('control', 5000, control: true),
        allocationVariant('bold', 5000),
      ],
  layer: layer,
  layerStart: layerStart,
  layerEnd: layerEnd,
  holdoutBp: holdoutBp,
  segment: segment,
  startsAt: startsAt,
  endsAt: endsAt,
);

fbs.TargetingRuleObjectBuilder _os(String value) =>
    condition('os', fbs.RuleOperator.Eq, fbs.RuleValueType.String, [value]);

/// Flags, experiment allocations and the global holdout of the control
/// document, as the generated builder writes them and the runtime reads and
/// evaluates them (ABT-002, ABT-004, ABT-006, REL-051, SEC-052).
void main() {
  late ControlFixture control;
  setUp(() async => control = await ControlFixture.create());

  Future<ControlSnapshot> read(Uint8List doc) async {
    final verifier = MetadataVerifier(
      root: control.root,
      now: controlNow,
      floor: const MetadataFloor(control: 0),
    );
    final envelope = await verifier.control(
      await control.file(doc),
      controlScope,
    );
    return readControlDocument(envelope, _limits);
  }

  Future<PluxException> refused(Uint8List doc) async {
    try {
      await read(doc);
    } on PluxException catch (e) {
      return e;
    }
    fail('the document was accepted');
  }

  group('flags', () {
    test('the type, default and segments are read from the document '
        '[ABT-006]', () async {
      final snapshot = await read(
        control.document(
          flags: [
            flagValue('beta', fbs.FlagType.Bool, 'false', [
              _segment('ios', _os('ios'), 'true'),
            ]),
            flagValue('ratio', fbs.FlagType.Number, '0.5'),
            flagValue('label', fbs.FlagType.String, '"Hello"'),
            flagValue('theme', fbs.FlagType.Json, '{"a":[1,2]}'),
          ],
        ),
      );
      expect(snapshot.flags.map((f) => f.key), [
        'beta',
        'ratio',
        'label',
        'theme',
      ]);
      expect(snapshot.flags.map((f) => f.type), [
        FlagType.boolean,
        FlagType.number,
        FlagType.string,
        FlagType.json,
      ]);
      expect(snapshot.flags.map((f) => f.defaultValue), [
        false,
        0.5,
        'Hello',
        {
          'a': [1, 2],
        },
      ]);
      final beta = snapshot.flags.first;
      expect(beta.segments.single.segment, 'ios');
      expect(beta.valueFor(const {'os': 'ios'}), isTrue);
      expect(beta.valueFor(const {'os': 'android'}), isFalse);
      expect(beta.valueFor(const {}), isFalse);
    });

    test('a device evaluates the flags of a real document: the first '
        'matching segment wins, else the default of the document '
        '[ABT-006]', () async {
      final snapshot = await read(
        control.document(
          flags: [
            flagValue('beta', fbs.FlagType.Bool, 'false', [
              _segment('ios', _os('ios'), 'false'),
              _segment(
                'recent',
                condition(
                  'build',
                  fbs.RuleOperator.Gte,
                  fbs.RuleValueType.Number,
                  ['120'],
                ),
                'true',
              ),
            ]),
            flagValue('label', fbs.FlagType.String, '"Default"'),
          ],
        ),
      );
      ExperimentService on(Map<String, Object> facts) => ExperimentService(
        snapshot: () => snapshot,
        consent: () => true,
        facts: () => facts,
        userId: () => null,
        installId: () => 'i',
        persistence: StatePersistence.inMemory(),
        record: (name, {fields = const {}}) {},
      );
      expect(on({'os': 'android', 'build': 130}).flag('beta', false), isTrue);
      expect(on({'os': 'android', 'build': 99}).flag('beta', true), isFalse);
      expect(on({'os': 'ios', 'build': 130}).flag('beta', true), isFalse);
      expect(on(const {}).flag('label', 'App'), 'Default');
    });

    test('a flag this runtime cannot read is left out, so the app default '
        'holds [ABT-006]', () async {
      final snapshot = await read(
        control.document(
          flags: [
            flagValue('unspecified', fbs.FlagType.Unspecified, 'true'),
            flagValue('notJson', fbs.FlagType.String, 'plain text'),
            flagValue('wrongType', fbs.FlagType.Bool, '"yes"'),
            flagValue('ok', fbs.FlagType.Bool, 'true'),
          ],
        ),
      );
      expect(snapshot.flags.map((f) => f.key), ['ok']);
    });

    test('a segment whose rule cannot be evaluated, or whose value is not '
        'of the flag type, never applies [ABT-006]', () async {
      final snapshot = await read(
        control.document(
          flags: [
            flagValue('beta', fbs.FlagType.Bool, 'false', [
              _segment(
                'unknown operator',
                condition(
                  'os',
                  fbs.RuleOperator.Unspecified,
                  fbs.RuleValueType.String,
                  ['android'],
                ),
                'true',
              ),
              _segment(
                'not a number',
                condition(
                  'build',
                  fbs.RuleOperator.Eq,
                  fbs.RuleValueType.Number,
                  ['abc'],
                ),
                'true',
              ),
              _segment('wrong value', _os('android'), '"yes"'),
              _segment('not json', _os('android'), 'yes'),
              _segment('good', _os('android'), 'true'),
            ]),
          ],
        ),
      );
      expect(snapshot.flags.single.segments.map((s) => s.segment), ['good']);
    });
  });

  group('rules', () {
    Future<TargetingRule> only(fbs.TargetingRuleObjectBuilder rule) async {
      final snapshot = await read(
        control.document(
          flags: [
            flagValue('f', fbs.FlagType.Bool, 'false', [
              _segment('s', rule, 'true'),
            ]),
          ],
        ),
      );
      return snapshot.flags.single.segments.single.rule;
    }

    test('every operator and value type reads as the device compares them '
        '[REL-051] [ABT-006]', () async {
      for (final (rule, facts, expected)
          in <(fbs.TargetingRuleObjectBuilder, Map<String, Object>, bool)>[
            (_os('ios'), {'os': 'ios'}, true),
            (_os('ios'), {'os': 'android'}, false),
            (
              condition('os', fbs.RuleOperator.Neq, fbs.RuleValueType.String, [
                'ios',
              ]),
              {'os': 'android'},
              true,
            ),
            (
              condition('os', fbs.RuleOperator.In, fbs.RuleValueType.String, [
                'ios',
                'android',
              ]),
              {'os': 'android'},
              true,
            ),
            (
              condition(
                'os',
                fbs.RuleOperator.NotIn,
                fbs.RuleValueType.String,
                ['ios'],
              ),
              {'os': 'ios'},
              false,
            ),
            (
              condition(
                'build',
                fbs.RuleOperator.Lt,
                fbs.RuleValueType.Number,
                ['120'],
              ),
              {'build': 99},
              true,
            ),
            (
              condition(
                'build',
                fbs.RuleOperator.Lte,
                fbs.RuleValueType.Number,
                ['120'],
              ),
              {'build': 120},
              true,
            ),
            (
              condition(
                'build',
                fbs.RuleOperator.Gt,
                fbs.RuleValueType.Number,
                ['0.5'],
              ),
              {'build': 1},
              true,
            ),
            (
              condition(
                'build',
                fbs.RuleOperator.Gte,
                fbs.RuleValueType.Number,
                ['121'],
              ),
              {'build': 120},
              false,
            ),
            (
              condition(
                'runtimeVersion',
                fbs.RuleOperator.Gte,
                fbs.RuleValueType.String,
                ['1.9.0'],
              ),
              {'runtimeVersion': '1.10.0'},
              true,
            ),
            (
              condition(
                'user.vip',
                fbs.RuleOperator.Eq,
                fbs.RuleValueType.Bool,
                ['true'],
              ),
              {'user.vip': true},
              true,
            ),
            (
              fbs.TargetingRuleObjectBuilder(
                all: [
                  _os('ios'),
                  condition(
                    'build',
                    fbs.RuleOperator.Gt,
                    fbs.RuleValueType.Number,
                    ['1'],
                  ),
                ],
              ),
              {'os': 'ios', 'build': 2},
              true,
            ),
            (
              fbs.TargetingRuleObjectBuilder(
                all: [
                  _os('ios'),
                  condition(
                    'build',
                    fbs.RuleOperator.Gt,
                    fbs.RuleValueType.Number,
                    ['1'],
                  ),
                ],
              ),
              {'os': 'ios', 'build': 1},
              false,
            ),
            (
              fbs.TargetingRuleObjectBuilder(any: [_os('ios'), _os('android')]),
              {'os': 'android'},
              true,
            ),
            (
              fbs.TargetingRuleObjectBuilder(any: [_os('ios'), _os('android')]),
              {'os': 'web'},
              false,
            ),
          ]) {
        expect((await only(rule)).matches(facts), expected, reason: '$facts');
      }
    });

    test(
      'a rule that cannot be read is dropped with its segment [ABT-006]',
      () async {
        var nested = _os('ios');
        for (var i = 0; i < 20; i++) {
          nested = fbs.TargetingRuleObjectBuilder(all: [nested]);
        }
        final snapshot = await read(
          control.document(
            flags: [
              flagValue('beta', fbs.FlagType.Bool, 'false', [
                _segment(
                  'empty group',
                  fbs.TargetingRuleObjectBuilder(),
                  'true',
                ),
                _segment(
                  'both groups',
                  fbs.TargetingRuleObjectBuilder(
                    all: [_os('ios')],
                    any: [_os('ios')],
                  ),
                  'true',
                ),
                _segment(
                  'unspecified type',
                  condition(
                    'os',
                    fbs.RuleOperator.Eq,
                    fbs.RuleValueType.Unspecified,
                    ['ios'],
                  ),
                  'true',
                ),
                _segment(
                  'bad bool',
                  condition('x', fbs.RuleOperator.Eq, fbs.RuleValueType.Bool, [
                    'maybe',
                  ]),
                  'true',
                ),
                _segment(
                  'two operands for eq',
                  condition(
                    'os',
                    fbs.RuleOperator.Eq,
                    fbs.RuleValueType.String,
                    ['a', 'b'],
                  ),
                  'true',
                ),
                _segment(
                  'no operands',
                  condition(
                    'os',
                    fbs.RuleOperator.In,
                    fbs.RuleValueType.String,
                    [],
                  ),
                  'true',
                ),
                _segment(
                  'infinite',
                  condition(
                    'b',
                    fbs.RuleOperator.Eq,
                    fbs.RuleValueType.Number,
                    ['Infinity'],
                  ),
                  'true',
                ),
                _segment('too deep', nested, 'true'),
              ]),
            ],
          ),
        );
        expect(snapshot.flags.single.segments, isEmpty);
      },
    );
  });

  group('experiments', () {
    test('the allocation, the layer, the segment, the window and the global '
        'holdout are read from the document [ABT-002] [ABT-004]', () async {
      final snapshot = await read(
        control.document(
          experiments: [
            _allocation(
              scope: fbs.ExperimentScope.Prop,
              layer: 'checkout',
              layerStart: 1000,
              layerEnd: 6000,
              holdoutBp: 500,
              segment: _os('ios'),
              startsAt: 1790000000,
              endsAt: 1800000000,
            ),
          ],
          holdout: fbs.GlobalHoldoutObjectBuilder(basisPoints: 250, salt: 'h'),
        ),
      );
      final e = snapshot.experiments.single;
      expect(e.key, 'cta');
      expect(e.scope, ExperimentScope.prop);
      expect(e.salt, 'cta-1');
      expect(e.variants.map((v) => (v.name, v.weightBp, v.control)), [
        ('control', 5000, true),
        ('bold', 5000, false),
      ]);
      expect(e.layer, 'checkout');
      expect((e.layerStart, e.layerEnd), (1000, 6000));
      expect(e.holdoutBp, 500);
      expect(e.segment!.matches(const {'os': 'ios'}), isTrue);
      expect(e.segment!.matches(const {'os': 'android'}), isFalse);
      expect(e.startsAt, DateTime.utc(2026, 9, 21, 14, 13, 20));
      expect(e.endsAt, DateTime.utc(2027, 1, 15, 8));
      expect(snapshot.holdout!.salt, 'h');
      expect(snapshot.holdout!.basisPoints, 250);
      final again = ControlSnapshot.fromJson(snapshot.toJson())!;
      expect(again.toJson(), snapshot.toJson());
    });

    test('a document without experiments or a holdout reads as none '
        '[ABT-004]', () async {
      final snapshot = await read(control.document());
      expect(snapshot.flags, isEmpty);
      expect(snapshot.experiments, isEmpty);
      expect(snapshot.holdout, isNull);
    });

    test('a device assigns itself from a real document by the procedure of '
        'the schema, offline '
        '[ABT-002] [ABT-004]', () async {
      // Buckets computed outside the runtime (SHA-256, first eight bytes,
      // big-endian, modulo 10000): under salt cta-1 user-1 is 2346 and
      // user-3 is 5767; under cta-1:holdout user-3 is 2132; under h user-1
      // is 7598 and user-3 is 8499.
      Future<String?> variantOf(
        String user, {
        int holdoutBp = 0,
        int? globalHoldout,
      }) async {
        final snapshot = await read(
          control.document(
            experiments: [_allocation(holdoutBp: holdoutBp)],
            holdout: globalHoldout == null
                ? null
                : fbs.GlobalHoldoutObjectBuilder(
                    basisPoints: globalHoldout,
                    salt: 'h',
                  ),
          ),
        );
        return ExperimentService(
          snapshot: () => snapshot,
          consent: () => true,
          facts: () => const {},
          userId: () => user,
          installId: () => 'install',
          persistence: StatePersistence.inMemory(),
          record: (name, {fields = const {}}) {},
          clock: () => DateTime.utc(2026, 11),
        ).variantOf('cta');
      }

      expect(await variantOf('user-1'), 'control');
      expect(await variantOf('user-3'), 'bold');
      expect(
        await variantOf('user-3', holdoutBp: 3000),
        'control',
        reason: 'the experiment holdout shows the control',
      );
      expect(
        await variantOf('user-1', globalHoldout: 8000),
        isNull,
        reason: 'the global holdout keeps the subject out',
      );
      expect(await variantOf('user-3', globalHoldout: 8000), 'bold');
      expect(bucketOf('cta-1', 'user-3'), 5767);
    });

    test(
      'an experiment this runtime cannot run is left out [ABT-002]',
      () async {
        final snapshot = await read(
          control.document(
            experiments: [
              _allocation(
                key: 'unspecified',
                scope: fbs.ExperimentScope.Unspecified,
              ),
              _allocation(
                key: 'noControl',
                variants: [
                  allocationVariant('a', 5000),
                  allocationVariant('b', 5000),
                ],
              ),
              _allocation(
                key: 'twoControls',
                variants: [
                  allocationVariant('a', 5000, control: true),
                  allocationVariant('b', 5000, control: true),
                ],
              ),
              _allocation(key: 'noVariants', variants: []),
              _allocation(
                key: 'badLayer',
                layer: 'l',
                layerStart: 5,
                layerEnd: 99999,
              ),
              _allocation(
                key: 'backwards',
                layer: 'l',
                layerStart: 9,
                layerEnd: 5,
              ),
              _allocation(key: 'farFuture', endsAt: 9000000000000),
              _allocation(key: 'ok'),
            ],
          ),
        );
        expect(snapshot.experiments.map((e) => e.key), ['ok']);
      },
    );

    test('a segment this runtime cannot evaluate matches no device, so the '
        'experiment shows its control [ABT-002]', () async {
      final snapshot = await read(
        control.document(
          experiments: [
            _allocation(
              segment: condition(
                'os',
                fbs.RuleOperator.Eq,
                fbs.RuleValueType.Unspecified,
                ['ios'],
              ),
            ),
          ],
        ),
      );
      final e = snapshot.experiments.single;
      expect(e.segment, isA<TargetingNever>());
      for (var i = 0; i < 50; i++) {
        expect(drawVariant(e, 'user-$i', const {'os': 'ios'}), isNull);
      }
    });
  });

  group('verification', () {
    test('a truncated document that carries flags and experiments is '
        'refused before it is read [SEC-052]', () async {
      final whole = control.document(
        flags: [
          flagValue('beta', fbs.FlagType.Bool, 'true', [
            _segment('s', _os('ios'), 'false'),
          ]),
        ],
        experiments: [_allocation(segment: _os('ios'))],
        holdout: fbs.GlobalHoldoutObjectBuilder(basisPoints: 1, salt: 'h'),
      );
      for (final keep in [whole.length ~/ 3, whole.length - 8]) {
        final cut = Uint8List.sublistView(whole, 0, keep);
        expect(
          (await refused(cut)).code,
          PluxErrorCode.controlSignatureInvalid,
          reason: 'cut at $keep',
        );
      }
    });

    test('a document with more flags or experiments than the registry allows '
        'is refused [SEC-052]', () async {
      final flags = control.document(
        flags: [
          for (var i = 0; i < 10001; i++)
            flagValue('f$i', fbs.FlagType.Bool, 'true'),
        ],
      );
      expect(
        (await refused(flags)).code,
        PluxErrorCode.controlSignatureInvalid,
      );
      final experiments = control.document(
        experiments: [for (var i = 0; i < 501; i++) _allocation(key: 'e$i')],
      );
      expect(
        (await refused(experiments)).code,
        PluxErrorCode.controlSignatureInvalid,
      );
    });
  });
}
