// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/control/control_reader.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/control/live_control.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';
import 'package:plux_flutter/src/verify/update_metadata.dart';

import 'control_support.dart';

const _limits = VerifierLimits(maxDepth: 64, maxVisits: 1000000);

const _ios = {'os': 'ios'};
const _android = {'os': 'android'};

/// A kill switch of the plugin [target] that applies to the devices whose
/// `os` is [os].
fbs.KillSwitchObjectBuilder _targetedSwitch(String target, String os) =>
    fbs.KillSwitchObjectBuilder(
      scope: fbs.SwitchScope.Plugin,
      target: target,
      rule: condition('os', fbs.RuleOperator.Eq, fbs.RuleValueType.String, [
        os,
      ]),
    );

/// Kill switches with targeting rules (REL-060): a switch applies only on
/// the devices its rule matches, to every device when it has no rule, and to
/// every device when its rule cannot be read.
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

  test('a targeted switch applies only on the devices its rule matches '
      '[REL-060]', () async {
    final snapshot = await read(
      control.document(switches: [_targetedSwitch('loans', 'ios')]),
    );
    expect(snapshot.isOff(SwitchKind.plugin, 'loans', () => _ios), isTrue);
    expect(snapshot.isOff(SwitchKind.plugin, 'loans', () => _android), isFalse);
  });

  test('a switch with no rule applies on every device [REL-060]', () async {
    final snapshot = await read(
      control.document(switches: [killSwitch(fbs.SwitchScope.Plugin, 'loans')]),
    );
    expect(snapshot.isOff(SwitchKind.plugin, 'loans', () => _ios), isTrue);
    expect(snapshot.isOff(SwitchKind.plugin, 'loans', () => _android), isTrue);
  });

  test('a switch whose rule cannot be read applies on every device '
      '[REL-060]', () async {
    final snapshot = await read(
      control.document(
        switches: [killSwitch(fbs.SwitchScope.Plugin, 'loans', withRule: true)],
      ),
    );
    expect(snapshot.isOff(SwitchKind.plugin, 'loans', () => _ios), isTrue);
    expect(snapshot.isOff(SwitchKind.plugin, 'loans', () => _android), isTrue);
  });

  test('a switch applies to a device if any of the switches of its target '
      'matches [REL-060]', () async {
    final snapshot = await read(
      control.document(
        switches: [
          _targetedSwitch('loans', 'ios'),
          _targetedSwitch('loans', 'web'),
        ],
      ),
    );
    expect(snapshot.isOff(SwitchKind.plugin, 'loans', () => _ios), isTrue);
    expect(snapshot.isOff(SwitchKind.plugin, 'loans', () => _android), isFalse);
    expect(
      snapshot.isOff(SwitchKind.plugin, 'loans', () => const {'os': 'web'}),
      isTrue,
    );
  });

  test('the live control applies a targeted switch only on the devices it '
      'matches [REL-060]', () async {
    final snapshot = await read(
      control.document(switches: [_targetedSwitch('loans', 'ios')]),
    );
    final live = LiveControl(snapshot: snapshot);
    expect(live.isOff(SwitchKind.plugin, 'loans', () => _ios), isTrue);
    expect(live.isOff(SwitchKind.plugin, 'loans', () => _android), isFalse);
  });

  test(
    'a targeted switch keeps its rule in the stored form [REL-060]',
    () async {
      final snapshot = await read(
        control.document(switches: [_targetedSwitch('loans', 'ios')]),
      );
      final back = ControlSnapshot.fromJson(snapshot.toJson())!;
      expect(back.isOff(SwitchKind.plugin, 'loans', () => _ios), isTrue);
      expect(back.isOff(SwitchKind.plugin, 'loans', () => _android), isFalse);
    },
  );
}
