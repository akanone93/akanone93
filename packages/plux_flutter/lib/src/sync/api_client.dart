// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The device side of the Plux API (ADR-0005, ADR-0021): ConnectRPC with
/// its JSON encoding over the `http` client, so the runtime needs no
/// protobuf library. Only the calls a device makes are here.
library;

import 'dart:convert';
import 'dart:io' show gzip;
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/src/data/live_config.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/assurance.dart';
import 'package:plux_flutter/src/security/attestation.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/dpop.dart';
import 'package:plux_flutter/src/security/sca.dart';
import 'package:plux_flutter/src/sync/device_targeting.dart';

/// A failed call: the HTTP status, the Connect error code and the server's
/// `Retry-After`, when it sent one.
final class ApiError implements Exception {
  /// Creates the error.
  const ApiError(this.status, this.code, this.message, {this.retryAfter});

  /// The HTTP status; 0 when no response arrived.
  final int status;

  /// The Connect code, such as `unavailable` or `unauthenticated`.
  final String code;

  /// The server's message.
  final String message;

  /// How long the server asked the client to wait.
  final Duration? retryAfter;

  /// The number of the Plux error code the server's message starts with
  /// (`PLX-6007: …` is 6007), or null when it carries none.
  int? get plxCode {
    final m = RegExp(r'^PLX-(\d{4})').firstMatch(message);
    return m == null ? null : int.parse(m[1]!);
  }

  /// Whether retrying the same call may succeed.
  bool get retryable =>
      status == 0 ||
      status == 408 ||
      status == 429 ||
      status >= 500 ||
      code == 'unavailable' ||
      code == 'resource_exhausted';

  /// The error as the runtime reports it (`PLX-3050`).
  PluxException toException(String call) => PluxException(
    PluxErrorCode.syncFailed,
    '$call failed: $status $code: $message',
    details: {'call': call, 'status': '$status', 'code': code},
  );

  @override
  String toString() => 'ApiError($status, $code, $message)';
}

/// A device's registration: its ID and the thumbprint of the DPoP key it
/// registered. The key's private half stays in the platform (SEC-001), and
/// there is no shared secret (SEC-020).
final class DeviceCredential {
  /// Creates a credential.
  const DeviceCredential(this.deviceId, this.jkt);

  /// The device ID.
  final String deviceId;

  /// The thumbprint (RFC 7638) of the DPoP public key bound to the device.
  final String jkt;

  @override
  String toString() => 'DeviceCredential($deviceId)';
}

/// An access token with the proofs that bind requests to it (RFC 9449): the
/// token alone is useless without the device's key.
final class DeviceToken {
  /// Creates a token.
  const DeviceToken(
    this.value,
    this.expiresAt,
    this.proofs, {
    this.assurance = 0,
  });

  /// The access token; never logged or reported (SEC-092).
  final String value;

  /// When the server stops accepting [value].
  final DateTime expiresAt;

  /// Builds the DPoP proofs of the device key the token is bound to.
  final DpopProofs proofs;

  /// The device's assurance level, 0 to 3 for `AL0` to `AL3`, as the
  /// server computed it (SEC-007); 0 when the server named none.
  final int assurance;

  @override
  String toString() => 'DeviceToken([redacted])';
}

/// The header that carries the Plux user token on a gateway call, outside the
/// headers the upstream receives (SEC-026).
const String pluxUserTokenHeader = 'Plux-User-Token';

/// The RFC 8693 token type of the host's access token.
const String accessTokenType = 'urn:ietf:params:oauth:token-type:access_token';

/// The gateway's answer: the upstream's status, the response headers the
/// server passes on and the upstream's body.
final class GatewayReply {
  /// Creates the reply.
  const GatewayReply({
    required this.status,
    required this.headers,
    required this.body,
  });

  /// The upstream's HTTP status.
  final int status;

  /// The response headers, names in lower case.
  final Map<String, String> headers;

  /// The upstream's body.
  final Uint8List body;
}

/// The answer of `FunctionService.Invoke`: the function's output, or the
/// business error it returned (FN-022).
final class FunctionInvokeReply {
  /// The function's output, UTF-8 JSON.
  const FunctionInvokeReply.output(Uint8List this.output)
    : code = null,
      messageKey = null,
      params = null;

  /// A business error the function returned.
  const FunctionInvokeReply.business({
    required String this.code,
    required String this.messageKey,
    required Map<String, Object?> this.params,
  }) : output = null;

  /// The output, or null for a business error.
  final Uint8List? output;

  /// The function-defined error code.
  final String? code;

  /// The message key.
  final String? messageKey;

  /// The message values.
  final Map<String, Object?>? params;
}

/// A Plux user token (SEC-026): the host's end-user token exchanged for one
/// scoped to an upstream, kept in memory only and never logged.
final class UserToken {
  /// Creates a token.
  const UserToken(this.value, this.expiresAt);

  /// The token.
  final String value;

  /// When the server stops accepting it.
  final DateTime expiresAt;

  @override
  String toString() => 'UserToken([redacted])';
}

/// What a device reports about itself at registration.
final class DeviceInfo {
  /// Creates the description.
  const DeviceInfo({
    required this.platform,
    required this.osVersion,
    required this.runtimeVersion,
    required this.hostBuild,
  });

