// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The device's standing with the server (SEC-002, SEC-021, SEC-025,
/// ADR-0012): registration with platform evidence for a hardware-held DPoP
/// key, access tokens obtained by proof of that key, re-attestation when
/// the server asks for it, and registering again when it no longer knows
/// the device.
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/attestation.dart';
import 'package:plux_flutter/src/security/device_keys.dart';
import 'package:plux_flutter/src/security/dpop.dart';
import 'package:plux_flutter/src/sync/api_client.dart';
import 'package:plux_flutter/src/sync/sync_engine.dart';

/// Registers the device and keeps a device token for it.
///
/// The token is refreshed thirty seconds before it expires. A server that
/// no longer knows the device (`PLX-6006`, `not_found`, `unauthenticated`)
/// makes the device register again, once per sync ([beginSync]); one that
/// asks for re-attestation (`PLX-6007`) gets fresh evidence for the same
/// key.
final class DeviceAuth {
  /// Creates the authority for one app and environment.
  DeviceAuth({
    required this.api,
    required this.keys,
    required this.attestation,
    required this.credentials,
    required this.appId,
    required this.environment,
    required this.device,
    bool enrolSca = false,
    this.onAttestation,
    DateTime Function()? clock,
  }) : _sca = enrolSca,
       _clock = clock ?? DateTime.now;

  /// The API client.
  final PluxApiClient api;

  /// The hardware-held keys.
  final DeviceKeys keys;

  /// The platform's attestation services.
  final Attestation attestation;

  /// Where the registration is kept.
  final CredentialStore credentials;

  /// The app.
  final String appId;

  /// The environment key.
  final String environment;

  /// What the device reports about itself at registration.
  final DeviceInfo device;

  /// Told the fields of an `attestation_result` telemetry event after each
  /// registration, re-attestation and SCA enrolment, whether it succeeded or
  /// not: `provider`, `result` (`success` or `failure`), `assurance_level`
  /// and, on a failure, `error_code`. Nothing that identifies the device.
  final void Function(Map<String, Object?> fields)? onAttestation;

  final DateTime Function() _clock;

  /// Whether a registration includes an SCA key (SEC-027): set by the host's
  /// configuration, and by [ensureSca] for a device registered without one.
  bool _sca;
  var _scaTried = false;

  /// How long before its expiry a token counts as expired.
  static const refreshMargin = Duration(seconds: 30);

  DeviceToken? _token;
  Future<DeviceToken>? _pending;
  var _reregistered = false;
  var _assurance = 0;

  /// The assurance level (0 to 3) of the latest token, `AL0` until one
  /// arrived, and again after the server refused the device with `PLX-6002`
  /// or `PLX-6006` (SEC-007).
  int get assurance => _assurance;

  /// Lowers the level to `AL0` at once, after the server refused the device
  /// for its assurance or revoked it; the next token restores what the
  /// server says.
  void lowerAssurance() => _assurance = 0;

  String get _alias => dpopKeyAlias(appId, environment);
  String get _agreementAlias => agreementKeyAlias(appId, environment);

  String get _scaAlias => scaKeyAlias(appId, environment);

  /// The ID the server knows this device by, registering it on first use.
  Future<String> deviceId() async {
    await token();
    final c = await credentials.read();
    if (c == null) {
      throw const ApiError(0, 'unknown', 'the device is not registered');
    }
    return c.deviceId;
  }

  /// Whether the device has an SCA key registered with the server, and if
  /// it has none, enrols one (SEC-027): a key that the platform invalidated
  /// (a new fingerprint was enrolled) is gone. A registered device enrols
  /// the new key with `EnrolScaKey`, bound to a fresh challenge; only a
  /// device with no registration at all registers again. Tried once per
  /// launch; false when the platform cannot create the key, for example
  /// when the device has no screen lock.
  Future<bool> ensureSca() async {
    if (await _scaKey() != null) return true;
    if (_scaTried) return false;
    _scaTried = true;
    _sca = true;
    final session = await _session();
    if (session == null) {
      _token = null;
      await _forget();
      await _register();
      return await _scaKey() != null;
    }
    return _enrolSca(session);
  }

