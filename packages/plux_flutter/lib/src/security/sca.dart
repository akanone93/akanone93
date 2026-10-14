// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Strong customer authentication with dynamic linking (SEC-027, SEC-028,
/// SEC-032, ADR-0056): the user confirms the amount, the currency and the
/// payee exactly as they will be sent, and the device's SCA key signs them
/// together with a single-use challenge from the Plux server. Only the
/// server verifies the signature; the runtime never decides that a
/// transaction is authorised.
///
/// The order is fixed, and a test pins it: challenge, confirmation sheet,
/// system prompt (biometric or device credential), signature, and only
/// then the call through the gateway.
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/security/device_keys.dart';

/// The user declined the transaction, on the confirmation sheet or at the
/// system prompt (SEC-027). Nothing was signed or sent.
final class ScaDeclined implements Exception {
  /// Creates the exception.
  const ScaDeclined();

  @override
  String toString() => 'ScaDeclined()';
}

/// What the user confirms: the amount, currency and payee as displayed
/// (SEC-028), and an optional human-readable description.
///
/// A payee the request leaves out is null: the operation binds it to a
/// server-held value, which the server puts into the summary the user
/// signs (DAT-093).
final class TransactionSummary {
  /// Creates a summary; every field is shown exactly as given.
  const TransactionSummary({
    required this.amount,
    required this.currency,
    required this.payee,
    this.description,
  });

  /// Reads the summary from [source], the JSON the operation sends, with the
  /// JSON Pointers (RFC 6901) of [pointers].
  ///
  /// The amount must be a string or an integer, the currency and the payee
  /// strings: a value that does not read the same way on the server is not
  /// something the user can be asked to confirm. A field that is missing or
  /// of another type fails with [PluxErrorCode.scaDynamicLinkingMismatch]
  /// before anything is asked or sent, except a missing payee, which the
  /// server fills from a binding or refuses the challenge for (DAT-093).
  factory TransactionSummary.read(
    Object? source, {
    required String amount,
    required String currency,
    required String payee,
    String? description,
  }) {
    String field(String name, String pointer, {bool number = false}) {
      final v = readPointer(source, pointer);
      if (v is String && v.isNotEmpty) return v;
      if (number && v is int) return '$v';
      throw PluxException(
        PluxErrorCode.scaDynamicLinkingMismatch,
        'the $name of the transaction is not at its mapped place in the '
        'request, or is not a ${number ? 'string or integer' : 'string'}',
      );
    }

    return TransactionSummary(
      amount: field('amount', amount, number: true),
      currency: field('currency', currency),
      payee: readPointer(source, payee) == null ? null : field('payee', payee),
      description: description == null ? null : field('summary', description),
    );
  }

  /// The amount, as the request carries it.
  final String amount;

  /// The currency, as the request carries it (ISO 4217).
  final String currency;

  /// The payee, as the request carries it; null when the server binds it
  /// (DAT-093).
  final String? payee;

  /// A human-readable description, or null.
  final String? description;

  /// The members by their names in [jcs], null ones left out.
  Map<String, String> get members => {
    'amount': amount,
    'currency': currency,
    'payee': ?payee,
    'summary': ?description,
  };

  /// The summary as RFC 8785 canonical JSON (UTF-8): the object with the
  /// members `amount`, `currency`, `payee` and `summary` that are not null,
  /// in that order, which is the order of their UTF-16 code units, with no
  /// insignificant whitespace. The server binds the challenge to its hash
  /// (SEC-028).
  Uint8List get jcs => utf8.encode(
    '{${[for (final m in members.entries) '"${m.key}":${_jsonString(m.value)}'].join(',')}}',
  );

  /// Whether [other] is the same transaction, byte for byte.
  bool sameAs(TransactionSummary other) => _equal(jcs, other.jcs);

  @override
  String toString() => 'TransactionSummary([redacted])';
}

/// The string [s] in the form RFC 8785 §3.2.2.2 prescribes, which is
/// ECMAScript's `JSON.stringify`: quotes and backslashes and the control
/// characters `\b \f \n \r \t` with their short escapes, the other control
/// characters as lower-case `\u00xx`, everything else as it is.
String _jsonString(String s) {
  final out = StringBuffer('"');
  for (final unit in s.codeUnits) {
    switch (unit) {
      case 0x22:
        out.write(r'\"');
      case 0x5c:
        out.write(r'\\');
      case 0x08:
        out.write(r'\b');
      case 0x09:
        out.write(r'\t');
      case 0x0a:
        out.write(r'\n');
      case 0x0c:
        out.write(r'\f');
      case 0x0d:
        out.write(r'\r');
      case < 0x20:
        out.write('\\u00${unit.toRadixString(16).padLeft(2, '0')}');
      default:
        out.writeCharCode(unit);
    }
  }
  return (out..write('"')).toString();
}

