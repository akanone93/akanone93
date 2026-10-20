// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Which variant of an experiment a subject gets (ABT-002, ABT-004): a pure
/// function of the experiment's allocation, the subject and the facts the
/// device has, so the same inputs give the same variant on every device and
/// in every test. `variant = weightedPick(sha256(salt ‖ subject))`; layers
/// give experiments disjoint ranges of one bucket space, and a global
/// holdout keeps a fixed share of subjects out of every experiment.
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

/// The hashed form of a user ID or device ID in which it is used as the
/// subject: identifiers never enter a hash, an event or a store in the
/// clear.
String subjectOf(String id) => sha256.convert(utf8.encode(id)).toString();

/// The variant of [experiment] that the weights of its variants give the
/// bucket [bucket]; null when every weight is zero.
String? weightedPick(ControlExperiment experiment, int bucket) {
  final total = experiment.variants.fold<int>(0, (sum, v) => sum + v.weight);
  if (total <= 0) return null;
  final point = bucket * total ~/ bucketCount;
  var upTo = 0;
  for (final v in experiment.variants) {
    upTo += v.weight;
    if (point < upTo) return v.name;
  }
  return experiment.control.name;
}

/// Whether the schedule of [experiment] covers [now].
bool isScheduled(ControlExperiment experiment, DateTime now) {
  final start = experiment.startsAt, end = experiment.endsAt;
  return (start == null || !now.isBefore(start)) &&
      (end == null || now.isBefore(end));
}

/// Whether [subject] is in the global [holdout] (ABT-004).
bool isHeldOut(ControlHoldout? holdout, String subject) =>
    holdout != null && bucketOf(holdout.salt, subject) < holdout.size;

/// The variant [subject] is drawn into, or null when it is not in the
/// experiment: its targeting rule does not match [facts], its bucket in
/// the layer lies outside the experiment's range, or it is in the
/// experiment's own holdout. The schedule and the global holdout are the
/// caller's to check (they also gate a stored assignment).
String? drawVariant(
  ControlExperiment experiment,
  String subject,
  TargetingFacts facts,
) {
  if (experiment.rule case final rule? when !rule.matches(facts)) return null;
  final traffic = bucketOf(
    experiment.layer ?? '${experiment.salt}/traffic',
    subject,
  );
  if (traffic < experiment.rangeStart || traffic >= experiment.rangeEnd) {
    return null;
  }
  if (bucketOf('${experiment.salt}/holdout', subject) < experiment.holdout) {
    return null;
  }
  return weightedPick(experiment, bucketOf(experiment.salt, subject));
}