  /// `android` or `ios`.
  final String platform;

  /// The operating system version.
  final String osVersion;

  /// The runtime's version.
  final String runtimeVersion;

  /// The host app's build.
  final String hostBuild;
}

/// The longest version or build string the server records for a device.
const maxDeviceField = 64;

/// The operating system version a device reports, from the platform's own
/// string: without the build details Linux kernels, and so Android, append
/// after ` #` (`Linux 5.10.157-android13-4 #1 SMP PREEMPT <date>`), in
/// ASCII and at most [maxDeviceField] bytes; the server refuses a
/// registration with a longer one.
String deviceOsVersion(String platformVersion) {
  final build = platformVersion.indexOf(' #');
  final s = (build < 0 ? platformVersion : platformVersion.substring(0, build))
      .trim();
  return String.fromCharCodes(
    s.runes.where((r) => r < 0x80).take(maxDeviceField),
  );
}

/// One bundle of a served manifest with this device's step for it.
final class ServedBundle {
  const ServedBundle._({
    required this.key,
    required this.hash,
    required this.size,
    required this.url,
    required this.action,
    required this.from,
    required this.stepUrl,
    required this.stepSize,
  });

  /// The plugin key; empty for the app bundle.
  final String key;

  /// The bundle hash, `sha256:<hex>`.
  final String hash;

  /// The bundle's size.
  final int size;

  /// Where the full bundle is.
  final Uri url;

  /// `keep`, `delta` or `full`.
  final String action;

  /// For a delta, the installed bundle it applies to.
  final String from;

  /// The delta or full bundle to download.
  final Uri? stepUrl;

  /// The size of what [stepUrl] serves.
  final int stepSize;
}

/// The update metadata versions a manifest belongs to (SEC-050): hints for
/// what to fetch, never trusted; the signed documents decide.
final class MetadataRef {
  /// Creates the reference.
  const MetadataRef({
    required this.rootVersion,
    required this.snapshotVersion,
    required this.timestampVersion,
    this.environmentId = '',
  });

  /// The root the metadata was signed under.
  final int rootVersion;

  /// The snapshot that pins the manifest.
  final int snapshotVersion;

  /// The timestamp that named that snapshot.
  final int timestampVersion;

  /// The environment whose files hold the metadata.
  final String environmentId;
}

/// A key of the environment's current root, as `GetRootKeys` lists it.
final class RootKeyInfo {
  /// Creates the description.
  const RootKeyInfo({
    required this.keyId,
    required this.algorithm,
    required this.role,
    required this.environmentType,
    required this.publicKey,
  });

  /// The key's ID.
  final String keyId;

  /// The algorithm name.
  final String algorithm;

  /// The role the key holds.
  final String role;

  /// `production` or `development` (SEC-056).
  final String environmentType;

  /// The raw public key.
  final Uint8List publicKey;
}

/// The answer of `GetRootKeys`: the keys, and the root files after the
/// version the device trusts, oldest first (SEC-051).
final class RootKeys {
  /// Creates the answer.
  const RootKeys(this.keys, this.roots);

  /// The keys of the current root.
  final List<RootKeyInfo> keys;

  /// The root files after the version asked for.
  final List<Uint8List> roots;
}

/// A manifest response: not modified, or the signed document with this
/// device's plan.
final class ManifestResponse {
  const ManifestResponse._({
    required this.notModified,
    required this.etag,
    this.installedRequired = false,
    this.signed,
    this.signatures = const [],
    this.bundles = const [],
    this.metadata,
    this.configPatch,
    this.configFullRequired = false,
    this.contentKey,
  });

  /// Whether the manifest matches the ETag the device sent (NFR-006).
  final bool notModified;

  /// The manifest's ETag.
  final String etag;

  /// Whether the server asks for the installed bundles a request sent
  /// only the digest of: the manifest changed, and its plan needs them.
  final bool installedRequired;

  /// The signed document's canonical bytes.
  final Uint8List? signed;

  /// Its signatures, as `{keyid, alg, sig}`.
  final List<Map<String, Object?>> signatures;

  /// The plan per bundle: the app bundle and every plugin.
  final List<ServedBundle> bundles;

  /// The update metadata the manifest belongs to; null for an environment
  /// without a root (SEC-050).
  final MetadataRef? metadata;

  /// The RFC 7396 merge patch, as UTF-8 JSON, from the configuration
  /// version the device sent to the one the manifest names; outside the
  /// signed part, so it is checked against the manifest's hash (SEC-182).
  final Uint8List? configPatch;

  /// Whether the server could give no patch from the version the device
  /// sent: the device asks again from version 0 (SEC-182).
  final bool configFullRequired;

  /// The content key of a confidential release, wrapped for this device:
  /// the server's ephemeral key and the AES-KW output. Outside the signed
  /// part (SEC-053); null when no bundle is confidential.
  final ({Uint8List ephemeralPublicKey, Uint8List wrappedKey})? contentKey;
}

/// A control document response: not modified, or the signed file.
final class ControlResponse {
  const ControlResponse._({
    required this.notModified,
    required this.etag,
    this.signed,
  });

  /// Whether the document matches the ETag the device sent.
  final bool notModified;

  /// The document's ETag.
  final String etag;

  /// The signed file, not yet verified.
  final Uint8List? signed;
}