bool _equal(Uint8List a, Uint8List b) {
  if (a.length != b.length) return false;
  var diff = 0;
  for (var i = 0; i < a.length; i++) {
    diff |= a[i] ^ b[i];
  }
  return diff == 0;
}

/// The value at the JSON Pointer [pointer] (RFC 6901) of [document], or null
/// when there is none. The empty pointer is the document itself.
Object? readPointer(Object? document, String pointer) {
  if (pointer.isEmpty) return document;
  if (!pointer.startsWith('/')) return null;
  Object? at = document;
  for (final raw in pointer.substring(1).split('/')) {
    final token = raw.replaceAll('~1', '/').replaceAll('~0', '~');
    if (at is Map<String, Object?>) {
      at = at[token];
    } else if (at is List<Object?>) {
      final i = int.tryParse(token);
      if (i == null || i < 0 || i >= at.length || '$i' != token) return null;
      at = at[i];
    } else {
      return null;
    }
  }
  return at;
}

/// The bytes the SCA key signs (SEC-027, ADR-0056): the challenge followed
/// by the canonical summary, [TransactionSummary.jcs]. The key signs with
/// ES256, which hashes them with SHA-256: the signature is over
/// `SHA-256(challenge ‖ summary)`, as the ADR states.
Uint8List scaSigningInput(Uint8List challenge, Uint8List summaryJcs) =>
    Uint8List.fromList([...challenge, ...summaryJcs]);

/// A transaction the runtime asks the user to confirm.
final class TransactionRequest {
  /// Creates the request.
  const TransactionRequest({
    required this.dataSource,
    required this.operation,
    required this.summary,
  });

  /// The data source the operation belongs to.
  final String dataSource;

  /// The operation to confirm.
  final String operation;

  /// What the user confirms.
  final TransactionSummary summary;
}

/// A value the server resolved for the transaction and shows the user next
/// to the device's own summary, such as a payee the operation binds to
/// server-held configuration (DAT-093). The server binds it into the
/// challenge the SCA key signs; the device only shows it.
final class DisplayedValue {
  /// Creates the value.
  const DisplayedValue(this.label, this.value);

  /// What the value is, for the user.
  final String label;

  /// The value, exactly as the server returned it.
  final String value;

  @override
  String toString() => 'DisplayedValue([redacted])';
}

/// A single-use challenge from the Plux server (SEC-027).
final class TransactionChallenge {
  /// Creates the challenge.
  const TransactionChallenge({
    required this.id,
    required this.bytes,
    required this.expiresAt,
    this.displayed = const [],
    this.summaryJcs,
  });

  /// What the call through the gateway names the challenge by.
  final String id;

  /// The random bytes the SCA key signs together with the summary.
  final Uint8List bytes;

  /// When the server stops accepting the challenge.
  final DateTime expiresAt;

  /// The values the server resolved for the user to see (DAT-093).
  final List<DisplayedValue> displayed;

  /// The canonical summary the server bound the challenge to: the device's
  /// with the [displayed] values put in (DAT-093); null when the server
  /// returned none, which means the device's own.
  final Uint8List? summaryJcs;
}

/// Asks the Plux server for challenges: the sync isolate holds the device's
/// token and DPoP key, so the request is made there (L-6).
abstract interface class ChallengeSource {
  /// A challenge bound to the hash of [summaryJcs] for [dataSource] and
  /// [operation] (`ScaService.CreateTransactionChallenge`).
  Future<TransactionChallenge> challenge({
    required String dataSource,
    required String operation,
    required Uint8List summaryJcs,
  });

  /// Makes sure the device has an SCA key registered with the server,
  /// registering again with one when it has none (the server has no call
  /// that enrols a key on a registered device); whether it has one now.
  Future<bool> ensureEnrolled();
}

/// A server function invocation the runtime asks the user to confirm
/// (FN-020, SEC-027): the function, its version and the input, exactly as
/// they will be sent.
final class InvocationRequest {
  /// Creates the request; [input] is the UTF-8 JSON the call carries.
  const InvocationRequest({
    required this.function,
    required this.version,
    required this.input,
  });

