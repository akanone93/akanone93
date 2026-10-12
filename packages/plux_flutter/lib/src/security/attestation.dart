// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Platform attestation of the device's keys (SEC-002–SEC-005,
/// ADR-0012 §Registration).
///
/// The evidence is bound to the server's registration challenge and to the
/// thumbprints of every key the device registers: Play Integrity's request
/// hash is base64url([bindingHash]) and App Attest's client data hash is
/// [bindingHash] itself, so a rooted device cannot register a software key
/// beside an attested one. Android Key Attestation attests each key
/// itself, through the certificate chain the key was created with. Development evidence is offered only by debug and profile builds
/// on emulators and simulators; production environments refuse it
/// (SEC-008).
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';

/// SHA-256(challenge ‖ jkt ‖ "." ‖ kaJkt ‖ "." ‖ scaJkt): what the platform
/// evidence is bound to. [jkt], [kaJkt] and [scaJkt] are the RFC 7638
/// thumbprints of the DPoP, key-agreement and SCA keys; a key the device does
/// not have contributes the empty string, and the separators stay so that a
/// thumbprint cannot move from one field to another. The one implementation
/// of the binding, matching the server's `attest/binding` package.
Uint8List bindingHash(
  Uint8List challenge,
  String jkt,
  String kaJkt,
  String scaJkt,
) => Uint8List.fromList(
  sha256.convert([...challenge, ...utf8.encode('$jkt.$kaJkt.$scaJkt')]).bytes,
);

/// The evidence a device offers for its DPoP key: exactly one form.
sealed class AttestationEvidence {
  const AttestationEvidence();
}

/// Android: the Key Attestation chain of the DPoP key (DER, leaf first)
/// and a Play Integrity token whose request hash is the binding hash.
final class AndroidEvidence extends AttestationEvidence {
  /// Creates Android evidence.
  const AndroidEvidence({
    required this.keyAttestationChain,
    required this.playIntegrityToken,
  });

  /// The DER certificates of the DPoP key's attestation, leaf first.
  final List<Uint8List> keyAttestationChain;

  /// The Play Integrity token; never logged (SEC-092).
  final String playIntegrityToken;

  @override
  String toString() => 'AndroidEvidence([redacted])';
}

/// iOS: an App Attest key and its attestation object for the binding hash.
final class IosEvidence extends AttestationEvidence {
  /// Creates iOS evidence.
  const IosEvidence({required this.keyId, required this.attestationObject});

  /// The App Attest key identifier (the raw 32 bytes).
  final Uint8List keyId;

  /// The CBOR attestation object App Attest returned.
  final Uint8List attestationObject;

  @override
  String toString() => 'IosEvidence([redacted])';
}

/// iOS, for an SCA key enrolled after registration: an App Attest assertion
/// over the binding hash, made with the key attested at registration.
final class IosAssertionEvidence extends AttestationEvidence {
  /// Creates iOS assertion evidence.
  const IosAssertionEvidence(this.assertion);

  /// The assertion; never logged (SEC-092).
  final Uint8List assertion;

  @override
  String toString() => 'IosAssertionEvidence([redacted])';
}

/// A development build on an emulator or simulator (SEC-008).
final class DevelopmentEvidence extends AttestationEvidence {
  /// Creates development evidence.
  const DevelopmentEvidence(this.buildId);

  /// Identifies the development build.
  final String buildId;
}

/// The platform's attestation services.
abstract interface class Attestation {
  /// Evidence for registration or re-attestation of the DPoP key with
  /// thumbprint [jkt], bound to [challenge] and to the thumbprints [kaJkt]
  /// of the key-agreement key and [scaJkt] of the SCA key (empty for a key
  /// the device does not have). [keyAttestationChain] is the chain the
  /// Android key was created with for this challenge (empty on iOS).
  Future<AttestationEvidence> attest({
    required Uint8List challenge,
    required String jkt,
    required String kaJkt,
    required String scaJkt,
    required List<Uint8List> keyAttestationChain,
  });

  /// Evidence for enrolling an SCA key with thumbprint [scaJkt] on a device
  /// that is already registered, bound to the fresh [challenge] and to the
  /// registered thumbprints [jkt] and [kaJkt] (ADR-0012, Revisions
  /// 2026-10-11). [scaKeyChain] is the chain the Android SCA key was created
  /// with for [challenge]; iOS answers with an App Attest assertion over the
  /// binding hash.
  Future<AttestationEvidence> scaEnrolment({
    required Uint8List challenge,
    required String jkt,
    required String kaJkt,
    required String scaJkt,
    required List<Uint8List> scaKeyChain,
  });

  /// An App Attest assertion over [clientDataHash] with the device's App
  /// Attest key, for a token refresh on iOS (SEC-025); null on other
  /// platforms or when the device registered with development evidence.
  Future<Uint8List?> assertion(Uint8List clientDataHash);

  /// A Play Integrity token whose request hash is [requestHash], for a
  /// re-attestation on Android (SEC-025); null on other platforms.
  Future<String?> integrityToken(String requestHash);
}
