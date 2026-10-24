// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The pairing link a dev session shows as a QR code or a deep link
/// (DEV-002, ADR-0087): `plux-dev://pair?server=<https url>&token=<token>`,
/// and the short code `plux dev` prints beside it to type instead.
library;

import 'package:flutter/foundation.dart';

/// What a device pairs with: a [DevPairingLink] or a [DevPairingCode].
@immutable
sealed class DevPairingCredential {
  const DevPairingCredential();
}

/// A parsed pairing link.
@immutable
final class DevPairingLink extends DevPairingCredential {
  /// Creates a link from its parts; [parse] checks them.
  const DevPairingLink({required this.server, required this.token});

  /// The scheme of a pairing link.
  static const String scheme = 'plux-dev';

  /// Parses [text]. Only the `plux-dev` scheme with the `pair` host is
  /// accepted, and the server must be an `https` URL; `http` is accepted
  /// for a loopback address or the Android emulator's host alias
  /// (`10.0.2.2`) when [allowInsecureLoopback] is true, which it is in debug
  /// builds only. Throws a [FormatException] for anything else.
  factory DevPairingLink.parse(
    String text, {
    bool allowInsecureLoopback = kDebugMode,
  }) {
    final Uri link;
    try {
      link = Uri.parse(text.trim());
    } on FormatException {
      throw FormatException('not a pairing link', text);
    }
    if (link.scheme != scheme || link.host != 'pair') {
      throw FormatException('not a plux-dev://pair link', text);
    }
    final params = link.queryParameters;
    final token = params['token'];
    if (token == null || token.isEmpty) {
      throw const FormatException('the pairing link has no token');
    }
    final serverText = params['server'];
    if (serverText == null || serverText.isEmpty) {
      throw const FormatException('the pairing link has no server');
    }
    final Uri server;
    try {
      server = Uri.parse(serverText);
    } on FormatException {
      throw const FormatException('the pairing link has an invalid server');
    }
    if (!server.hasAuthority || server.host.isEmpty || server.hasFragment) {
      throw const FormatException('the pairing link has an invalid server');
    }
    final secure = server.scheme == 'https';
    final loopback = server.scheme == 'http' && _isLoopback(server.host);
    if (!secure && !(loopback && allowInsecureLoopback)) {
      throw const FormatException('the pairing server must use https');
    }
    return DevPairingLink(
      server: Uri(
        scheme: server.scheme,
        host: server.host,
        port: server.hasPort ? server.port : null,
        path: server.path,
      ),
      token: token,
    );
  }

  /// The server's base URL.
  final Uri server;

  /// The single-use pairing token.
  final String token;

  /// The URL of the dev socket: `wss` for an `https` server, `ws` for the
  /// loopback `http` one, under the server's base path.
  Uri get socketUrl => socketUrlOf(server);

  /// The URL of the dev socket of the server at [server]: `wss` for an
  /// `https` server, `ws` for an `http` one, under the server's base path.
  static Uri socketUrlOf(Uri server) {
    final base = server.path.endsWith('/')
        ? server.path.substring(0, server.path.length - 1)
        : server.path;
    return Uri(
      scheme: server.scheme == 'https' ? 'wss' : 'ws',
      host: server.host,
      port: server.hasPort ? server.port : null,
      path: '$base/dev/socket',
    );
  }

  @override
  String toString() => 'DevPairingLink($server, [redacted])';
}

/// A short pairing code a person typed (DEV-002): eight Crockford base32
/// characters, shown as `XXXX-XXXX`. It names no server; the device pairs
/// against the server its runtime talks to.
@immutable
final class DevPairingCode extends DevPairingCredential {
  const DevPairingCode._(this.value);

  /// Crockford's base32 alphabet: no I, L, O or U.
  static const String _alphabet = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

  /// The number of characters of a code.
  static const int length = 8;

  /// Reads [text] as a code: case, dashes and spaces do not matter, and O,
  /// I and L are read as 0, 1 and 1, as the server reads them. Returns null
  /// when [text] is not a code.
  static DevPairingCode? tryParse(String text) {
    final out = StringBuffer();
    for (final rune in text.toUpperCase().runes) {
      var c = String.fromCharCode(rune);
      if (c == '-' || c == ' ') continue;
      c = switch (c) {
        'O' => '0',
        'I' || 'L' => '1',
        _ => c,
      };
      if (!_alphabet.contains(c)) return null;
      out.write(c);
    }
    final value = out.toString();
    return value.length == length ? DevPairingCode._(value) : null;
  }

  /// Reads [text] as a code; throws a [FormatException] when it is not one.
  factory DevPairingCode.parse(String text) =>
      tryParse(text) ??
      (throw const FormatException('not a pairing code (XXXX-XXXX)'));

  /// The code, in capitals and without its dash, as the server takes it.
  final String value;

  @override
  bool operator ==(Object other) =>
      other is DevPairingCode && other.value == value;

  @override
  int get hashCode => value.hashCode;

  @override
  String toString() => 'DevPairingCode([redacted])';
}

bool _isLoopback(String host) =>
    host == 'localhost' ||
    host == '127.0.0.1' ||
    host == '::1' ||
    host == '10.0.2.2';
