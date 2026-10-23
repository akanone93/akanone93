// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The calls the viewer host makes to a Plux server (SEC-142, ADR-0082):
/// ConnectRPC with its JSON encoding, as the runtime's own API client does,
/// so the viewer needs no protobuf library. The credential is the auditor's
/// access token, sent as a bearer token to the server it was issued by and
/// to nothing else.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;
import 'package:plux_flutter/plux_flutter.dart';

/// The client name the viewer gives the server when it asks for a token.
const String viewerClientName = 'plux-viewer';

/// The version of the root the runtime makes of pinned keys, so the version
/// whose successors `GetRootKeys` is asked for (SEC-051).
const int pinnedRootVersion = 1;

/// The permissions the viewer's token asks for, and no more: opening
/// reconstructions and reading the apps and keys they belong to.
const List<String> viewerScopes = ['reconstruction.view', 'app.read'];

/// A failed call: the HTTP status, the Connect code and the server's
/// message. The message is the server's, shown to the auditor; it never
/// holds the token.
final class ViewerApiException implements Exception {
  /// Creates the error.
  const ViewerApiException(this.status, this.code, this.message);

  /// The HTTP status; 0 when no response arrived.
  final int status;

  /// The Connect code, such as `unauthenticated`.
  final String code;

  /// What the server, or the viewer, says went wrong.
  final String message;

  /// Whether the token was refused, so that signing in again may help.
  bool get unauthenticated => status == 401 || code == 'unauthenticated';

  @override
  String toString() => message.isEmpty ? '$status $code' : message;
}

/// A device authorization the auditor approves in a browser (RFC 8628).
final class DeviceGrant {
  /// Creates a grant.
  const DeviceGrant({
    required this.deviceCode,
    required this.userCode,
    required this.verificationUri,
    required this.verificationUriComplete,
    required this.interval,
    required this.expiresAt,
  });

  /// The code the viewer polls with; never shown.
  final String deviceCode;

  /// The code the auditor enters.
  final String userCode;

  /// Where the auditor approves.
  final String verificationUri;

  /// [verificationUri] with the code filled in.
  final String verificationUriComplete;

  /// How often to poll.
  final Duration interval;

  /// When the grant lapses.
  final DateTime expiresAt;
}

/// How a poll of a device authorization stands.
enum PollStatus {
  /// Not approved yet.
  pending,

  /// Poll less often.
  slowDown,

  /// Approved; the poll carries the token.
  approved,

  /// Refused by the approving person.
  denied,
}

/// The result of one poll.
final class DevicePoll {
  /// Creates a result.
  const DevicePoll(this.status, [this.token]);

  /// How the grant stands.
  final PollStatus status;

  /// The access token, once, when [status] is [PollStatus.approved].
  final String? token;
}

/// An app the auditor can see.
final class AppSummary {
  /// Creates a summary.
  const AppSummary({required this.id, required this.name});

  /// The app's identifier.
  final String id;

  /// Its name.
  final String name;
}

/// A reconstruction release in a list.
final class ReconstructionSummary {
  /// Creates a summary.
  const ReconstructionSummary({
    required this.id,
    required this.appId,
    required this.deviceId,
    required this.moment,
    required this.releaseSequence,
    required this.state,
    required this.createdAt,
  });

  /// The release's identifier.
  final String id;

  /// The app it belongs to.
  final String appId;

  /// The device the moment was resolved on.
  final String deviceId;

  /// The reconstructed instant.
  final DateTime moment;

  /// The release the device ran.
  final int releaseSequence;

  /// `pending` until the server has signed the release, then `signed`.
  final String state;

  /// When the reconstruction was requested.
  final DateTime createdAt;

  /// Whether the release is signed, and so can be opened.
  bool get openable => state == 'signed';
}

/// One bundle of a reconstruction release, with where to download it.
final class BundleInfo {
  /// Creates the description.
  const BundleInfo({
    required this.key,
    required this.hash,
    required this.size,
    required this.url,
  });

  /// The plugin key; empty for the app bundle.
  final String key;

  /// `sha256:<hex>` as the manifest names it.
  final String hash;

  /// The declared size in bytes.
  final int size;

  /// Where to download it, absolute or relative to the server.
  final String url;
}

