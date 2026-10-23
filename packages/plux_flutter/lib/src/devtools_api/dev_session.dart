// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The seams of the dev session on the device (§20, ADR-0087, ADR-0091).
///
/// The channel receives dev bundles and commands from the session and sends
/// events back; what it receives is applied by a [DevBundleApplier] and
/// carried out by a [DevCommandHandler], and what happens on the device
/// reaches it through a [DevEventSink]. Every implementation sits behind
/// [kPluxDevtools], so a release build without the define contains none of
/// it (`DEV-050`).
library;

import 'package:flutter/foundation.dart';

import 'package:plux_flutter/src/errors/plux_exception.dart';

/// Whether the dev channel is compiled in: in debug builds, and in builds
/// made with `--dart-define=plux.devtools=true` such as the Plux Dev app
/// (ADR-0091, ADR-0093). The compiler removes every branch it guards from
/// any other build.
const bool kPluxDevtools = kDebugMode || bool.fromEnvironment('plux.devtools');

/// A dev bundle as the session sent it: a manifest and its sections, signed
/// with the development key of the session's environment and not yet
/// verified (`SEC-052`).
@immutable
final class DevBundle {
  /// Creates a dev bundle.
  const DevBundle({
    required this.sequence,
    required this.manifest,
    required this.sections,
  });

  /// The bundle's number in the session's own sequence space, which never
  /// meets the production store's.
  final int sequence;

  /// The signed manifest, as received.
  final Uint8List manifest;

  /// The sections the manifest names, by section ID, as received.
  final Map<String, Uint8List> sections;
}

/// What applying a dev bundle did (`DEV-010`).
@immutable
final class DevApplyResult {
  /// Creates a result.
  const DevApplyResult({
    this.pagesApplied = const [],
    this.scopesReset = const [],
    this.error,
  });

  /// The pages re-rendered from the bundle.
  final List<String> pagesApplied;

  /// The state scopes reset because their schema changed (`PLX-9231`).
  final List<String> scopesReset;

  /// Why the bundle was refused, or null when it was applied.
  final PluxException? error;
}

/// Verifies dev bundles against the pinned development roots and applies
/// them to the session overlay; the production store is never written.
abstract interface class DevBundleApplier {
  /// Verifies [bundle] and applies it, hot, to the running pages.
  Future<DevApplyResult> apply(DevBundle bundle);

  /// Discards the session overlay: the pages render the active release
  /// again.
  Future<void> discard();
}

/// Carries out a command of the session on the device (`DEV-011`,
/// `DEV-031`).
abstract interface class DevCommandHandler {
  /// Carries out [command], the proto3 JSON mapping of the command oneof of
  /// `DevSessionServiceSendCommandRequest` (for example
  /// `{"openScreen": {"route": "/home"}}`), and returns the proto3 JSON
  /// mapping of its `CommandResultEvent`.
  Future<Map<String, Object?>> handle(Map<String, Object?> command);
}

/// Receives what happens on the device, as `DevEvent` payloads.
abstract interface class DevEventSink {
  /// Queues [payload], the proto3 JSON mapping of one `DevEvent` payload
  /// keyed by its oneof field name (for example `{"log": {...}}`). The sink
  /// adds the sequence number, device, time and trace ID; it never blocks
  /// and drops the oldest events when its buffer is full.
  void emit(Map<String, Object?> payload, {String? traceId});
}