/// What a device sends in place of its installed bundles on an
/// up-to-date check (NFR-006, ADR-0037): the SHA-256 of one line
/// `<key>:<sha256 hex>\n` per bundle, sorted by key, the app bundle's key
/// empty. [installed] maps a key to its bundle's hex hash.
List<int> installedDigest(Map<String, String> installed) {
  final keys = installed.keys.toList()..sort();
  final lines = StringBuffer();
  for (final k in keys) {
    lines.write('$k:${installed[k]}\n');
  }
  return sha256.convert(utf8.encode(lines.toString())).bytes;
}

/// The device API client.
final class PluxApiClient {
  /// Creates a client for the server at [endpoint], which may carry a
  /// base path (`https://example.com/plux`).
  PluxApiClient(this._http, Uri endpoint)
    : endpoint = endpoint.path.endsWith('/')
          ? endpoint
          : endpoint.replace(path: '${endpoint.path}/');

  final http.Client _http;

  /// The server's base URL.
  final Uri endpoint;

  /// The latest `DPoP-Nonce` the server sent; every proof carries it
  /// (RFC 9449 §8).
  String? _nonce;

  /// Asks for a registration challenge (SEC-002): bytes the evidence and
  /// the DPoP key are bound to, valid once and briefly.
  Future<Uint8List> registrationChallenge({
    required String appId,
    required String environment,
  }) async {
    final r = await _call('plux.v1.DeviceService/CreateRegistrationChallenge', {
      'appId': appId,
      'environment': environment,
    });
    try {
      return base64.decode(r['challenge']! as String);
    } on Object {
      throw const ApiError(0, 'unknown', 'the server sent no challenge');
    }
  }

  /// Registers this installation with the platform's evidence for [key]
  /// (GOV-010, SEC-002); the device ID.
  Future<String> registerAttestedDevice({
    required String appId,
    required String environment,
    required DeviceInfo device,
    required Uint8List challenge,
    required DeviceKey key,
    required AttestationEvidence evidence,
    DeviceKey? agreementKey,
    DeviceKey? scaKey,
    AttestationEvidence? scaEvidence,
  }) async {
    final r = await _call('plux.v1.DeviceService/RegisterAttestedDevice', {
      'appId': appId,
      'environment': environment,
      'platform': device.platform,
      'osVersion': device.osVersion,
      'runtimeVersion': device.runtimeVersion,
      'hostBuild': device.hostBuild,
      'challenge': base64.encode(challenge),
      'dpopPublicKeyJwk': base64.encode(
        utf8.encode(jsonEncode(key.publicKey.jwk)),
      ),
      // Development evidence proves nothing about the key, so the device
      // claims only software storage with it (SEC-008).
      'keyStorage': _keyStorage(
        evidence is DevelopmentEvidence ? KeyStorage.software : key.storage,
      ),
      'evidence': _evidence(evidence),
      // The key-agreement key lets the server wrap the content key of a
      // confidential release to this device (SEC-053).
      if (agreementKey != null)
        'keyAgreementPublicKeyJwk': base64.encode(
          utf8.encode(jsonEncode(agreementKey.publicKey.jwk)),
        ),
      // With Android evidence, the key-agreement key's own Key Attestation
      // chain: the server requires hardware for it wherever the DPoP key is
      // hardware. Beside any other evidence a chain is refused (PLX-6001).
      if (agreementKey != null &&
          evidence is AndroidEvidence &&
          agreementKey.attestationChain.isNotEmpty)
        'keyAgreementKeyAttestationChain': [
          for (final c in agreementKey.attestationChain) base64.encode(c),
        ],
      // The key that signs transaction confirmations, with its own
      // evidence (SEC-027).
      if (scaKey != null && scaEvidence != null) ...{
        'scaPublicKeyJwk': base64.encode(
          utf8.encode(jsonEncode(scaKey.publicKey.jwk)),
        ),
        'scaEvidence': _evidence(scaEvidence),
      },
    });
    final d = r['device'];
    final id = d is Map<String, Object?> ? d['id'] : null;
    if (id is! String) {
      throw const ApiError(0, 'unknown', 'the server sent no device');
    }
    return id;
  }

  /// Enrols the SCA key [scaKey] of a registered device (SEC-027), bound to
  /// the fresh [challenge] by [evidence]; the key's thumbprint as the server
  /// stored it.
  Future<String> enrolScaKey({
    required DeviceToken token,
    required Uint8List challenge,
    required DeviceKey scaKey,
    required AttestationEvidence evidence,
  }) async {
    final r = await _call('plux.v1.DeviceService/EnrolScaKey', {
      'challenge': base64.encode(challenge),
      'scaPublicKeyJwk': base64.encode(
        utf8.encode(jsonEncode(scaKey.publicKey.jwk)),
      ),
      'scaEvidence': _evidence(evidence),
    }, token: token);
    final jkt = r['scaJkt'];
    if (jkt is! String || jkt.isEmpty) {
      throw const ApiError(0, 'unknown', 'the server sent no SCA key');
    }
    return jkt;
  }

