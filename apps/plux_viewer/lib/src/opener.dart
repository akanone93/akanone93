// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Opens a reconstruction release in the runtime's reconstruction mode
/// (SEC-142, ADR-0082): fetch the release, fetch the environment's root
/// chain, download the bundles, and hand all of it, with the root keys the
/// viewer was built with, to `Plux.initialize`. The runtime walks the chain
/// from those keys to the `targets` keys (SEC-051), verifies the manifest's
/// signature and every bundle before it loads anything (SEC-052). The viewer
/// trusts none of what it downloads, and opens nothing without pinned keys.
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;
import 'package:plux_components/plux_components.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_viewer/src/api.dart';

/// Starts the runtime; [startRuntime] in the app, a stand-in in tests.
/// Completes with null when the reconstruction is ready to show, else with
/// why it is not.
typedef RuntimeStarter = Future<String?> Function(PluxConfig config);

/// Starts the runtime on [config] with `Plux.initialize`, which verifies the
/// reconstruction before it loads anything (SEC-052).
Future<String?> startRuntime(PluxConfig config) async {
  final startup = await Plux.initialize(config);
  if (startup.ready) return null;
  final error = startup.error;
  return error == null
      ? 'The reconstruction could not be opened.'
      : 'The reconstruction was refused: ${error.code.id} ${error.message}';
}

/// Why a viewer built without root keys opens nothing.
const String noRootKeysMessage =
    'This build of the viewer pins no root keys, so it cannot tell a genuine '
    'reconstruction from a forged one. Build it with '
    '--dart-define=PLUX_VIEWER_ROOT_KEYS=<keys.json>.';

/// Why a reconstruction could not be opened.
final class OpenException implements Exception {
  /// Creates the exception with [message] for the auditor.
  const OpenException(this.message);

  /// What went wrong.
  final String message;

  @override
  String toString() => message;
}

/// The client the runtime is configured with. In reconstruction mode the
/// runtime makes no request at all except to the Plux server, so it has
/// nothing to pin: the viewer opens releases of servers it cannot know the
/// keys of in advance.
http.Client viewerHttpClient() => http.Client();

/// Opens reconstructions through [api].
final class ReconstructionOpener {
  /// Creates an opener that trusts the root keys [rootKeys], pinned at
  /// build time, and none other; with none it opens nothing. [start] and
  /// [stop] default to the runtime's.
  ReconstructionOpener(
    this.api, {
    required this.rootKeys,
    RuntimeStarter? start,
    Future<void> Function()? stop,
  }) : _start = start ?? startRuntime,
       _stop = stop ?? Plux.dispose;

  /// The signed-in client.
  final ViewerApi api;

  /// The root keys the viewer was built with (SEC-051).
  final List<PluxPublicKey> rootKeys;

  final RuntimeStarter _start;
  final Future<void> Function() _stop;
  Future<void> _stopping = Future<void>.value();

  /// Stops the runtime, which removes the reconstruction from the device.
  /// A later [open] waits for it, so a release opened again never meets the
  /// end of the previous run.
  Future<void> close() => _stopping = _stopping.then((_) => _stop());

  /// Opens reconstruction [id], reporting each step to [progress]. Completes
  /// when the runtime is ready to show it; throws [OpenException] with a
  /// message for the auditor otherwise, having stopped the runtime again.
  ///
  /// [cancelled] tells the opener that nobody waits any more, such as the
  /// auditor leaving the screen: it then starts nothing further, and stops
  /// the runtime if it was started.
  Future<void> open(
    String id, {
    void Function(String step) progress = _ignore,
    bool Function() cancelled = _never,
  }) async {
    if (rootKeys.isEmpty) throw const OpenException(noRootKeysMessage);
    await _stopping;
    if (Plux.isInitialized) await close();
    progress('Fetching the reconstruction');
    final release = await _fetch(id);
    final target = _target(release);
    progress('Fetching the environment keys');
    final roots = await _roots(target);
    progress('Downloading ${release.bundles.length} bundles');
    final bundles = <PluxReconstructionBundle>[];
    for (final b in release.bundles) {
      bundles.add(
        PluxReconstructionBundle(key: b.key, bytes: await _download(b)),
      );
    }
    if (cancelled()) throw const OpenException('Cancelled.');
    progress('Verifying');
    final refusal = await _start(
      PluxConfig(
        appId: target.app,
        endpoint: api.server,
        environment: target.environment,
        channel: 'audit',
        rootKeys: rootKeys,
        baseline: null,
        hostBuild: 'plux-viewer',
        httpClient: viewerHttpClient,
        widgetPackages: const [PluxComponents()],
        reconstruction: PluxReconstruction(
          id: release.id,
          manifest: release.manifest,
          signatures: release.signatures,
          bundles: bundles,
          roots: roots,
        ),
      ),
    );
    if (cancelled()) {
      await close();
      throw const OpenException('Cancelled.');
    }
    if (refusal != null) {
      await close();
      throw OpenException(refusal);
    }
  }

  Future<ReconstructionRelease> _fetch(String id) async {
    final ReconstructionRelease release;
    try {
      release = await api.getReconstruction(id);
    } on ViewerApiException catch (e) {
      throw OpenException(_describe(e));
    }
    if (release.manifest.isEmpty) {
      throw const OpenException(
        'The server has not signed this reconstruction yet. Try again in a '
        'moment.',
      );
    }
    return release;
  }

  Future<List<Uint8List>> _roots(({String app, String environment}) t) async {
    try {
      return await api.rootChain(t.app, t.environment);
    } on ViewerApiException catch (e) {
      throw OpenException(_describe(e));
    }
  }

  Future<Uint8List> _download(BundleInfo b) async {
    try {
      return await api.downloadBundle(b);
    } on ViewerApiException catch (e) {
      throw OpenException(_describe(e));
    }
  }

  /// The app and environment the manifest names, read only to know which
  /// root chain to ask for. Nothing else of the unverified manifest is used: the
  /// runtime verifies the signature before it reads any other field.
  static ({String app, String environment}) _target(
    ReconstructionRelease release,
  ) {
    try {
      final m =
          jsonDecode(utf8.decode(release.manifest)) as Map<String, Object?>;
      final app = m['app'];
      final environment = m['environment'];
      if (app is String && environment is String) {
        return (app: app, environment: environment);
      }
    } on Object {
      // Falls through to the error below.
    }
    throw const OpenException('The server returned an unreadable manifest.');
  }

  static String _describe(ViewerApiException e) => e.unauthenticated
      ? 'The server refused the sign-in. Sign in again.'
      : e.message.isEmpty
      ? 'The server answered ${e.status} ${e.code}.'
      : e.message;
}

void _ignore(String step) {}

bool _never() => false;