  Future<bool> _enrolSca(_Session s) async {
    // A token first: obtaining it may register the device again, with an
    // SCA key, which settles the matter.
    final access = await token();
    if (await _scaKey() != null) return true;
    final challenge = await api.registrationChallenge(
      appId: appId,
      environment: environment,
    );
    final sca = await _createSca(challenge);
    if (sca == null) return false;
    var provider = _platformProvider;
    try {
      final evidence = await attestation.scaEnrolment(
        challenge: challenge,
        jkt: s.key.publicKey.thumbprint,
        kaJkt: (await _loadAgreementKey())?.publicKey.thumbprint ?? '',
        scaJkt: sca.publicKey.thumbprint,
        scaKeyChain: sca.attestationChain,
      );
      provider = _providerOf(evidence);
      await api.enrolScaKey(
        token: access,
        challenge: challenge,
        scaKey: sca,
        evidence: evidence,
      );
    } on Object catch (e) {
      // A key the server did not accept must not look enrolled.
      await keys.delete(_scaAlias);
      _report(provider, _assurance, error: e);
      rethrow;
    }
    _report(provider, _assurance);
    return true;
  }

  /// The provider the platform attests with, when no evidence says.
  String get _platformProvider => switch (device.platform) {
    'android' => 'play_integrity',
    'ios' => 'app_attest',
    _ => 'development',
  };

  static String _providerOf(AttestationEvidence e) => switch (e) {
    AndroidEvidence() => 'play_integrity',
    IosEvidence() || IosAssertionEvidence() => 'app_attest',
    DevelopmentEvidence() => 'development',
  };

  /// The provider and outcome of a registration or re-attestation whose
  /// assurance level the next token names.
  String? _awaiting;

  void _report(String provider, int assurance, {Object? error}) {
    final sink = onAttestation;
    if (sink == null) return;
    sink({
      'provider': provider,
      'result': error == null ? 'success' : 'failure',
      'assurance_level': assurance,
      if (error != null) 'error_code': _errorCode(error),
    });
  }

  static String _errorCode(Object e) => switch (e) {
    ApiError(:final plxCode?) => 'PLX-${plxCode.toString().padLeft(4, '0')}',
    ApiError(:final code) => code,
    PluxException(:final code) => code.id,
    _ => 'unknown',
  };

  /// Reports a registration or re-attestation of [provider] with the level
  /// of the token that follows it; one still waiting is reported first.
  void _await(String provider) {
    final earlier = _awaiting;
    if (earlier != null) _report(earlier, _assurance);
    _awaiting = provider;
  }

  void _settle(int assurance, {Object? error}) {
    final provider = _awaiting;
    if (provider == null) return;
    _awaiting = null;
    _report(provider, assurance, error: error);
  }

  Future<DeviceKey?> _loadAgreementKey() async {
    try {
      return await keys.load(_agreementAlias, KeyPurpose.agreement);
    } on PluxException catch (e) {
      if (e.code != PluxErrorCode.deviceKeyUnavailable) rethrow;
      return null;
    }
  }

  Future<DeviceKey?> _scaKey() async {
    try {
      return await keys.load(_scaAlias, KeyPurpose.sca);
    } on PluxException catch (e) {
      if (e.code != PluxErrorCode.deviceKeyUnavailable) rethrow;
      return null;
    }
  }

  /// Starts a sync: registering again after a refusal is allowed once more.
  void beginSync() => _reregistered = false;

  /// Forgets the held token, after the server refused it.
  void forgetToken() => _token = null;

  /// Offers the server fresh evidence for the registered key now, for a
  /// risk signal such as a RASP detection (SEC-006, SEC-025), and forgets
  /// the held token so that the next one carries the level the server
  /// computes from it. Does nothing for a device that is not registered.
  Future<void> reattestNow() async {
    final s = await _session();
    if (s == null) return;
    await _reattest(s);
    _token = null;
  }