  /// The function's dotted name.
  final String function;

  /// The pinned version.
  final int version;

  /// The input; the user is shown it as it is.
  final Uint8List input;

  @override
  String toString() => 'InvocationRequest([redacted])';
}

/// Asks the Plux server for the challenge that confirms a function
/// invocation; the sync isolate holds the device's token and DPoP key, so
/// the request is made there (L-6).
abstract interface class InvocationChallenges {
  /// A challenge for invoking [function] at [version] with [input]
  /// (`ScaService.CreateTransactionChallenge`). The implementation hashes
  /// the input off the UI isolate and returns the challenge only if its
  /// summary names that function, version and hash (SEC-028).
  Future<TransactionChallenge> invocationChallenge({
    required String function,
    required int version,
    required Uint8List input,
    required Uint8List summaryJcs,
  });
}

/// Checks that the summary the server bound [challenge] to is the one
/// invocation of [function] at [version] with an input whose SHA-256 is
/// [inputSha256], and nothing else: exactly the members `function`,
/// `version` and `inputSha256` (SEC-028). Throws
/// [PluxErrorCode.scaDynamicLinkingMismatch] otherwise.
void checkInvocationSummary(
  TransactionChallenge challenge,
  String function,
  int version,
  Uint8List inputSha256,
) {
  Never mismatch() => throw const PluxException(
    PluxErrorCode.scaDynamicLinkingMismatch,
    'the server bound the challenge to an invocation the user is not shown; '
    'it is not signed',
  );
  final raw = challenge.summaryJcs;
  if (raw == null || raw.isEmpty) mismatch();
  final Object? json;
  try {
    json = jsonDecode(utf8.decode(raw));
  } on FormatException {
    mismatch();
  }
  if (json is! Map<String, Object?> ||
      json.length != 3 ||
      json['function'] != function ||
      json['version'] != '$version' ||
      json['inputSha256'] !=
          base64Url.encode(inputSha256).replaceAll('=', '')) {
    mismatch();
  }
}

/// Shows the user a function invocation and learns whether they confirm it.
abstract interface class InvocationConfirmer {
  /// Shows [request], the function, the version and the input exactly as
  /// they are bound, on [context], and completes with whether the user
  /// confirmed. Completes with false when the user dismisses it.
  Future<bool> confirmInvocation(
    BuildContext context,
    InvocationRequest request,
  );
}

/// A confirmed transaction: the challenge and the SCA key's signature over
/// it and the summary, which the call through the gateway carries
/// (`CallRequest.transaction_challenge_id` and `transaction_signature`).
final class Confirmation {
  /// Creates the confirmation.
  const Confirmation({
    required this.challengeId,
    required this.signature,
    required this.summary,
    required this.expiresAt,
  });

  /// The challenge's ID.
  final String challengeId;

  /// The 64-byte ES256 signature (`r || s`).
  final Uint8List signature;

  /// The device's summary of the transaction the user confirmed; the
  /// signature covers it with the server-held values put in (DAT-093).
  final TransactionSummary summary;

  /// When the server stops accepting the challenge.
  final DateTime expiresAt;

  @override
  String toString() => 'Confirmation([redacted])';
}

/// Shows the user a transaction and learns whether they confirm it.
abstract interface class TransactionConfirmer {
  /// Shows [summary], with the amount, the currency and the payee exactly as
  /// they are signed, on [context], and completes with whether the user
  /// confirmed. The [displayed] values are the ones the server resolved
  /// (DAT-093); they are shown as rows after the summary's. Completes with
  /// false when the user dismisses it.
  Future<bool> confirm(
    BuildContext context,
    TransactionSummary summary, {
    List<DisplayedValue> displayed = const [],
  });
}

/// Authorises a transaction request with a confirmation (SEC-027).
abstract interface class TransactionAuthoriser {
  /// The confirmation for [request]: a held one for the same transaction,
  /// else a new one the user gives now. Fails with [ScaDeclined] when the
  /// user declines, and with a [PluxException] otherwise.
  Future<Confirmation> authorise(TransactionRequest request);
}

/// The text of the system prompt that unlocks the SCA key.
const String scaPromptTitle = 'Confirm transaction';

