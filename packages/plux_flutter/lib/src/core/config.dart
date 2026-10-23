// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The host's configuration of the runtime (spec Appendix H.2).
library;

import 'dart:convert';
import 'dart:io' show HttpClient;
import 'dart:typed_data';

import 'package:flutter/material.dart' show ThemeMode;
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/db/adapter.dart';
import 'package:plux_flutter/src/device/types.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/functions/engine.dart';
import 'package:plux_flutter/src/native_catalogue/registration.dart';
import 'package:plux_flutter/src/navigation/delegate.dart';
import 'package:plux_flutter/src/reconstruction/reconstruction.dart';
import 'package:plux_flutter/src/render/widget_package.dart';
import 'package:plux_flutter/src/security/rasp.dart';
import 'package:plux_flutter/src/verify/manifest.dart';

/// How `Plux.initialize` treats the network (SYN-003).
final class StartupPolicy {
  /// Render the cached or baseline release at once and sync in the
  /// background (the default).
  const StartupPolicy.useCacheThenSync() : timeout = null;

  /// Wait for the sync up to [timeout], then continue with what is on disk.
  const StartupPolicy.blockUntilSynced(Duration this.timeout);

  /// How long `initialize` waits for the sync; null to not wait.
  final Duration? timeout;

  @override
  String toString() => timeout == null
      ? 'StartupPolicy.useCacheThenSync'
      : 'StartupPolicy.blockUntilSynced($timeout)';
}

/// When a staged release replaces the active one (SYN-004). A release is
/// never swapped under a visible Plux page.
enum ActivationPolicy {
  /// As soon as no Plux page is on screen.
  immediate,

  /// At the next safe point: no Plux page is on screen (from P4 also the
  /// return to the navigation root). The default.
  atSafePoint,

  /// At the next `Plux.initialize`.
  nextLaunch,

  /// For mandatory updates (REL-070): like [immediate]; the `staged` event
  /// says the release is mandatory so the host can close Plux pages.
  forced,
}

/// Which theme Plux pages use (HST-012, THM-003, ADR-0032).
enum PluxThemeSource {
  /// The host's `Theme` and `CupertinoTheme` (the default).
  host,

  /// The host's themes with every role the Plux theme defines replaced.
  pluxOverHost,

  /// Themes built from the Plux design tokens alone.
  plux,
}

/// What the user has consented to (SEC-161, ANL-003).
final class PluxConsent {
  /// Creates a consent record.
  const PluxConsent({this.analytics = false, this.experiments = false});

  /// Only what is strictly necessary: no analytics, no experiments.
  static const necessaryOnly = PluxConsent();

  /// Whether analytics telemetry may be sent.
  final bool analytics;

  /// Whether experiment assignments may be used.
  final bool experiments;
}

/// The signed-in user as rollouts and experiments see them (HST-011): a
/// pseudonymous ID and targeting attributes.
final class PluxUser {
  /// Creates a user context.
  const PluxUser({required this.id, this.attributes = const {}});

  /// A pseudonymous ID, never an e-mail address or a name.
  final String id;

  /// Targeting attributes, such as tier or region.
  final Map<String, String> attributes;
}

/// Supplies the end user's access token to data sources and functions
/// (HST-010). Plux never implements end-user login; it is used from P4.
///
/// Through the gateway (SEC-030) the token is the *subject* of a token
/// exchange (SEC-026, RFC 8693): the runtime trades it with the Plux server,
/// which verifies it against the customer's identity provider and issues a
/// short-lived token for the upstream; the upstream sees the verified user in
/// the server's assertion, never the raw token. The exchanged token is kept
/// in memory only. A token the provider rejects is refreshed once, as a
/// `401` is.
abstract interface class PluxAuthDelegate {
  /// Whether a user is signed in. Guards and other expressions read it as
  /// `user.authenticated` (ADR-0040); it is read on every evaluation, so
  /// it answers from memory, without I/O.
  bool get isAuthenticated;

  /// The current access token, or null when signed out.
  Future<String?> accessToken();

  /// A fresh token after a `401`, or null.
  Future<String?> refresh();

  /// Called when Plux learns that the session has ended.
  void onLogout();
}

/// A public key the app embeds (SEC-051), as `plux pull` writes it into
/// `keys.json`.
final class PluxPublicKey {
  /// Creates a key.
  const PluxPublicKey({
    required this.keyId,
    required this.algorithm,
    required this.role,
    required this.publicKey,
  });