/// A reconstruction release as the server returns it.
final class ReconstructionRelease {
  /// Creates a release.
  const ReconstructionRelease({
    required this.id,
    required this.manifest,
    required this.signatures,
    required this.bundles,
  });

  /// The release's identifier.
  final String id;

  /// The canonical manifest; empty while the release is pending.
  final Uint8List manifest;

  /// The signatures over [manifest].
  final List<PluxReconstructionSignature> signatures;

  /// The bundles and where they are.
  final List<BundleInfo> bundles;
}

/// The Plux server calls of the viewer.
final class ViewerApi {
  /// Creates a client for [server] (with a trailing slash). [token] is the
  /// auditor's access token; without it only the sign-in calls work.
  ViewerApi(this.server, {this.token, http.Client? client})
    : _http = client ?? http.Client();

  /// The server.
  final Uri server;

  /// The access token, kept in memory only.
  final String? token;

  final http.Client _http;

  /// Starts a device authorization (RFC 8628) for the viewer's scopes.
  Future<DeviceGrant> startDeviceAuthorization() async {
    final r = await _call('plux.v1.IdentityService/StartDeviceAuthorization', {
      'client': viewerClientName,
      'scopes': viewerScopes,
    }, authenticated: false);
    return DeviceGrant(
      deviceCode: _string(r['deviceCode']),
      userCode: _string(r['userCode']),
      verificationUri: _string(r['verificationUri']),
      verificationUriComplete: _string(r['verificationUriComplete']),
      interval: Duration(seconds: _int(r['intervalSeconds']).clamp(1, 60)),
      expiresAt:
          DateTime.tryParse(_string(r['expiresAt']))?.toUtc() ??
          DateTime.fromMillisecondsSinceEpoch(0, isUtc: true),
    );
  }

  /// Polls a device authorization.
  Future<DevicePoll> pollDeviceAuthorization(String deviceCode) async {
    final r = await _call('plux.v1.IdentityService/PollDeviceAuthorization', {
      'deviceCode': deviceCode,
    }, authenticated: false);
    final status = switch (_string(r['status'])) {
      'approved' => PollStatus.approved,
      'denied' => PollStatus.denied,
      'slow_down' => PollStatus.slowDown,
      _ => PollStatus.pending,
    };
    final secret = _string(r['secret']);
    return DevicePoll(
      status,
      status == PollStatus.approved && secret.isNotEmpty ? secret : null,
    );
  }

  /// The apps of the auditor's organisation, the first page.
  Future<List<AppSummary>> listApps() async {
    final r = await _call('plux.v1.AppService/ListApps', {
      'page': {'pageSize': 50},
    });
    return [
      for (final a in _list(r['apps']))
        AppSummary(id: _string(a['id']), name: _string(a['name'])),
    ];
  }

  /// The reconstruction releases of [appId], newest first, the first page.
  Future<List<ReconstructionSummary>> listReconstructions(String appId) async {
    final r = await _call('plux.v1.AuditService/ListReconstructions', {
      'appId': appId,
      'page': {'pageSize': 50},
    });
    return [for (final m in _list(r['reconstructions'])) _summary(m)];
  }

  /// Opens reconstruction release [id]; the server audits the view.
  Future<ReconstructionRelease> getReconstruction(String id) async {
    final r = await _call('plux.v1.AuditService/GetReconstruction', {'id': id});
    return ReconstructionRelease(
      id: id,
      manifest: _bytes(r['manifest']),
      signatures: [
        for (final s in _list(r['signatures']))
          PluxReconstructionSignature(
            keyId: _string(s['keyId']),
            algorithm: _string(s['algorithm']),
            signature: _bytes(s['signature']),
          ),
      ],
      bundles: [
        for (final b in _list(r['bundles']))
          BundleInfo(
            key: _string(b['key']),
            hash: _string(b['sha256']),
            size: _int(b['size']),
            url: _string(b['url']),
          ),
      ],
    );
  }

