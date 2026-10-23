// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Signed control documents for the tests (SYN-060, ADR-0077): an Ed25519
/// key made at test time plays the online `control` role, and the files it
/// signs have the shape the server writes (`control.schema.json`): the
/// canonical signed JSON, whose `document` member holds the FlatBuffers
/// document of `schema/control.fbs` built with the generated builder.
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart' show SimpleKeyPair;
import 'package:cryptography/dart.dart';
import 'package:plux_flutter/src/control/fbs/control_controlfbs_generated.dart'
    as fbs;
import 'package:plux_flutter/src/verify/jcs.dart';
import 'package:plux_flutter/src/verify/manifest.dart';
import 'package:plux_flutter/src/verify/update_metadata.dart';

final _ed = DartEd25519(sha512: const DartSha512());

/// The moment the fixtures treat as now.
final DateTime controlNow = DateTime.utc(2026, 10, 20, 12);

/// The app, environment and channel the documents are for.
const controlScope = ControlScope('shop', 'production', 'stable');

String _time(DateTime t) =>
    t.toUtc().toIso8601String().replaceFirst('.000', '');

/// One kill switch of a built document.
fbs.KillSwitchObjectBuilder killSwitch(
  fbs.SwitchScope scope,
  String target, {
  bool withRule = false,
}) => fbs.KillSwitchObjectBuilder(
  scope: scope,
  target: target,
  rule: withRule ? fbs.TargetingRuleObjectBuilder() : null,
);

/// A condition of a targeting rule, with its operands as the server writes
/// them (canonical text).
fbs.TargetingRuleObjectBuilder condition(
  String attribute,
  fbs.RuleOperator op,
  fbs.RuleValueType type,
  List<String> values,
) => fbs.TargetingRuleObjectBuilder(
  attribute: attribute,
  $operator: op,
  valueType: type,
  values: values,
);

/// A flag of a built document.
fbs.FlagValueObjectBuilder flagValue(
  String key,
  fbs.FlagType type,
  String defaultValue, [
  List<fbs.FlagSegmentValueObjectBuilder> segments = const [],
]) => fbs.FlagValueObjectBuilder(
  key: key,
  type: type,
  defaultValue: defaultValue,
  segments: segments,
);

/// One variant of an experiment of a built document.
fbs.AllocationVariantObjectBuilder allocationVariant(
  String name,
  int weightBp, {
  bool control = false,
}) => fbs.AllocationVariantObjectBuilder(
  name: name,
  weightBp: weightBp,
  control: control,
);

/// A key playing the `control` role, and the files it signs.
final class ControlFixture {
  ControlFixture._(this._pair, this.publicKey)
    : keyId = metadataKeyId(publicKey);

  /// Makes a fresh key.
  static Future<ControlFixture> create() async {
    final pair = await _ed.newKeyPair();
    final pub = Uint8List.fromList((await pair.extractPublicKey()).bytes);
    return ControlFixture._(pair, pub);
  }

  final SimpleKeyPair _pair;

  /// The raw public key.
  final Uint8List publicKey;

  /// The key's identifier.
  final String keyId;

  /// The key as the app embeds it, for [role].
  TrustedKey trusted([String role = controlRole]) => TrustedKey(
    keyId: keyId,
    algorithm: 'ed25519',
    role: role,
    publicKey: publicKey,
  );

  /// The root made from this key alone, delegating the control role.
  RootMetadata get root => rootFromKeys([trusted()]);

  /// The FlatBuffers document.
  Uint8List document({
    int version = 2,
    DateTime? expires,
    DateTime? issued,
    String app = 'shop',
    String environment = 'production',
    String channel = 'stable',
    List<fbs.KillSwitchObjectBuilder> switches = const [],
    fbs.MandatoryUpdateObjectBuilder? mandatory,
    fbs.ForcedRollbackObjectBuilder? rollback,
    List<fbs.FlagValueObjectBuilder> flags = const [],
    List<fbs.ExperimentAllocationObjectBuilder> experiments = const [],
    fbs.GlobalHoldoutObjectBuilder? holdout,
  }) => fbs.ControlDocumentObjectBuilder(
    version: version,
    appId: app,
    environment: environment,
    channel: channel,
    issuedAt: (issued ?? controlNow).millisecondsSinceEpoch ~/ 1000,
    expiresAt:
        (expires ?? controlNow.add(const Duration(hours: 24)))
            .millisecondsSinceEpoch ~/
        1000,
    killSwitches: switches,
    mandatoryUpdate: mandatory,
    forcedRollback: rollback,
    flags: flags,
    experiments: experiments,
    holdout: holdout,
  ).toBytes('PLXC');

  /// The signed file for [document] (built with the same [version],
  /// [expires], [issued] and scope). [afterSigning] changes the signed part
  /// once it is signed, as an attacker would; [signer] signs instead of
  /// this key.
  Future<Uint8List> file(
    Uint8List document, {
    int version = 2,
    DateTime? expires,
    DateTime? issued,
    String app = 'shop',
    String environment = 'production',
    String channel = 'stable',
    ControlFixture? signer,
    void Function(Map<String, Object?> signed)? afterSigning,
  }) async {
    final signed = <String, Object?>{
      '_type': controlRole,
      'version': version,
      'expires': _time(expires ?? controlNow.add(const Duration(hours: 24))),
      'specVersion': '1.0.0',
      'app': app,
      'environment': environment,
      'channel': channel,
      'issuedAt': _time(issued ?? controlNow),
      'document': base64.encode(document),
    };
    final by = signer ?? this;
    final signature = await _ed.sign(
      utf8.encode(canonicalJson(signed)),
      keyPair: by._pair,
    );
    afterSigning?.call(signed);
    return Uint8List.fromList(
      utf8.encode(
        canonicalJson({
          'signed': signed,
          'signatures': [
            {
              'keyid': by.keyId,
              'alg': 'Ed25519',
              'sig': base64.encode(signature.bytes),
            },
          ],
        }),
      ),
    );
  }
}