  /// Reads the keys of a `keys.json` file.
  static List<PluxPublicKey> parseKeysJson(String json) {
    try {
      return [
        for (final k
            in (jsonDecode(json) as List<Object?>).cast<Map<String, Object?>>())
          PluxPublicKey(
            keyId: k['keyId']! as String,
            algorithm: k['algorithm']! as String,
            role: k['role']! as String,
            publicKey: hexDecode(k['publicKey']! as String),
          ),
      ];
    } on Object {
      throw const PluxException(
        PluxErrorCode.manifestSignatureInvalid,
        'keys.json cannot be read',
      );
    }
  }

  /// The key's ID.
  final String keyId;

  /// Its algorithm (`ed25519`).
  final String algorithm;

  /// Its update-metadata role (`targets` for manifests and bundles).
  final String role;

  /// The raw public key.
  final Uint8List publicKey;

  /// The key as the verifier takes it.
  TrustedKey toTrustedKey() => TrustedKey(
    keyId: keyId,
    algorithm: algorithm,
    role: role,
    publicKey: publicKey,
  );
}

/// Called with every error the runtime reports to the host.
typedef PluxErrorHandler = void Function(
  PluxException error,
  StackTrace? stack,
);

/// Builds the screen Plux shows in place of every page while a mandatory
/// update is pending (REL-070): [messageKey] is the translation key the
/// control document names. The update has already been requested; the
/// screen only informs, and goes away once the release is active.
typedef PluxUpdateScreenBuilder = Widget Function(
  BuildContext context,
  String messageKey,
);

/// Builds what Plux shows in place of content it cannot render: a page or
/// component instance that failed to build (RT-020), a plugin switched off
/// or failing verification (RT-022), or no release yet.
typedef PluxFallbackBuilder = Widget Function(
  BuildContext context,
  PluxException error,
);

/// The runtime's configuration (Appendix H.2).
final class PluxConfig {
  /// Creates a configuration.
  const PluxConfig({
    required this.appId,
    required this.endpoint,
    this.environment = 'production',
    this.channel = 'production',
    this.rootKeys = const [],
    this.pins = const [],
    this.baseline = 'assets/plux',
    this.startup = const StartupPolicy.useCacheThenSync(),
    this.activation = ActivationPolicy.atSafePoint,
    this.downloadParallelism = 4,
    this.diskQuota,
    this.themeMode = ThemeMode.system,
    this.themeSource = PluxThemeSource.host,
    this.brand,
    this.locale,
    this.consent = PluxConsent.necessaryOnly,
    this.telemetrySampling = const {},
    this.authDelegate,
    this.container,
    this.parentContainer,
    this.hostBuild = '',
    this.playIntegrityCloudProjectNumber,
    this.storageDirectory,
    this.httpClient,
    this.webSocketClient,
    this.onError,
    this.fallbackBuilder,
    this.updateScreen,
    this.pluginFallbackBuilders = const {},
    this.notFoundBuilder,
    this.navigationDelegate,
    this.navigatorKey,
    this.nativeRoutes = const {},
    this.nativeSlots = const {},
    this.nativeActions = const {},
    this.router,
    this.devicePackages = const [],
    this.widgetPackages = const [],
    this.functionEngine,
    this.allowedCapabilities,
    this.databaseAdapter,
    this.securityModules = const [],
    this.strongCustomerAuthentication = false,
    this.reconstruction,
  }) : assert(downloadParallelism > 0, 'at least one download at a time'),
       assert(
         hostBuild.length <= 64,
         'the server records a host build of at most 64 characters',
       ),
       assert(
         container == null || parentContainer == null,
         'share or nest, not both',
       );

  /// The Plux app ID.
  final String appId;

  /// The Plux server's base URL.
  final Uri endpoint;

  /// The environment key.
  final String environment;

  /// The release channel.
  final String channel;

  /// The embedded public keys (SEC-051). When empty, the runtime reads
  /// `keys.json` from the [baseline] directory of the host's assets.
  final List<PluxPublicKey> rootKeys;

  /// The pins of the Plux server's TLS key (SEC-041): the base64 of the
  /// SHA-256 of a certificate's SubjectPublicKeyInfo, as in RFC 7469, at
  /// least two distinct ones, one of them a backup key not yet in use.
  /// Connections to the [endpoint]'s host are accepted only after the
  /// platform has validated the certificate chain and the key matches a
  /// pin; a mismatch fails the request with `PLX-6020` and is reported.
  /// Pins are updated later only through signed update metadata (SEC-050).
  ///
  /// A release build whose [endpoint] is `https` and that has no pins
  /// fails `Plux.initialize`. Debug and profile builds, and endpoints that
  /// are not `https` (a local development server), may leave it empty and
  /// run unpinned. A custom [httpClient] is the host's own and is not
  /// pinned by Plux. On Android and iOS a pin may be the key of any
  /// certificate in the server's chain, such as the intermediate that
  /// issues it; on `dart:io` platforms (desktop development) only the leaf
  /// is visible, so a pin there is a leaf key.
  final List<String> pins;