  /// Offers fresh evidence for the registered key of [deviceId] after the
  /// server asked for it (SEC-025, `PLX-6007`); authenticated by the DPoP
  /// proof alone.
  Future<void> reattestDevice({
    required String deviceId,
    required Uint8List challenge,
    required AttestationEvidence evidence,
    required DpopProofs proofs,
  }) async {
    await _call('plux.v1.DeviceService/ReattestDevice', {
      'deviceId': deviceId,
      'challenge': base64.encode(challenge),
      'evidence': _evidence(evidence),
    }, proofs: proofs);
  }

  /// Exchanges a DPoP proof for a short-lived access token bound to the
  /// device key, kept in memory only (SEC-021). With [assertion] the
  /// request carries an App Attest assertion over the SHA-256 of that
  /// request's proof, when the callback returns one (SEC-025).
  Future<DeviceToken> refreshToken({
    required String deviceId,
    required DpopProofs proofs,
    required DateTime now,
    Future<Uint8List?> Function(Uint8List clientDataHash)? assertion,
  }) async {
    final r = await _call(
      'plux.v1.TokenService/RefreshDeviceToken',
      {'deviceId': deviceId},
      proofs: proofs,
      extend: assertion == null
          ? null
          : (proof) async {
              final a = await assertion(
                Uint8List.fromList(sha256.convert(utf8.encode(proof)).bytes),
              );
              return {if (a != null) 'appAttestAssertion': base64.encode(a)};
            },
    );
    final token = r['accessToken'];
    if (token is! String || token.isEmpty) {
      throw const ApiError(0, 'unknown', 'the server sent no access token');
    }
    final expires = DateTime.tryParse(r['expiresAt'] as String? ?? '');
    return DeviceToken(
      token,
      expires ?? now,
      proofs,
      assurance: assuranceOf(r['assuranceLevel']),
    );
  }

  /// Fetches the channel's manifest with this device's plan (REL-032).
  /// [targeting] tells the server what a rollout may target the device by,
  /// hashed user attributes only (REL-051).
  Future<ManifestResponse> manifest({
    required DeviceToken token,
    required String appId,
    required String environment,
    required String channel,
    required int installedSequence,
    required Map<String, String> installed,
    required String ifNoneMatch,
    List<int> installedDigest = const [],
    int configVersion = 0,
    DeviceTargeting? targeting,
  }) async {
    final r = await _call('plux.v1.ManifestService/GetManifest', {
      'appId': appId,
      'environment': environment,
      'channel': channel,
      'installedSequence': '$installedSequence',
      'installed': [
        for (final MapEntry(:key, :value) in installed.entries)
          {'key': key, 'sha256': value},
      ],
      'ifNoneMatch': ifNoneMatch,
      'configVersion': '$configVersion',
      if (installedDigest.isNotEmpty)
        'installedDigest': base64.encode(installedDigest),
      if (targeting != null) 'deviceTargeting': targeting.toJson(),
    }, token: token);
    final etag = r['etag'] as String? ?? '';
    if (r['notModified'] == true) {
      return ManifestResponse._(notModified: true, etag: etag);
    }
    if (r['installedRequired'] == true) {
      return ManifestResponse._(
        notModified: false,
        etag: etag,
        installedRequired: true,
      );
    }
    final m = r['manifest']! as Map<String, Object?>;
    Uint8List? patch;
    final encoded = r['configPatch'];
    if (encoded is String && encoded.isNotEmpty) {
      try {
        patch = base64.decode(encoded);
      } on FormatException {
        // Not a patch: the device asks for the whole configuration.
        patch = null;
      }
    }
    ServedBundle bundle(String key, Map<String, Object?> b) {
      final step = b['sync'] as Map<String, Object?>? ?? const {};
      final stepUrl = step['url'] as String?;
      return ServedBundle._(
        key: key,
        hash: b['sha256']! as String,
        size: _int(b['size']),
        url: endpoint.resolve(b['url'] as String? ?? ''),
        action: step['action'] as String? ?? 'full',
        from: step['from'] as String? ?? '',
        stepUrl: stepUrl == null || stepUrl.isEmpty
            ? null
            : endpoint.resolve(stepUrl),
        stepSize: _int(step['size']),
      );
    }

    final ref = m['metadata'];
    return ManifestResponse._(
      notModified: false,
      etag: etag,
      metadata: ref is Map<String, Object?>
          ? MetadataRef(
              rootVersion: _int(ref['rootVersion']),
              snapshotVersion: _int(ref['snapshotVersion']),
              timestampVersion: _int(ref['timestampVersion']),
              environmentId: ref['environmentId'] as String? ?? '',
            )
          : null,
      configPatch: patch,
      configFullRequired: r['configFullRequired'] == true,
      contentKey: switch (r['contentKey']) {
        final Map<String, Object?> k => (
          ephemeralPublicKey: base64.decode(
            k['ephemeralPublicKey'] as String? ?? '',
          ),
          wrappedKey: base64.decode(k['wrappedKey'] as String? ?? ''),
        ),
        _ => null,
      },
      signed: base64.decode(m['signed']! as String),
      signatures: [
        for (final s
            in (m['signatures'] as List<Object?>? ?? const [])
                .cast<Map<String, Object?>>())
          {'keyid': s['keyId'], 'alg': s['algorithm'], 'sig': s['signature']},
      ],
      bundles: [
        bundle('', m['appBundle']! as Map<String, Object?>),
        for (final p
            in (m['plugins'] as List<Object?>? ?? const [])
                .cast<Map<String, Object?>>())
          bundle(p['key']! as String, p['bundle']! as Map<String, Object?>),
      ],
    );
  }