/// The device side of strong customer authentication (SEC-027, SEC-028):
/// challenge, confirmation sheet, system prompt, signature.
final class ScaService implements TransactionAuthoriser {
  /// Creates the service.
  ///
  /// [alias] is the SCA key's alias ([scaKeyAlias]); [context] gives the
  /// build context the confirmation sheet shows on, or null where there is
  /// none; [clock] defaults to the system clock.
  ScaService({
    required this.challenges,
    required this.keys,
    required this.signer,
    required this.confirmer,
    required this.alias,
    required this.context,
    this.invocations,
    this.invocationConfirmer,
    DateTime Function()? clock,
  }) : _clock = clock ?? DateTime.now;

  /// Where challenges come from.
  final ChallengeSource challenges;

  /// The device keys, to find the SCA key.
  final DeviceKeys keys;

  /// Signs with the SCA key after the system prompt.
  final UserAuthSigner signer;

  /// Shows the transaction.
  final TransactionConfirmer confirmer;

  /// The SCA key's alias.
  final String alias;

  /// The context the confirmation sheet shows on, or null.
  final BuildContext? Function() context;

  /// Where the challenges of function invocations come from, or null where
  /// the runtime cannot confirm one.
  final InvocationChallenges? invocations;

  /// Shows a function invocation, or null where the runtime cannot confirm
  /// one.
  final InvocationConfirmer? invocationConfirmer;

  final DateTime Function() _clock;
  final Map<String, Confirmation> _held = {};

  /// Confirms [request] with the user and signs it; the confirmation is
  /// held for the call that sends the same transaction, see [authorise].
  ///
  /// [on] is the context to show the sheet on; the runtime's own when null.
  /// Fails with [ScaDeclined] when the user declines, with
  /// [PluxErrorCode.deviceKeyUnavailable] when the device has no SCA key or
  /// cannot show the prompt, and otherwise with the server's refusal.
  Future<Confirmation> confirm(
    TransactionRequest request, {
    BuildContext? on,
  }) async {
    final showOn = on ?? context();
    if (showOn == null || !showOn.mounted) {
      throw const PluxException(
        PluxErrorCode.deviceKeyUnavailable,
        'there is no screen to show the transaction on, so it is not signed',
      );
    }
    if (await keys.load(alias, KeyPurpose.sca) == null &&
        !(await challenges.ensureEnrolled())) {
      throw const PluxException(
        PluxErrorCode.deviceKeyUnavailable,
        'this device has no strong customer authentication key',
      );
    }
    final summary = request.summary;
    final challenge = await challenges.challenge(
      dataSource: request.dataSource,
      operation: request.operation,
      summaryJcs: summary.jcs,
    );
    final signed = _signed(summary, challenge);
    // The context may have gone while the challenge was on its way.
    if (!showOn.mounted) throw const ScaDeclined();
    if (!await confirmer.confirm(
      showOn,
      summary,
      displayed: challenge.displayed,
    )) {
      throw const ScaDeclined();
    }
    final Uint8List signature;
    try {
      signature = await signer.signWithUser(
        alias,
        scaSigningInput(challenge.bytes, signed.jcs),
        title: scaPromptTitle,
        subtitle: '${signed.amount} ${signed.currency} to ${signed.payee}',
      );
    } on UserAuthenticationCancelled {
      throw const ScaDeclined();
    }
    return Confirmation(
      challengeId: challenge.id,
      signature: signature,
      summary: summary,
      expiresAt: challenge.expiresAt,
    );
  }