  /// The asset directory `plux pull` wrote the baseline to (SYN-007), or
  /// null for none.
  final String? baseline;

  /// How start-up treats the network (SYN-003).
  final StartupPolicy startup;

  /// When a staged release is activated (SYN-004).
  final ActivationPolicy activation;

  /// Downloads at once (SYN-010).
  final int downloadParallelism;

  /// A tighter disk quota than the app's `device.diskQuota`, in bytes.
  final int? diskQuota;

  /// Light, dark or the system setting (THM-002).
  final ThemeMode themeMode;

  /// Which theme Plux pages use (HST-012).
  final PluxThemeSource themeSource;

  /// The white-label brand overlay, if any (THM-003).
  final String? brand;

  /// The locale of Plux pages; the host's when null.
  final Locale? locale;

  /// What the user consented to (SEC-161).
  final PluxConsent consent;

  /// The share of each consent-gated telemetry event kept, by event name
  /// (such as `screen_view`), from 0 to 1 (ANL-003). It can only lower
  /// the rate the app sets in its document; operational events
  /// (`session_start`, `sync_result`, `error`) are never sampled.
  final Map<String, double> telemetrySampling;

  /// Supplies the end user's token (HST-010), from P4.
  final PluxAuthDelegate? authDelegate;

  /// The host's Riverpod container, to share (RT-003, ADR-0008).
  final ProviderContainer? container;

  /// The host's Riverpod container, to nest Plux's container under.
  final ProviderContainer? parentContainer;

  /// The host app's build, reported at registration; at most 64 ASCII
  /// characters, the server's limit.
  final String hostBuild;

  /// The Google Cloud project number Play Integrity standard requests are
  /// made under (SEC-003); required on Android release builds, ignored
  /// elsewhere.
  final int? playIntegrityCloudProjectNumber;

  /// Where the release store lives; the platform's non-backed-up app
  /// storage when null.
  final String? storageDirectory;

  /// Creates the HTTP client of the sync isolate; the platform's HTTP/2
  /// client (Cronet, `URLSession`) when null. It must be a top-level or
  /// static function, since it runs on another isolate.
  final http.Client Function()? httpClient;

  /// Creates the `dart:io` client WebSocket connections are made with
  /// (for a host that trusts its own certificate authority, say); the
  /// `dart:io` default when null. `httpClient` does not reach WebSockets.
  /// It must be a top-level or static function, since it runs on the
  /// data isolate.
  final HttpClient Function()? webSocketClient;

  /// Called with every error the runtime reports.
  final PluxErrorHandler? onError;

  /// The app-level fallback (RT-020, RT-022); Plux's themed
  /// `PluxDefaultFallback` when null.
  final PluxFallbackBuilder? fallbackBuilder;

  /// The screen shown in place of every Plux page while a mandatory update
  /// is pending (REL-070); Plux's simple `PluxDefaultUpdateScreen` when
  /// null.
  final PluxUpdateScreenBuilder? updateScreen;

  /// Fallbacks for single plugins by plugin key, used instead of
  /// [fallbackBuilder] for that plugin's pages and component instances
  /// (RT-020). A plugin that is switched off shows the fallback page it
  /// declares first (RT-022).
  final Map<String, PluxFallbackBuilder> pluginFallbackBuilders;

  /// Builds the page shown for an unknown route [route] when the app's
  /// document names no not-found route (NAV-011); Plux's own page when
  /// null.
  final Widget Function(BuildContext context, String route)? notFoundBuilder;

  /// The seam every navigation goes through (ADR-0040): Plux resolves and
  /// checks the route, the delegate changes the stack. The nearest
  /// `Navigator`, with the plain API, when null; `plux_go_router` and
  /// `plux_auto_route` provide delegates for their routers.
  final PluxNavigationDelegate? navigationDelegate;

  /// The key of the app's root `Navigator` (`MaterialApp.navigatorKey`):
  /// where `Plux.handleDeepLink` and `Plux.handlePushPayload` open their
  /// routes, since links and notifications arrive outside any widget
  /// (NAV-008). Without it they open nothing and report `PLX-4102`; a
  /// router adapter's key is used when it is null.
  final GlobalKey<NavigatorState>? navigatorKey;

  /// The host's routes plugins may open by name (NAV-002, HST-031), checked
  /// against the native catalogue; an unregistered one reports `PLX-4200`.
  final Map<String, PluxNativeRoute<Object?, Object?>> nativeRoutes;