  /// A token that is valid for at least [refreshMargin], registering the
  /// device on first use. Concurrent callers share one exchange.
  Future<DeviceToken> token() {
    final t = _token;
    if (t != null && t.expiresAt.subtract(refreshMargin).isAfter(_clock())) {
      return Future.value(t);
    }
    return _pending ??= _obtain().whenComplete(() => _pending = null);
  }

  Future<DeviceToken> _obtain() async {
    try {
      final t = await _obtainToken();
      _assurance = t.assurance;
      _settle(t.assurance);
      return t;
    } on ApiError catch (e) {
      _settle(_assurance, error: e);
      if (e.plxCode == PluxErrorCode.assuranceInsufficient.code ||
          e.plxCode == PluxErrorCode.deviceRevoked.code) {
        lowerAssurance();
      }
      rethrow;
    }
  }

  Future<DeviceToken> _obtainToken() async {
    var session = await _session() ?? await _register();
    try {
      return _token = await _refresh(session);
    } on ApiError catch (e) {
      if (e.plxCode == PluxErrorCode.reattestationRequired.code) {
        await _reattest(session);
        return _token = await _refresh(session);
      }
      if (!_unknownDevice(e) || _reregistered) rethrow;
      _reregistered = true;
      await _forget();
      session = await _register();
      return _token = await _refresh(session);
    }
  }

  bool _unknownDevice(ApiError e) =>
      e.plxCode == PluxErrorCode.deviceRevoked.code ||
      e.code == 'not_found' ||
      e.code == 'unauthenticated';

  /// The stored registration, when its key is still in the platform.
  Future<_Session?> _session() async {
    final c = await credentials.read();
    if (c == null) return null;
    final DeviceKey? key;
    try {
      key = await keys.load(_alias, KeyPurpose.dpop);
    } on PluxException catch (e) {
      if (e.code != PluxErrorCode.deviceKeyUnavailable) rethrow;
      return null;
    }
    if (key == null || key.publicKey.thumbprint != c.jkt) return null;
    return _Session(c.deviceId, key, _proofs(key));
  }

  /// Creates the SCA key (user authentication required) for the
  /// registration [challenge]; null when the platform cannot, which leaves
  /// the device without strong customer authentication but registered.
  Future<DeviceKey?> _createSca(Uint8List challenge) async {
    try {
      return await keys.create(_scaAlias, KeyPurpose.sca, challenge: challenge);
    } on PluxException catch (e) {
      if (e.code != PluxErrorCode.deviceKeyUnavailable) rethrow;
      return null;
    }
  }

  /// The evidence of the SCA key: on Android its own Key Attestation chain
  /// with the Play Integrity token of the registration, which the server
  /// has verified for the device; the other platforms offer the
  /// registration's evidence itself, since App Attest attests one key per
  /// installation.
  AttestationEvidence _scaEvidence(AttestationEvidence e, DeviceKey sca) =>
      switch (e) {
        AndroidEvidence() when sca.attestationChain.isNotEmpty =>
          AndroidEvidence(
            keyAttestationChain: sca.attestationChain,
            playIntegrityToken: e.playIntegrityToken,
          ),
        _ => e,
      };

  DpopProofs _proofs(DeviceKey key) => DpopProofs(
    keys: keys,
    alias: _alias,
    publicKey: key.publicKey,
    now: _clock,
  );

  Future<void> _forget() async {
    await credentials.clear();
    await keys.delete(_alias);
    await keys.delete(_agreementAlias);
    if (_sca) await keys.delete(_scaAlias);
  }

  /// Creates the key-agreement key for confidential bundles (SEC-053), with
  /// its own Key Attestation for the registration [challenge] on Android. A
  /// platform that cannot make one (Android before API 31) registers the
  /// device without it: it then receives no content keys.
  Future<DeviceKey?> _createAgreementKey(Uint8List challenge) async {
    try {
      return await keys.create(
        _agreementAlias,
        KeyPurpose.agreement,
        challenge: challenge,
      );
    } on PluxException catch (e) {
      if (e.code != PluxErrorCode.deviceKeyUnavailable) rethrow;
      return null;
    }
  }

