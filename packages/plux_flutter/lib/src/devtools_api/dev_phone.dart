// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// A phone signed in to the Plux Dev app (DEV-002, ADR-0104): it lists its
/// person's apps and joins their Studio sessions, each join confirmed with a
/// number Studio shows. Its sign-in is a phone token bound to a key the
/// platform keeps (hardware where it has one), sent only with a DPoP proof
/// of that key, and kept in the platform's encrypted secret store.
library;

import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/devtools_api/dev_pairing_link.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart'
    show kPluxDevtools;
import 'package:plux_flutter/src/platform/platform_services.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/dpop.dart';
import 'package:plux_flutter/src/store/kv_store.dart';
import 'package:plux_flutter/src/sync/api_client.dart';

/// Why a phone's call failed, in the terms the Dev app shows.
enum DevPhoneFailure {
  /// The number picked is not the one Studio showed (PLX-9210).
  wrongNumber,

  /// The person has no live Studio session of the app (PLX-9211).
  noLiveSession,

  /// The QR code or the request expired, was used, cancelled or denied
  /// (PLX-9212).
  closed,

  /// The phone's sign-in is no longer valid: it lapsed or was revoked; the
  /// phone is signed out.
  signedOut,

  /// Too many attempts; the server asks to wait.
  rateLimited,

  /// The server could not be reached.
  unreachable,

  /// Anything else; the message says what.
  other,
}

/// A failed call of a phone.
final class DevPhoneException implements Exception {
  /// Creates the exception.
  const DevPhoneException(this.failure, this.message);

  /// What failed.
  final DevPhoneFailure failure;

  /// The server's words, for a detail line.
  final String message;

  @override
  String toString() => 'DevPhoneException(${failure.name}): $message';
}

/// Whom a phone is signed in as, on which server.
@immutable
final class DevPhoneAccount {
  /// Creates the description.
  const DevPhoneAccount({
    required this.server,
    required this.phoneId,
    required this.userDisplay,
    required this.organizationName,
  });

  /// The server's base URL.
  final Uri server;

  /// The phone's identifier on the server.
  final String phoneId;

  /// The person's display name.
  final String userDisplay;

  /// The organisation's name.
  final String organizationName;
}

/// An app a signed-in phone may open.
@immutable
final class DevPhoneApp {
  /// Creates the description.
  const DevPhoneApp({required this.id, required this.name, required this.live});

  /// The app's identifier.
  final String id;

  /// The app's name.
  final String name;

  /// Whether the person has the app open in Studio, so that the phone can
  /// join its session now.
  final bool live;
}

/// A phone's request to join a session: the numbers to offer, or, for a
/// phone that joined this session before, its pairing link at once.
@immutable
final class DevJoin {
  /// Creates the request.
  const DevJoin({
    required this.requestId,
    required this.sessionId,
    required this.environment,
    required this.options,
    required this.expiresAt,
    this.pairingLink,
  });

  /// The request's identifier; empty when no number is needed.
  final String requestId;

  /// The session the phone asks to join.
  final String sessionId;

  /// The key of the session's environment, the one the runtime syncs.
  final String environment;

  /// The numbers to offer, in the order to show them; one is the number
  /// Studio shows. Empty when [pairingLink] is set.
  final List<int> options;

  /// When the request stops being accepted.
  final DateTime? expiresAt;

  /// The `plux-dev://pair?...` link to pair with, when no number is needed;
  /// a credential, never logged.
  final String? pairingLink;

  /// Whether the phone needs to pick a number first.
  bool get needsNumber => pairingLink == null;

  @override
  String toString() => 'DevJoin($sessionId, $environment, $options)';
}

/// A sign-in a phone claimed: the numbers to offer and whom it signs in as.
final class DevPhoneClaim {
  DevPhoneClaim._(
    this._link,
    this._api,
    this._proofs,
    this._secrets,
    this.options,
    this.expiresAt,
    this.userDisplay,
    this.organizationName,
  );

  final DevSignInLink _link;
  final PluxApiClient _api;
  final DpopProofs _proofs;
  final SecretStore _secrets;

  /// The numbers to offer, in the order to show them; one is the number
  /// Studio shows.
  final List<int> options;

  /// When the sign-in stops being accepted.
  final DateTime? expiresAt;

  /// The person the phone signs in as.
  final String userDisplay;

  /// Their organisation.
  final String organizationName;

