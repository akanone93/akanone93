// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Which variant of an experiment a subject gets (ABT-002, ABT-004): a pure
/// function of the experiment's allocation, the subject and the facts the
/// device has, so the same inputs give the same variant on every device and
/// in every test. It follows the procedure that `ExperimentAllocation` of
/// `schema/control.fbs` defines, step by step: the schedule, the global
/// holdout, the segment, the layer, the experiment's own holdout and then
/// the weights. The subject is the user identifier `setUserContext` gave,
/// otherwise the installation identifier; it enters the bucket hash and
/// goes nowhere else.
library;

import 'dart:convert';

import 'package:crypto/crypto.dart';
import 'package:plux_flutter/src/experiments/control_models.dart';
import 'package:plux_flutter/src/experiments/targeting.dart';

/// The bucket of [subject] under [salt]: the first eight bytes of
/// `sha256(salt ‖ subject)` as an unsigned big-endian integer, modulo
/// [bucketCount] (REL-050).
int bucketOf(String salt, String subject) {
  final digest = sha256.convert(utf8.encode(salt) + utf8.encode(subject)).bytes;
  var high = 0, low = 0;
  for (var i = 0; i < 4; i++) {
    high = (high << 8) | digest[i];
    low = (low << 8) | digest[4 + i];
  }
  // (high * 2^32 + low) mod n without leaving 64-bit signed range.
  const twoPow32 = 0x100000000;
  return ((high % bucketCount) * (twoPow32 % bucketCount) + low) % bucketCount;
}

/// The variant whose running total of `weightBp` first exceeds [bucket], or
/// null when the weights stop short of it.
String? weightedPick(ControlExperiment experiment, int bucket) {
  var upTo = 0;
  for (final v in experiment.variants) {
    upTo += v.weightBp;
    if (bucket < upTo) return v.name;
  }
  return null;
}

/// Whether the schedule of [experiment] covers [now].
bool isScheduled(ControlExperiment experiment, DateTime now) {
  final start = experiment.startsAt, end = experiment.endsAt;
  return (start == null || !now.isBefore(start)) &&
      (end == null || now.isBefore(end));
}

/// Whether [subject] is in the global [holdout] (ABT-004).
bool isHeldOut(ControlHoldout? holdout, String subject) =>
    holdout != null && bucketOf(holdout.salt, subject) < holdout.basisPoints;

/// The variant [subject] is drawn into, or null when it is not in the
/// experiment: its segment does not match [facts], its bucket in the layer
/// lies outside the experiment's range, or the weights leave it out. A
/// subject in the experiment's own holdout is drawn into the control. The
/// schedule and the global holdout are the caller's to check (they also
/// gate a stored assignment).
String? drawVariant(
  ControlExperiment experiment,
  String subject,
  TargetingFacts facts,
) {
  if (experiment.segment case final rule? when !rule.matches(facts)) {
    return null;
  }
  if (experiment.layer.isNotEmpty) {
    final traffic = bucketOf(experiment.layer, subject);
    if (traffic < experiment.layerStart || traffic >= experiment.layerEnd) {
      return null;
    }
  }
  if (bucketOf('${experiment.salt}:holdout', subject) < experiment.holdoutBp) {
    return experiment.control.name;
  }
  return weightedPick(experiment, bucketOf(experiment.salt, subject));
}