  /// Fetches the signed control document of [channel] (SYN-060), with the
  /// ETag of the one the device holds; the answer is not modified, or the
  /// file to verify. Nothing in it is trusted yet (SEC-052).
  Future<ControlResponse> signedControl({
    required DeviceToken token,
    required String channel,
    required String ifNoneMatch,
  }) async {
    final r = await _call('plux.v1.ManifestService/GetSignedControl', {
      'channel': channel,
      'ifNoneMatch': ifNoneMatch,
    }, token: token);
    final etag = r['etag'] as String? ?? '';
    if (r['notModified'] == true) {
      return ControlResponse._(notModified: true, etag: etag);
    }
    final signed = r['signed'];
    if (signed is! String || signed.isEmpty) {
      throw const ApiError(200, 'unknown', 'the control document is missing');
    }
    try {
      return ControlResponse._(
        notModified: false,
        etag: etag,
        signed: base64.decode(signed),
      );
    } on FormatException {
      throw const ApiError(
        200,
        'unknown',
        'the control document is not base64',
      );
    }
  }

  /// Fetches the root files after [sinceRootVersion] and the keys of the
  /// current root (SEC-051).
  Future<RootKeys> rootKeys({
    required DeviceToken token,
    required String appId,
    required String environment,
    required int sinceRootVersion,
  }) async {
    final r = await _call('plux.v1.ManifestService/GetRootKeys', {
      'appId': appId,
      'environment': environment,
      'sinceRootVersion': '$sinceRootVersion',
    }, token: token);
    Uint8List bytes(Object? v) => base64.decode(v as String? ?? '');
    return RootKeys(
      [
        for (final k
            in (r['keys'] as List<Object?>? ?? const [])
                .cast<Map<String, Object?>>())
          RootKeyInfo(
            keyId: k['keyId'] as String? ?? '',
            algorithm: k['algorithm'] as String? ?? '',
            role: k['role'] as String? ?? '',
            environmentType: k['environmentType'] as String? ?? '',
            publicKey: bytes(k['publicKey']),
          ),
      ],
      [
        for (final root in r['roots'] as List<Object?>? ?? const [])
          bytes(root),
      ],
    );
  }

  /// Fetches a metadata file of an environment, such as `timestamp.json`
  /// or `7.snapshot.json`, at most [maxBytes] long. The files are public
  /// and signed, so no credential is sent (SEC-050).
  Future<Uint8List> metadataFile(
    String environmentId,
    String file, {
    required int maxBytes,
  }) async {
    final http.Response res;
    try {
      res = await _http.get(
        endpoint.resolve('v1/metadata/$environmentId/$file'),
      );
    } on PluxException {
      // A failed certificate pin is not an outage (SEC-041).
      rethrow;
    } on Exception catch (e) {
      throw ApiError(0, 'unavailable', '$e');
    }
    if (res.statusCode != 200) {
      throw ApiError(
        res.statusCode,
        res.statusCode == 404 ? 'not_found' : 'unavailable',
        'metadata file $file: ${res.reasonPhrase ?? res.statusCode}',
      );
    }
    if (res.bodyBytes.length > maxBytes) {
      throw ApiError(
        res.statusCode,
        'unknown',
        'metadata file $file is too large',
      );
    }
    return res.bodyBytes;
  }

  /// Reports the release this device activated, with what the user saw it
  /// in ([context], SEC-142).
  Future<void> reportInstalled(
    DeviceToken token,
    String deviceId,
    int sequence, {
    InstalledContext context = const InstalledContext(),
  }) => _call('plux.v1.DeviceService/ReportInstalled', {
    'deviceId': deviceId,
    'releaseSequence': '$sequence',
    ...context.toJson(),
  }, token: token);

  /// Sends a batch of telemetry events (ANL-002, ADR-0034), compressed
  /// with gzip; the server's counts of accepted and refused events.
  Future<({int accepted, int rejected})> ingestEvents(
    DeviceToken token, {
    required String appId,
    required String environment,
    required List<Map<String, Object?>> events,
  }) async {
    final r = await _call(
      'plux.v1.TelemetryService/IngestEvents',
      {'appId': appId, 'environment': environment, 'events': events},
      token: token,
      compress: true,
    );
    return (accepted: _int(r['accepted']), rejected: _int(r['rejected']));
  }

