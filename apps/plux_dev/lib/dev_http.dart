// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The HTTP and WebSocket clients of the Dev app (DEV-001): every request it
/// makes, its sign-in, the runtime's sync and the dev channel, trusts the
/// system's certificate authorities and, when the build names one, the
/// development certificate authority of the installation it previews.
///
/// A development server such as `make dev`'s presents a certificate of its
/// own authority. Installing that authority on the device does not help:
/// `dart:io`, which every Flutter request and WebSocket goes through here,
/// trusts only the system's authorities on Android and never the ones a user
/// installs. The authority is therefore given to the build, as the root keys
/// are, and only that build trusts it. Without it the Dev app trusts the
/// system's authorities alone.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:http/http.dart' as http;
import 'package:http/io_client.dart';

/// The `--dart-define` that carries the authority: the PEM text of the
/// installation's development certificate authority (`make dev`:
/// `deploy/compose/.secrets/ca/ca.crt`).
const String devCertificateAuthorityDefine = 'PLUX_DEV_CA';

const String _authority = String.fromEnvironment(devCertificateAuthorityDefine);

/// The certificates of [pem] with their lines restored. A define reaches the
/// build on one line (Flutter passes the defines to the Dart compiler on a
/// command line that a line break cuts on Windows), while the TLS library
/// reads PEM in lines of at most 64 characters: each certificate's base64 is
/// wrapped again, whatever lines it had, and its markers may have lost their
/// space (`scripts/dev-app-defines.sh` removes every space and line break).
String wrapPem(String pem) {
  final certificate = RegExp(
    r'-----BEGIN\s*CERTIFICATE-----(.*?)-----END\s*CERTIFICATE-----',
    dotAll: true,
  );
  final out = StringBuffer();
  for (final m in certificate.allMatches(pem)) {
    final body = m.group(1)!.replaceAll(RegExp(r'\s'), '');
    out.writeln('-----BEGIN CERTIFICATE-----');
    for (var i = 0; i < body.length; i += 64) {
      out.writeln(body.substring(i, min(i + 64, body.length)));
    }
    out.writeln('-----END CERTIFICATE-----');
  }
  return out.toString();
}

/// The security context of [pem]: the system's authorities, and [pem]'s
/// when it is not blank. Throws [FormatException] naming
/// [devCertificateAuthorityDefine] when [pem] holds no certificate.
SecurityContext devSecurityContext(String pem) {
  final context = SecurityContext(withTrustedRoots: true);
  if (pem.trim().isEmpty) return context;
  final certificates = wrapPem(pem);
  try {
    if (certificates.isEmpty) throw const TlsException();
    context.setTrustedCertificatesBytes(utf8.encode(certificates));
  } on TlsException {
    throw const FormatException(
      '$devCertificateAuthorityDefine is not a PEM certificate: pass the '
      "text of the development certificate authority's ca.crt",
    );
  }
  return context;
}

/// Checks the authority the build carries, so that a build with a mistake in
/// it fails at start instead of at its first request.
void checkDevCertificateAuthority() => devSecurityContext(_authority);

/// The `dart:io` client of the Dev app: WebSockets connect with it. A
/// top-level function, since the runtime calls it on other isolates.
HttpClient devIoClient() => HttpClient(context: devSecurityContext(_authority));

/// The HTTP client of the Dev app, over [devIoClient]. A top-level function,
/// since the runtime calls it on other isolates.
http.Client devHttpClient() => IOClient(devIoClient());