  /// Picks [number]: Studio's number signs the phone in and keeps its
  /// account in the platform's secret store; another ends the sign-in
  /// ([DevPhoneFailure.wrongNumber]).
  Future<DevPhone> complete(int number) async {
    final r = await _guard(
      () =>
          _api.phoneCompleteSignIn(_proofs, grant: _link.grant, number: number),
    );
    final token = r['token'];
    if (token is! String || token.isEmpty) {
      throw const DevPhoneException(
        DevPhoneFailure.other,
        'the server sent no phone token',
      );
    }
    final account = DevPhoneAccount(
      server: _link.server,
      phoneId: r['phoneId'] as String? ?? '',
      userDisplay: r['userDisplay'] as String? ?? userDisplay,
      organizationName: r['organizationName'] as String? ?? organizationName,
    );
    await _secrets.write(
      DevPhone.secretName,
      jsonEncode({
        'server': account.server.toString(),
        'token': token,
        'phoneId': account.phoneId,
        'userDisplay': account.userDisplay,
        'organizationName': account.organizationName,
      }),
    );
    return DevPhone._(account, _api, _phoneToken(token, _proofs), _secrets);
  }
}

/// A phone signed in to the Plux Dev app (DEV-002, ADR-0104).
///
/// ```dart
/// final claim = await DevPhone.claim(link, client: devHttpClient, name: 'Pixel 8', platform: 'android');
/// final phone = await claim.complete(picked);   // the number Studio shows
/// final apps = await phone.apps();
/// final join = await phone.requestJoin(apps.first.id);
/// final link = join.pairingLink ?? await phone.completeJoin(join, picked);
/// ```
final class DevPhone {
  DevPhone._(this.account, this._api, this._token, this._secrets);

  /// The alias of the phone's DPoP key, one per installation of the Dev
  /// app, in the platform's keystore.
  static const String keyAlias = 'dev.plux.phone';

  /// The name of the secret the phone's account is kept under, encrypted by
  /// the platform (ADR-0029).
  static const String secretName = 'dev.plux.phone.account';

  /// Whom the phone is signed in as.
  final DevPhoneAccount account;

  final PluxApiClient _api;
  final DeviceToken _token;
  final SecretStore _secrets;

  /// Claims the sign-in of [link], the QR code Studio shows, with the
  /// phone's key, created when the phone has none. [name] and [platform]
  /// describe the phone in its person's list. [client] makes the HTTP client
  /// (the Dev app's trusts its development authority); [keys] and [secrets]
  /// replace the platform's in tests.
  static Future<DevPhoneClaim> claim(
    DevSignInLink link, {
    required http.Client Function() client,
    required String name,
    required String platform,
    DeviceKeys keys = const PlatformDeviceKeys(),
    SecretStore secrets = const PlatformSecretStore(),
  }) async {
    _available();
    final proofs = await _proofsOf(keys, create: true);
    final api = PluxApiClient(client(), link.server);
    final r = await _guard(
      () => api.phoneClaimSignIn(
        proofs!,
        grant: link.grant,
        name: name,
        platform: platform,
      ),
    );
    return DevPhoneClaim._(
      link,
      api,
      proofs!,
      secrets,
      _numbers(r['options']),
      DateTime.tryParse(r['expiresAt'] as String? ?? ''),
      r['userDisplay'] as String? ?? '',
      r['organizationName'] as String? ?? '',
    );
  }

  /// The phone signed in earlier on this device, or null when there is none
  /// or its key is gone (the platform cleared it): it signs in again.
  static Future<DevPhone?> restore({
    required http.Client Function() client,
    DeviceKeys keys = const PlatformDeviceKeys(),
    SecretStore secrets = const PlatformSecretStore(),
  }) async {
    _available();
    final stored = await secrets.read(secretName);
    if (stored == null) return null;
    final Map<String, Object?> j;
    final Uri server;
    try {
      j = jsonDecode(stored) as Map<String, Object?>;
      server = Uri.parse(j['server']! as String);
    } on Object {
      await secrets.delete(secretName);
      return null;
    }
    final token = j['token'];
    final proofs = await _proofsOf(keys, create: false);
    if (token is! String || token.isEmpty || proofs == null) {
      await secrets.delete(secretName);
      return null;
    }
    return DevPhone._(
      DevPhoneAccount(
        server: server,
        phoneId: j['phoneId'] as String? ?? '',
        userDisplay: j['userDisplay'] as String? ?? '',
        organizationName: j['organizationName'] as String? ?? '',
      ),
      PluxApiClient(client(), server),
      _phoneToken(token, proofs),
      secrets,
    );
  }