  /// Forwards one request to an upstream through the gateway (SEC-030,
  /// `GatewayService.Call`): authenticated by [token] and a DPoP proof. A
  /// [userToken] — the Plux user token of the token exchange — travels in
  /// the `Plux-User-Token` header, outside the headers the upstream
  /// receives (SEC-026). A transaction carries the [challengeId] and the
  /// [signature] of the SCA key (SEC-032). The [configVersion] of the `live`
  /// configuration the device used travels in `Plux-Config-Version`
  /// (DAT-096), and [onHeaders] sees the headers of the response, success or
  /// refusal, for the `Plux-Config-Epoch` (DAT-095).
  Future<GatewayReply> gatewayCall({
    required DeviceToken token,
    required String upstream,
    required String operation,
    required String method,
    required String path,
    required Map<String, String> headers,
    Uint8List? body,
    String plugin = '',
    String? userToken,
    String challengeId = '',
    Uint8List? signature,
    int? configVersion,
    void Function(Map<String, String> headers)? onHeaders,
  }) async {
    final r = await _call(
      'plux.v1.GatewayService/Call',
      {
        'upstream': upstream,
        'operation': operation,
        if (plugin.isNotEmpty) 'plugin': plugin,
        'method': method,
        'path': path,
        'headers': headers,
        if (body != null && body.isNotEmpty) 'body': base64.encode(body),
        if (challengeId.isNotEmpty) 'transactionChallengeId': challengeId,
        if (signature != null && signature.isNotEmpty)
          'transactionSignature': base64.encode(signature),
      },
      token: token,
      extraHeaders: {
        pluxUserTokenHeader: ?userToken,
        if (configVersion != null) pluxConfigVersionHeader: '$configVersion',
      },
      onHeaders: onHeaders,
    );
    try {
      final h = r['headers'];
      return GatewayReply(
        status: _int(r['status']),
        headers: {
          if (h is Map<String, Object?>)
            for (final e in h.entries)
              if (e.value is String) e.key.toLowerCase(): e.value! as String,
        },
        body: base64.decode(r['body'] as String? ?? ''),
      );
    } on FormatException {
      throw const ApiError(0, 'unknown', 'the gateway sent a malformed body');
    }
  }

  /// Invokes a server-placed function (FN-020, `FunctionService.Invoke`) over
  /// the DPoP-bound channel. [idempotencyKey] makes a repeat of the same
  /// step run once on the server. The reply is the function's output, or the
  /// business error it returned.
  ///
  /// A function that declares `sca` needs the [challengeId] and the
  /// [signature] of the confirmed transaction; one that declares
  /// `attestation` needs [evidence], which is asked for once per proof, with
  /// the binding over the function, the version, the input and the proof's
  /// identifier (FN-020, SEC-004).
  ///
  /// [userToken], the Plux user token of the exchange for the audience
  /// `function:<name>` (SEC-026), travels in `Plux-User-Token`; a function
  /// that declares `claims` reads the verified user from it.
  Future<FunctionInvokeReply> invokeFunction({
    required DeviceToken token,
    required String function,
    required int version,
    required Uint8List input,
    required String idempotencyKey,
    String? userToken,
    String challengeId = '',
    Uint8List? signature,
    Future<InvocationEvidence> Function(Uint8List binding)? evidence,
  }) async {
    final r = await _call(
      'plux.v1.FunctionService/Invoke',
      {
        'function': function,
        'version': version,
        'input': base64.encode(input),
        'idempotencyKey': idempotencyKey,
        if (challengeId.isNotEmpty) 'transactionChallengeId': challengeId,
        if (signature != null && signature.isNotEmpty)
          'transactionSignature': base64.encode(signature),
      },
      token: token,
      extraHeaders: {pluxUserTokenHeader: ?userToken},
      extend: evidence == null
          ? null
          : (proof) async {
              final e = await evidence(
                invocationBinding(
                  function: function,
                  version: version,
                  input: input,
                  jti: _jtiOf(proof),
                ),
              );
              return {
                if (e.assertion != null)
                  'appAttestAssertion': base64.encode(e.assertion!),
                'playIntegrityToken': ?e.integrityToken,
              };
            },
    );
    try {
      final e = r['businessError'];
      if (e is Map<String, Object?>) {
        final params = e['params'];
        final decoded = params is String && params.isNotEmpty
            ? jsonDecode(utf8.decode(base64.decode(params)))
            : const <String, Object?>{};
        return FunctionInvokeReply.business(
          code: e['code']! as String,
          messageKey: e['messageKey']! as String,
          params: decoded as Map<String, Object?>,
        );
      }
      return FunctionInvokeReply.output(
        base64.decode(r['output'] as String? ?? ''),
      );
    } on Object {
      throw const ApiError(0, 'unknown', 'the server sent a malformed result');
    }
  }

  /// The URL and headers of a request to the gateway's HTTP route,
  /// `/gateway/v1/{upstream}/{path}` (SEC-030): the route that carries
  /// streams and transfers. The headers authenticate this request alone:
  /// `Authorization: DPoP` with [token] and a proof of [method] and the URL,
  /// the Plux [userToken], the [operation] and, for a transaction, the
  /// [challengeId] and the base64url [signature] (SEC-032). [pathAndQuery]
  /// begins with a slash; its query is not part of the proof (RFC 9449).
  Future<({Uri url, Map<String, String> headers})> gatewayRoute({
    required DeviceToken token,
    required String upstream,
    required String method,
    required String pathAndQuery,
    String operation = '',
    String plugin = '',
    String? userToken,
    String challengeId = '',
    Uint8List? signature,
  }) async {
    final base = endpoint.toString();
    final url = Uri.parse(
      '$base${base.endsWith('/') ? '' : '/'}gateway/v1/$upstream$pathAndQuery',
    );
    final proof = await token.proofs.proof(
      method: method,
      uri: url,
      accessToken: token.value,
      nonce: _nonce,
    );
    return (
      url: url,
      headers: {
        'Authorization': 'DPoP ${token.value}',
        'DPoP': proof,
        pluxUserTokenHeader: ?userToken,
        if (operation.isNotEmpty) 'Plux-Operation': operation,
        if (plugin.isNotEmpty) 'Plux-Plugin': plugin,
        if (challengeId.isNotEmpty) 'Plux-Challenge-Id': challengeId,
        if (signature != null && signature.isNotEmpty)
          'Plux-Transaction-Signature': base64Url
              .encode(signature)
              .replaceAll('=', ''),
      },
    );
  }