  /// Creates a new DPoP key for a challenge and registers it with the
  /// platform's evidence.
  Future<_Session> _register() async {
    var provider = _platformProvider;
    try {
      final session = await _registerWith((p) => provider = p);
      _await(provider);
      return session;
    } on Object catch (e) {
      _report(provider, _assurance, error: e);
      rethrow;
    }
  }

  Future<_Session> _registerWith(void Function(String provider) named) async {
    final challenge = await api.registrationChallenge(
      appId: appId,
      environment: environment,
    );
    await credentials.clear();
    final key = await keys.create(
      _alias,
      KeyPurpose.dpop,
      challenge: challenge,
    );
    final agreementKey = await _createAgreementKey(challenge);
    final sca = _sca ? await _createSca(challenge) : null;
    // The evidence vouches for every key the device registers (ADR-0012).
    final evidence = await attestation.attest(
      challenge: challenge,
      jkt: key.publicKey.thumbprint,
      kaJkt: agreementKey?.publicKey.thumbprint ?? '',
      scaJkt: sca?.publicKey.thumbprint ?? '',
      keyAttestationChain: key.attestationChain,
    );
    named(_providerOf(evidence));
    final id = await api.registerAttestedDevice(
      appId: appId,
      environment: environment,
      device: device,
      challenge: challenge,
      key: key,
      evidence: evidence,
      agreementKey: agreementKey,
      scaKey: sca,
      scaEvidence: sca == null ? null : _scaEvidence(evidence, sca),
    );
    await credentials.write(DeviceCredential(id, key.publicKey.thumbprint));
    return _Session(id, key, _proofs(key));
  }

  Future<DeviceToken> _refresh(_Session s) => api.refreshToken(
    deviceId: s.deviceId,
    proofs: s.proofs,
    now: _clock(),
    assertion: attestation.assertion,
  );

  /// Offers new evidence for the registered key (SEC-025): Android a Play
  /// Integrity token bound to the challenge and the device's keys, the other
  /// platforms the evidence of registration without a key attestation.
  Future<void> _reattest(_Session s) async {
    var provider = _platformProvider;
    try {
      await _reattestWith(s, (p) => provider = p);
      _await(provider);
    } on Object catch (e) {
      _report(provider, _assurance, error: e);
      rethrow;
    }
  }

  Future<void> _reattestWith(
    _Session s,
    void Function(String provider) named,
  ) async {
    final jkt = s.key.publicKey.thumbprint;
    final challenge = await api.registrationChallenge(
      appId: appId,
      environment: environment,
    );
    final kaJkt = (await _loadAgreementKey())?.publicKey.thumbprint ?? '';
    final scaJkt = (await _scaKey())?.publicKey.thumbprint ?? '';
    final binding = base64Url
        .encode(bindingHash(challenge, jkt, kaJkt, scaJkt))
        .replaceAll('=', '');
    final token = device.platform == 'android'
        ? await attestation.integrityToken(binding)
        : null;
    final AttestationEvidence evidence = token != null
        ? AndroidEvidence(
            keyAttestationChain: <Uint8List>[],
            playIntegrityToken: token,
          )
        : await attestation.attest(
            challenge: challenge,
            jkt: jkt,
            kaJkt: kaJkt,
            scaJkt: scaJkt,
            keyAttestationChain: const [],
          );
    named(_providerOf(evidence));
    await api.reattestDevice(
      deviceId: s.deviceId,
      challenge: challenge,
      evidence: evidence,
      proofs: s.proofs,
    );
  }
}

final class _Session {
  const _Session(this.deviceId, this.key, this.proofs);
  final String deviceId;
  final DeviceKey key;
  final DpopProofs proofs;
}