  /// Confirms the invocation [request] of a function that declares `sca`
  /// (FN-020, SEC-027): the user is shown the function, the version and the
  /// input, and the SCA key signs the challenge with the summary that binds
  /// exactly those. Like [confirm] it fails with [ScaDeclined] when the user
  /// declines and with [PluxErrorCode.deviceKeyUnavailable] when the device
  /// has no SCA key or no screen to show it on. Nothing is held: a
  /// confirmation is for the one call that follows.
  Future<Confirmation> confirmInvocation(
    InvocationRequest request, {
    BuildContext? on,
  }) async {
    final source = invocations;
    final confirmer = invocationConfirmer;
    if (source == null || confirmer == null) {
      throw const PluxException(
        PluxErrorCode.deviceKeyUnavailable,
        'this runtime cannot confirm a function invocation',
      );
    }
    final showOn = on ?? context();
    if (showOn == null || !showOn.mounted) {
      throw const PluxException(
        PluxErrorCode.deviceKeyUnavailable,
        'there is no screen to show the invocation on, so it is not signed',
      );
    }
    if (await keys.load(alias, KeyPurpose.sca) == null &&
        !(await challenges.ensureEnrolled())) {
      throw const PluxException(
        PluxErrorCode.deviceKeyUnavailable,
        'this device has no strong customer authentication key',
      );
    }
    final challenge = await source.invocationChallenge(
      function: request.function,
      version: request.version,
      input: request.input,
      summaryJcs: utf8.encode('{}'),
    );
    final signed = challenge.summaryJcs;
    if (signed == null || signed.isEmpty) {
      throw const PluxException(
        PluxErrorCode.scaDynamicLinkingMismatch,
        'the server bound the challenge to no summary; it is not signed',
      );
    }
    if (!showOn.mounted) throw const ScaDeclined();
    if (!await confirmer.confirmInvocation(showOn, request)) {
      throw const ScaDeclined();
    }
    final Uint8List signature;
    try {
      signature = await signer.signWithUser(
        alias,
        scaSigningInput(challenge.bytes, signed),
        title: scaPromptTitle,
        subtitle: '${request.function} v${request.version}',
      );
    } on UserAuthenticationCancelled {
      throw const ScaDeclined();
    }
    return Confirmation(
      challengeId: challenge.id,
      signature: signature,
      summary: const TransactionSummary(amount: '0', currency: '', payee: null),
      expiresAt: challenge.expiresAt,
    );
  }

  /// Confirms [request] as `signTransaction` does and holds the result for
  /// the call that sends the same transaction, at most until the challenge
  /// expires. A newer confirmation of the same operation replaces it.
  Future<Confirmation> confirmAhead(
    TransactionRequest request, {
    BuildContext? on,
  }) async {
    final c = await confirm(request, on: on);
    _held['${request.dataSource}.${request.operation}'] = c;
    return c;
  }

  @override
  Future<Confirmation> authorise(TransactionRequest request) async {
    final key = '${request.dataSource}.${request.operation}';
    final held = _held.remove(key);
    if (held == null) return confirm(request);
    // A held confirmation is single use: it is spent by this call, whether
    // it fits or not.
    if (!held.expiresAt.isAfter(_clock())) return confirm(request);
    if (!held.summary.sameAs(request.summary)) {
      throw const PluxException(
        PluxErrorCode.scaDynamicLinkingMismatch,
        'the request is not the transaction the user confirmed; it is not '
        'sent',
      );
    }
    return held;
  }

  /// Forgets every held confirmation, at logout.
  void clear() => _held.clear();
}

/// The summary the SCA key signs for [challenge]: the server's, after
/// checking that it is [device]'s with exactly the displayed values put in
/// the members [device] leaves out, so the user sees all of what is signed
/// and nothing else (SEC-028, DAT-093).
TransactionSummary _signed(
  TransactionSummary device,
  TransactionChallenge challenge,
) {
  Never mismatch() => throw const PluxException(
    PluxErrorCode.scaDynamicLinkingMismatch,
    'the server bound the challenge to a summary the user is not shown; it '
    'is not signed',
  );
  final raw = challenge.summaryJcs;
  if (raw == null || raw.isEmpty) {
    // Values shown that the signature would not cover.
    if (challenge.displayed.isNotEmpty) mismatch();
    return device;
  }
  final Object? json;
  try {
    json = jsonDecode(utf8.decode(raw));
  } on FormatException {
    mismatch();
  }
  if (json is! Map<String, Object?> ||
      json.values.any((v) => v is! String) ||
      json['amount'] is! String ||
      json['currency'] is! String) {
    mismatch();
  }
  final signed = TransactionSummary(
    amount: json['amount']! as String,
    currency: json['currency']! as String,
    payee: json['payee'] as String?,
    description: json['summary'] as String?,
  );
  // Byte for byte: no member beyond the four, and nothing that reads
  // differently from what the sheet shows.
  if (!_equal(signed.jcs, raw)) mismatch();
  final shown = [for (final d in challenge.displayed) d.value];
  for (final MapEntry(:key, :value) in signed.members.entries) {
    final own = device.members[key];
    if (own != null) {
      if (own != value) mismatch();
    } else if (!shown.remove(value)) {
      mismatch();
    }
  }
  if (shown.isNotEmpty) mismatch();
  return signed;
}