  /// Asks for a single-use challenge bound to the hash of the canonical
  /// summary [summaryJcs] (SEC-027, `ScaService.CreateTransactionChallenge`).
  Future<TransactionChallenge> transactionChallenge({
    required DeviceToken token,
    required String deviceId,
    required String dataSource,
    required String operation,
    required Uint8List summaryJcs,
  }) => _challenge(token, {
    'deviceId': deviceId,
    'dataSource': dataSource,
    'operation': operation,
    'transactionSummaryJcs': base64.encode(summaryJcs),
  });

  /// Asks for a single-use challenge to confirm the invocation of the server
  /// [function] at [version] with an input whose SHA-256 is [inputSha256]
  /// (FN-020, SEC-027). The server adds the function, the version and the
  /// hash to [summaryJcs] and returns the result for the SCA key to sign.
  Future<TransactionChallenge> invocationChallenge({
    required DeviceToken token,
    required String deviceId,
    required String function,
    required int version,
    required Uint8List inputSha256,
    required Uint8List summaryJcs,
  }) => _challenge(token, {
    'deviceId': deviceId,
    'function': function,
    'functionVersion': version,
    'inputSha256': base64.encode(inputSha256),
    'transactionSummaryJcs': base64.encode(summaryJcs),
  });

  Future<TransactionChallenge> _challenge(
    DeviceToken token,
    Map<String, Object?> body,
  ) async {
    final r = await _call(
      'plux.v1.ScaService/CreateTransactionChallenge',
      body,
      token: token,
    );
    final id = r['challengeId'];
    final expires = DateTime.tryParse(r['expiresAt'] as String? ?? '');
    Uint8List? challenge;
    try {
      challenge = base64.decode(r['challenge'] as String? ?? '');
    } on FormatException {
      challenge = null;
    }
    if (id is! String ||
        id.isEmpty ||
        challenge == null ||
        challenge.isEmpty ||
        expires == null) {
      throw const ApiError(0, 'unknown', 'the server sent no challenge');
    }
    final shown = r['displayed'];
    Uint8List? summary;
    try {
      summary = base64.decode(r['transactionSummaryJcs'] as String? ?? '');
    } on FormatException {
      throw const ApiError(0, 'unknown', 'the server sent a malformed summary');
    }
    return TransactionChallenge(
      id: id,
      bytes: challenge,
      expiresAt: expires,
      displayed: [
        if (shown is List<Object?>)
          for (final d in shown)
            if (d is Map<String, Object?> &&
                d['label'] is String &&
                d['value'] is String)
              DisplayedValue(d['label']! as String, d['value']! as String),
      ],
      summaryJcs: summary.isEmpty ? null : summary,
    );
  }

  /// The `live` configuration values of the app and environment
  /// (DAT-094, `ConfigService.GetLiveConfig`), over the device's DPoP-bound
  /// channel. Never cached by this client: the values are held in memory by
  /// the caller only.
  Future<LiveConfigSnapshot> getLiveConfig(
    DeviceToken token, {
    required String appId,
    required String environment,
  }) async {
    final r = await _call('plux.v1.ConfigService/GetLiveConfig', {
      'appId': appId,
      'environment': environment,
    }, token: token);
    try {
      return LiveConfigSnapshot.fromJson(r);
    } on FormatException {
      throw const ApiError(
        0,
        'unknown',
        'the server sent malformed configuration',
      );
    }
  }

  /// Trades the host's end-user token [subjectToken] for a short-lived Plux
  /// user token scoped to [audience], an upstream (SEC-026, RFC 8693,
  /// `TokenService.ExchangeUserToken`).
  Future<UserToken> exchangeUserToken({
    required DeviceToken token,
    required String subjectToken,
    required String audience,
    required DateTime now,
  }) async {
    final r = await _call('plux.v1.TokenService/ExchangeUserToken', {
      'subjectToken': subjectToken,
      'subjectTokenType': accessTokenType,
      'audience': audience,
    }, token: token);
    final access = r['accessToken'];
    if (access is! String || access.isEmpty) {
      throw const ApiError(0, 'unknown', 'the server sent no user token');
    }
    return UserToken(
      access,
      DateTime.tryParse(r['expiresAt'] as String? ?? '') ?? now,
    );
  }

  /// The `jti` claim of a compact JWS DPoP [proof] this client signed.
  static String _jtiOf(String proof) {
    final claims = jsonDecode(
      utf8.decode(base64Url.decode(base64Url.normalize(proof.split('.')[1]))),
    );
    return (claims as Map<String, Object?>)['jti']! as String;
  }

