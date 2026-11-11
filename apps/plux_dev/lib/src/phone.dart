// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Dev app's view of the phone's sign-in (DEV-002, ADR-0104): what the
/// screens need of `DevPhone`, so that tests replace the platform's keys,
/// secret store and server.
library;

import 'dart:convert';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:plux_flutter/plux_flutter.dart';

/// A signed-in phone: `DevPhone`.
abstract interface class SignedInPhone {
  /// Whom it is signed in as.
  DevPhoneAccount get account;

  /// The apps it may open.
  Future<List<DevPhoneApp>> apps();

  /// Asks to join the person's Studio session of [appId].
  Future<DevJoin> requestJoin(String appId);

  /// Picks [number] for [join] and returns the pairing link.
  Future<String> completeJoin(DevJoin join, int number);

  /// Signs the phone out.
  Future<void> signOut();
}

/// A sign-in the phone claimed: `DevPhoneClaim`.
abstract interface class PhoneSignIn {
  /// The numbers to offer.
  List<int> get options;

  /// When the sign-in expires.
  DateTime? get expiresAt;

  /// Whom it signs in as.
  String get userDisplay;

  /// Their organisation.
  String get organizationName;

  /// Picks [number].
  Future<SignedInPhone> complete(int number);
}

/// Where the phone's sign-in comes from.
abstract interface class PhoneDirectory {
  /// The phone signed in earlier, or null.
  Future<SignedInPhone?> restore();

  /// Claims the sign-in of [link].
  Future<PhoneSignIn> claim(DevSignInLink link);
}

/// [PhoneDirectory] over the runtime's `DevPhone`: the platform's keystore
/// and encrypted secret store, the Dev app's HTTP client.
final class RuntimePhones implements PhoneDirectory {
  /// Creates the directory. [client] makes the HTTP client; [deviceName] is
  /// the name the phone gives itself.
  const RuntimePhones({required this.client, required this.deviceName});

  /// Makes the HTTP client.
  final http.Client Function() client;

  /// The phone's name in its person's list.
  final String deviceName;

  @override
  Future<SignedInPhone?> restore() async {
    final p = await DevPhone.restore(client: client);
    return p == null ? null : _Phone(p);
  }

  @override
  Future<PhoneSignIn> claim(DevSignInLink link) async => _Claim(
    await DevPhone.claim(
      link,
      client: client,
      name: deviceName,
      platform: Platform.operatingSystem,
    ),
  );
}

final class _Phone implements SignedInPhone {
  _Phone(this._p);

  final DevPhone _p;

  @override
  DevPhoneAccount get account => _p.account;

  @override
  Future<List<DevPhoneApp>> apps() => _p.apps();

  @override
  Future<DevJoin> requestJoin(String appId) => _p.requestJoin(appId);

  @override
  Future<String> completeJoin(DevJoin join, int number) =>
      _p.completeJoin(join, number);

  @override
  Future<void> signOut() => _p.signOut();
}

final class _Claim implements PhoneSignIn {
  _Claim(this._c);

  final DevPhoneClaim _c;

  @override
  List<int> get options => _c.options;

  @override
  DateTime? get expiresAt => _c.expiresAt;

  @override
  String get userDisplay => _c.userDisplay;

  @override
  String get organizationName => _c.organizationName;

  @override
  Future<SignedInPhone> complete(int number) async =>
      _Phone(await _c.complete(number));
}

/// The channel of the Dev app's own native call.
const MethodChannel _device = MethodChannel('dev.plux.dev/device');

/// The phone's name, such as "Google Pixel 8", from the Dev app's native
/// side; a name for the platform when it has none.
Future<String> phoneName() async {
  try {
    final name = await _device.invokeMethod<String>('name');
    if (name != null && name.trim().isNotEmpty) return name.trim();
  } on Object {
    // No native side (a test, or an older build): the platform's name.
  }
  return Platform.isIOS ? 'iPhone' : 'Android phone';
}

/// The app a pairing link's token names, read without checking the token:
/// it only chooses the app to start, and the server checks the token when
/// the phone pairs. Null when the link holds no readable token.
String? pairingAppOf(String link) {
  try {
    final token = Uri.parse(link.trim()).queryParameters['token'];
    final parts = token?.split('.') ?? const <String>[];
    if (parts.length != 3) return null;
    final payload = utf8Json(parts[1]);
    final app = payload['app'];
    return app is String && app.isNotEmpty ? app : null;
  } on Object {
    return null;
  }
}

/// The JSON object of a base64url segment.
Map<String, Object?> utf8Json(String segment) =>
    jsonDecode(utf8.decode(base64Url.decode(base64Url.normalize(segment))))
        as Map<String, Object?>;