  /// The signed root files of an environment after the root the runtime
  /// makes of the viewer's pinned keys, oldest first. They are untrusted
  /// until the runtime has walked them from the pinned keys (SEC-051): the
  /// server cannot make the viewer trust a key by listing it.
  Future<List<Uint8List>> rootChain(String appId, String environment) async {
    final r = await _call('plux.v1.ManifestService/GetRootKeys', {
      'appId': appId,
      'environment': environment,
      'sinceRootVersion': '$pinnedRootVersion',
    });
    return [
      for (final root in r['roots'] as List<Object?>? ?? const []) _bytes(root),
    ];
  }

  /// Downloads [bundle]. No credential is sent: a bundle is addressed by its
  /// hash and the runtime verifies it, and the URL may be on another host.
  /// The download stops as soon as it exceeds the size the release declares,
  /// and a declared size beyond the registry's bound for a bundle is
  /// refused before anything is fetched.
  Future<Uint8List> downloadBundle(BundleInfo bundle) async {
    if (bundle.size < 0 || bundle.size > PluxLimit.bundlePluginSize.max) {
      throw ViewerApiException(
        0,
        'invalid_argument',
        'bundle "${bundle.key}" declares an impossible size',
      );
    }
    final url = server.resolve(bundle.url);
    if (url.scheme != 'https' && url.scheme != server.scheme) {
      throw const ViewerApiException(
        0,
        'invalid_argument',
        'a bundle is offered over an insecure address',
      );
    }
    final http.StreamedResponse res;
    try {
      res = await _http.send(http.Request('GET', url));
    } on Exception catch (e) {
      throw ViewerApiException(0, 'unavailable', 'download failed: $e');
    }
    if (res.statusCode != 200) {
      throw ViewerApiException(
        res.statusCode,
        'unavailable',
        'bundle "${bundle.key}" could not be downloaded (${res.statusCode})',
      );
    }
    final out = BytesBuilder(copy: false);
    await for (final chunk in res.stream) {
      out.add(chunk);
      if (out.length > bundle.size) {
        throw ViewerApiException(
          0,
          'invalid_argument',
          'bundle "${bundle.key}" is larger than the ${bundle.size} bytes '
              'declared',
        );
      }
    }
    return out.takeBytes();
  }

  /// Releases the connection pool.
  void close() => _http.close();

  Future<Map<String, Object?>> _call(
    String procedure,
    Map<String, Object?> body, {
    bool authenticated = true,
  }) async {
    final bearer = token;
    if (authenticated && bearer == null) {
      throw const ViewerApiException(401, 'unauthenticated', 'sign in first');
    }
    final http.Response res;
    try {
      res = await _http.post(
        server.resolve(procedure),
        headers: {
          'Content-Type': 'application/json',
          'Connect-Protocol-Version': '1',
          if (authenticated) 'Authorization': 'Bearer $bearer',
        },
        body: utf8.encode(jsonEncode(body)),
      );
    } on Exception catch (e) {
      throw ViewerApiException(
        0,
        'unavailable',
        'the server is unreachable: $e',
      );
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
      throw ViewerApiException(
        res.statusCode,
        _string(json['code'], 'unknown'),
        _string(json['message'], res.reasonPhrase ?? ''),
      );
    }
    return json;
  }

  static ReconstructionSummary _summary(Map<String, Object?> m) =>
      ReconstructionSummary(
        id: _string(m['id']),
        appId: _string(m['appId']),
        deviceId: _string(m['deviceId']),
        moment: _time(m['moment']),
        releaseSequence: _int(m['releaseSequence']),
        state: _string(m['state']),
        createdAt: _time(m['createdAt']),
      );

  static String _string(Object? v, [String fallback = '']) =>
      v is String ? v : fallback;

  /// Proto JSON writes 64-bit integers as strings.
  static int _int(Object? v) => switch (v) {
    int() => v,
    String() => int.tryParse(v) ?? 0,
    _ => 0,
  };

  static DateTime _time(Object? v) =>
      DateTime.tryParse(_string(v))?.toUtc() ??
      DateTime.fromMillisecondsSinceEpoch(0, isUtc: true);

  static Uint8List _bytes(Object? v) {
    try {
      return base64.decode(_string(v));
    } on FormatException {
      return Uint8List(0);
    }
  }

  static List<Map<String, Object?>> _list(Object? v) => [
    if (v is List<Object?>)
      for (final e in v)
        if (e is Map<String, Object?>) e,
  ];
}