  /// Posts one call. With [proofs] the request carries a DPoP proof (with
  /// the [token], when there is one, as `Authorization: DPoP`); [extend]
  /// adds body members that depend on the proof; [onHeaders] sees the
  /// response headers, names in lower case. A 401 that asks for a
  /// nonce is repeated once with a new proof carrying it (RFC 9449 §8).
  Future<Map<String, Object?>> _call(
    String procedure,
    Map<String, Object?> body, {
    DeviceToken? token,
    DpopProofs? proofs,
    Future<Map<String, Object?>> Function(String proof)? extend,
    bool compress = false,
    Map<String, String> extraHeaders = const {},
    void Function(Map<String, String> headers)? onHeaders,
  }) async {
    final uri = endpoint.resolve(procedure);
    final signer = proofs ?? token?.proofs;
    for (var attempt = 0; ; attempt++) {
      final proof = signer == null
          ? null
          : await signer.proof(
              method: 'POST',
              uri: uri,
              accessToken: token?.value,
              nonce: _nonce,
            );
      final sent = proof == null || extend == null
          ? body
          : {...body, ...await extend(proof)};
      final http.Response res;
      try {
        final json = utf8.encode(jsonEncode(sent));
        res = await _http.post(
          uri,
          headers: {
            'Content-Type': 'application/json',
            'Connect-Protocol-Version': '1',
            if (compress) 'Content-Encoding': 'gzip',
            if (token != null) 'Authorization': 'DPoP ${token.value}',
            'DPoP': ?proof,
            ...extraHeaders,
          },
          body: compress ? gzip.encode(json) : json,
        );
      } on PluxException {
        // A failed certificate pin is not an outage (SEC-041).
        rethrow;
      } on Exception catch (e) {
        throw ApiError(0, 'unavailable', '$e');
      }
      onHeaders?.call(res.headers);
      final nonce = res.headers['dpop-nonce'];
      if (nonce != null && nonce.isNotEmpty) _nonce = nonce;
      if (res.statusCode == 401 &&
          attempt == 0 &&
          signer != null &&
          (res.headers['www-authenticate'] ?? '').contains('use_dpop_nonce')) {
        continue;
      }
      Map<String, Object?> json;
      try {
        json = res.body.isEmpty
            ? const {}
            : jsonDecode(res.body) as Map<String, Object?>;
      } on Object {
        json = const {};
      }
      if (res.statusCode != 200) {
        throw ApiError(
          res.statusCode,
          json['code'] as String? ?? 'unknown',
          json['message'] as String? ?? res.reasonPhrase ?? '',
          retryAfter: parseRetryAfter(
            res.headers['retry-after'],
            DateTime.now(),
          ),
        );
      }
      return json;
    }
  }
}

String _keyStorage(KeyStorage s) => switch (s) {
  KeyStorage.strongbox => 'KEY_STORAGE_STRONGBOX',
  KeyStorage.tee => 'KEY_STORAGE_TEE',
  KeyStorage.secureEnclave => 'KEY_STORAGE_SECURE_ENCLAVE',
  KeyStorage.software => 'KEY_STORAGE_SOFTWARE',
};

Map<String, Object?> _evidence(AttestationEvidence e) => switch (e) {
  AndroidEvidence() => {
    'android': {
      'keyAttestationChain': [
        for (final c in e.keyAttestationChain) base64.encode(c),
      ],
      'playIntegrityToken': e.playIntegrityToken,
    },
  },
  IosEvidence() => {
    'ios': {
      'appAttestKeyId': base64.encode(e.keyId),
      'attestationObject': base64.encode(e.attestationObject),
    },
  },
  IosAssertionEvidence() => {
    'ios': {'appAttestAssertion': base64.encode(e.assertion)},
  },
  DevelopmentEvidence() => {
    'development': {'buildId': e.buildId},
  },
};

int _int(Object? v) => switch (v) {
  int() => v,
  String() => int.tryParse(v) ?? 0,
  _ => 0,
};

/// Reads a `Retry-After` header — seconds or an HTTP date — relative to
/// [now]; null when absent or unreadable. Capped at five minutes.
Duration? parseRetryAfter(String? header, DateTime now) {
  if (header == null || header.trim().isEmpty) return null;
  final seconds = int.tryParse(header.trim());
  Duration? d;
  if (seconds != null) {
    d = Duration(seconds: seconds);
  } else {
    try {
      d = HttpDateParser.parse(header.trim()).difference(now);
    } on FormatException {
      return null;
    }
  }
  if (d.isNegative) return Duration.zero;
  const cap = Duration(minutes: 5);
  return d > cap ? cap : d;
}

/// Parses the IMF-fixdate form of HTTP dates (RFC 9110), such as
/// `Sun, 06 Nov 1994 08:49:37 GMT`.
abstract final class HttpDateParser {
  static const _months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];

  /// Parses [s]; throws [FormatException].
  static DateTime parse(String s) {
    final m = RegExp(
      r'^[A-Za-z]{3}, (\d{2}) ([A-Za-z]{3}) (\d{4}) (\d{2}):(\d{2}):(\d{2}) GMT$',
    ).firstMatch(s);
    final month = m == null ? -1 : _months.indexOf(m[2]!);
    if (m == null || month < 0) throw FormatException('not an HTTP date', s);
    return DateTime.utc(
      int.parse(m[3]!),
      month + 1,
      int.parse(m[1]!),
      int.parse(m[4]!),
      int.parse(m[5]!),
      int.parse(m[6]!),
    );
  }
}