  /// The host's widgets plugin pages place as native slots, by the type
  /// name of the catalogue (WGT-033); an unregistered one shows the neutral
  /// placeholder and reports `PLX-4201`.
  final Map<String, PluxNativeSlot> nativeSlots;

  /// The host's functions plugins call with `callNative` (ACT-060); an
  /// unregistered one fails its step with `PLX-4202`.
  final Map<String, PluxNativeAction<Object?, Object?>> nativeActions;

  /// A router adapter, such as `PluxGoRouter(router)` of `plux_go_router`:
  /// Plux navigates through the router, unless [navigationDelegate] is set,
  /// and plugins open the router's named routes as native routes
  /// ([nativeRoutes] entries of the same name win) (HST-031, ADR-0040).
  final PluxRouterAdapter? router;

  /// The optional device packages the app installs, such as `PluxMedia()` of
  /// `plux_media` (RT-060): their actions run only when registered here; a
  /// plugin that uses another fails its step with `PLX-5401`. Each package
  /// is recorded in the native catalogue by `plux native scan`.
  final List<PluxDevicePackage> devicePackages;

  /// The optional widget packages the app installs, such as `PluxCharts()`
  /// of `plux_charts` (RT-060, ADR-0071): their builders render the
  /// widgets the packages' descriptors declare. A node of a package that
  /// is not registered here shows the node fallback and is reported as
  /// `PLX-4403`, once per widget type.
  final List<PluxWidgetPackage> widgetPackages;

  /// The device function engine, `PluxFunctions()` of `plux_functions`
  /// (FN-050, E9): functions placed on the device run only when it is
  /// registered; without it a device-placed `invokeFunction` step fails with
  /// `PLX-5401`. Server-placed functions need no package.
  final PluxFunctionEngine? functionEngine;

  /// Narrows the device APIs plugins may use to these, such as `camera` or
  /// `location`, at run time (SEC-080): an operation outside the set is
  /// blocked and reported with `PLX-5400`, whatever the app approved. Null
  /// allows every device API the app approved and the plugin declares.
  final Set<String>? allowedCapabilities;

  /// Where plugins' collections and key-value entries are stored
  /// (DB-001, DB-003, ADR-0049): `PluxDriftAdapter` of `plux_db_drift`, or
  /// the host's own adapter, for example over a database the app already
  /// has. Without one, the core's encrypted built-in store keeps the
  /// key-value entries and a plugin that declares a collection reports
  /// `PLX-5201`.
  final PluxDatabaseAdapter? databaseAdapter;

  /// The security modules the app installs, such as `PluxSecurity()` of
  /// `plux_security` (RT-060, SEC-070, ADR-0057): each is attached once the
  /// runtime is up and detached when it is disposed. Without one nothing
  /// is detected, and a device never reaches `AL3`.
  final List<PluxSecurityModule> securityModules;

  /// Whether the device registers a second hardware key that signs
  /// transaction confirmations (SEC-027, ADR-0056). The key needs the user's
  /// biometric or device credential for every signature, so it can be made
  /// only on a device with a screen lock; where it cannot be made, the
  /// device registers without it and transactions fail with `PLX-6017`.
  /// Set it for an app whose data sources declare a `transaction`; on Android
  /// the host activity must be a `FlutterFragmentActivity`, since the system
  /// prompt needs a `FragmentActivity`, and on iOS the app's `Info.plist`
  /// needs `NSFaceIDUsageDescription`. A device registered before the flag
  /// was set registers again with the key the first time it signs.
  final bool strongCustomerAuthentication;

  /// A reconstruction release to show instead of the app's own content
  /// (SEC-142, ADR-0082): the runtime runs in reconstruction mode, as the
  /// viewer host uses it. [rootKeys] must hold the root keys the host
  /// pinned at build time, from which the runtime walks the release's signed
  /// root chain to the `targets` keys it verifies the manifest with
  /// (SEC-051); with none the release is refused. [appId], [environment] and
  /// [endpoint] name the reconstructed app, its environment and the server.
  ///
  /// The release is verified like any other, before anything in it is
  /// parsed (SEC-052), and installed into a store of its own, which is
  /// deleted when the runtime is disposed. Nothing is synced, nothing is
  /// reported to telemetry, no data source is reached, and no request goes
  /// to any host but the [endpoint]. Actions that reach beyond the page
  /// (data sources, functions, the host, the device, links out of the app)
  /// fail with `PLX-4010`. Show the result with `PluxReconstructionView`.
  final PluxReconstruction? reconstruction;
}