  /// The apps the phone's person may open, and which they have open in
  /// Studio. A phone whose sign-in lapsed or was revoked is signed out here
  /// ([DevPhoneFailure.signedOut]).
  Future<List<DevPhoneApp>> apps() async {
    final r = await _call('ListPhoneApps', const {});
    return [
      for (final a in (r['apps'] as List<Object?>? ?? const []))
        if (a case final Map<String, Object?> m)
          DevPhoneApp(
            id: m['id'] as String? ?? '',
            name: m['name'] as String? ?? '',
            live: m['live'] == true,
          ),
    ];
  }

  /// Asks to join the person's live Studio session of [appId]
  /// ([DevPhoneFailure.noLiveSession] when Studio has none open).
  Future<DevJoin> requestJoin(String appId) async {
    final r = await _call('RequestJoin', {'appId': appId});
    final link = r['pairingLink'] as String?;
    return DevJoin(
      requestId: r['requestId'] as String? ?? '',
      sessionId: r['sessionId'] as String? ?? '',
      environment: r['environment'] as String? ?? '',
      options: _numbers(r['options']),
      expiresAt: DateTime.tryParse(r['expiresAt'] as String? ?? ''),
      pairingLink: link == null || link.isEmpty ? null : link,
    );
  }

  /// Picks [number] for [join]; Studio's number returns the pairing link to
  /// pair the runtime with, another refuses ([DevPhoneFailure.wrongNumber])
  /// and keeps the phone out until its person lets it try again in Studio.
  Future<String> completeJoin(DevJoin join, int number) async {
    if (join.pairingLink case final link?) return link;
    final r = await _call('CompleteJoin', {
      'requestId': join.requestId,
      'number': number,
    });
    final link = r['pairingLink'];
    if (link is! String || link.isEmpty) {
      throw const DevPhoneException(
        DevPhoneFailure.other,
        'the server sent no pairing link',
      );
    }
    return link;
  }

  /// Signs the phone out on the server, where it can, and forgets it here.
  Future<void> signOut() async {
    try {
      await _call('SignOutPhone', const {});
    } on DevPhoneException {
      // Signed out already, or unreachable: it is forgotten here either way.
    }
    await forget();
  }

  /// Forgets the phone's sign-in on this device.
  Future<void> forget() => _secrets.delete(secretName);

  Future<Map<String, Object?>> _call(
    String method,
    Map<String, Object?> body,
  ) async {
    try {
      return await _guard(() => _api.phoneCall(_token, method, body));
    } on DevPhoneException catch (e) {
      if (e.failure == DevPhoneFailure.signedOut) await forget();
      rethrow;
    }
  }

  static void _available() {
    if (!kPluxDevtools) {
      throw StateError('the dev channel is not part of this build');
    }
  }

  /// The proofs of the phone's key: loaded, or, with [create], made when
  /// the phone has none; null when there is none and [create] is false.
  static Future<DpopProofs?> _proofsOf(
    DeviceKeys keys, {
    required bool create,
  }) async {
    var key = await keys.load(keyAlias, KeyPurpose.dpop);
    if (key == null && create) {
      key = await keys.create(keyAlias, KeyPurpose.dpop);
    }
    if (key == null) return null;
    return DpopProofs(keys: keys, alias: keyAlias, publicKey: key.publicKey);
  }
}

/// A phone token as the API client sends it: `Authorization: DPoP` with a
/// proof of the phone's key. It lapses on the server, never here.
DeviceToken _phoneToken(String value, DpopProofs proofs) =>
    DeviceToken(value, DateTime.utc(9999), proofs);

List<int> _numbers(Object? v) => [
  for (final n in (v as List<Object?>? ?? const []))
    if (n is int)
      n
    else if (n is String && int.tryParse(n) != null)
      int.parse(n),
];

/// Runs [call] and turns its failure into a [DevPhoneException].
Future<Map<String, Object?>> _guard(
  Future<Map<String, Object?>> Function() call,
) async {
  try {
    return await call();
  } on ApiError catch (e) {
    throw DevPhoneException(_failureOf(e), e.message);
  }
}

DevPhoneFailure _failureOf(ApiError e) {
  switch (e.plxCode) {
    case 9210:
      return DevPhoneFailure.wrongNumber;
    case 9211:
      return DevPhoneFailure.noLiveSession;
    case 9212:
      return DevPhoneFailure.closed;
  }
  if (e.status == 0) return DevPhoneFailure.unreachable;
  if (e.status == 401) return DevPhoneFailure.signedOut;
  if (e.status == 429 || e.code == 'resource_exhausted') {
    return DevPhoneFailure.rateLimited;
  }
  return DevPhoneFailure.other;
}
