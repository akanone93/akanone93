// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The device side of the signed control document (SYN-060, REL-060,
/// ADR-0077): fetch it with `If-None-Match` and DPoP like the manifest,
/// verify it before reading a byte of it (SEC-052), keep it, and carry out
/// the forced rollback it orders. It runs on the sync isolate (L-6); the UI
/// isolate gets a [ControlOutcome] to apply.
///
/// The kill switches of the newest verified document stay in force when no
/// fresh document can be fetched, also past its expiry: the device fails
/// closed for switches only, and the app keeps working.
library;

import 'package:plux_flutter/src/control/control_reader.dart';
import 'package:plux_flutter/src/control/control_snapshot.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/store/control_store.dart';
import 'package:plux_flutter/src/store/release_store.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/metadata_sync.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart';

/// What one check of the control document found.
final class ControlOutcome {
  /// Creates an outcome.
  const ControlOutcome({
    this.snapshot,
    this.fresh = false,
    this.expired = false,
    this.error,
    this.rolledBackTo,
  });

  /// The newest verified document, or null when the device never had one.
  final ControlSnapshot? snapshot;

  /// Whether this check accepted a document newer than the one held.
  final bool fresh;

  /// Whether [snapshot] is past its expiry. Its switches stay in force; its
  /// mandatory update and rollback are not acted on.
  final bool expired;

  /// Why the check could not use a document, or why the rollback failed:
  /// `PLX-3100`, `PLX-3101`, `PLX-3102` or `PLX-3003`. An unreachable
  /// server is not an error here, it is the offline case.
  final PluxException? error;

  /// The sequence of the release the forced rollback activated, or null
  /// when this check rolled nothing back.
  final int? rolledBackTo;
}

/// Fetches, verifies and keeps the control document.
final class ControlSync {
  /// Creates the check for [channel].
  ControlSync({
    required this.api,
    required this.metadata,
    required this.releases,
    required this.documents,
    required this.limits,
    required this.channel,
    required this.token,
    required DateTime Function() clock,
  }) : _clock = clock; // ignore: prefer_initializing_formals

  /// The API client.
  final PluxApiClient api;

  /// Verifies the document under the trusted root.
  final MetadataSync metadata;

  /// The release store, for a forced rollback.
  final ReleaseStore releases;

  /// Where the document is kept.
  final ControlDocumentStore documents;

  /// The FlatBuffers verifier's limits.
  final VerifierLimits limits;

  /// The channel the device follows.
  final String channel;

  /// The device token, registering the device when it has none.
  final Future<DeviceToken> Function() token;

  final DateTime Function() _clock;

  /// Checks the server once. Never throws: a failure is in the outcome,
  /// and the document held stays in force.
  Future<ControlOutcome> check() async {
    final held = documents.read();
    var current = held;
    PluxException? error;
    try {
      final device = await token();
      final res = await api.signedControl(
        token: device,
        channel: channel,
        ifNoneMatch: held?.etag ?? '',
      );
      if (!res.notModified) {
        current = await _accept(device, res, held);
      }
    } on PluxException catch (e) {
      error = e;
    } on ApiError {
      // Offline or refused by the network: the document held stays.
    }
    return _outcome(current, held, error);
  }

  /// Verifies and reads the file of [res], and keeps it.
  Future<StoredControl> _accept(
    DeviceToken device,
    ControlResponse res,
    StoredControl? held,
  ) async {
    final envelope = await metadata.verifyControl(device, res.signed!, channel);
    final snapshot = readControlDocument(envelope, limits);
    metadata.commitControl(snapshot.version);
    final next = StoredControl(
      etag: res.etag,
      snapshot: snapshot,
      rollbackAppliedVersion: held?.rollbackAppliedVersion ?? 0,
    );
    documents.write(next);
    return next;
  }

  ControlOutcome _outcome(
    StoredControl? current,
    StoredControl? held,
    PluxException? error,
  ) {
    if (current == null) return ControlOutcome(error: error);
    final snapshot = current.snapshot;
    final expired = !_clock().isBefore(snapshot.expiresAt);
    final fresh = held == null || held.snapshot.version != snapshot.version;
    final order = snapshot.rollback;
    if (expired ||
        order == null ||
        snapshot.version <= current.rollbackAppliedVersion) {
      return ControlOutcome(
        snapshot: snapshot,
        fresh: fresh,
        expired: expired,
        error: error,
      );
    }
    return _rollBack(current, order, fresh, error);
  }

  /// Carries out the forced rollback of [current], once: the document's
  /// version is recorded whether or not the release could be activated,
  /// since a release that is not retained will not become so.
  ControlOutcome _rollBack(
    StoredControl current,
    ForcedRollback order,
    bool fresh,
    PluxException? error,
  ) {
    final snapshot = current.snapshot;
    var rolled = false;
    PluxException? failure;
    try {
      rolled = releases.rollbackTo(order.releaseSequence);
      if (!rolled && releases.pointer.active != order.releaseSequence) {
        failure = _notRetained(order);
      }
    } on PluxException catch (e) {
      failure = e;
    }
    documents.write(current.copyWith(rollbackAppliedVersion: snapshot.version));
    return ControlOutcome(
      snapshot: snapshot,
      fresh: fresh,
      error: error ?? failure,
      rolledBackTo: rolled ? order.releaseSequence : null,
    );
  }

  PluxException _notRetained(ForcedRollback order) => PluxException(
    PluxErrorCode.rollbackRejected,
    'the forced rollback names release ${order.releaseId}, which the device '
    'does not retain',
    details: {'sequence': '${order.releaseSequence}'},
  );
}
