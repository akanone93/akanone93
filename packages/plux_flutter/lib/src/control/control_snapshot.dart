// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What a verified control document says (SYN-060, REL-060, REL-070,
/// ADR-0077), as plain values: the kill switches by scope, the mandatory
/// update and the forced rollback. It is made on the sync isolate once the
/// document has been verified and read, kept on disk, and sent to the UI
/// isolate, which applies it. Nothing here parses FlatBuffers.
library;

/// What a kill switch disables (`SwitchScope` of `schema/control.fbs`,
/// without the unspecified member).
enum SwitchKind {
  /// A whole plugin, by key; its pages show their fallback (RT-022).
  plugin,

  /// One page, by document UUID; it shows its fallback (RT-022).
  page,

  /// One action graph, by document UUID.
  actionGraph,

  /// One data source, by document UUID.
  dataSource,

  /// One function, by dotted name.
  function,
}

/// One kill switch.
final class ControlSwitch {
  /// Creates a switch.
  const ControlSwitch(this.kind, this.target);

  /// Reads the stored form; null when it is not one.
  static ControlSwitch? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final kind = SwitchKind.values.asNameMap()[json['kind']];
    final target = json['target'];
    if (kind == null || target is! String) return null;
    return ControlSwitch(kind, target);
  }

  /// What it disables.
  final SwitchKind kind;

  /// The plugin key, document UUID or function name, by [kind].
  final String target;

  /// The stored form.
  Map<String, Object?> toJson() => {'kind': kind.name, 'target': target};
}

/// A mandatory update (REL-070): the release every Plux page waits for.
final class MandatoryUpdate {
  /// Creates the requirement.
  const MandatoryUpdate({
    required this.releaseId,
    required this.releaseSequence,
    required this.messageKey,
  });

  /// Reads the stored form; null when it is not one.
  static MandatoryUpdate? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final id = json['releaseId'], sequence = json['releaseSequence'];
    final key = json['messageKey'];
    if (id is! String || sequence is! int || key is! String) return null;
    return MandatoryUpdate(
      releaseId: id,
      releaseSequence: sequence,
      messageKey: key,
    );
  }

  /// The release's ID.
  final String releaseId;

  /// The release's sequence; the update is satisfied once a release of at
  /// least this sequence is active.
  final int releaseSequence;

  /// The translation key of the message the update screen shows.
  final String messageKey;

  /// The stored form.
  Map<String, Object?> toJson() => {
    'releaseId': releaseId,
    'releaseSequence': releaseSequence,
    'messageKey': messageKey,
  };
}

/// A forced rollback (REL-060): the retained earlier release to run.
final class ForcedRollback {
  /// Creates the requirement.
  const ForcedRollback({
    required this.releaseId,
    required this.releaseSequence,
  });

  /// Reads the stored form; null when it is not one.
  static ForcedRollback? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final id = json['releaseId'], sequence = json['releaseSequence'];
    if (id is! String || sequence is! int) return null;
    return ForcedRollback(releaseId: id, releaseSequence: sequence);
  }

  /// The release's ID.
  final String releaseId;

  /// The release's sequence.
  final int releaseSequence;

  /// The stored form.
  Map<String, Object?> toJson() => {
    'releaseId': releaseId,
    'releaseSequence': releaseSequence,
  };
}

/// The content of a verified control document.
final class ControlSnapshot {
  /// Creates a snapshot.
  ControlSnapshot({
    required this.version,
    required this.expiresAt,
    this.switches = const [],
    this.mandatory,
    this.rollback,
  }) : _off = {for (final s in switches) _key(s.kind, s.target)};

  /// Reads the stored form; null when it is not one.
  static ControlSnapshot? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final version = json['version'], expires = json['expiresAt'];
    final list = json['switches'];
    if (version is! int || expires is! int || list is! List<Object?>) {
      return null;
    }
    final switches = [
      for (final s in list)
        if (ControlSwitch.fromJson(s) case final ControlSwitch parsed) parsed,
    ];
    return ControlSnapshot(
      version: version,
      expiresAt: DateTime.fromMillisecondsSinceEpoch(expires, isUtc: true),
      switches: switches,
      mandatory: MandatoryUpdate.fromJson(json['mandatory']),
      rollback: ForcedRollback.fromJson(json['rollback']),
    );
  }

  /// The document's version.
  final int version;

  /// When the document stops being fresh. The switches stay in force past
  /// it; the mandatory update and the rollback are acted on only while it
  /// is fresh.
  final DateTime expiresAt;

  /// The kill switches.
  final List<ControlSwitch> switches;

  /// The mandatory update, or null.
  final MandatoryUpdate? mandatory;

  /// The forced rollback, or null.
  final ForcedRollback? rollback;

  final Set<String> _off;

  static String _key(SwitchKind kind, String target) =>
      '${kind.name}\u0000$target';

  /// Whether a switch turns off the [kind] named [target].
  bool isOff(SwitchKind kind, String target) =>
      _off.contains(_key(kind, target));

  /// How many switches differ between this document and [before]: those
  /// it turns on that were not on, and those it turns off that were.
  int switchesChangedFrom(ControlSnapshot? before) {
    final was = before?._off ?? const <String>{};
    return _off.difference(was).length + was.difference(_off).length;
  }

  /// The stored form.
  Map<String, Object?> toJson() => {
    'version': version,
    'expiresAt': expiresAt.millisecondsSinceEpoch,
    'switches': [for (final s in switches) s.toJson()],
    'mandatory': mandatory?.toJson(),
    'rollback': rollback?.toJson(),
  };
}
